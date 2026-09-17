package profile_builders

import (
	"fmt"

	"github.com/lanceman/zqk/pkg/config"
	"github.com/lanceman/zqk/pkg/errfmt"
)

// ProfileBuilder is an interface for builders that generate profile definitions at a specific version
type ProfileBuilder interface {
	// Build returns the profile at this builder's version
	Build() *config.UnifiedProfile
	// GetVersion returns the version this builder generates
	GetVersion() string
	// GetName returns the profile name this builder generates
	GetName() string
}

// VersionedProfileBuilderRegistry manages versioned profile builders
type VersionedProfileBuilderRegistry struct {
	builders map[string]map[string]ProfileBuilder // name -> version -> builder
}

// NewVersionedProfileBuilderRegistry creates a new registry
func NewVersionedProfileBuilderRegistry() *VersionedProfileBuilderRegistry {
	return &VersionedProfileBuilderRegistry{
		builders: make(map[string]map[string]ProfileBuilder),
	}
}

// Register registers a builder for a specific profile name and version
func (r *VersionedProfileBuilderRegistry) Register(builder ProfileBuilder) {
	name := builder.GetName()
	version := builder.GetVersion()

	if r.builders[name] == nil {
		r.builders[name] = make(map[string]ProfileBuilder)
	}
	r.builders[name][version] = builder
}

// GetBuilder returns the builder for a specific profile name and version
func (r *VersionedProfileBuilderRegistry) GetBuilder(name, version string) (ProfileBuilder, error) {
	if r.builders[name] == nil {
		return nil, errfmt.Errorf("no builders found for profile: %s", name)
	}
	builder, ok := r.builders[name][version]
	if !ok {
		return nil, errfmt.Errorf("no builder found for profile %s at version %s", name, version)
	}
	return builder, nil
}

// GetLatestVersion returns the latest version for a profile name
func (r *VersionedProfileBuilderRegistry) GetLatestVersion(name string) (string, error) {
	if len(r.builders[name]) == 0 {
		return "", errfmt.Errorf("no builders found for profile: %s", name)
	}

	// Find highest semantic version
	latestVersion := ""
	var maxMajor, maxMinor, maxPatch = -1, -1, -1

	for version := range r.builders[name] {
		// Parse semantic version (format "v1_0_0" -> major=1, minor=0, patch=0)
		var major, minor, patch int
		if _, err := fmt.Sscanf(version, "v%d_%d_%d", &major, &minor, &patch); err == nil {
			// Compare semantic versions (major.minor.patch)
			if major > maxMajor ||
				(major == maxMajor && minor > maxMinor) ||
				(major == maxMajor && minor == maxMinor && patch > maxPatch) {
				maxMajor = major
				maxMinor = minor
				maxPatch = patch
				latestVersion = version
			}
		}
	}

	if latestVersion == emptyValue {
		// Fallback: return any version if parsing failed
		for version := range r.builders[name] {
			return version, nil
		}
	}

	return latestVersion, nil
}

// GetAllNames returns all profile names with builders
func (r *VersionedProfileBuilderRegistry) GetAllNames() []string {
	names := make([]string, 0, len(r.builders))
	for name := range r.builders {
		names = append(names, name)
	}
	return names
}

// GetVersions returns all versions for a profile name
func (r *VersionedProfileBuilderRegistry) GetVersions(name string) []string {
	if r.builders[name] == nil {
		return nil
	}
	versions := make([]string, 0, len(r.builders[name]))
	for version := range r.builders[name] {
		versions = append(versions, version)
	}
	return versions
}
