package objects

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// SpecFieldSummary captures the minimal, spec-derived metadata we want to cache per field.
type SpecFieldSummary struct {
	Name         string   `json:"name"`
	Type         string   `json:"type,omitempty"`
	SemanticType string   `json:"semantic_type,omitempty"`
	Traits       []string `json:"traits,omitempty"`
	Required     bool     `json:"required,omitempty"`
	EnumValues   []string `json:"enum_values,omitempty"`
	// IsRef indicates this field references another kind (e.g. workstream_ref).
	IsRef bool `json:"is_ref,omitempty"`
	// EdgeRole is membership | composition | associate when the spec annotates the field.
	EdgeRole string `json:"edge_role,omitempty"`

	// Trait-derived convenience flags for CLI/validation usage.
	Groupable  bool `json:"groupable,omitempty"`
	Filterable bool `json:"filterable,omitempty"`
	Sortable   bool `json:"sortable,omitempty"`

	// Validation/constraint hints for completions and UX.
	Numeric       bool    `json:"numeric,omitempty"`
	MinLength     int     `json:"min_length,omitempty"`
	MaxLength     int     `json:"max_length,omitempty"`
	DisplayLength int     `json:"display_length,omitempty"`
	MinValue      float64 `json:"min_value,omitempty"`
	MaxValue      float64 `json:"max_value,omitempty"`
	Pattern       string  `json:"pattern,omitempty"`
	TimeLike      bool    `json:"time_like,omitempty"`
	DateLike      bool    `json:"date_like,omitempty"`
	Format        string  `json:"format,omitempty"`
}

// SpecKindSummary is the cached view of a single kind.
type SpecKindSummary struct {
	Kind   string             `json:"kind"`
	Fields []SpecFieldSummary `json:"fields"`
	// StorageProfile is the data-cell storage mechanism (object_specs storage_profile); inherited along extends.
	StorageProfile string `json:"storage_profile,omitempty"`
	// KernelCritical is the resolved object_specs kernel_critical flag (omit when unset in older indexes).
	// EffectiveKernelCritical() applies storage_profile inference when this pointer is nil.
	KernelCritical *bool `json:"kernel_critical,omitempty"`
}

// EffectiveKernelCritical reports kernel-critical policy from a materialized kind summary.
// TRACK: BLI-REDACTED
func (ks SpecKindSummary) EffectiveKernelCritical() bool {
	if ks.KernelCritical != nil {
		return *ks.KernelCritical
	}
	return defaultKernelCriticalForProfile(ks.StorageProfile)
}

// SpecIndex is a read-optimized, immutable snapshot of specs keyed by kind.
type SpecIndex struct {
	Kinds map[string]SpecKindSummary `json:"kinds"`
	// BuilderSpecCacheRevision is SpecCacheRevision() of the SpecLoader used to build this index
	// (see BuildSpecIndexFromSpecsDir). Correlates on-disk snapshot with that loader's invalidation generation.
	BuilderSpecCacheRevision uint64 `json:"builder_spec_cache_revision,omitempty"`
	// GlobalSpecCacheRevision is optional: SpecCacheRevision() of GetGlobalSpecLoader() at write time
	// (e.g. system generate-spec-index) for correlating with process-wide spec cache invalidation.
	GlobalSpecCacheRevision uint64 `json:"global_spec_cache_revision,omitempty"`
}

// specIndexKindsMissing reports whether idx is nil or has no Kinds map.
func specIndexKindsMissing(idx *SpecIndex) bool {
	return idx == nil || idx.Kinds == nil
}

// GetKindSummary returns the summary for a kind if present.
func (idx *SpecIndex) GetKindSummary(kind string) (SpecKindSummary, bool) {
	if specIndexKindsMissing(idx) {
		return SpecKindSummary{}, false
	}
	ks, ok := idx.Kinds[kind]
	return ks, ok
}

// GetGroupableFields returns all groupable fields for a kind.
func (idx *SpecIndex) GetGroupableFields(kind string) []SpecFieldSummary {
	ks, ok := idx.GetKindSummary(kind)
	if !ok {
		return nil
	}
	out := make([]SpecFieldSummary, 0, len(ks.Fields))
	for _, f := range ks.Fields {
		if f.Groupable {
			out = append(out, f)
		}
	}
	return out
}

// GetFilterableFields returns all filterable fields for a kind.
func (idx *SpecIndex) GetFilterableFields(kind string) []SpecFieldSummary {
	ks, ok := idx.GetKindSummary(kind)
	if !ok {
		return nil
	}
	out := make([]SpecFieldSummary, 0, len(ks.Fields))
	for _, f := range ks.Fields {
		if f.Filterable {
			out = append(out, f)
		}
	}
	return out
}

