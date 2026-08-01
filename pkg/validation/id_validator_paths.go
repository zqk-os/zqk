package validation

import (
	"os"
	"path/filepath"

	"github.com/lanceman/zqk/pkg/paths"
)

// discoverSpecsDir attempts to find the object specs directory
// Uses paths configuration instead of hardcoded paths
func discoverSpecsDir() string {
	config := GetGlobalPathsConfig()
	if config != nil {
		if path := config.FindPath("object_specs"); path != emptyValue {
			return path
		}
	}

	// Fallback to default if config not available
	return findPathWithDefaults(paths.ProcessInternalObjectSpecsDir, true)
}

// findPathWithDefaults is a fallback helper that uses default search strategy
// Uses paths config to determine search strategy instead of hardcoded paths
// findPathWithDefaults finds a path using default search strategy
//
//nolint:gocyclo // Function orchestrates multiple search strategies; complexity reduced via helper functions
func findPathWithDefaults(relativePath string, isDir bool) string {
	config := GetGlobalPathsConfig()
	if config == nil {
		config = getDefaultPathsConfig()
	}

	wd, err := os.Getwd()
	if err != nil {
		return ""
	}

	// Try relative paths first
	if foundPath := findPathInRelativePaths(wd, relativePath, isDir, config); foundPath != emptyValue {
		return foundPath
	}

	// Walk up directory tree looking for markers
	return findPathByWalkingUp(wd, relativePath, isDir, config)
}

// findPathInRelativePaths searches for path in relative paths from current working directory
func findPathInRelativePaths(wd, relativePath string, isDir bool, config *PathsConfig) string {
	for _, relPath := range config.SearchStrategy.RelativePaths {
		fullPath := filepath.Join(wd, relPath, relativePath)
		if matchesPathType(fullPath, isDir) {
			return fullPath
		}
	}
	return ""
}

// findPathByWalkingUp walks up directory tree looking for markers and path
func findPathByWalkingUp(wd, relativePath string, isDir bool, config *PathsConfig) string {
	dir := wd
	for {
		if hasMarkerInDir(dir, config) {
			fullPath := filepath.Join(dir, relativePath)
			if matchesPathType(fullPath, isDir) {
				return fullPath
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return ""
}

// hasMarkerInDir checks if a directory has any of the configured markers
func hasMarkerInDir(dir string, config *PathsConfig) bool {
	for _, marker := range config.SearchStrategy.Markers {
		markerPath := filepath.Join(dir, marker)
		if _, err := os.Stat(markerPath); err == nil {
			return true
		}
	}
	return false
}

// matchesPathType checks if a path exists and matches the expected type (file or directory)
func matchesPathType(fullPath string, isDir bool) bool {
	info, err := os.Stat(fullPath)
	if err != nil {
		return false
	}

	if isDir && info.IsDir() {
		return true
	}
	if !isDir && !info.IsDir() {
		return true
	}

	return false
}
