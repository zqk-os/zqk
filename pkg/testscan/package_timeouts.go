package testscan

import (
	"strings"
)

// GetMinTimeoutSecondsForPackage returns a minimum job/test timeout in seconds
// for a package path. Used by remaining scan-tests job generation and
// SuggestedTimeoutForPackage. Package-specific YAML
// (scan_tests_package_timeouts.yaml) was scan-tests-only and is gone;
// test_case orchestration does not load it.
func GetMinTimeoutSecondsForPackage(_, packagePath string) int {
	return defaultMinTimeoutSecondsForPackage(strings.TrimPrefix(packagePath, "./"))
}

func defaultMinTimeoutSecondsForPackage(packagePath string) int {
	switch {
	case strings.HasPrefix(packagePath, "cmd/zqk/object") || packagePath == "cmd/zqk/object":
		return 600
	case strings.HasPrefix(packagePath, "cmd/zqk") || packagePath == "cmd/zqk":
		return 600
	case strings.HasPrefix(packagePath, "pkg/storage") || packagePath == "pkg/storage":
		return 600
	default:
		return 0
	}
}