// GetSortableFields returns all sortable fields for a kind.
func (idx *SpecIndex) GetSortableFields(kind string) []SpecFieldSummary {
	ks, ok := idx.GetKindSummary(kind)
	if !ok {
		return nil
	}
	out := make([]SpecFieldSummary, 0, len(ks.Fields))
	for _, f := range ks.Fields {
		if f.Sortable {
			out = append(out, f)
		}
	}
	return out
}

// BuildSpecIndexFromSpecsDir builds a SpecIndex by loading all specs under specsDir
// (typically .zqk/specs/objects).
// Unreadable specs are skipped so partial temp trees and exploratory tests keep working.
func BuildSpecIndexFromSpecsDir(specsDir string) (*SpecIndex, error) {
	return buildSpecIndexFromSpecsDir(specsDir, true)
}

// BuildSpecIndexFromSpecsDirStrict builds a SpecIndex and fails if any non-skipped spec YAML
// cannot be loaded (including invalid storage_profile or inheritance errors).
// Use for materialized .zqk/specs/spec_index.json so deploy-time typos cannot be silently omitted.
func BuildSpecIndexFromSpecsDirStrict(specsDir string) (*SpecIndex, error) {
	return buildSpecIndexFromSpecsDir(specsDir, false)
}

func buildSpecIndexFromSpecsDir(specsDir string, skipFailedSpecs bool) (*SpecIndex, error) {
	if specsDir == emptyValue {
		return nil, errfmt.Errorf("specsDir is required")
	}

	if _, err := fileutil.Stat(specsDir); err != nil {
		return nil, errfmt.Newf("failed to read specs directory").Wrap(err)
	}

	loader := NewSpecLoader(specsDir)
	ontologies, err := loader.DiscoverOntologies()
	if err != nil {
		return nil, errfmt.Newf("failed to read specs directory").Wrap(err)
	}

	index := &SpecIndex{
		Kinds: make(map[string]SpecKindSummary),
	}

	config := GetGlobalKindMappingsConfig()
	for _, base := range ontologies {
		if base == "_placeholder" {
			continue
		}
		if config != nil && config.ShouldSkipSpec(base) {
			continue
		}

		name := base + ".yaml"
		spec, err := loader.LoadSpecWithInheritance(name)
		if err != nil {
			if !skipFailedSpecs {
				return nil, errfmt.Errorf("load spec %s: %w", name, err)
			}
			continue
		}
		if spec == nil {
			if !skipFailedSpecs {
				return nil, errfmt.Errorf("load spec %s: empty spec", name)
			}
			continue
		}

		kind := spec.Ontology
		if kind == emptyValue {
			kind = base
		}

		fields := make([]SpecFieldSummary, 0, len(spec.ResolvedFields))
		for fieldName, raw := range spec.ResolvedFields {
			fieldMap, ok := raw.(map[string]any)
			if !ok {
				continue
			}

			const (
				specKeyType         = "type"
				specKeySemanticType = "semantic_type"
				specKeyTraits       = "traits"
				specKeyRequired     = "required"
				specKeyValidation   = "validation"
				specKeyEnum         = "enum"
			)

			traits := extractStringSlice(fieldMap[specKeyTraits])

			summary := SpecFieldSummary{
				Name:         fieldName,
				Type:         asString(fieldMap[specKeyType]),
				SemanticType: asString(fieldMap[specKeySemanticType]),
				Traits:       traits,
				Required:     asBool(fieldMap[specKeyRequired]),
			}

			if v, ok := fieldMap[specKeyValidation].(map[string]any); ok {
				if ev, okEnum := v[specKeyEnum].([]any); okEnum {
					for _, rawEnum := range ev {
						if s, ok := rawEnum.(string); ok && s != emptyValue {
							summary.EnumValues = append(summary.EnumValues, s)
						}
					}
				}

				// Numeric and range hints.
				if isNumericType(summary.Type, summary.SemanticType) {
					summary.Numeric = true
				}
				if minVal, ok := asFloat(v["min"]); ok {
					summary.MinValue = minVal
				}
				if maxVal, ok := asFloat(v["max"]); ok {
					summary.MaxValue = maxVal
				}

				// Length constraints (min/max/display_length).
				if lengthMap, okLen := v["length"].(map[string]any); okLen {
					if minLen, ok := asInt(lengthMap["min"]); ok {
						summary.MinLength = minLen
					}
					if maxLen, ok := asInt(lengthMap["max"]); ok {
						summary.MaxLength = maxLen
					}
				}
				if minLen, ok := asInt(v["min_length"]); ok {
					summary.MinLength = minLen
				}
				if maxLen, ok := asInt(v["max_length"]); ok {
					summary.MaxLength = maxLen
				}
				if dl, ok := asInt(v["display_length"]); ok {
					summary.DisplayLength = dl
				}

				// Pattern and format hints.
				if p, ok := v["pattern"].(string); ok && p != emptyValue {
					summary.Pattern = p
				}
				if f, ok := v[FieldKeyFormat].(string); ok && f != emptyValue {
					summary.Format = normalizeFormatHint(f)
				}
			}

			// Heuristic: treat *_ref and *_refs as reference fields.
			if strings.HasSuffix(fieldName, "_ref") || strings.HasSuffix(fieldName, "_refs") {
				summary.IsRef = true
			}
			if role, ok := EdgeRoleFromFieldDef(fieldMap); ok {
				summary.EdgeRole = string(role)
			}

			// Trait-derived flags: groupable/filterable/sortable.
			if hasTrait(traits, "groupable") {
				summary.Groupable = true
			}
			if hasTrait(traits, "filterable") {
				summary.Filterable = true
			}
			if hasTrait(traits, "sortable") {
				summary.Sortable = true
			}

			// Time/date hints from semantic_type when format not explicitly set.
			if summary.SemanticType != emptyValue && summary.Format == emptyValue {
				if isDateLike(summary.SemanticType) {
					summary.DateLike = true
					// Default date format hint.
					summary.Format = "YYYY-MM-DD"
				}
				if isTimeLike(summary.SemanticType) {
					summary.TimeLike = true
					if summary.Format == emptyValue {
						// Default timestamp format hint.
						summary.Format = "RFC3339"
					}
				}
			}

			fields = append(fields, summary)
		}

		sort.Slice(fields, func(i, j int) bool {
			return fields[i].Name < fields[j].Name
		})

		index.Kinds[kind] = SpecKindSummary{
			Kind:           kind,
			Fields:         fields,
			StorageProfile: spec.StorageProfile,
			KernelCritical: cloneBoolPtr(spec.KernelCritical),
		}
	}

	index.BuilderSpecCacheRevision = loader.SpecCacheRevision()

	return index, nil
}

