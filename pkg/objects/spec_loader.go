package objects

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/config"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/loader"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	yamlspec "github.com/zqk-os/zqk/pkg/specbuilder/yaml"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Spec represents a loaded object specification with resolved inheritance.
// Location is the path reference (prefix: or abs:) for the backing YAML file so it can be re-used and resolved where necessary.
type Spec struct {
	SchemaVersion string `yaml:"schema_version"`
	Ontology      string `yaml:"ontology"`
	URN           string `yaml:"urn,omitempty"`
	Extends       string `yaml:"extends"`
	// Composes lists mixin ontologies merged after extends (child wins).
	// Independent of the single-parent extends DAG. Occupancy and remaining_open
	// use this so timesheets keep extends: work_unit (or Gantt keep work_interval)
	// without putting those mixins under the parent kind.
	Composes    []string `yaml:"composes,omitempty"`
	Visibility  string   `yaml:"visibility"`
	Namespace   string   `yaml:"namespace,omitempty"`
	Description string   `yaml:"description"`
	// IDPrefixes defines the valid ID prefixes for this kind (e.g., ["GOAL-"]).
	// Derives id_prefixes_config.yaml entries during spec generation.
	IDPrefixes []string `yaml:"id_prefixes,omitempty"`
	// IDSynonyms defines standardized aliases for this kind (e.g., ["backlog"]).
	// Derives id_prefixes_config.yaml synonyms during spec generation.
	IDSynonyms []string `yaml:"id_synonyms,omitempty"`
	// StorageProfile is the data-cell storage mechanism for this kind (optional; inherited from parent when empty).
	// See pkg/datacell.StorageProfile and DATA_CELL_MODEL.md.
	StorageProfile string `yaml:"storage_profile,omitempty"`
	// KernelCritical marks kinds that must not be silently hard-deleted and require break_glass for
	// lifecycle Force. nil means inherit from parent; after resolve, EffectiveKernelCritical applies
	// defaults (cas_entity → true, stream/light_file → false). TRACK
	KernelCritical *bool          `yaml:"kernel_critical,omitempty"`
	Traits         []string       `yaml:"traits"`
	ExcludeTraits  []string       `yaml:"exclude_traits,omitempty"`
	Fields         map[string]any `yaml:"fields"`
	// Resolved fields after inheritance
	ResolvedFields map[string]any `yaml:"-"`
	ResolvedTraits []string       `yaml:"-"`
	// Location is set by the loader to a path reference (prefix:relPath or abs:absolutePath) for the backing YAML file.
	Location string `yaml:"-"`
}

// SpecResolvedFieldsMissing reports whether spec is nil or has no resolved fields map (e.g. not loaded).
func SpecResolvedFieldsMissing(spec *Spec) bool {
	return spec == nil || spec.ResolvedFields == nil
}

// cachedSpec stores a spec with its file modification time for staleness detection
type cachedSpec struct {
	spec  *Spec
	mtime time.Time // File modification time when cached
}

// VersionedBuilderRegistryInterface allows SpecLoader to use versioned builders without import cycles
// This interface should be implemented by pkg/specbuilder/builders.VersionedBuilderRegistry
type VersionedBuilderRegistryInterface interface {
	GetBuilder(ontology, version string) (SpecBuilderInterface, error)
	GetLatestVersion(ontology string) (string, error)
	GetVersions(ontology string) []string
}

// SpecBuilderInterface allows SpecLoader to use spec builders without import cycles
// This interface should be implemented by pkg/specbuilder/builders.SpecBuilder
type SpecBuilderInterface interface {
	Build() *Spec
	GetVersion() string
	GetOntology() string
}

// builderRegistryHolder wraps the versioned registry for atomic.Pointer (atomic stores need a concrete type).
type builderRegistryHolder struct {
	reg VersionedBuilderRegistryInterface
}

// fileContentEntry holds raw file bytes and mtime for staleness detection
type fileContentEntry struct {
	data  []byte
	mtime time.Time
}

// specShard represents a shard of the spec cache
// Uses sync.Map for lock-free reads (cache hits are hot path)
// sync.Map handles all synchronization internally - no mutex needed
type specShard struct {
	cache            sync.Map // map[string]*cachedSpec - ontology -> cached spec
	ontologyCache    sync.Map // map[string]string - file path -> ontology (for getOntologyFromFile)
	pathCache        sync.Map // map[string]*cachedSpec - absolute path -> cached spec (avoids double ReadFile on first load)
	fileContentCache sync.Map // map[string]*fileContentEntry - absolute path -> content+mtime (avoids double ReadFile; re-read if mtime changed)
}

