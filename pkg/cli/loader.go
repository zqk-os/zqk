package cli

import (
	"maps"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const legacyCLINamespaceIDSuffix = ":kernel:cli"

// Profile represents a CLI context profile
type Profile struct {
	SchemaVersion string
	NamespaceID   string `yaml:"namespace_id"` // Subordinate namespace ID (e.g., "zqk:kernel:cli")
	Name          string
	Extends       string
	Description   string
	Spec          map[string]any
	ResolvedSpec  map[string]any // After inheritance resolution
}

// ProfileLoader loads and resolves CLI profiles with inheritance
type ProfileLoader struct {
	profilesDir string
	cache       stampmemo.Table[*Profile] // keyed by profile name (closed CLI profile tree)
}

// NewProfileLoader creates a new CLI profile loader
func NewProfileLoader(profilesDir string) *ProfileLoader {
	if profilesDir == emptyValue {
		profilesDir = findCLIProfilesDir()
	}
	return &ProfileLoader{
		profilesDir: profilesDir,
	}
}

// LoadProfile loads a profile and resolves its inheritance chain
func (pl *ProfileLoader) LoadProfile(name string) (*Profile, error) {
	path := pl.resolveProfilePath(name)
	return pl.cache.Load(name, stampmemo.Of(path), func() (*Profile, error) {
		return pl.loadProfileRecursive(name, make(map[string]bool))
	})
}

func (pl *ProfileLoader) readProfileData(name string, visited map[string]bool) (string, []byte, error) {
	if visited[name] {
		return "", nil, errfmt.Errorf("circular inheritance detected: %s", name)
	}
	visited[name] = true

	profilePath := pl.resolveProfilePath(name)
	if profilePath == emptyValue {
		return "", nil, errfmt.Errorf("CLI profile not found: %s", name)
	}

	data, err := fileutil.ReadFile(profilePath)
	if err != nil {
		return "", nil, errfmt.Errorf("failed to read CLI profile file %s: %w", profilePath, err)
	}
	return profilePath, data, nil
}

// loadProfileRecursive recursively loads profile and resolves inheritance
func (pl *ProfileLoader) loadProfileRecursive(name string, visited map[string]bool) (*Profile, error) {
	profilePath, data, err := pl.readProfileData(name, visited)
	if err != nil {
		return nil, err
	}

	var profile Profile
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return nil, errfmt.Errorf("failed to parse CLI profile file %s: %w", profilePath, err)
	}

	// Validate namespace_id is required
	if profile.NamespaceID == emptyValue {
		// Backward/forward compatibility: allow namespace_id omission and default to current configured namespace.
		profile.NamespaceID = paths.CLINamespaceID
	}

	// Validate namespace_id matches expected subordinate namespace.
	// For white-labeling, accept legacy prefixes as long as the suffix matches.
	// then normalize to the configured namespace.
	if profile.NamespaceID != paths.CLINamespaceID {
		if strings.HasSuffix(profile.NamespaceID, legacyCLINamespaceIDSuffix) {
			profile.NamespaceID = paths.CLINamespaceID
		} else {
			return nil, errfmt.Errorf("invalid namespace_id for CLI profile %s: %s (expected %s)",
				name, profile.NamespaceID, paths.CLINamespaceID)
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
			} else {
				// For maps, merge recursively
				if childMap, ok := profile.ResolvedSpec[k].(map[string]any); ok {
					if parentMap, ok := v.(map[string]any); ok {
						merged := make(map[string]any)
						// Start with parent values
						maps.Copy(merged, parentMap)
						// Override with child values
						maps.Copy(merged, childMap)
						profile.ResolvedSpec[k] = merged
					}
				}
			}
		}
	}

	return &profile, nil
}

// resolveProfilePath finds the profile file path
func (pl *ProfileLoader) resolveProfilePath(name string) string {
	return paths.ResolveYAMLVariant(pl.profilesDir, name)
}

// findCLIProfilesDir attempts to find the CLI profiles directory
func findCLIProfilesDir() string {
	return paths.FirstExistingFromCwd(paths.CLIProfilesDir)
}

// ClearCache clears the profile cache
func (pl *ProfileLoader) ClearCache() {
	pl.cache.Reset()
}
