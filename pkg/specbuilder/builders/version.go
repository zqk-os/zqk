package builders

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
)

// CurrentBuilderVersion is the current default builder version
// This should be updated when moving to a new major version
const CurrentBuilderVersion = "v2_0_0"

// CurrentBuilderPackage is the current default builder package name
// This corresponds to CurrentBuilderVersion (e.g., v2_0_0 -> bldr_v2)
const CurrentBuilderPackage = "bldr_v2"

// IncrementVersion increments a builder version (e.g., v1_0_0 -> v1_0_1, v1_0_9 -> v1_1_0)
// Supports incrementing major, minor, or patch versions
func IncrementVersion(version string, level string) (string, error) {
	// Remove leading 'v' if present
	version = strings.TrimPrefix(version, "v")

	parts := strings.Split(version, "_")
	if len(parts) != 3 {
		return "", errfmt.Errorf("invalid version format: %s (expected format: v1_0_0)", version)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return "", errfmt.Errorf("invalid major version: %s", parts[0])
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", errfmt.Errorf("invalid minor version: %s", parts[1])
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", errfmt.Errorf("invalid patch version: %s", parts[2])
	}

	switch level {
	case "major":
		major++
		minor = 0
		patch = 0
	case "minor":
		minor++
		patch = 0
	case "patch":
		patch++
	default:
		// Default to patch increment
		patch++
	}

	return fmt.Sprintf("v%d_%d_%d", major, minor, patch), nil
}

// GetNextVersion determines the next version for a spec based on the latest builder version
// Defaults to patch increment if no level is specified
func GetNextVersion(registry *VersionedBuilderRegistry, ontology string, level string) (string, error) {
	latestVersion, err := registry.GetLatestVersion(ontology)
	if err != nil {
		// No existing version, start at the default instance schema version.
		return ParseInstanceVersion(objects.InitialFieldVersion), nil
	}

	return IncrementVersion(latestVersion, level)
}

// ParseInstanceVersion parses an instance schema_version (e.g., InitialFieldVersion) to builder format.
func ParseInstanceVersion(instanceVersion string) string {
	if instanceVersion == emptyValue {
		instanceVersion = objects.InitialFieldVersion
	}
	// Remove leading 'v' if present, then add 'v' and replace dots with underscores
	version := strings.TrimPrefix(instanceVersion, "v")
	return "v" + strings.ReplaceAll(version, ".", "_")
}

// FormatBuilderVersion formats a builder version (e.g., "v1_0_0") to instance format (InitialFieldVersion).
func FormatBuilderVersion(builderVersion string) string {
	if builderVersion == emptyValue {
		return objects.InitialFieldVersion
	}
	// Remove leading 'v', then replace underscores with dots
	version := strings.TrimPrefix(builderVersion, "v")
	return strings.ReplaceAll(version, "_", ".")
}

// versionToPackageName converts a builder version to a package name
// For now, we use a simple mapping: all v1_*_* versions go to bldr_v1
// This matches the current structure where multiple builders coexist in the same package
// v1_0_0 -> bldr_v1
// v1_1_0 -> bldr_v1
// v2_0_0 -> bldr_v2 (future enhancement)
func versionToPackageName(version string) string {
	// Remove leading 'v'
	version = strings.TrimPrefix(version, "v")

	parts := strings.Split(version, "_")
	if len(parts) < 1 {
		return defaultVersionPackage
	}

	// For now, use major version only: v1_*_* -> bldr_v1
	// This keeps the package structure simple and matches current usage
	major := parts[0]
	if major == emptyValue {
		return defaultVersionPackage
	}

	return fmt.Sprintf("bldr_v%s", major)
}
