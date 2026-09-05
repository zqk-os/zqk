package storage

import (
	"github.com/lanceman/zqk/pkg/datacell"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/appledouble"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
)

// ValidateObjectFileLocation validates that an object file is in the correct directory
// based on its kind. This prevents issues where files are placed in wrong directories
// (e.g., scheduler_job files in scheduler/jobs/ instead of scheduler_jobs/).
// For bucketed kinds (e.g. audit_event under audit/YYYY-MM/... or audit/YYYY-MM-DD/.../completed/),
// the file is valid if its path lies under the kind's expected top-level dir (e.g. "audit").
func ValidateObjectFileLocation(filePath, kind string) error {
	// Reject known external brain directories for agent state and task tracking
	if strings.Contains(filePath, "/.gemini/") || strings.Contains(filePath, "/.ide/") || strings.Contains(filePath, "/.claude/") {
		return errfmt.Errorf("Object file path %s contains restricted external brain directory. State must exclusively use ZQK graph or .zqk-state/system-state.csnap.", filePath)
	}

	expectedDirName := objects.GetDirectoryFromKind(kind)
	if expectedDirName == emptyValue {
		return nil
	}

	// Normalize path and check whether the file lives under the expected directory.
	// This accepts both flat (metrics/file.yaml) and deep bucketed (audit/2026-02-27/audit_aggregation_metric/completed/file.yaml) layouts.
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil // Skip if we can't normalize
	}
	for part := range strings.SplitSeq(filepath.ToSlash(absPath), "/") {
		if part == expectedDirName {
			// File path contains the expected directory as a segment (e.g. .../audit/2026-02-27/...)
			// so it's under the correct kind root
			return nil
		}
	}

	// Legacy: also accept immediate parent or grandparent match for compatibility
	actualDir := filepath.Dir(filePath)
	actualDirName := filepath.Base(actualDir)
	parentDir := filepath.Dir(actualDir)
	parentDirName := filepath.Base(parentDir)
	if parentDirName == expectedDirName || actualDirName == expectedDirName {
		return nil
	}

	return errfmt.Errorf(ConstMiscObjectFileSKindSIsInWrongDirectorySExpec,
		filepath.Base(filePath), kind, actualDir, expectedDirName)
}

// FindMisplacedObjectFiles scans for object files in incorrect directories
// Returns a map of kind -> []file paths that are in wrong locations
func FindMisplacedObjectFiles(projectRoot string) (map[string][]string, error) {
	processDir := datacell.ProcessPrimaryDir(projectRoot)
	if _, err := fileutil.Stat(processDir); err != nil {
		return nil, errfmt.Newf(ConstMiscProcessDirectoryNotFound).Wrap(err)
	}

	misplaced := make(map[string][]string)
	kindMapper := objects.GetGlobalKindMapper()
	if err := kindMapper.Initialize(); err != nil {
		return nil, errfmt.Newf(ConstMiscFailedToInitializeKindMapper).Wrap(err)
	}

	// Walk through all YAML files in process directory
	err := filepath.Walk(processDir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories (and do not descend into _internal: config/spec/migrations, not process data)
		if info.IsDir() {
			if filepath.Base(path) == "_internal" {
				return filepath.SkipDir
			}
			return nil
		}

		if appledouble.SkipPathInTreeWalk(path) {
			return nil
		}

		// Only check YAML files
		if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
			return nil
		}

		// Skip hash registry files and other metadata
		if strings.HasPrefix(filepath.Base(path), ".") {
			return nil
		}

		// Try to read the file to get kind
		data, err := fileutil.ReadFile(path)
		if err != nil {
			return nil // Skip files we can't read
		}

		// Extract the top-level `kind:` field from YAML without a full parse.
		// IMPORTANT: do NOT TrimSpace before the prefix check. Top-level YAML fields
		// have zero leading whitespace. Trimming first would match `kind:` lines embedded
		// inside block scalars or multi-line string values (e.g. code examples in a policy's
		// `examples:` field), producing false-positive misplacement reports.
		lines := strings.Split(string(data), "\n")
		var kind string
		for _, line := range lines {
			if strings.HasPrefix(line, "kind:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					kind = strings.TrimSpace(parts[1])
					break
				}
			}
		}

		if kind == emptyValue {
			return nil // Can't determine kind, skip
		}

		// Validate location
		if err := ValidateObjectFileLocation(path, kind); err != nil {
			// File is misplaced
			if misplaced[kind] == nil {
				misplaced[kind] = make([]string, 0)
			}
			misplaced[kind] = append(misplaced[kind], path)
		}

		return nil
	})

	return misplaced, err
}