// SpecLoader loads and resolves object specifications with inheritance.
// Spec memos are stamp-invalidated (pkg/stampmemo); keys are the closed spec tree.
// Optional EnsureReady(ctx) uses the component loader pattern (pkg/loader) to warm base specs once with timeout.
type SpecLoader struct {
	specsDir        string
	dependencyGraph *SpecDependencyGraph                  // Optional: for load order validation
	specsByPath     stampmemo.Table[*Spec]                // keyed by abs spec path (closed spec tree)
	specsByOntology stampmemo.Table[*Spec]                // keyed by ontology (closed spec tree)
	ontologies      stampmemo.Table[string]               // keyed by abs spec path
	fileBytes       stampmemo.Table[[]byte]               // keyed by abs spec path; stamp is generation
	specIndex       stampmemo.Table[map[string]string]    // keyed by specsDir; basename/ontology → abs path
	fieldRefs       stampmemo.Table[map[string]any]       // keyed by ref name (closed field-ref set)
	builderRegistry atomic.Pointer[builderRegistryHolder] // Optional: for version-aware loading (atomic load/store; no mutex)
	globalMu        sync.RWMutex                          // Protects specsDir updates in ensure-ready (see doEnsureReady)
	metrics         *SpecLoaderMetrics                    // Metrics for lock operations
	logger          concurrency.LockLogger                // Logger for lock operations
	readyRunner     *loader.Runner                        // Optional: one-shot "ensure ready" (warm base specs)
	readyRunnerOnce sync.Once
	// cacheRevision increments on ClearCache / InvalidateSpec / InvalidateSpecByFile so callers
	// can correlate derived materializations (spec index, kind lists) with invalidation (see SPEC_ORIGIN_PLANE.md).
	cacheRevision atomic.Uint64
	// specStorage backs spec reads (CRIT-9031 / SpecStorageProvider). When nil, readSpecFile uses os.ReadFile.
	specStorage SpecStorageProvider
}

// NewSpecLoader creates a new spec loader.
//
// If specsDir is empty, the object-specs directory is discovered via findSpecsDir().
// If specsDir is a project root (caller passed the repo/workspace root rather than
// .zqk/specs/objects), it is normalized when that nested tree
// contains base_object.yaml so relative spec names like backlog_item.yaml resolve
// correctly. Spec reads go through FileSpecStorageProvider (non-nil) when the
// resolved specs directory is non-empty.
func NewSpecLoader(specsDir string) *SpecLoader {
	if specsDir == emptyValue {
		specsDir = findSpecsDir()
	} else {
		specsDir = normalizeSpecsDirIfProjectRoot(specsDir)
	}

	loader := &SpecLoader{
		specsDir: specsDir,
		metrics:  GetGlobalSpecLoaderMetrics(),
		logger:   &specLoaderLockLogger{logger: logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))},
	}
	loader.dependencyGraph = NewSpecDependencyGraph(loader)
	if specsDir != emptyValue {
		if p, err := NewFileSpecStorageProvider(specsDir); err == nil {
			loader.specStorage = p
		}
	}
	return loader
}

// readSpecFile loads raw bytes and modification time for an absolute spec path.
func (sl *SpecLoader) readSpecFile(ctx context.Context, absPath string) ([]byte, time.Time, error) {
	if sl != nil && sl.specStorage != nil && !pathInExtraSpecRoot(absPath) {
		data, meta, err := sl.specStorage.ReadSpecBytes(ctx, absPath)
		return data, meta.ModTime, err
	}
	data, err := fileutil.ReadFile(absPath)
	if err != nil {
		return nil, time.Time{}, err
	}
	var mt time.Time
	if st, err2 := fileutil.Stat(absPath); err2 == nil {
		mt = st.ModTime()
	}
	return data, mt, nil
}

// specLoaderLockLogger adapts logging.Logger to concurrency.LockLogger
type specLoaderLockLogger struct {
	logger logging.Logger
}

func (l *specLoaderLockLogger) Debug(msg string, fields ...concurrency.LockField) {
	// Convert LockField to logging fields
	logFields := make([]logging.Field, 0, len(fields))
	for _, f := range fields {
		// Convert value to appropriate logging field type
		switch v := f.Value.(type) {
		case string:
			logFields = append(logFields, logging.String(f.Key, v))
		case int:
			logFields = append(logFields, logging.Int(f.Key, v))
		case int64:
			logFields = append(logFields, logging.Int(f.Key, int(v)))
		case error:
			logFields = append(logFields, logging.Error(v))
		default:
			// For other types, convert to string
			logFields = append(logFields, logging.String(f.Key, fmt.Sprintf("%v", v)))
		}
	}
	entry := logging.Fluent(l.logger).Debug(msg)
	if len(logFields) > 0 {
		entry = entry.WithFields(logFields...)
	}
	entry.Log()
}

func (l *specLoaderLockLogger) Warn(msg string, fields ...concurrency.LockField) {
	// Convert LockField to logging fields
	logFields := make([]logging.Field, 0, len(fields))
	for _, f := range fields {
		// Convert value to appropriate logging field type
		switch v := f.Value.(type) {
		case string:
			logFields = append(logFields, logging.String(f.Key, v))
		case int:
			logFields = append(logFields, logging.Int(f.Key, v))
		case int64:
			logFields = append(logFields, logging.Int(f.Key, int(v)))
		case error:
			logFields = append(logFields, logging.Error(v))
		default:
			// For other types, convert to string
			logFields = append(logFields, logging.String(f.Key, fmt.Sprintf("%v", v)))
		}
	}
	entry := logging.Fluent(l.logger).Warn(msg)
	if len(logFields) > 0 {
		entry = entry.WithFields(logFields...)
	}
	entry.Log()
}

