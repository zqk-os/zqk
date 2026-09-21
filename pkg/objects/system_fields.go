package objects

import (
	"maps"
	"path/filepath"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
)

// SystemFieldsRegistry derives system-generated fields from specs.
// Fields are considered system-generated if they have:
//   - checklist.authority == "automation" (or contains "automation")
//   - permissions == "r-x" (read-only)
//   - lifecycle == "immutable" AND authority contains "automation"
//
// Memo is stamp-invalidated from base_object.yaml + auditable.yaml. Reload
// Delete()s the key after an out-of-band spec write whose mtime may not have moved.
type SystemFieldsRegistry struct {
	specLoader *SpecLoader
}

var (
	globalSystemFieldsRegistry     *SystemFieldsRegistry
	globalSystemFieldsRegistryOnce sync.Once
	systemFieldsBySpecsDir         stampmemo.Table[map[string]bool]
)

// GetGlobalSystemFieldsRegistry returns the global system fields registry
func GetGlobalSystemFieldsRegistry() *SystemFieldsRegistry {
	globalSystemFieldsRegistryOnce.Do(func() {
		globalSystemFieldsRegistry = &SystemFieldsRegistry{
			specLoader: GetGlobalSpecLoader(),
		}
	})
	return globalSystemFieldsRegistry
}

// IsSystemGeneratedField checks if a field is system-generated for CREATE operations
func (sfr *SystemFieldsRegistry) IsSystemGeneratedField(fieldName string) (bool, error) {
	cache, err := sfr.cachedFields()
	if err != nil {
		return false, err
	}
	return cache[fieldName], nil
}

// GetSystemGeneratedFields returns all system-generated field names
func (sfr *SystemFieldsRegistry) GetSystemGeneratedFields() (map[string]bool, error) {
	cache, err := sfr.cachedFields()
	if err != nil {
		return nil, err
	}
	return maps.Clone(cache), nil
}

func (sfr *SystemFieldsRegistry) specsDir() string {
	if sfr == nil || sfr.specLoader == nil {
		return findSpecsDir()
	}
	if sfr.specLoader.specsDir != emptyValue {
		return sfr.specLoader.specsDir
	}
	return findSpecsDir()
}

func (sfr *SystemFieldsRegistry) cachedFields() (map[string]bool, error) {
	dir := sfr.specsDir()
	base := paths.FindDomainFile(dir, "base_object.yaml")
	aud := paths.FindDomainFile(dir, "auditable.yaml")
	if base == emptyValue {
		base = filepath.Join(dir, "base_object.yaml")
	}
	if aud == emptyValue {
		aud = filepath.Join(dir, "auditable.yaml")
	}
	return systemFieldsBySpecsDir.Load(dir, stampmemo.OfAll(base, aud), func() (map[string]bool, error) {
		return sfr.readSystemFields()
	})
}

func (sfr *SystemFieldsRegistry) readSystemFields() (map[string]bool, error) {
	loader := sfr.specLoader
	if loader == nil {
		loader = GetGlobalSpecLoader()
	}
	baseSpec, err := loader.LoadSpecWithInheritance("base_object.yaml")
	if err != nil {
		return nil, errfmt.Newf("failed to load base_object spec").Wrap(err)
	}
	auditableSpec, err := loader.LoadSpecWithInheritance("auditable.yaml")
	if err != nil {
		auditableSpec = nil
	}
	cache := make(map[string]bool)
	add := func(spec *Spec) {
		if spec == nil || spec.ResolvedFields == nil {
			return
		}
		for fieldName, fieldDef := range spec.ResolvedFields {
			fieldDefMap, ok := fieldDef.(map[string]any)
			if !ok {
				continue
			}
			if isSystemGeneratedFieldDef(fieldDefMap) {
				cache[fieldName] = true
			}
		}
	}
	add(baseSpec)
	add(auditableSpec)
	return cache, nil
}

func isSystemGeneratedFieldDef(fieldDef map[string]any) bool {
	if permissions, ok := fieldDef[FieldKeyPermissions].(string); ok {
		if permissions == "r-x" {
			return true
		}
	}
	if checklist, ok := fieldDef["checklist"].(map[string]any); ok {
		if authority, ok := checklist[FieldKeyAuthority].(string); ok {
			if strings.Contains(strings.ToLower(authority), "automation") {
				return true
			}
		}
	}
	return false
}

// Reload drops the memo and reloads system fields from specs.
func (sfr *SystemFieldsRegistry) Reload() error {
	systemFieldsBySpecsDir.Delete(sfr.specsDir())
	_, err := sfr.cachedFields()
	return err
}