// WriteSpecIndex writes the index as JSON to the given path, creating parent dirs as needed.
func WriteSpecIndex(path string, idx *SpecIndex) error {
	if idx == nil {
		return errfmt.Errorf("spec index is nil")
	}
	if err := fileutil.EnsureDir(filepath.Dir(path)); err != nil {
		return errfmt.Newf("failed to create spec index directory").Wrap(err)
	}

	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return errfmt.Newf("failed to marshal spec index").Wrap(err)
	}

	if err := fileutil.WriteSecureFile(path, data); err != nil {
		return errfmt.Newf("failed to write spec index").Wrap(err)
	}
	return nil
}

// BuildAndWriteMaterializedSpecIndex rebuilds the JSON spec index from specsDir, writes it to outputPath,
// and clears the global spec loader cache first so GlobalSpecCacheRevision reflects this materialization
// boundary (same contract as `zqk system generate-spec-index`).
func BuildAndWriteMaterializedSpecIndex(specsDir, outputPath string) (*SpecIndex, error) {
	if specsDir == emptyValue || outputPath == emptyValue {
		return nil, errfmt.Errorf("specsDir and outputPath are required")
	}
	GetGlobalSpecLoader().ClearCache()
	idx, err := BuildSpecIndexFromSpecsDirStrict(specsDir)
	if err != nil {
		return nil, err
	}
	idx.GlobalSpecCacheRevision = GetGlobalSpecLoader().SpecCacheRevision()
	if err := WriteSpecIndex(outputPath, idx); err != nil {
		return nil, err
	}
	return idx, nil
}

// RefreshMaterializedSpecIndex writes .zqk/specs/spec_index.json under projectRoot using
// .zqk/specs/objects. Call after spec YAML files change on disk (e.g. update-specs).
// When .zqk/specs/configs/high_volume_kinds.yaml exists, validates stream kinds against the
// materialized index (data-cell stewardship contract; see ValidateHighVolumeStreamKindsMatchSpecIndex).
func RefreshMaterializedSpecIndex(projectRoot string) (*SpecIndex, error) {
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf("project root is required")
	}
	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	outputPath := filepath.Join(projectRoot, paths.ProcessInternalDir, "spec_index.json")
	idx, err := BuildAndWriteMaterializedSpecIndex(specsDir, outputPath)
	if err != nil {
		return nil, err
	}
	if err := ValidateHighVolumeStreamKindsMatchSpecIndex(idx, projectRoot); err != nil {
		return nil, err
	}
	return idx, nil
}

