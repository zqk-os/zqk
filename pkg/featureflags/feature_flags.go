// Package featureflags loads feature toggles from .zqk/config/feature_flags.json.
//
// On-disk path is defined by [github.com/lanceman/zqk/pkg/datacell.FeatureFlagsPath] (runtime organism layout).
// Broader data-cell work: docs/architecture/DATA_CELL_RUNTIME_ORGANISM.md, ITEM-EXAMPLE.
// Team vocabulary: a **lite file** is this style of bounded project-local JSON with CLI integration;
// glossary_term GLS-EXAMPLE.
package featureflags

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
)

// FeatureFlag represents a feature flag configuration
type FeatureFlag struct {
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description,omitempty"`
}

// FeatureFlags manages feature flags for the system
type FeatureFlags struct {
	flags    map[string]*FeatureFlag
	mu       sync.RWMutex
	filePath string
}

var (
	globalFeatureFlags *FeatureFlags
	featureFlagsOnce   sync.Once
)

// GetGlobalFeatureFlags returns the global feature flags instance
func GetGlobalFeatureFlags(projectRoot string) *FeatureFlags {
	featureFlagsOnce.Do(func() {
		globalFeatureFlags = NewFeatureFlags(projectRoot)
		_ = globalFeatureFlags.Load() //nolint:errcheck // Load errors use default flags
	})
	return globalFeatureFlags
}

// NewFeatureFlags creates a new feature flags manager
func NewFeatureFlags(projectRoot string) *FeatureFlags {
	filePath := datacell.FeatureFlagsPath(projectRoot)
	return &FeatureFlags{
		flags:    make(map[string]*FeatureFlag),
		filePath: filePath,
	}
}

// Load loads feature flags from disk
// Fast, synchronous operation - no goroutines or timeouts that could hang
func (ff *FeatureFlags) Load() error {
	return concurrency.RunInLockWithLogger(
		&ff.mu, LockNameFeatureFlagsLoad, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {

			// Create directory if it doesn't exist
			if err := os.MkdirAll(filepath.Dir(ff.filePath), paths.DirPerm755); err != nil {
				return errfmt.Newf("failed to create feature flags directory").Wrap(err)
			}

			// Check if file exists
			if _, err := os.Stat(ff.filePath); os.IsNotExist(err) {
				// Initialize with default flags
				ff.initializeDefaults()
				// Don't save on first load - save only when explicitly changed
				return nil
			}

			// Read file synchronously (fast operation, shouldn't hang)
			data, err := os.ReadFile(ff.filePath)
			if err != nil {
				// File read failed - initialize defaults and continue
				ff.initializeDefaults()
				return nil // Don't error on read failure, just use defaults
			}

			var flagsData struct {
				Flags []*FeatureFlag `json:"flags"`
			}
			if err := json.Unmarshal(data, &flagsData); err != nil {
				// Invalid file, initialize defaults
				ff.initializeDefaults()
				return nil // Don't error on parse failure, just use defaults
			}

			// Load flags into map
			for _, flag := range flagsData.Flags {
				ff.flags[flag.Name] = flag
			}

			// Ensure defaults exist
			ff.initializeDefaults()

			return nil
		},
	)
}

// Save saves feature flags to disk
func (ff *FeatureFlags) Save() error {
	return concurrency.RunInRLockWithLogger(
		&ff.mu, LockNameFeatureFlagsSave, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			return ff.saveUnlocked()
		},
	)
}

// saveUnlocked saves feature flags to disk without acquiring a lock
// Caller must hold the lock (either RLock or Lock)
func (ff *FeatureFlags) saveUnlocked() error {
	// Convert map to array
	flags := make([]*FeatureFlag, 0, len(ff.flags))
	for _, flag := range ff.flags {
		flags = append(flags, flag)
	}

	flagsData := struct {
		Flags []*FeatureFlag `json:"flags"`
	}{
		Flags: flags,
	}

	data, err := json.MarshalIndent(flagsData, "", "  ")
	if err != nil {
		return errfmt.Newf("failed to marshal feature flags").Wrap(err)
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(ff.filePath), paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create feature flags directory").Wrap(err)
	}

	if err := os.WriteFile(ff.filePath, data, paths.FilePerm644); err != nil {
		return errfmt.Newf("failed to write feature flags file").Wrap(err)
	}

	return nil
}

// initializeDefaults initializes default feature flags
func (ff *FeatureFlags) initializeDefaults() {
	defaults := []*FeatureFlag{
		{
			Name:        "async_validation",
			Enabled:     false, // Default to sync for safety
			Description: "Use async validator for system check (experimental)",
		},
		{
			Name:        "async_validation_parallel",
			Enabled:     false,
			Description: "Enable parallel validation in async mode",
		},
		{
			Name:        "deferred_hash_updates",
			Enabled:     true, // Enabled by default
			Description: "Defer hash updates until all operations complete",
		},
		{
			Name:        "storage_orchestration",
			Enabled:     true, // Enabled by default
			Description: "Use storage orchestrator for multi-backend coordination",
		},
	}

	for _, flag := range defaults {
		if _, exists := ff.flags[flag.Name]; !exists {
			ff.flags[flag.Name] = flag
		}
	}
}

// IsEnabled checks if a feature flag is enabled
func (ff *FeatureFlags) IsEnabled(name string) bool {
	var enabled bool
	_ = concurrency.RunInRLockWithLogger(
		&ff.mu, LockNameFeatureFlagsIsEnabled, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			flag, exists := ff.flags[name]
			if !exists {
				enabled = false
				return nil
			}
			enabled = flag.Enabled
			return nil
		},
	)
	return enabled
}

// SetEnabled sets a feature flag's enabled state
func (ff *FeatureFlags) SetEnabled(name string, enabled bool) error {
	return concurrency.RunInLockWithLogger(
		&ff.mu, LockNameFeatureFlagsSetEnabled, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			flag, exists := ff.flags[name]
			if !exists {
				// Create new flag
				flag = &FeatureFlag{
					Name:    name,
					Enabled: enabled,
				}
				ff.flags[name] = flag
			} else {
				flag.Enabled = enabled
			}

			// Call saveUnlocked since we already hold the lock
			return ff.saveUnlocked()
		},
	)
}

// GetFlag returns a feature flag by name
func (ff *FeatureFlags) GetFlag(name string) (*FeatureFlag, bool) {
	var flag *FeatureFlag
	var exists bool
	_ = concurrency.RunInRLockWithLogger(
		&ff.mu, LockNameFeatureFlagsGetFlag, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			flag, ok = ff.flags[name]
			exists = ok
			return nil
		},
	)
	return flag, exists
}

// GetAllFlags returns all feature flags
func (ff *FeatureFlags) GetAllFlags() map[string]*FeatureFlag {
	var result map[string]*FeatureFlag
	_ = concurrency.RunInRLockWithLogger(
		&ff.mu, LockNameFeatureFlagsGetAll, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Return a copy
			result = make(map[string]*FeatureFlag)
			for name, flag := range ff.flags {
				result[name] = &FeatureFlag{
					Name:        flag.Name,
					Enabled:     flag.Enabled,
					Description: flag.Description,
				}
			}
			return nil
		},
	)
	return result
}

// Feature flag names
const (
	FlagAsyncValidation         = "async_validation"
	FlagAsyncValidationParallel = "async_validation_parallel"
	FlagDeferredHashUpdates     = "deferred_hash_updates"
	FlagStorageOrchestration    = "storage_orchestration"
)
