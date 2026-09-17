package objects

import (
	"maps"
	"strings"
	"sync"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
)

// SystemFieldsRegistry derives and caches system-generated fields from specs
// Fields are considered system-generated if they have:
//   - checklist.authority == "automation" (or contains "automation")
//   - permissions == "r-x" (read-only)
//   - lifecycle == "immutable" AND authority contains "automation"
//
// This follows the spec-driven pattern: derive from specs, cache the result,
// and rely on SpecLoader's cache invalidation to keep it consistent.
type SystemFieldsRegistry struct {
	specLoader *SpecLoader
	cache      map[string]bool // field name -> is system-generated
	mu         sync.RWMutex
	loaded     bool
}

var (
	globalSystemFieldsRegistry     *SystemFieldsRegistry
	globalSystemFieldsRegistryOnce sync.Once
)

// GetGlobalSystemFieldsRegistry returns the global system fields registry
func GetGlobalSystemFieldsRegistry() *SystemFieldsRegistry {
	globalSystemFieldsRegistryOnce.Do(func() {
		specLoader := GetGlobalSpecLoader()
		globalSystemFieldsRegistry = &SystemFieldsRegistry{
			specLoader: specLoader,
			cache:      make(map[string]bool),
		}
	})
	return globalSystemFieldsRegistry
}

// IsSystemGeneratedField checks if a field is system-generated for CREATE operations
// Returns cached result, reloading from specs if cache is invalid.
// Uses RunInRLock (no logger in critical section) to avoid lock-order deadlock with
// logging when called from many goroutines (e.g. template generation).
func (sfr *SystemFieldsRegistry) IsSystemGeneratedField(fieldName string) (bool, error) {
	var loaded bool
	_ = concurrency.RunInRLock(&sfr.mu, func() error {
		loaded = sfr.loaded
		return nil
	})

	if !loaded {
		if err := sfr.loadSystemFields(); err != nil {
			return false, err
		}
	}

	var result bool
	_ = concurrency.RunInRLock(&sfr.mu, func() error {
		result = sfr.cache[fieldName]
		return nil
	})
	return result, nil
}

// GetSystemGeneratedFields returns all system-generated field names
func (sfr *SystemFieldsRegistry) GetSystemGeneratedFields() (map[string]bool, error) {
	var loaded bool
	_ = concurrency.RunInRLock(&sfr.mu, func() error {
		loaded = sfr.loaded
		return nil
	})

	if !loaded {
		if err := sfr.loadSystemFields(); err != nil {
			return nil, err
		}
	}

	var result map[string]bool
	_ = concurrency.RunInRLock(&sfr.mu, func() error {
		result = maps.Clone(sfr.cache)
		return nil
	})
	return result, nil
}

// loadSystemFields derives system-generated fields from base_object and auditable specs
// This is cached - reload happens when SpecLoader cache invalidates
func (sfr *SystemFieldsRegistry) loadSystemFields() error {
	var alreadyLoaded bool
	err := concurrency.RunInLockWithLogger(
		&sfr.mu, LockNameSystemFieldsLoadCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			alreadyLoaded = sfr.loaded
			return nil
		},
	)
	if err != nil {
		return err
	}

	// Double-check after acquiring write lock
	if alreadyLoaded {
		return nil
	}

	// Load base_object spec (uses cached SpecLoader)
	baseSpec, err := sfr.specLoader.LoadSpecWithInheritance("base_object.yaml")
	if err != nil {
		return errfmt.Newf("failed to load base_object spec").Wrap(err)
	}

	// Load auditable spec (uses cached SpecLoader)
	auditableSpec, err := sfr.specLoader.LoadSpecWithInheritance("auditable.yaml")
	if err != nil {
		// auditable might not exist, that's okay
		auditableSpec = nil
	}

	// Build cache outside lock (copy-out/process pattern)
	cache := make(map[string]bool)

	// Derive system-generated fields from base_object
	if baseSpec.ResolvedFields != nil {
		for fieldName, fieldDef := range baseSpec.ResolvedFields {
			fieldDefMap, ok := fieldDef.(map[string]any)
			if !ok {
				continue
			}
			if sfr.isSystemGeneratedField(fieldName, fieldDefMap) {
				cache[fieldName] = true
			}
		}
	}

	// Derive system-generated fields from auditable
	if auditableSpec != nil && auditableSpec.ResolvedFields != nil {
		for fieldName, fieldDef := range auditableSpec.ResolvedFields {
			fieldDefMap, ok := fieldDef.(map[string]any)
			if !ok {
				continue
			}
			if sfr.isSystemGeneratedField(fieldName, fieldDefMap) {
				cache[fieldName] = true
			}
		}
	}

	// Copy-in: Update cache with lock
	err = concurrency.RunInLockWithLogger(
		&sfr.mu, LockNameSystemFieldsLoadUpdate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Double-check after re-acquiring lock
			if sfr.loaded {
				return nil
			}
			// Update cache
			maps.Copy(sfr.cache, cache)
			sfr.loaded = true
			return nil
		},
	)
	return err
}

// isSystemGeneratedField checks if a field definition indicates it's system-generated
// Fields are system-generated if:
//   - checklist.authority contains "automation"
//   - permissions == "r-x" (read-only)
func (sfr *SystemFieldsRegistry) isSystemGeneratedField(_ string, fieldDef map[string]any) bool {
	// Check permissions (r-x means read-only, system-managed)
	if permissions, ok := fieldDef[FieldKeyPermissions].(string); ok {
		if permissions == "r-x" {
			return true
		}
	}

	// Check checklist.authority
	if checklist, ok := fieldDef["checklist"].(map[string]any); ok {
		if authority, ok := checklist[FieldKeyAuthority].(string); ok {
			// Authority contains "automation" -> system-generated
			if strings.Contains(strings.ToLower(authority), "automation") {
				return true
			}
		}
	}

	return false
}

// Reload clears the cache and reloads system fields from specs
// This is called when specs change (via SpecLoader invalidation)
func (sfr *SystemFieldsRegistry) Reload() error {
	err := concurrency.RunInLockWithLogger(
		&sfr.mu, LockNameSystemFieldsReload, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			sfr.cache = make(map[string]bool)
			sfr.loaded = false
			return nil
		},
	)
	if err != nil {
		return err
	}

	return sfr.loadSystemFields()
}
