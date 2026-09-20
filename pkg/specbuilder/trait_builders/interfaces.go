package trait_builders

import (
	"fmt"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TraitBuilder is an interface for builders that generate trait definitions at a specific version
type TraitBuilder interface {
	// Build returns the trait at this builder's version
	Build() *objects.TraitDefinition
	// GetVersion returns the version this builder generates
	GetVersion() string
	// GetName returns the trait name this builder generates
	GetName() string
}

// VersionedTraitBuilderRegistry manages versioned trait builders
type VersionedTraitBuilderRegistry struct {
	builders map[string]map[string]TraitBuilder // name -> version -> builder
}

// NewVersionedTraitBuilderRegistry creates a new registry
func NewVersionedTraitBuilderRegistry() *VersionedTraitBuilderRegistry {
	return &VersionedTraitBuilderRegistry{
		builders: make(map[string]map[string]TraitBuilder),
	}
}

// Register registers a builder for a specific trait name and version
func (r *VersionedTraitBuilderRegistry) Register(builder TraitBuilder) {
	name := builder.GetName()
	version := builder.GetVersion()

	if r.builders[name] == nil {
		r.builders[name] = make(map[string]TraitBuilder)
	}
	r.builders[name][version] = builder
}

// GetBuilder returns the builder for a specific trait name and version
func (r *VersionedTraitBuilderRegistry) GetBuilder(name, version string) (TraitBuilder, error) {
	if r.builders[name] == nil {
		return nil, errfmt.Errorf("no builders found for trait: %s", name)
	}
	builder, ok := r.builders[name][version]
	if !ok {
		return nil, errfmt.Errorf("no builder found for trait %s at version %s", name, version)
	}
	return builder, nil
}

// GetLatestVersion returns the latest version for a trait name
func (r *VersionedTraitBuilderRegistry) GetLatestVersion(name string) (string, error) {
	if len(r.builders[name]) == 0 {
		return "", errfmt.Errorf("no builders found for trait: %s", name)
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

// GetAllNames returns all trait names with builders
func (r *VersionedTraitBuilderRegistry) GetAllNames() []string {
	names := make([]string, 0, len(r.builders))
	for name := range r.builders {
		names = append(names, name)
	}
	return names
}

// GetVersions returns all versions for a trait name
func (r *VersionedTraitBuilderRegistry) GetVersions(name string) []string {
	if r.builders[name] == nil {
		return nil
	}
	versions := make([]string, 0, len(r.builders[name]))
	for version := range r.builders[name] {
		versions = append(versions, version)
	}
	return versions
}