// SetBuilderRegistry sets the builder registry for version-aware loading
// This allows version-aware loading without creating import cycles
func (sl *SpecLoader) SetBuilderRegistry(registry VersionedBuilderRegistryInterface) {
	if registry == nil {
		sl.builderRegistry.Store(nil)
		return
	}
	sl.builderRegistry.Store(&builderRegistryHolder{reg: registry})
}

// getBuilderRegistry returns the registry set by SetBuilderRegistry (may be nil).
// Uses atomic load so hot paths avoid globalMu and WithRLockTimeout (which runs the callback in
// another goroutine and can false-timeout under scheduler delay while "holding rlock").
func (sl *SpecLoader) getBuilderRegistry() VersionedBuilderRegistryInterface {
	h := sl.builderRegistry.Load()
	if h == nil {
		return nil
	}
	return h.reg
}

// getReadyRunner returns the shared loader.Runner for "ensure ready" (lazily created).
func (sl *SpecLoader) getReadyRunner() *loader.Runner {
	sl.readyRunnerOnce.Do(func() {
		sl.readyRunner = loader.NewRunner("spec_loader", func(ctx context.Context) error {
			return sl.doEnsureReady(ctx)
		})
	})
	return sl.readyRunner
}

// EnsureReady ensures the spec loader is ready: specs dir is set and base specs (base_object, auditable) are warmed.
// Uses the component loader pattern (pkg/loader) so concurrent callers wait with timeout instead of each doing I/O.
// Optional: call before heavy use to avoid "ID patterns currently loading" style races; per-spec loading remains lock-free.
func (sl *SpecLoader) EnsureReady(ctx context.Context) error {
	return sl.getReadyRunner().Load(ctx)
}

// doEnsureReady runs once per loader; sets specsDir if empty and preloads base specs.
func (sl *SpecLoader) doEnsureReady(ctx context.Context) error { //nolint:unparam // ctx for LoadFn signature
	_ = concurrency.RunInLockWithLogger(
		&sl.globalMu,
		LockNameSpecLoaderEnsureReadyDir,
		sl.logger,
		func() error {
			if sl.specsDir == emptyValue {
				sl.specsDir = findSpecsDir()
			}
			return nil
		},
	)
	// Warm base specs so dependency graph and common inheritance are cached
	_, _ = sl.LoadSpecWithInheritance("base_object.yaml")
	_, _ = sl.LoadSpecWithInheritance("auditable.yaml")
	return nil
}

// ValidateDependencyGraph validates the spec dependency graph
// Returns errors if cycles or missing parents are detected
func (sl *SpecLoader) ValidateDependencyGraph() []*GraphValidationError {
	if err := sl.dependencyGraph.BuildGraph(); err != nil {
		return []*GraphValidationError{{
			Type:    "build_error",
			Message: err.Error(),
			Specs:   []string{},
		}}
	}
	return sl.dependencyGraph.ValidateGraph()
}

// GetLoadOrder returns specs in topological order (parents before children)
// Validates the graph first and returns error if cycles are detected
func (sl *SpecLoader) GetLoadOrder() ([]*Spec, error) {
	if err := sl.dependencyGraph.BuildGraph(); err != nil {
		return nil, errfmt.Newf("failed to build dependency graph").Wrap(err)
	}

	// Validate before sorting
	errors := sl.dependencyGraph.ValidateGraph()
	if len(errors) > 0 {
		return nil, errfmt.Errorf("dependency graph validation failed: %v", errors)
	}

	return sl.dependencyGraph.TopologicalSort()
}