// LoadSpecIndex attempts to load a SpecIndex from the given path.
func LoadSpecIndex(path string) (*SpecIndex, error) {
	data, err := fileutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var idx SpecIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	return &idx, nil
}

// GetAllKindsFromIndex returns sorted kinds from a spec index (or nil if index is nil).
func GetAllKindsFromIndex(idx *SpecIndex) []string {
	if specIndexKindsMissing(idx) {
		return nil
	}
	kinds := make([]string, 0, len(idx.Kinds))
	for k := range idx.Kinds {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return kinds
}

// KindNamesFromSpecIndex returns the same set shape as kindnames.LoadKindNamesFromSpecsDir
// from a loaded [SpecIndex] (no object_specs directory walk). Prefer when the index is already
// materialized — aligns with the spec origin plane snapshot story and [SpecLoader.SpecCacheRevision].
func KindNamesFromSpecIndex(idx *SpecIndex) (map[string]struct{}, error) {
	if specIndexKindsMissing(idx) {
		return nil, errfmt.Errorf("spec index is nil or has no kinds")
	}
	out := make(map[string]struct{}, len(idx.Kinds))
	for k := range idx.Kinds {
		out[k] = struct{}{}
	}
	return out, nil
}

// Global helpers for runtime callers (e.g. CLI) to use the index if present.

// DefaultSpecIndexPathRef is the path-cache reference for the spec index file.
// Resolution goes through the path alias cache (prefix:process_internal/...) so projectRoot
// and bootstrap layout differences are handled centrally.
const DefaultSpecIndexPathRef = paths.PathSchemePrefix + "process_internal/spec_index.json"

type cachedSpecIndex struct {
	idx     *SpecIndex
	modTime time.Time
	size    int64
}

var (
	specIndexCache   = make(map[string]cachedSpecIndex)
	specIndexCacheMu sync.RWMutex
)

// TryLoadSpecIndexForProjectRoot loads the spec index for the given projectRoot using the
// path resolver. Callers should fall back to existing behavior if this returns nil or error.
func TryLoadSpecIndexForProjectRoot(projectRoot string) *SpecIndex {
	if projectRoot == emptyValue {
		return nil
	}
	abs, err := paths.ResolvePathStrict(projectRoot, DefaultSpecIndexPathRef)
	if err != nil || abs == emptyValue {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Debug("Spec index path not resolvable via path cache; falling back to dynamic spec loading").
			WithError(err).
			Log()
		return nil
	}

	info, err := fileutil.Stat(abs)
	if err != nil {
		return nil
	}

	specIndexCacheMu.RLock()
	cached, ok := specIndexCache[abs]
	specIndexCacheMu.RUnlock()

	if ok && cached.modTime.Equal(info.ModTime()) && cached.size == info.Size() {
		return cached.idx
	}

	idx, err := LoadSpecIndex(abs)
	if err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Debug("Spec index not available or failed to load; falling back to dynamic spec loading").
			WithError(err).
			Log()
		return nil
	}

	specIndexCacheMu.Lock()
	specIndexCache[abs] = cachedSpecIndex{
		idx:     idx,
		modTime: info.ModTime(),
		size:    info.Size(),
	}
	specIndexCacheMu.Unlock()

	return idx
}

// Small helpers for safe type conversions.

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asBool(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

func extractStringSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, val := range raw {
		if s, ok := val.(string); ok && s != emptyValue {
			out = append(out, s)
		}
	}
	return out
}

func hasTrait(traits []string, target string) bool {
	for _, t := range traits {
		if t == target {
			return true
		}
	}
	expanded, err := lookupTraitRegistry().ExpandTraits(traits)
	if err != nil {
		return false
	}
	for _, t := range expanded {
		if t == target {
			return true
		}
	}
	return false
}

// Constraint helpers.

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		// YAML numbers often decode as float64.
		return int(n), true
	default:
		return 0, false
	}
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float32:
		return float64(n), true
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func isNumericType(t, semantic string) bool {
	if t == "integer" || t == "number" {
		return true
	}
	switch semantic {
	case "integer", "number", "count", "duration":
		return true
	default:
		return false
	}
}

func isDateLike(semantic string) bool {
	switch semantic {
	case "date":
		return true
	default:
		return false
	}
}

func isTimeLike(semantic string) bool {
	switch semantic {
	case "timestamp", "datetime", "time", "duration":
		return true
	default:
		return false
	}
}

// normalizeFormatHint maps raw validation formats to short, user-facing hints suitable for completions.
func normalizeFormatHint(f string) string {
	switch strings.ToLower(f) {
	case "date-time", "datetime", "timestamp", "rfc3339":
		return "RFC3339"
	case "date":
		return "YYYY-MM-DD"
	default:
		return f
	}
}
