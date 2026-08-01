package testenvroot

import (
	"os"
	"path/filepath"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// CopyObjectSpecsFromProject copies YAML object specs from projectRoot into testRoot
// (under docs/architecture/_internal/object_specs).
func CopyObjectSpecsFromProject(testRoot, projectRoot string) error {
	sourceSpecsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	targetSpecsDir := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir)
	if _, err := os.Stat(sourceSpecsDir); os.IsNotExist(err) {
		return errfmt.Errorf("source specs directory does not exist: %s", sourceSpecsDir)
	}
	if err := fileutil.EnsureDir(targetSpecsDir); err != nil {
		return errfmt.Newf("failed to create target specs directory").Wrap(err)
	}
	entries, err := os.ReadDir(sourceSpecsDir)
	if err != nil {
		return errfmt.Newf("failed to read source specs directory").Wrap(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		sourcePath := filepath.Join(sourceSpecsDir, entry.Name())
		targetPath := filepath.Join(targetSpecsDir, entry.Name())
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			return errfmt.Errorf("failed to read source file %s: %w", sourcePath, err)
		}
		if err := fileutil.WriteSecureFile(targetPath, data); err != nil { //nolint:gosec // spec files
			return errfmt.Errorf("failed to write target file %s: %w", targetPath, err)
		}
	}
	return nil
}

// BootstrapRoot creates the minimal directory layout under testRoot and, when projectRoot
// is non-empty, copies object spec files from projectRoot (same contract as pkg/testing.BootstrapTestRoot).
func BootstrapRoot(testRoot, projectRoot string) error {
	if _, err := Setup(testRoot); err != nil {
		return err
	}
	if projectRoot == "" {
		return nil
	}
	return CopyObjectSpecsFromProject(testRoot, projectRoot)
}
