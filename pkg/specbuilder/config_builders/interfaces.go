package config_builders

import (
	"fmt"

	"github.com/lanceman/zqk/pkg/errfmt"
)

const emptyValue = ""

// ConfigBuilder is an interface for builders that generate config YAML at a specific version
// Note: Configs use raw YAML structure due to diverse config types
type ConfigBuilder interface {
	// Build returns the config as YAML bytes at this builder's version
	Build() ([]byte, error)
	// GetVersion returns the version this builder generates
	GetVersion() string
	// GetFileName returns the config file name this builder generates
	GetFileName() string
}

// VersionedConfigBuilderRegistry manages versioned config builders
type VersionedConfigBuilderRegistry struct {
	builders map[string]map[string]ConfigBuilder // fileName -> version -> builder
}

// NewVersionedConfigBuilderRegistry creates a new registry
func NewVersionedConfigBuilderRegistry() *VersionedConfigBuilderRegistry {
	return &VersionedConfigBuilderRegistry{
		builders: make(map[string]map[string]ConfigBuilder),
	}
}

// Register registers a builder for a specific file name and version
func (r *VersionedConfigBuilderRegistry) Register(builder ConfigBuilder) {
	fileName := builder.GetFileName()
	version := builder.GetVersion()

	if r.builders[fileName] == nil {
		r.builders[fileName] = make(map[string]ConfigBuilder)
	}
	r.builders[fileName][version] = builder
}

// GetBuilder returns the builder for a specific file name and version
func (r *VersionedConfigBuilderRegistry) GetBuilder(fileName, version string) (ConfigBuilder, error) {
	if r.builders[fileName] == nil {
		return nil, errfmt.Errorf("no builders found for config: %s", fileName)
	}
	builder, ok := r.builders[fileName][version]
	if !ok {
		return nil, errfmt.Errorf("no builder found for config %s at version %s", fileName, version)
	}
	return builder, nil
}

// GetLatestVersion returns the latest version for a file name
func (r *VersionedConfigBuilderRegistry) GetLatestVersion(fileName string) (string, error) {
	if len(r.builders[fileName]) == 0 {
		return "", errfmt.Errorf("no builders found for config: %s", fileName)
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

// GetAllFileNames returns all config file names with builders
func (r *VersionedConfigBuilderRegistry) GetAllFileNames() []string {
	fileNames := make([]string, 0, len(r.builders))
	for fileName := range r.builders {
		fileNames = append(fileNames, fileName)
	}
	return fileNames
}

// GetVersions returns all versions for a file name
func (r *VersionedConfigBuilderRegistry) GetVersions(fileName string) []string {
	if r.builders[fileName] == nil {
		return nil
	}
	versions := make([]string, 0, len(r.builders[fileName]))
	for version := range r.builders[fileName] {
		versions = append(versions, version)
	}
	return versions
}
