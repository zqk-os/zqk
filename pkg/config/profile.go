package config

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"

	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	"gopkg.in/yaml.v3"
)

// ProfileType represents the type of profile
type ProfileType string

const (
	ProfileTypeCLIContext        ProfileType = "cli_context"
	ProfileTypeMetricsSampler    ProfileType = "metrics_sampler"
	ProfileTypeTransceiverRouter ProfileType = "transceiver_router"
)

const (
	profileKindExpected = "profile"
	nullStringValue     = "null"
	yamlFileExtension   = ".yaml"
	hyphenChar          = "-"
	underscoreChar      = "_"

	errCircularInheritanceFmt = "circular inheritance detected: %s"
	errProfileNotFoundFmt     = "profile not found: %s"
	errReadProfileFmt         = "failed to read profile file %s: %w"
	errParseProfileFmt        = "failed to parse profile file %s: %w"
	errInvalidProfileKindFmt  = "invalid profile kind: %s (expected 'profile')"
	errLoadParentProfileFmt   = "failed to load parent profile %s: %w"
	errProfileTypeMismatchFmt = "profile type mismatch: %s extends %s but types differ (%s vs %s)"
	dotDotPathSegment         = ".."
	emptyPath                 = ""
)

// UnifiedProfile represents the unified profile schema
type UnifiedProfile struct {
	// Shared Header
	SchemaVersion string          `yaml:"schema_version"`
	Kind          string          `yaml:"kind"` // Should be "profile"
	Type          ProfileType     `yaml:"type"` // "cli_context" or "metrics_sampler"
	Metadata      ProfileMetadata `yaml:"metadata"`

	// Domain-Specific Spec (routed based on Type)
	Spec map[string]any `yaml:"spec"`

	// Resolved values after inheritance (computed)
	ResolvedSpec map[string]any `yaml:"-"`
}

