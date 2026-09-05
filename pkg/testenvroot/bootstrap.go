package testenvroot

import (
	"path/filepath"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// copyYAMLFilesFromProject copies top-level *.yaml/*.yml from projectRoot/relDir into testRoot/relDir.
func copyYAMLFilesFromProject(testRoot, projectRoot, relDir string) error {
	sourceDir := filepath.Join(projectRoot, relDir)
	targetDir := filepath.Join(testRoot, relDir)
	if _, err := fileutil.Stat(sourceDir); fileutil.IsNotExist(err) {
		return errfmt.Errorf("source directory does not exist: %s", sourceDir)
	}
	if err := fileutil.EnsureDir(targetDir); err != nil {
		return errfmt.Newf("failed to create target directory").Wrap(err)
	}
	entries, err := fileutil.ReadDir(sourceDir)
	if err != nil {
		return errfmt.Newf("failed to read source directory").Wrap(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		sourcePath := filepath.Join(sourceDir, entry.Name())
		targetPath := filepath.Join(targetDir, entry.Name())
		data, err := fileutil.ReadFile(sourcePath)
		if err != nil {
			return errfmt.Errorf("failed to read source file %s: %w", sourcePath, err)
		}
		if err := fileutil.WriteSecureFile(targetPath, data); err != nil { //nolint:gosec // fixture YAML
			return errfmt.Errorf("failed to write target file %s: %w", targetPath, err)
		}
	}
	return nil
}

// CopyObjectSpecsFromProject copies YAML object specs from projectRoot into testRoot
// (under docs/process/_internal/object_specs).
func CopyObjectSpecsFromProject(testRoot, projectRoot string) error {
	return copyYAMLFilesFromProject(testRoot, projectRoot, paths.ProcessInternalObjectSpecsDir)
}

// CopyLifecyclesFromProject copies YAML lifecycle files from projectRoot into testRoot.
// FileObjectStorage binds a LifecycleLoader to <testRoot>/docs/process/_internal/lifecycles;
// empty TEST_ROOT dirs make Create fail with "failed to read lifecycle file".
func CopyLifecyclesFromProject(testRoot, projectRoot string) error {
	return copyYAMLFilesFromProject(testRoot, projectRoot, paths.ProcessInternalLifecyclesDir)
}

// CopyTraitsFromProject copies YAML trait files from projectRoot into testRoot
// (under docs/process/_internal/traits).
func CopyTraitsFromProject(testRoot, projectRoot string) error {
	return copyYAMLFilesFromProject(testRoot, projectRoot, paths.ProcessInternalTraitsDir)
}

// CopyConfigsFromProject copies YAML config files from projectRoot into testRoot
// (under docs/process/_internal/configs).
func CopyConfigsFromProject(testRoot, projectRoot string) error {
	return copyYAMLFilesFromProject(testRoot, projectRoot, paths.ProcessInternalConfigsDir)
}

// BootstrapRoot creates the minimal directory layout under testRoot and, when projectRoot
// is non-empty, copies object spec, lifecycle, trait, and config YAML from projectRoot
// (same contract as pkg/testing.BootstrapTestRoot, plus lifecycles/traits/configs required
// for hermetic storage Create and ID validation without repo-tree fallback).
func BootstrapRoot(testRoot, projectRoot string) error {
	if _, err := Setup(testRoot); err != nil {
		return err
	}
	if projectRoot == "" {
		return nil
	}
	if err := CopyObjectSpecsFromProject(testRoot, projectRoot); err != nil {
		return err
	}
	if err := CopyLifecyclesFromProject(testRoot, projectRoot); err != nil {
		return err
	}
	if err := CopyTraitsFromProject(testRoot, projectRoot); err != nil {
		return err
	}
	return CopyConfigsFromProject(testRoot, projectRoot)
}