// ValidateTraitContracts verifies trait consistency and constraints across loaded specs
// before kind registration into the type registry (BLI-SPEC-TRAIT-VALIDATOR-002).
func (sl *SpecLoader) ValidateTraitContracts(specs []*Spec) []ValidationError {
	validator := NewSpecValidator(sl, nil)
	var allErrors []ValidationError
	for _, spec := range specs {
		if spec == nil {
			continue
		}
		// Validate object-level traits
		traitErrors := validator.traitRegistry.ValidateTraits(spec.ResolvedTraits, "object")
		for _, traitErr := range traitErrors {
			allErrors = append(allErrors, ValidationError{
				Field:       "traits",
				MissingItem: traitErr.Category,
				CriteriaRef: "CRIT-1788679768850001000-aaaa",
				Message:     fmt.Sprintf("Spec %s trait validation error: %s", spec.Ontology, traitErr.Message),
			})
		}
		for fieldName, fieldDef := range spec.ResolvedFields {
			fieldMap, ok := fieldDef.(map[string]any)
			if !ok {
				continue
			}
			fieldTraits := ExtractFieldTraits(fieldMap)
			if len(fieldTraits) > 0 {
				expandedObjectTraits, _ := validator.traitRegistry.ExpandTraits(spec.ResolvedTraits)
				fieldTraitErrors := validator.traitRegistry.ValidateTraitsWithContext(fieldTraits, "field", expandedObjectTraits)
				for _, traitErr := range fieldTraitErrors {
					allErrors = append(allErrors, ValidationError{
						Field:       fieldName,
						MissingItem: traitErr.Category,
						CriteriaRef: "CRIT-1788679768850001000-aaaa",
						Message:     fmt.Sprintf("Spec %s field %s trait error: %s", spec.Ontology, fieldName, traitErr.Message),
					})
				}
				consistencyErrors := validator.traitRegistry.ValidateTraitFieldConsistency(spec.ResolvedTraits, fieldTraits, fieldName)
				for _, traitErr := range consistencyErrors {
					allErrors = append(allErrors, ValidationError{
						Field:       fieldName,
						MissingItem: traitErr.Category,
						CriteriaRef: "CRIT-1788679768850001000-aaaa",
						Message:     traitErr.Message,
					})
				}
			}
		}
	}
	return allErrors
}

func (sl *SpecLoader) applicableExtraSpecRoots() []string {
	all := ExtraSpecRoots()
	if len(all) == 0 {
		return nil
	}
	mod, err := paths.ModuleRootFromPath(sl.specsDir)
	if err != nil || mod == "" {
		if fallback, ok := moduleRootForSpecs(); ok && fallback != "" && strings.HasPrefix(sl.specsDir, fallback) {
			mod = fallback
		} else {
			return nil
		}
	}
	var out []string
	for _, root := range all {
		if strings.HasPrefix(root, mod) {
			out = append(out, root)
		}
	}
	return out
}

func (sl *SpecLoader) specNameIndex() map[string]string {
	if sl == nil || sl.specsDir == emptyValue {
		return nil
	}
	roots := sl.applicableExtraSpecRoots()
	idx, _ := sl.specIndex.Load(specIndexKey(sl.specsDir, roots), specIndexStamp(sl.specsDir, roots), func() (map[string]string, error) {
		return mergeSpecIndexes(sl.specsDir, roots), nil
	})
	return idx
}

// resolveSpecFilePath resolves a spec file path directly, or via recursive index across subdirectories
func (sl *SpecLoader) resolveSpecFilePath(specFile string) string {
	if specFile == emptyValue {
		return ""
	}
	if filepath.IsAbs(specFile) {
		return specFile
	}
	if hit := paths.FindDomainFile(sl.specsDir, specFile); hit != "" {
		return hit
	}
	if base := filepath.Base(specFile); base != specFile {
		if hit := paths.FindDomainFile(sl.specsDir, base); hit != "" {
			return hit
		}
	}
	idx := sl.specNameIndex()
	base := filepath.Base(specFile)
	if hit := idx[base]; hit != "" {
		return hit
	}
	ontology := strings.TrimSuffix(base, ".yaml")
	if hit := idx[ontology]; hit != "" {
		return hit
	}
	return filepath.Join(sl.specsDir, specFile)
}

// loadFieldReference loads and parses a field definition from built_ins/fields/ or fields/
func (sl *SpecLoader) loadFieldReference(ref string) map[string]any {
	if ref == emptyValue {
		return nil
	}
	cands := sl.fieldRefCandidates(ref)
	parsed, _ := sl.fieldRefs.Load(ref, stampmemo.OfAll(cands...), func() (map[string]any, error) {
		for _, absPath := range cands {
			data, err := fileutil.ReadFile(absPath)
			if err != nil {
				continue
			}
			var parsed map[string]any
			if err := yaml.Unmarshal(data, &parsed); err == nil {
				return parsed, nil
			}
		}
		return nil, nil
	})
	return parsed
}

func (sl *SpecLoader) fieldRefCandidates(ref string) []string {
	base := filepath.Base(ref)
	cands := []string{
		ref,
		ref + ".yaml",
		filepath.Join(sl.specsDir, ref),
		filepath.Join(sl.specsDir, ref+".yaml"),
		filepath.Join(sl.specsDir, "built_ins", "fields", ref+".yaml"),
		filepath.Join(sl.specsDir, "built_ins", "fields", base+".yaml"),
		filepath.Join(sl.specsDir, "fields", ref+".yaml"),
		filepath.Join(sl.specsDir, "fields", base+".yaml"),
	}
	out := make([]string, 0, len(cands))
	for _, p := range cands {
		if filepath.IsAbs(p) {
			out = append(out, p)
			continue
		}
		out = append(out, filepath.Join(sl.specsDir, p))
	}
	return out
}

