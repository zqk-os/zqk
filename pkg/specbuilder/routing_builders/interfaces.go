package routing_builders

import (
	"fmt"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

const emptyValue = ""

// RoutingRuleBuilder is an interface for builders that generate routing rule YAML at a specific version
// Note: Routing rules use raw YAML structure due to field name mismatches between YAML and Go structs
type RoutingRuleBuilder interface {
	// Build returns the routing rules as YAML bytes at this builder's version
	Build() ([]byte, error)
	// GetVersion returns the version this builder generates
	GetVersion() string
	// GetFileName returns the file name this builder generates rules for
	GetFileName() string
}

// VersionedRoutingRuleBuilderRegistry manages versioned routing rule builders
type VersionedRoutingRuleBuilderRegistry struct {
	builders map[string]map[string]RoutingRuleBuilder // fileName -> version -> builder
}

// NewVersionedRoutingRuleBuilderRegistry creates a new registry
func NewVersionedRoutingRuleBuilderRegistry() *VersionedRoutingRuleBuilderRegistry {
	return &VersionedRoutingRuleBuilderRegistry{
		builders: make(map[string]map[string]RoutingRuleBuilder),
	}
}

// Register registers a builder for a specific file name and version
func (r *VersionedRoutingRuleBuilderRegistry) Register(builder RoutingRuleBuilder) {
	fileName := builder.GetFileName()
	version := builder.GetVersion()

	if r.builders[fileName] == nil {
		r.builders[fileName] = make(map[string]RoutingRuleBuilder)
	}
	r.builders[fileName][version] = builder
}

// GetBuilder returns the builder for a specific file name and version
func (r *VersionedRoutingRuleBuilderRegistry) GetBuilder(fileName, version string) (RoutingRuleBuilder, error) {
	if r.builders[fileName] == nil {
		return nil, errfmt.Errorf("no builders found for file: %s", fileName)
	}
	builder, ok := r.builders[fileName][version]
	if !ok {
		return nil, errfmt.Errorf("no builder found for file %s at version %s", fileName, version)
	}
	return builder, nil
}

// GetLatestVersion returns the latest version for a file name
func (r *VersionedRoutingRuleBuilderRegistry) GetLatestVersion(fileName string) (string, error) {
	if len(r.builders[fileName]) == 0 {
		return "", errfmt.Errorf("no builders found for file: %s", fileName)
	}

	// Find highest semantic version
	latestVersion := ""
	var maxMajor, maxMinor, maxPatch = -1, -1, -1

	for version := range r.builders[fileName] {
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
		for version := range r.builders[fileName] {
			return version, nil
		}
	}

	return latestVersion, nil
}

// GetAllFileNames returns all file names with builders
func (r *VersionedRoutingRuleBuilderRegistry) GetAllFileNames() []string {
	fileNames := make([]string, 0, len(r.builders))
	for fileName := range r.builders {
		fileNames = append(fileNames, fileName)
	}
	return fileNames
}

// GetVersions returns all versions for a file name
func (r *VersionedRoutingRuleBuilderRegistry) GetVersions(fileName string) []string {
	if r.builders[fileName] == nil {
		return nil
	}
	versions := make([]string, 0, len(r.builders[fileName]))
	for version := range r.builders[fileName] {
		versions = append(versions, version)
	}
	return versions
}