// ProfileMetadata contains profile metadata
type ProfileMetadata struct {
	Name        string `yaml:"name"`
	Extends     string `yaml:"extends,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// ProfileLoader loads and resolves unified profiles with inheritance
type ProfileLoader struct {
	profilesDir string
	cache       map[string]*UnifiedProfile // Cache for loaded profiles (name -> profile)
	mu          sync.RWMutex               // Protects cache
}

// NewProfileLoader creates a new unified profile loader
func NewProfileLoader(profilesDir string) *ProfileLoader {
	if profilesDir == emptyPath {
		profilesDir = findProfilesDir()
	}
	return &ProfileLoader{
		profilesDir: profilesDir,
		cache:       make(map[string]*UnifiedProfile),
	}
}

// LoadProfileWithInheritance loads a profile and resolves its inheritance chain
func (pl *ProfileLoader) LoadProfileWithInheritance(profileName string) (*UnifiedProfile, error) {
	var cached *UnifiedProfile
	var fromCache bool
	_ = concurrency.RunInRLock(&pl.mu, func() error {
		if c, ok := pl.cache[profileName]; ok {
			cached = c
			fromCache = true
		}
		return nil
	})
	if fromCache {
		return cached, nil
	}

	visited := make(map[string]bool)
	profile, err := pl.loadProfileWithInheritanceRecursive(profileName, visited)
	if err != nil {
		return nil, err
	}

	var final *UnifiedProfile
	_ = concurrency.RunInLock(&pl.mu, func() error {
		if c, ok := pl.cache[profileName]; ok {
			final = c
			return nil
		}
		pl.cache[profileName] = profile
		final = profile
		return nil
	})
	return final, nil
}

// loadProfileWithInheritanceRecursive recursively loads profile and resolves inheritance
func (pl *ProfileLoader) loadProfileWithInheritanceRecursive(profileName string, visited map[string]bool) (*UnifiedProfile, error) {
	// Check for circular inheritance
	if visited[profileName] {
		return nil, errfmt.Errorf(errCircularInheritanceFmt, profileName)
	}
	visited[profileName] = true

	// Resolve profile file path
	profilePath := pl.resolveProfilePath(profileName)
	if profilePath == emptyPath {
		return nil, errfmt.Errorf(errProfileNotFoundFmt, profileName)
	}

	// Load the profile file
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return nil, errfmt.Errorf(errReadProfileFmt, profilePath, err)
	}

	var profile UnifiedProfile
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return nil, errfmt.Errorf(errParseProfileFmt, profilePath, err)
	}

	// Validate profile structure
	if profile.Kind != profileKindExpected {
		return nil, errfmt.Errorf(errInvalidProfileKindFmt, profile.Kind)
	}

	// Initialize resolved spec with current profile's spec
	profile.ResolvedSpec = make(map[string]any)
	if profile.Spec != nil {
		maps.Copy(profile.ResolvedSpec, profile.Spec)
	}

	// If this profile extends another, load and merge parent
	if profile.Metadata.Extends != emptyPath && profile.Metadata.Extends != nullStringValue {
		parentProfile, err := pl.loadProfileWithInheritanceRecursive(profile.Metadata.Extends, visited)
		if err != nil {
			return nil, errfmt.Errorf(errLoadParentProfileFmt, profile.Metadata.Extends, err)
		}

		// Validate that parent has same type
		if parentProfile.Type != profile.Type {
			return nil, errfmt.Errorf(errProfileTypeMismatchFmt,
				profileName, profile.Metadata.Extends, profile.Type, parentProfile.Type)
		}

		// Merge parent's resolved spec (child overrides parent)
		for k, v := range parentProfile.ResolvedSpec {
			if _, exists := profile.ResolvedSpec[k]; !exists {
				profile.ResolvedSpec[k] = v
			} else {
				// For maps, merge recursively
				if childMap, ok := profile.ResolvedSpec[k].(map[string]any); ok {
					if parentMap, ok := v.(map[string]any); ok {
						merged := make(map[string]any)
						// Start with parent values
						for pk, pv := range parentMap {
							merged[pk] = pv
						}
						// Override with child values
						for ck, cv := range childMap {
							merged[ck] = cv
						}
						profile.ResolvedSpec[k] = merged
					}
				}
			}
		}
	}

	return &profile, nil
}

// resolveProfilePath finds the profile file path
func (pl *ProfileLoader) resolveProfilePath(profileName string) string {
	// Try different file name formats
	possibleNames := []string{
		profileName + yamlFileExtension,
		strings.ReplaceAll(profileName, hyphenChar, underscoreChar) + yamlFileExtension,
		strings.ReplaceAll(profileName, underscoreChar, hyphenChar) + yamlFileExtension,
	}

	for _, name := range possibleNames {
		profilePath := filepath.Join(pl.profilesDir, name)
		if info, err := os.Stat(profilePath); err == nil && !info.IsDir() {
			return profilePath
		}
	}

	return ""
}

// findProfilesDir attempts to find the profile specs directory
func findProfilesDir() string {
	// Try common locations
	possiblePaths := []string{
		paths.ProcessInternalProfileSpecsDir,
		filepath.Join(dotDotPathSegment, paths.ProcessInternalProfileSpecsDir),
		filepath.Join(dotDotPathSegment, dotDotPathSegment, paths.ProcessInternalProfileSpecsDir),
	}

	wd, err := os.Getwd()
	if err != nil {
		return emptyPath
	}

	for _, path := range possiblePaths {
		absPath := filepath.Join(wd, path)
		if info, err := os.Stat(absPath); err == nil && info.IsDir() {
			return absPath
		}
	}

	// Walk up directory tree looking for project root
	dir := wd
	for {
		profilesDir := filepath.Join(dir, paths.ProcessInternalProfileSpecsDir)
		if info, err := os.Stat(profilesDir); err == nil && info.IsDir() {
			return profilesDir
		}

		// Check if we're at project root
		if _, err := os.Stat(filepath.Join(dir, paths.ProjectDataDir)); err == nil {
			// Try profile_specs at this level
			if info, err := os.Stat(profilesDir); err == nil && info.IsDir() {
				return profilesDir
			}
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			break // Reached filesystem root
		}
		dir = parent
	}

	return emptyPath
}

// ClearCache clears the profile cache, forcing reload of all profiles on next access
func (pl *ProfileLoader) ClearCache() {
	_ = concurrency.RunInLockWithLogger(
		&pl.mu, LockNameConfigLoaderClearCache, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			pl.cache = make(map[string]*UnifiedProfile)
			return nil
		},
	)
}