// resolveFieldReferences scans spec.ResolvedFields for $ref / ref references, resolves them, and merges definitions
func (sl *SpecLoader) resolveFieldReferences(spec *Spec) {
	if spec == nil || len(spec.ResolvedFields) == 0 {
		return
	}
	for fieldName, fieldDef := range spec.ResolvedFields {
		fieldMap, ok := fieldDef.(map[string]any)
		if !ok {
			continue
		}
		refVal, hasRef := fieldMap["$ref"]
		if !hasRef {
			refVal, hasRef = fieldMap["ref"]
		}
		if !hasRef {
			continue
		}
		refStr, ok := refVal.(string)
		if !ok || refStr == emptyValue {
			continue
		}
		resolvedRef := sl.loadFieldReference(refStr)
		if resolvedRef != nil {
			merged := make(map[string]any, len(resolvedRef)+len(fieldMap))
			for k, v := range resolvedRef {
				merged[k] = v
			}
			for k, v := range fieldMap {
				if k != "$ref" && k != "ref" {
					merged[k] = v
				}
			}
			spec.ResolvedFields[fieldName] = merged
		}
	}
}

// maxSpecInheritanceDepth limits recursion to prevent hang from cycles or bugs (see GENERATE_BUILDERS_SAMPLE_ANALYSIS.md)
const maxSpecInheritanceDepth = 30

// pathRefForSpecFile returns a path reference (prefix:relPath or abs:absolutePath) for the given spec file path.
// Enables spec-like objects backed by YAML to reference their location for reuse and resolution.
func pathRefForSpecFile(filePath string) string {
	if filePath == emptyValue {
		return ""
	}
	if filepath.IsAbs(filePath) {
		return paths.PathSchemeAbs + filePath
	}
	return paths.PathRefFromRelPath(filePath)
}

// loadSpecWithInheritanceRecursive recursively loads spec and resolves inheritance
// This is called by LoadSpecWithInheritance which handles caching
// ontology parameter is the ontology of the current spec (already known from parent call)
// depth is the current recursion depth (0 = top level); used to cap recursion and avoid unbounded stack growth.
func (sl *SpecLoader) loadSpecWithInheritanceRecursive(specFile string, visited map[string]bool, ontology string, depth int) (*Spec, error) {
	if depth > maxSpecInheritanceDepth {
		return nil, errfmt.Errorf("spec inheritance depth exceeded %d (possible cycle or bug): %s", maxSpecInheritanceDepth, specFile)
	}
	specPath := sl.absSpecPath(specFile)

	if ontology == emptyValue {
		ontology = sl.getOntologyFromFile(specPath)
	}

	if ontology != emptyValue {
		if visited[ontology] {
			return nil, errfmt.Errorf("circular inheritance detected: %s", ontology)
		}
		visited[ontology] = true
	}

	data, err := sl.readCachedBytes(specPath)
	if err != nil {
		return nil, errfmt.Errorf("failed to read spec file %s: %w", specPath, err)
	}

	// Validate against JSON schema if $schema is present (optional validation)
	// Skip when ZQK_SKIP_SPEC_SCHEMA_VALIDATION=1 to avoid deep recursion/hang in generate-builders (GENERATE_BUILDERS_SAMPLE_ANALYSIS.md)
	var tempSpec struct {
		Schema string `yaml:"$schema"`
	}
	if !config.ValidationSkipSpecSchemaValidation().OrDefault(false) && yaml.Unmarshal(data, &tempSpec) == nil {
		if schemaRef := tempSpec.Schema; schemaRef != emptyValue {
			// Try to find schemas directory relative to project root
			schemasDir := filepath.Join(paths.ProjectDataDir, "cli", "specs", "schemas")
			// Try to resolve from specsDir
			if sl.specsDir != emptyValue {
				dir := sl.specsDir
				for i := 0; i < 10; i++ { // Limit depth
					testPath := filepath.Join(dir, "..", "..", "..", paths.ProjectDataDir, "cli", "specs", "schemas")
					if abs, err := filepath.Abs(testPath); err == nil {
						if _, err := fileutil.Stat(abs); err == nil {
							schemasDir = abs
							break
						}
					}
					parent := filepath.Dir(dir)
					if parent == dir {
						break
					}
					dir = parent
				}
			}
			validator := yamlspec.NewSchemaValidator(schemasDir)
			if err := validator.ValidateYAML(specPath, schemaRef); err != nil {
				// Log warning but don't fail (schema validation is optional for now)
				// In the future, this could be made strict
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				logging.Fluent(logger).Warn("Schema validation failed for spec").
					String("spec_path", specPath).
					String("schema_ref", schemaRef).
					WithError(err).
					Log()
				// Uncomment to make schema validation strict:
				// return nil, stdErrors.New(errfmt.Newf("schema validation failed for %s: %w", specPath, err).Build())
			}
		}
	}

	var spec Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, errfmt.Errorf("failed to parse spec file %s: %w", specPath, err)
	}

	// Normalize "null" extends to empty string (YAML "null" is a valid value but we treat it as no parent)
	if spec.Extends == "null" {
		spec.Extends = ""
	}

	// Initialize resolved fields and traits with current spec's values
	// ResolvedFields stores the complete field definitions (including type, validation, checklist, etc.)
	spec.ResolvedFields = make(map[string]any)
	if spec.Fields != nil {
		// Fields is a map of field definitions, each containing type, validation, checklist, etc.
		for k, v := range spec.Fields {
			spec.ResolvedFields[k] = v
		}
	}
	spec.ResolvedTraits = make([]string, len(spec.Traits))
	copy(spec.ResolvedTraits, spec.Traits)

	// If this spec extends another, load and merge parent
	if spec.Extends != emptyValue && spec.Extends != "null" {
		parentFile := spec.Extends + ".yaml"
		parentSpec, err := sl.loadSpecCached(parentFile, visited, depth+1)
		if err != nil {
			return nil, errfmt.Errorf("failed to load parent spec %s: %w", parentFile, err)
		}

		mergeResolvedOverlay(&spec, parentSpec)
		// Do not inherit kernel_critical — profile inference (or this file's explicit value) owns it.
	}
	if err := sl.mergeComposes(&spec, depth); err != nil {
		return nil, err
	}
	applyTraitExclusions(&spec)
	sl.resolveFieldReferences(&spec)

	if spec.StorageProfile != "" {
		if _, err := datacell.ParseStorageProfile(spec.StorageProfile); err != nil {
			return nil, errfmt.Errorf("spec %s: %w", specPath, err)
		}
	}
	applyKernelCriticalDefaults(&spec)

	// Set location reference (prefix: or abs:) so the backing file can be re-used where necessary
	spec.Location = pathRefForSpecFile(specPath)

	return &spec, nil
}

