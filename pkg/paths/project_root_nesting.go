package paths

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var (
	// ErrNestedProjectRoot is returned when a project root is nested inside or encloses another project root.
	ErrNestedProjectRoot = errors.New("nested project root prohibited")
)

// ValidateProjectRootNesting verifies that candidateRoot neither resides inside an existing
// project root (ancestor violation) nor encloses another project root (descendant violation).
// Project roots must never be nested.
func ValidateProjectRootNesting(candidateRoot string) error {
	if strings.TrimSpace(candidateRoot) == "" {
		return errors.New("candidate project root cannot be empty")
	}

	abs, err := filepath.Abs(filepath.Clean(candidateRoot))
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path for %s: %w", candidateRoot, err)
	}

	// 1. Ancestor check: verify no parent directory is a project root.
	curr := filepath.Dir(abs)
	for {
		if curr == abs || curr == "." || curr == "/" || curr == filepath.Dir(curr) {
			break
		}
		if !isIgnoredNestedProjectRoot(curr) && IsValidProjectRoot(curr) {
			return fmt.Errorf("%w: %q is nested inside ancestor project root %q", ErrNestedProjectRoot, abs, curr)
		}
		curr = filepath.Dir(curr)
	}

	// 2. Descendant check: verify candidateRoot does not enclose any child project roots.
	nested, err := FindNestedProjectRoots(abs)
	if err != nil {
		return err
	}
	if len(nested) > 0 {
		return fmt.Errorf("%w: %q contains nested project root %q", ErrNestedProjectRoot, abs, nested[0])
	}

	return nil
}

// isIgnoredScanDir reports whether directory entries should be pruned from descendant scanning.
func isIgnoredScanDir(name string, fullPath string) bool {
	if name == ProjectDataDir || name == GitWorktreeMetadataEntry || name == "node_modules" || name == "vendor" || name == "dist-docs" {
		return true
	}
	if strings.HasPrefix(name, ".tmp") || name == "testdata" {
		return true
	}
	return isIgnoredNestedProjectRoot(fullPath)
}

// FindNestedProjectRoots scans projectRoot for any nested .zqk project markers
// excluding the topmost projectRoot/.zqk itself.
func FindNestedProjectRoots(projectRoot string) ([]string, error) {
	abs, err := filepath.Abs(filepath.Clean(projectRoot))
	if err != nil {
		return nil, fmt.Errorf("resolve project root: %w", err)
	}

	var nestedRoots []string
	walkErr := filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// Skip directories that cannot be read
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if p == abs {
			return nil
		}

		name := d.Name()
		if isIgnoredScanDir(name, p) {
			return filepath.SkipDir
		}

		// Check if this subdirectory is a project root
		candidateDataDir := filepath.Join(p, ProjectDataDir)
		if st, statErr := fileutil.Stat(candidateDataDir); statErr == nil && st.IsDir() {
			if !isIgnoredNestedProjectRoot(p) {
				nestedRoots = append(nestedRoots, p)
				return filepath.SkipDir // Do not recurse into nested root
			}
		}
		return nil
	})

	return nestedRoots, walkErr
}

// PurgeNestedProjectRoots removes any invalid nested .zqk directories located inside projectRoot,
// preserving the root projectRoot/.zqk untouched.
func PurgeNestedProjectRoots(projectRoot string) ([]string, error) {
	nested, err := FindNestedProjectRoots(projectRoot)
	if err != nil {
		return nil, err
	}

	var purged []string
	for _, nr := range nested {
		nestedDataDir := filepath.Join(nr, ProjectDataDir)
		if remErr := fileutil.RemoveAll(nestedDataDir); remErr == nil {
			purged = append(purged, nestedDataDir)
		} else {
			return purged, fmt.Errorf("failed to remove nested project data dir %s: %w", nestedDataDir, remErr)
		}
	}
	return purged, nil
}
