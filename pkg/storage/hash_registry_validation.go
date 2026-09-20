package storage

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// ValidateRegistryLocation ensures that a hash registry is located in the correct directory
// for a given file path and object kind.
//
// **File Backend Only**: This validation only applies to file-based storage. The graph backend
// uses a single registry node per kind (e.g., "HashRegistry:backlog_item") and doesn't have
// the same bucketing/location issue.
//
// For bucketed objects (audit_event, change_journal_entry), the registry should be in
// the file's directory (subdirectory). For non-bucketed objects, the registry should be
// in the kind directory (parent directory).
//
// This validation helps prevent issues where registries are created in the wrong location,
// leading to stale hash reads or hash mismatches.
//
// Note: For graph backend, this is a no-op since GraphHashRegistry doesn't use file paths.
func ValidateRegistryLocation(registryDir, filePath, kind string, isBucketed bool) error {
	// Skip validation if filePath is empty (graph backend doesn't use file paths)
	if filePath == emptyValue {
		return nil
	}

	fileDir := filepath.Dir(filePath)

	if isBucketed {
		// For bucketed objects, registry must be in the file's directory
		if registryDir != fileDir {
			return errfmt.Errorf(ConstMiscHashRegistryForBucketedObjectSMustBeInFi, kind, fileDir, registryDir)
		}
	} else {
		// For non-bucketed objects, registry should be in the kind directory
		// (which should match the file's directory for non-bucketed objects)
		if registryDir != fileDir {
			return errfmt.Errorf(ConstMiscHashRegistryForNonBucketedObjectSShouldB, kind, fileDir, registryDir)
		}
	}

	return nil
}

// GetRegistryDirectoryForFile returns the correct directory for a hash registry
// given a file path and whether the object kind is bucketed.
//
// This helper function ensures consistency in registry location across the codebase.
func GetRegistryDirectoryForFile(filePath string, isBucketed bool) string {
	if isBucketed {
		// For bucketed objects, use the file's directory
		return filepath.Dir(filePath)
	}
	// For non-bucketed objects, also use the file's directory
	// (which is the same as the kind directory)
	return filepath.Dir(filePath)
}
