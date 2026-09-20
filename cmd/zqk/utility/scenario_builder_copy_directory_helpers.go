package utility

import (
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/appledouble"
	"github.com/zqk-os/zqk/pkg/errfmt"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// validateSourceDirectory validates that the source directory exists and is a directory
func validateSourceDirectory(source string) (fileutil.FileInfo, error) {
	sourceInfo, err := fileutil.Stat(source)
	if fileutil.IsNotExist(err) {
		return nil, errfmt.Errorf("source directory does not exist: %s", source)
	}
	if !sourceInfo.IsDir() {
		return nil, errfmt.Errorf("source is not a directory: %s", source)
	}
	return sourceInfo, nil
}

// shouldSkipFile determines if a file should be skipped during copy
func shouldSkipFile(info fileutil.FileInfo, relPath string) bool {
	if appledouble.SkipPathInTreeWalk(relPath) {
		return true
	}
	if info.Name() != relPath {
		// This is a nested file, check parent
		parts := strings.Split(relPath, string(filepath.Separator))
		if len(parts) > 0 && strings.HasPrefix(parts[0], ".") && parts[0] != "." {
			return true
		}
	} else if strings.HasPrefix(info.Name(), ".") && info.Name() != "." && info.Name() != ".." {
		// Root-level hidden file/directory
		return true
	}
	return false
}

// copyFileOrDirectory copies a file or creates a directory
func (sb *ScenarioBuilder) copyFileOrDirectory(path, targetPath string, info fileutil.FileInfo) error {
	if info.IsDir() {
		return fileutil.MkdirAll(targetPath, info.Mode())
	}
	return sb.copyFile(path, targetPath)
}
