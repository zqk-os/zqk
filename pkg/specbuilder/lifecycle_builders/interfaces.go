package lifecycle_builders

import (
	"fmt"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
)

// LifecycleBuilder is an interface for builders that generate lifecycle definitions at a specific version
type LifecycleBuilder interface {
	// Build returns the lifecycle at this builder's version
	Build() *objects.Lifecycle
	// GetVersion returns the version this builder generates
	GetVersion() string
	// GetObjectType returns the object type this builder generates a lifecycle for
	GetObjectType() string
}

// VersionedLifecycleBuilderRegistry manages versioned lifecycle builders
type VersionedLifecycleBuilderRegistry struct {
	builders map[string]map[string]LifecycleBuilder // objectType -> version -> builder
}

// NewVersionedLifecycleBuilderRegistry creates a new registry
func NewVersionedLifecycleBuilderRegistry() *VersionedLifecycleBuilderRegistry {
	return &VersionedLifecycleBuilderRegistry{
		builders: make(map[string]map[string]LifecycleBuilder),
	}
}

// Register registers a builder for a specific object type and version
func (r *VersionedLifecycleBuilderRegistry) Register(builder LifecycleBuilder) {
	objectType := builder.GetObjectType()
	version := builder.GetVersion()

	if r.builders[objectType] == nil {
		r.builders[objectType] = make(map[string]LifecycleBuilder)
	}
	r.builders[objectType][version] = builder
}

// GetBuilder returns the builder for a specific object type and version
func (r *VersionedLifecycleBuilderRegistry) GetBuilder(objectType, version string) (LifecycleBuilder, error) {
	if r.builders[objectType] == nil {
		return nil, errfmt.Errorf("no builders found for object type: %s", objectType)
	}
	builder, ok := r.builders[objectType][version]
	if !ok {
		return nil, errfmt.Errorf("no builder found for object type %s at version %s", objectType, version)
	}
	return builder, nil
}

// GetLatestVersion returns the latest version for an object type
// Versions are expected to be in semantic format: "v1_0_0", "v1_1_0", "v2_0_0", etc.
func (r *VersionedLifecycleBuilderRegistry) GetLatestVersion(objectType string) (string, error) {
	if len(r.builders[objectType]) == 0 {
		return "", errfmt.Errorf("no builders found for object type: %s", objectType)
	}

	// Find highest semantic version
	latestVersion := ""
	var maxMajor, maxMinor, maxPatch = -1, -1, -1

	for version := range r.builders[objectType] {
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
		for version := range r.builders[objectType] {
			return version, nil
		}
	}

	return latestVersion, nil
}

// GetAllObjectTypes returns all object types with builders
func (r *VersionedLifecycleBuilderRegistry) GetAllObjectTypes() []string {
	objectTypes := make([]string, 0, len(r.builders))
	for objectType := range r.builders {
		objectTypes = append(objectTypes, objectType)
	}
	return objectTypes
}

// GetVersions returns all versions for an object type
func (r *VersionedLifecycleBuilderRegistry) GetVersions(objectType string) []string {
	if r.builders[objectType] == nil {
		return nil
	}
	versions := make([]string, 0, len(r.builders[objectType]))
	for version := range r.builders[objectType] {
		versions = append(versions, version)
	}
	return versions
}