// ConvertInstanceVersionToBuilderVersion converts instance schema_version format to builder version format
// Instance format: InitialFieldVersion -> Builder format: "v1_0_0"
func ConvertInstanceVersionToBuilderVersion(instanceVersion string) string {
	if instanceVersion == emptyValue {
		return ""
	}
	// Remove leading 'v' if present, then add 'v' and replace dots with underscores
	version := strings.TrimPrefix(instanceVersion, "v")
	return "v" + strings.ReplaceAll(version, ".", "_")
}

// ConvertBuilderVersionToInstanceVersion converts builder version format to instance schema_version format
// Builder format: "v1_0_0" -> Instance format: InitialFieldVersion
func ConvertBuilderVersionToInstanceVersion(builderVersion string) string {
	if builderVersion == emptyValue {
		return ""
	}
	// Remove leading 'v', then replace underscores with dots
	version := strings.TrimPrefix(builderVersion, "v")
	return strings.ReplaceAll(version, "_", ".")
}

// LoadSpecByVersion loads a spec using the builder registry for the specified ontology and version
// This allows loading specs at specific versions (e.g., ledger InitialFieldVersion vs DefaultSchemaVersion)
// Instance version format: InitialFieldVersion (will be converted to builder format "v1_0_0")
// Returns a spec with resolved inheritance
// Requires SetBuilderRegistry to be called first (typically done by initialization code)
func (sl *SpecLoader) LoadSpecByVersion(ontology, instanceVersion string) (*Spec, error) {
	registry := sl.getBuilderRegistry()

	if registry == nil {
		return nil, errfmt.Errorf("builder registry not set - cannot load spec by version")
	}

	// Convert instance version format to builder version format
	builderVersion := ConvertInstanceVersionToBuilderVersion(instanceVersion)

	// Get builder from registry
	builder, err := registry.GetBuilder(ontology, builderVersion)
	if err != nil {
		// If exact version not found, try to use latest version as fallback
		// This handles cases where builder version (v1_0_0) doesn't match instance schema_version (2.0.0)
		latestVersion, latestErr := registry.GetLatestVersion(ontology)
		if latestErr == nil && latestVersion != builderVersion {
			builder, err = registry.GetBuilder(ontology, latestVersion)
			if err == nil {
				// Successfully loaded using latest version
				// Builder version and instance schema_version can differ (builder v1_0_0 can generate schema_version 2.0.0)
			}
		}
		if err != nil {
			// Fallback: if builder is not compiled into the registry, load spec from YAML file
			specFile := ontology + ".yaml"
			if fallbackSpec, fallbackErr := sl.LoadSpecWithInheritance(specFile); fallbackErr == nil && fallbackSpec != nil {
				return fallbackSpec, nil
			}
			return nil, errfmt.Errorf("failed to get builder for ontology %s version %s (tried %s and latest): %w", ontology, instanceVersion, builderVersion, err)
		}
	}

	// Build the spec from the builder
	spec := builder.Build()

	// Builders don't resolve inheritance, so we need to resolve it
	// Use the existing inheritance resolution logic
	// IMPORTANT: Clear cache before resolving inheritance to ensure parent specs are loaded fresh
	// This prevents stale cached parent specs from being used (e.g., with old validation patterns)
	if spec.Extends != emptyValue && spec.Extends != "null" {
		sl.invalidateOntology(spec.Extends)
	}
	for _, mixin := range spec.Composes {
		sl.invalidateOntology(mixin)
	}

	visited := make(map[string]bool)
	resolvedSpec, err := sl.resolveSpecInheritance(spec, visited, 0)
	if err != nil {
		return nil, errfmt.Errorf("failed to resolve inheritance for ontology %s version %s: %w", ontology, instanceVersion, err)
	}

	return resolvedSpec, nil
}

