package metrics

import (
	"maps"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// SamplerProfile represents a metrics sampler profile
type SamplerProfile struct {
	SchemaVersion string
	NamespaceID   string `yaml:"namespace_id"` // Subordinate namespace ID (e.g., "zqk:kernel:metrics")
	Name          string
	Extends       string
	Description   string
	Spec          map[string]any
	ResolvedSpec  map[string]any // After inheritance resolution
}

// ProfileLoader loads and resolves metrics sampler profiles with inheritance
type ProfileLoader struct {
	profilesDir string
	cache       stampmemo.Table[*SamplerProfile] // keyed by profile name (closed sampler tree)
}

// NewProfileLoader creates a new metrics profile loader
func NewProfileLoader(profilesDir string) *ProfileLoader {
	if profilesDir == emptyValue {
		profilesDir = findMetricsProfilesDir()
	}
	return &ProfileLoader{
		profilesDir: profilesDir,
	}
}

// LoadProfile loads a profile and resolves its inheritance chain
func (pl *ProfileLoader) LoadProfile(name string) (*SamplerProfile, error) {
	path := pl.resolveProfilePath(name)
	return pl.cache.Load(name, stampmemo.Of(path), func() (*SamplerProfile, error) {
		return pl.loadProfileRecursive(name, make(map[string]bool))
	})
}

// loadProfileRecursive recursively loads profile and resolves inheritance
func (pl *ProfileLoader) loadProfileRecursive(name string, visited map[string]bool) (*SamplerProfile, error) {
	// Check for circular inheritance
	if visited[name] {
		return nil, errfmt.Errorf("circular inheritance detected: %s", name)
	}
	visited[name] = true

	// Resolve profile file path
	profilePath := pl.resolveProfilePath(name)
	if profilePath == emptyValue {
		return nil, errfmt.Errorf("metrics profile not found: %s", name)
	}

	// Load the profile file
	data, err := fileutil.ReadFile(profilePath)
	if err != nil {
		return nil, errfmt.Errorf("failed to read metrics profile file %s: %w", profilePath, err)
	}

	var profile SamplerProfile
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return nil, errfmt.Errorf("failed to parse metrics profile file %s: %w", profilePath, err)
	}

	// Validate namespace_id is required
	if profile.NamespaceID == emptyValue {
		// Backward/forward compatibility: allow namespace_id omission and default to current configured namespace.
		profile.NamespaceID = paths.MetricsNamespaceID
	}

	// Validate namespace_id matches expected subordinate namespace.
	// For white-labeling, accept legacy prefixes as long as the suffix matches (":kernel:metrics"),
	// then normalize to the configured namespace.
	if profile.NamespaceID != paths.MetricsNamespaceID {
		if strings.HasSuffix(profile.NamespaceID, ":kernel:metrics") {
			profile.NamespaceID = paths.MetricsNamespaceID
		} else {
			return nil, errfmt.Errorf("invalid namespace_id for metrics profile %s: %s (expected %s)",
				name, profile.NamespaceID, paths.MetricsNamespaceID)
		}
	}

	// Initialize resolved spec with current profile's spec
	profile.ResolvedSpec = make(map[string]any)
	if profile.Spec != nil {
		maps.Copy(profile.ResolvedSpec, profile.Spec)
	}

	// If this profile extends another, load and merge parent
	if profile.Extends != emptyValue && profile.Extends != paths.ProfileExtendsNull {
		parentProfile, err := pl.loadProfileRecursive(profile.Extends, visited)
		if err != nil {
			return nil, errfmt.Errorf("failed to load parent profile %s: %w", profile.Extends, err)
		}

		// Merge parent's resolved spec (child overrides parent)
		for k, v := range parentProfile.ResolvedSpec {
			if _, exists := profile.ResolvedSpec[k]; !exists {
				profile.ResolvedSpec[k] = v
			}
		}
	}

	return &profile, nil
}

// resolveProfilePath finds the profile file path
func (pl *ProfileLoader) resolveProfilePath(name string) string {
	// Try different file name formats
	possibleNames := []string{
		name + paths.YAMLExtension,
		strings.ReplaceAll(name, "-", "_") + paths.YAMLExtension,
		strings.ReplaceAll(name, "_", "-") + paths.YAMLExtension,
	}

	for _, fileName := range possibleNames {
		profilePath := filepath.Join(pl.profilesDir, fileName)
		if info, err := fileutil.Stat(profilePath); err == nil && !info.IsDir() {
			return profilePath
		}
	}

	return ""
}

// findMetricsProfilesDir attempts to find the metrics profiles directory
func findMetricsProfilesDir() string {
	return paths.FirstExistingFromCwd(paths.MetricsProfilesDir)
}


// ClearCache clears the profile cache
func (pl *ProfileLoader) ClearCache() {
	pl.cache.Reset()
}
