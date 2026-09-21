package objects

import (
	"sort"
	"sync"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// FieldInfo represents information about a field in an object spec
type FieldInfo struct {
	Name         string   // Field name
	Type         string   // Field type (string, integer, list, etc.)
	SemanticType string   // Semantic type (statement, reference, quantity, etc.)
	Traits       []string // Traits (listable, readable, writable, etc.)
	Required     bool     // Whether field is required
	Description  string   // Field description from checklist
	Kind         string   // Object kind this field belongs to (for specialized fields)
	Inherited    bool     // Whether field is inherited from base/auditable
	EnumValues   []string // Enum values (if type is enum)
	Lifecycle    string   // Lifecycle mode (immutable, read-only, mutable, etc.)
	StorageRole  string   // Storage role (primary, runtime_delta, etc.)
	Criticality  string   // Criticality (composition, association, etc.)
}

// FieldRegistry provides cached field information for all object kinds.
// Preferred pattern (CLI_PERFORMANCE §2): load once (PrewarmGlobalsForProjectRoot, in background),
// memo via stampmemo keyed by specs dir / SpecCacheRevision. Hot path uses
// GetFieldsForKindIfLoaded (Peek — never waits on LoadFields).
type FieldRegistry struct {
	specLoader *SpecLoader
}

type fieldRegistrySnapshot struct {
	cache        map[string]*KindFields
	commonFields []FieldInfo
}

var fieldRegistryMemos stampmemo.Table[fieldRegistrySnapshot]

// KindFields contains field information for a specific object kind
type KindFields struct {
	Kind              string      // Object kind
	AllFields         []FieldInfo // All fields (common + specialized)
	CommonFields      []FieldInfo // Fields inherited from base/auditable
	SpecializedFields []FieldInfo // Fields specific to this kind
}

// NewFieldRegistry creates a new field registry
func NewFieldRegistry(specLoader *SpecLoader) *FieldRegistry {
	return &FieldRegistry{specLoader: specLoader}
}

func (fr *FieldRegistry) memoKey() string {
	if fr == nil {
		return findSpecsDir()
	}
	if fr.specLoader != nil && fr.specLoader.specsDir != emptyValue {
		return fr.specLoader.specsDir
	}
	return findSpecsDir()
}

func (fr *FieldRegistry) memoStamp() stampmemo.Stamp {
	if fr != nil && fr.specLoader != nil {
		return stampmemo.Stamp(fr.specLoader.SpecCacheRevision())
	}
	return 0
}

func (fr *FieldRegistry) snapshot() (fieldRegistrySnapshot, bool) {
	return fieldRegistryMemos.Peek(fr.memoKey(), fr.memoStamp())
}

// LoadFields loads and caches field information for all object kinds
// Identifies common fields (from base_object, auditable) and specialized fields
func (fr *FieldRegistry) LoadFields() error {
	_, err := fieldRegistryMemos.Load(fr.memoKey(), fr.memoStamp(), fr.readFields)
	if err != nil {
		fieldRegistryMemos.Delete(fr.memoKey())
	}
	return err
}

func (fr *FieldRegistry) readFields() (fieldRegistrySnapshot, error) {
	// First, identify common fields from base_object and auditable (I/O outside lock).
	// Use fr.specLoader; if it fails (e.g. created before ZQK_TEST_ROOT was set), retry with
	// a loader from findSpecsDir() so tests and subprocesses that set ZQK_TEST_ROOT late still work.
	loader := fr.specLoader
	if loader == nil {
		loader = NewSpecLoader(findSpecsDir())
	}
	baseSpec, err := loader.LoadSpecWithInheritance("base_object.yaml")
	if err != nil {
		if specsDir := findSpecsDir(); specsDir != emptyValue {
			loader = NewSpecLoader(specsDir)
			baseSpec, err = loader.LoadSpecWithInheritance("base_object.yaml")
		}
		if err != nil {
			return fieldRegistrySnapshot{}, errfmt.Newf("failed to load base_object spec").Wrap(err)
		}
	}

	auditableSpec, err := loader.LoadSpecWithInheritance("auditable.yaml")
	if err != nil {
		// auditable might not exist, that's okay
		auditableSpec = nil
	}

	// Extract common fields from base_object (use ResolvedFields which includes inheritance)
	commonFieldsMap := make(map[string]FieldInfo)
	if baseSpec.ResolvedFields != nil {
		for fieldName, fieldDef := range baseSpec.ResolvedFields {
			fieldDefMap, ok := fieldDef.(map[string]any)
			if !ok {
				continue
			}
			fieldInfo := fr.extractFieldInfo(fieldName, fieldDefMap, "", true)
			commonFieldsMap[fieldName] = fieldInfo
		}
	}

	// Add auditable fields to common fields (use ResolvedFields)
	if auditableSpec != nil && auditableSpec.ResolvedFields != nil {
		for fieldName, fieldDef := range auditableSpec.ResolvedFields {
			// Only add if not already in base_object (avoid duplicates)
			if _, exists := commonFieldsMap[fieldName]; !exists {
				fieldDefMap, ok := fieldDef.(map[string]any)
				if !ok {
					continue
				}
				fieldInfo := fr.extractFieldInfo(fieldName, fieldDefMap, "", true)
				commonFieldsMap[fieldName] = fieldInfo
			}
		}
	}

	// Convert map to sorted slice
	commonFields := make([]FieldInfo, 0, len(commonFieldsMap))
	//nolint:gocritic // rangeValCopy: map iteration copies struct; acceptable for snapshot
	for _, field := range commonFieldsMap {
		commonFields = append(commonFields, field)
	}
	sort.Slice(commonFields, func(i, j int) bool {
		return commonFields[i].Name < commonFields[j].Name
	})

	// Now load fields for each object kind
	// Discover all spec files dynamically by scanning the specs directory
	specsDir := findSpecsDir()
	if specsDir == emptyValue {
		return fieldRegistrySnapshot{}, errfmt.Errorf("could not find specs directory")
	}

	// Verify directory exists
	if _, err := fileutil.Stat(specsDir); err != nil {
		return fieldRegistrySnapshot{}, errfmt.Newf("failed to read specs directory").Wrap(err)
	}

	ontologies, err := loader.DiscoverOntologies()
	if err != nil {
		return fieldRegistrySnapshot{}, errfmt.Newf("failed to discover ontologies").Wrap(err)
	}

	// Build cache outside lock (copy-out/process pattern)
	cache := make(map[string]*KindFields)
	config := GetGlobalKindMappingsConfig()

	for _, baseName := range ontologies {
		// Skip specs based on kind_mappings_config (avoids hardcoding)
		if config != nil && config.ShouldSkipSpec(baseName) {
			continue
		}

		// Load spec (use same loader as base_object/auditable so fallback path is consistent)
		specFile := baseName + ".yaml"
		spec, err := loader.LoadSpecWithInheritance(specFile)
		if err != nil {
			// Skip specs that can't be loaded
			continue
		}

		// Use ontology as kind (fallback to filename if ontology not set)
		kind := spec.Ontology
		if kind == emptyValue {
			kind = baseName
		}

		kindFields := &KindFields{
			Kind:              kind,
			AllFields:         []FieldInfo{},
			CommonFields:      make([]FieldInfo, len(commonFields)),
			SpecializedFields: []FieldInfo{},
		}

		// Copy common fields
		copy(kindFields.CommonFields, commonFields)

		// Extract specialized fields (fields not in common fields)
		// Use ResolvedFields which includes all inherited fields, but we filter out common ones
		specializedFieldsMap := make(map[string]FieldInfo)
		if spec.ResolvedFields != nil {
			for fieldName, fieldDef := range spec.ResolvedFields {
				// Check if this is a common field
				isCommon := false
				for i := range commonFields {
					commonField := &commonFields[i]
					if commonField.Name == fieldName {
						isCommon = true
						break
					}
				}

				if !isCommon {
					fieldDefMap, ok := fieldDef.(map[string]any)
					if !ok {
						continue
					}
					fieldInfo := fr.extractFieldInfo(fieldName, fieldDefMap, kind, false)
					specializedFieldsMap[fieldName] = fieldInfo
				}
			}
		}

		// Convert specialized fields to sorted slice
		kindFields.SpecializedFields = make([]FieldInfo, 0, len(specializedFieldsMap))
		//nolint:gocritic // rangeValCopy: map iteration copies struct; acceptable for snapshot
		for _, field := range specializedFieldsMap {
			kindFields.SpecializedFields = append(kindFields.SpecializedFields, field)
		}
		sort.Slice(kindFields.SpecializedFields, func(i, j int) bool {
			return kindFields.SpecializedFields[i].Name < kindFields.SpecializedFields[j].Name
		})

		// Inject status enum values from lifecycle registry
		if lifecycleLoader := GetGlobalLifecycleLoader(); lifecycleLoader != nil {
			if lc, err := lifecycleLoader.LoadLifecycle(kind); err == nil && lc != nil {
				var statuses []string
				for _, s := range lc.Statuses {
					statuses = append(statuses, s.Value)
				}
				for i := range kindFields.CommonFields {
					if kindFields.CommonFields[i].Name == "status" {
						kindFields.CommonFields[i].EnumValues = statuses
						break
					}
				}
				for i := range kindFields.SpecializedFields {
					if kindFields.SpecializedFields[i].Name == "status" {
						kindFields.SpecializedFields[i].EnumValues = statuses
						break
					}
				}
			}
		}

		// Rebuild AllFields array(common + specialized, sorted)
		allFieldsMap := make(map[string]FieldInfo)
		for i := range kindFields.CommonFields {
			allFieldsMap[kindFields.CommonFields[i].Name] = kindFields.CommonFields[i]
		}
		for i := range kindFields.SpecializedFields {
			allFieldsMap[kindFields.SpecializedFields[i].Name] = kindFields.SpecializedFields[i]
		}

		kindFields.AllFields = make([]FieldInfo, 0, len(allFieldsMap))
		//nolint:gocritic // rangeValCopy: map iteration copies struct; acceptable for snapshot
		for _, field := range allFieldsMap {
			kindFields.AllFields = append(kindFields.AllFields, field)
		}
		sort.Slice(kindFields.AllFields, func(i, j int) bool {
			return kindFields.AllFields[i].Name < kindFields.AllFields[j].Name
		})

		cache[kind] = kindFields
	}

	return fieldRegistrySnapshot{cache: cache, commonFields: commonFields}, nil
}

// GetFieldsForKindIfLoaded returns field info for kind only if the registry is already loaded.
// Does not call LoadFields(). Use on hot paths (e.g. object update, create) to avoid loading
// all 83 specs and spiking memory/latency (PRE_CHANGE_CHECKLIST §3).
func (fr *FieldRegistry) GetFieldsForKindIfLoaded(kind string) (*KindFields, bool) {
	snap, ok := fr.snapshot()
	if !ok {
		return nil, false
	}
	kindFields, exists := snap.cache[kind]
	if !exists {
		return nil, false
	}
	return kindFields, true
}

// GetFieldsForKind returns field information for a specific object kind
// Returns all fields (common + specialized), sorted by name.
// NOTE: Call GetFieldsForKindIfLoaded on create/update hot path to avoid LoadFields().
func (fr *FieldRegistry) GetFieldsForKind(kind string) (*KindFields, error) {
	if err := fr.LoadFields(); err != nil {
		return nil, err
	}
	snap, ok := fr.snapshot()
	if !ok {
		return nil, errfmt.Errorf("unknown object kind: %s", kind)
	}
	kindFields, exists := snap.cache[kind]
	if !exists {
		return nil, errfmt.Errorf("unknown object kind: %s", kind)
	}
	return kindFields, nil
}

// TryReloadFromFindSpecsDir resets the registry so the next LoadFields() will load from
// findSpecsDir() (e.g. after ZQK_TEST_ROOT is set). Used by RegisterDynamicKindCommands when
// the singleton was created before the test root was set. Returns true if specs dir was found.
func (fr *FieldRegistry) TryReloadFromFindSpecsDir() bool {
	specsDir := findSpecsDir()
	if specsDir == emptyValue {
		return false
	}
	fieldRegistryMemos.Delete(fr.memoKey())
	fr.specLoader = NewSpecLoader(specsDir)
	return true
}

// GetAllKinds returns a list of all object kinds that have field information
func (fr *FieldRegistry) GetAllKinds() ([]string, error) {
	if err := fr.LoadFields(); err != nil {
		return nil, err
	}
	snap, ok := fr.snapshot()
	if !ok {
		return nil, nil
	}
	kinds := make([]string, 0, len(snap.cache))
	for kind := range snap.cache {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds, nil
}

// GetCommonFields returns fields common to all object kinds
func (fr *FieldRegistry) GetCommonFields() ([]FieldInfo, error) {
	if err := fr.LoadFields(); err != nil {
		return nil, err
	}
	snap, ok := fr.snapshot()
	if !ok {
		return nil, nil
	}
	commonFields := make([]FieldInfo, len(snap.commonFields))
	copy(commonFields, snap.commonFields)
	return commonFields, nil
}

// Reload drops the memo and reloads field information
func (fr *FieldRegistry) Reload() error {
	fieldRegistryMemos.Delete(fr.memoKey())
	return fr.LoadFields()
}

// getBoolFromMap returns the bool value for key from m, or false if missing or not a bool.
func getBoolFromMap(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// getBoolFromMapAny returns the bool value for key from m, or false if missing or not a bool.
func getBoolFromMapAny(m map[interface{}]interface{}, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// getIntFromMap returns the first int value found for the given keys (e.g. "minCount", "min_count"). YAML may produce int or int64.
func getIntFromMap(m map[string]any, keys ...string) int {
	for _, key := range keys {
		v, ok := m[key]
		if !ok {
			continue
		}
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		}
	}
	return 0
}

// getIntFromMapAny returns the first int value found for the given keys from a map[interface{}]interface{}.
func getIntFromMapAny(m map[interface{}]interface{}, keys ...string) int {
	for _, key := range keys {
		v, ok := m[key]
		if !ok {
			continue
		}
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		}
	}
	return 0
}

// getStringFromMap returns the string value for key from m, or "" if missing or not a string.
func getStringFromMap(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// getStringFromMapAny returns the string value for key from m, or "" if missing or not a string.
func getStringFromMapAny(m map[interface{}]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// extractEnumValues appends string values from src ([]any from YAML) into dst.
func extractEnumValues(src interface{}, dst *[]string) {
	if dst == nil {
		return
	}
	s, ok := src.([]any)
	if !ok {
		return
	}
	for _, v := range s {
		if str, ok := v.(string); ok {
			*dst = append(*dst, str)
		}
	}
}

// extractFieldInfo extracts field information from a field definition
func (fr *FieldRegistry) extractFieldInfo(fieldName string, fieldDef map[string]any, kind string, inherited bool) FieldInfo {
	fieldInfo := FieldInfo{
		Name:      fieldName,
		Kind:      kind,
		Inherited: inherited,
	}

	// Extract type
	if typeVal, ok := fieldDef[FieldKeyType].(string); ok {
		fieldInfo.Type = typeVal
	}

	// Extract semantic_type
	if semanticType, ok := fieldDef["semantic_type"].(string); ok {
		fieldInfo.SemanticType = semanticType
	}

	// Extract traits
	if traits, ok := fieldDef["traits"].([]any); ok {
		fieldInfo.Traits = make([]string, 0, len(traits))
		for _, trait := range traits {
			if traitStr, ok := trait.(string); ok {
				fieldInfo.Traits = append(fieldInfo.Traits, traitStr)
			}
		}
	}

	// Extract required status from validation (YAML unmarshal may produce map[string]any or map[interface{}]interface{})
	if validationVal, ok := fieldDef["validation"]; ok && validationVal != nil {
		if validation, ok := validationVal.(map[string]any); ok {
			fieldInfo.Required = getBoolFromMap(validation, "required")
			if fieldInfo.Type == "enum" {
				extractEnumValues(validation["enum"], &fieldInfo.EnumValues)
			}
		} else if validation, ok := validationVal.(map[interface{}]interface{}); ok {
			fieldInfo.Required = getBoolFromMapAny(validation, "required")
			if fieldInfo.Type == "enum" {
				extractEnumValues(validation["enum"], &fieldInfo.EnumValues)
			}
		}
	}

	// Extract checklist information (lifecycle, storage_role, description)
	if checklistVal, ok := fieldDef["checklist"]; ok && checklistVal != nil {
		if checklist, ok := checklistVal.(map[string]any); ok {
			fieldInfo.Lifecycle = getStringFromMap(checklist, "lifecycle")
			if fieldInfo.Lifecycle == "" {
				fieldInfo.Lifecycle = getStringFromMap(checklist, "origin_lifecycle")
			}
			fieldInfo.StorageRole = getStringFromMap(checklist, "storage_role")
			fieldInfo.Description = getStringFromMap(checklist, "purpose")
			fieldInfo.Criticality = getStringFromMap(checklist, "criticality")
		} else if checklist, ok := checklistVal.(map[interface{}]interface{}); ok {
			fieldInfo.Lifecycle = getStringFromMapAny(checklist, "lifecycle")
			if fieldInfo.Lifecycle == "" {
				fieldInfo.Lifecycle = getStringFromMapAny(checklist, "origin_lifecycle")
			}
			fieldInfo.StorageRole = getStringFromMapAny(checklist, "storage_role")
			fieldInfo.Description = getStringFromMapAny(checklist, "purpose")
			fieldInfo.Criticality = getStringFromMapAny(checklist, "criticality")
		}
	}

	// minCount >= 1 or min_count >= 1 also implies required (SHACL-style)
	if !fieldInfo.Required && (fieldInfo.Type == "list" || fieldInfo.Type == "map" || fieldInfo.Type == "object") {
		if validationVal, ok := fieldDef["validation"]; ok && validationVal != nil {
			var minCount int
			if m, ok := validationVal.(map[string]any); ok {
				minCount = getIntFromMap(m, "minCount", "min_count")
			} else if m, ok := validationVal.(map[interface{}]interface{}); ok {
				minCount = getIntFromMapAny(m, "minCount", "min_count")
			}
			if minCount >= 1 {
				fieldInfo.Required = true
			}
		}
	}

	return fieldInfo
}

var (
	globalFieldRegistry   *FieldRegistry
	globalFieldRegistryMu sync.RWMutex
)

func GetGlobalFieldRegistry() *FieldRegistry {
	globalFieldRegistryMu.RLock()
	if globalFieldRegistry != nil {
		defer globalFieldRegistryMu.RUnlock()
		return globalFieldRegistry
	}
	globalFieldRegistryMu.RUnlock()

	globalFieldRegistryMu.Lock()
	defer globalFieldRegistryMu.Unlock()
	if globalFieldRegistry == nil {
		specLoader := NewSpecLoader(findSpecsDir())
		globalFieldRegistry = NewFieldRegistry(specLoader)
	}
	return globalFieldRegistry
}

// ResetGlobalFieldRegistryForTesting resets the global field registry singleton.
// This is required for test isolation when tests modify ZQK_TEST_ROOT.
func ResetGlobalFieldRegistryForTesting() {
	globalFieldRegistryMu.Lock()
	defer globalFieldRegistryMu.Unlock()
	fieldRegistryMemos.Reset()
	globalFieldRegistry = nil
}