// resolveSpecInheritance resolves inheritance for a spec (used by both YAML and builder loading)
func (sl *SpecLoader) resolveSpecInheritance(spec *Spec, visited map[string]bool, depth int) (*Spec, error) {
	if depth > maxSpecInheritanceDepth {
		return nil, errfmt.Errorf("spec inheritance depth exceeded %d (possible cycle or bug): %s", maxSpecInheritanceDepth, spec.Ontology)
	}
	// Check for circular inheritance
	if spec.Ontology != emptyValue {
		if visited[spec.Ontology] {
			return nil, errfmt.Errorf("circular inheritance detected: %s", spec.Ontology)
		}
		visited[spec.Ontology] = true
	}

	// Initialize resolved fields and traits with current spec's values
	resolvedSpec := &Spec{
		SchemaVersion:  spec.SchemaVersion,
		Ontology:       spec.Ontology,
		URN:            spec.URN,
		Namespace:      spec.Namespace,
		Extends:        spec.Extends,
		Composes:       append([]string(nil), spec.Composes...),
		Visibility:     spec.Visibility,
		Description:    spec.Description,
		StorageProfile: spec.StorageProfile,
		KernelCritical: cloneBoolPtr(spec.KernelCritical),
		Traits:         spec.Traits,
		ExcludeTraits:  spec.ExcludeTraits,
		Fields:         spec.Fields,
		ResolvedFields: make(map[string]any),
		ResolvedTraits: make([]string, len(spec.Traits)),
	}
	copy(resolvedSpec.ResolvedTraits, spec.Traits)

	// Copy fields to resolved fields
	if spec.Fields != nil {
		for k, v := range spec.Fields {
			resolvedSpec.ResolvedFields[k] = v
		}
	}

	// If this spec extends another, load and merge parent
	if spec.Extends != emptyValue && spec.Extends != "null" {
		// Try to load parent from builder registry first (if available)
		// Otherwise fall back to YAML file loading
		var parentSpec *Spec
		var err error

		// Try builder registry first - check if parent has builders registered
		registry := sl.getBuilderRegistry()

		if registry != nil {
			if versions := registry.GetVersions(spec.Extends); len(versions) > 0 {
				// Parent has builders - use latest version for inheritance resolution
				latestVersion, err := registry.GetLatestVersion(spec.Extends)
				if err == nil {
					builder, err := registry.GetBuilder(spec.Extends, latestVersion)
					if err == nil {
						parentSpecFromBuilder := builder.Build()
						// Recursively resolve parent's inheritance
						if resolvedParent, resolveErr := sl.resolveSpecInheritance(parentSpecFromBuilder, visited, depth+1); resolveErr == nil {
							parentSpec = resolvedParent
						}
					}
				}
			}
		}

		// Fall back to YAML file if builder didn't work
		if parentSpec == nil {
			parentFile := spec.Extends + ".yaml"
			sl.invalidateOntology(spec.Extends)
			if parentFile == KindBaseObject+".yaml" || parentFile == KindWorkInterval+".yaml" || parentFile == KindWorkUnit+".yaml" || parentFile == KindOccupancy+".yaml" || parentFile == KindRemainingOpen+".yaml" {
				sl.invalidateOntology(KindAuditable)
				sl.invalidateOntology(KindBaseObject)
				sl.invalidateOntology(KindWorkInterval)
				sl.invalidateOntology(KindWorkUnit)
				sl.invalidateOntology(KindOccupancy)
				sl.invalidateOntology(KindRemainingOpen)
			}
			parentSpec, err = sl.loadSpecWithInheritanceRecursive(parentFile, visited, "", depth+1)
			if err != nil {
				return nil, errfmt.Errorf("failed to load parent spec %s: %w", parentFile, err)
			}
		}

		mergeResolvedOverlay(resolvedSpec, parentSpec)
		// Do not inherit kernel_critical — keep only this file's explicit value; else profile defaults.
	}
	if err := sl.mergeComposes(resolvedSpec, depth); err != nil {
		return nil, err
	}
	applyTraitExclusions(resolvedSpec)

	if resolvedSpec.StorageProfile != "" {
		if _, err := datacell.ParseStorageProfile(resolvedSpec.StorageProfile); err != nil {
			return nil, errfmt.Errorf("spec %s: %w", resolvedSpec.Ontology, err)
		}
	}
	applyKernelCriticalDefaults(resolvedSpec)

	return resolvedSpec, nil
}

func applyTraitExclusions(spec *Spec) {
	if spec == nil || len(spec.ExcludeTraits) == 0 || len(spec.ResolvedTraits) == 0 {
		return
	}
	excluded := make(map[string]bool, len(spec.ExcludeTraits))
	for _, t := range spec.ExcludeTraits {
		if t != emptyValue {
			excluded[t] = true
		}
	}
	filtered := make([]string, 0, len(spec.ResolvedTraits))
	for _, t := range spec.ResolvedTraits {
		if !excluded[t] {
			filtered = append(filtered, t)
		}
	}
	spec.ResolvedTraits = filtered
}

var (
	globalSpecLoader *SpecLoader
	specLoaderOnce   sync.Once
)

// GetGlobalSpecLoader returns the global spec loader instance
// This ensures caches are shared across all validation operations (sync and async)
func GetGlobalSpecLoader() *SpecLoader {
	specLoaderOnce.Do(func() {
		globalSpecLoader = NewSpecLoader("")
	})
	return globalSpecLoader
}

// DiscoverOntologies returns all available ontologies in the specs directory and its subdirectories.
func (sl *SpecLoader) DiscoverOntologies() ([]string, error) {
	if sl == nil || sl.specsDir == emptyValue {
		return nil, nil
	}
	idx := sl.specNameIndex()
	var ontologies []string
	for name := range idx {
		if strings.HasSuffix(name, ".yaml") {
			continue
		}
		ontologies = append(ontologies, name)
	}
	sort.Strings(ontologies)
	return ontologies, nil
}

// LoadSpec loads a single spec by ontology (without inheritance).
func (sl *SpecLoader) LoadSpec(ontology string) (*Spec, error) {
	specPath := sl.resolveSpecFilePath(ontology + ".yaml")
	data, _, err := sl.readSpecFile(context.Background(), specPath)
	if err != nil {
		return nil, err
	}
	var spec Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, errfmt.Newf("failed to parse spec").Wrap(err)
	}
	if spec.Ontology == "" {
		spec.Ontology = ontology
	}
	return &spec, nil
}

// GetMetrics returns the metrics snapshot for this spec loader
// Useful for monitoring lock contention and timeout frequency
func (sl *SpecLoader) GetMetrics() SpecLoaderMetricsSnapshot {
	return sl.metrics.GetSnapshot()
}

// SpecCacheRevision returns a monotonic counter incremented whenever this loader clears or
// invalidates its in-memory spec cache (ClearCache, InvalidateSpec, InvalidateSpecByFile).
// It is the loader-local contribution to a unified spec-plane snapshot story; compare before/after
// regenerating spec-derived artifacts (spec index, dependency graph walks, kind lists).
func (sl *SpecLoader) SpecCacheRevision() uint64 {
	if sl == nil {
		return 0
	}
	return sl.cacheRevision.Load()
}

func (sl *SpecLoader) bumpSpecCacheRevision() {
	sl.cacheRevision.Add(1)
}

// FindSpecsDir returns the object specs directory (.zqk/specs/objects or equivalent).
// Used by SpecLoader and by scheduler cache prewarm to discover spec files.
func FindSpecsDir() string {
	return findSpecsDir()
}

// baseObjectSpecName is the spec file that must exist for a valid specs dir (avoids using a bare .zqk/specs/objects from a sibling .zqk project).
const baseObjectSpecName = "base_object.yaml"

// normalizeSpecsDirIfProjectRoot returns .zqk/specs/objects under root
// when root does not contain base_object.yaml but that nested path does. Otherwise returns
// specsDir unchanged (already the object_specs dir, or unknown layout).
func normalizeSpecsDirIfProjectRoot(specsDir string) string {
	if specsDir == emptyValue {
		return ""
	}
	clean := filepath.Clean(specsDir)
	if specsDirHasBaseSpec(clean) {
		return clean
	}
	nested := filepath.Join(clean, paths.ProcessInternalObjectSpecsDir)
	if specsDirHasBaseSpec(nested) {
		return nested
	}
	return clean
}

// specsDirHasBaseSpec returns true if the directory contains base_object.yaml (directly or in kernel/dna subdirectories).
func specsDirHasBaseSpec(specsDir string) bool {
	_, err := paths.FindObjectSpecFile(specsDir, KindBaseObject)
	return err == nil
}

var discoveredSpecsDirs stampmemo.Table[string] // keyed by cwd+\0+testRoot (closed per process)

func findSpecsDir() string {
	wd, err := fileutil.Getwd()
	if err != nil {
		wd = ""
	}
	testRoot := zqkenv.TestRoot().Get()
	dir, _ := discoveredSpecsDirs.Load(wd+"\x00"+testRoot, stampmemo.Of(wd), func() (string, error) {
		return locateSpecsDir(wd, testRoot), nil
	})
	return dir
}

func locateSpecsDir(wd, testRoot string) string {
	if testRoot != emptyValue {
		specsDir := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir)
		if info, err := fileutil.Stat(specsDir); err == nil && info.IsDir() && specsDirHasBaseSpec(specsDir) {
			return specsDir
		}
	}
	if hit := paths.FirstExistingFromCwd(paths.ProcessInternalObjectSpecsDir); hit != emptyValue && specsDirHasBaseSpec(hit) {
		return hit
	}
	return ""
}
