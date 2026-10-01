package testenvroot

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// linkOrCopyYAML copies sourcePath to targetPath so tests cannot mutate repository schemas.
func linkOrCopyYAML(sourcePath, targetPath string) error {
	data, err := fileutil.ReadFile(sourcePath)
	if err != nil {
		return errfmt.Errorf("failed to read source file %s: %w", sourcePath, err)
	}
	if err := fileutil.WriteSecureFile(targetPath, data); err != nil { //nolint:gosec // fixture YAML
		return errfmt.Errorf("failed to write target file %s: %w", targetPath, err)
	}
	return nil
}

// copyYAMLFilesFromProject copies *.yaml/*.yml from projectRoot/relDir recursively into testRoot/relDir.
func copyYAMLFilesFromProject(testRoot, projectRoot, relDir string) error {
	sourceDir := filepath.Join(projectRoot, relDir)
	targetDir := filepath.Join(testRoot, relDir)
	if _, err := fileutil.Stat(sourceDir); fileutil.IsNotExist(err) {
		return nil
	}
	return filepath.Walk(sourceDir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			if fileutil.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(targetDir, rel)
		if err := fileutil.EnsureDir(filepath.Dir(targetPath)); err != nil {
			return err
		}
		return linkOrCopyYAML(path, targetPath)
	})
}

// CopyObjectSpecsFromProject copies YAML object specs from projectRoot into testRoot
// (under .zqk/specs/objects).
func CopyObjectSpecsFromProject(testRoot, projectRoot string) error {
	if err := copyYAMLFilesFromProject(testRoot, projectRoot, paths.ProcessInternalObjectSpecsDir); err != nil {
		return err
	}
	packsDir := filepath.Join(projectRoot, "packs")
	if entries, err := fileutil.ReadDir(packsDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				packSpecsDir := filepath.Join(packsDir, entry.Name(), "specs")
				targetDir := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir, entry.Name())
				_ = copyYAMLFiles(packSpecsDir, targetDir)
			}
		}
	}
	return nil
}

// CopyLifecyclesFromProject copies YAML lifecycle files from projectRoot into testRoot.
// FileObjectStorage binds a LifecycleLoader to <testRoot>/.zqk/specs/lifecycles;
// empty TEST_ROOT dirs make Create fail with "failed to read lifecycle file".
func CopyLifecyclesFromProject(testRoot, projectRoot string) error {
	if err := copyYAMLFilesFromProject(testRoot, projectRoot, paths.ProcessInternalLifecyclesDir); err != nil {
		return err
	}
	packsDir := filepath.Join(projectRoot, "packs")
	if entries, err := fileutil.ReadDir(packsDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				packLifecyclesDir := filepath.Join(packsDir, entry.Name(), "lifecycles")
				targetDir := filepath.Join(testRoot, paths.ProcessInternalLifecyclesDir, entry.Name())
				_ = copyYAMLFiles(packLifecyclesDir, targetDir)
			}
		}
	}
	return nil
}

// CopyTraitsFromProject copies YAML trait files from projectRoot into testRoot
// (under .zqk/specs/traits).
func CopyTraitsFromProject(testRoot, projectRoot string) error {
	return copyYAMLFilesFromProject(testRoot, projectRoot, paths.ProcessInternalTraitsDir)
}

// CopyConfigsFromProject copies YAML config files from projectRoot into testRoot
// (under .zqk/specs/configs).
func CopyConfigsFromProject(testRoot, projectRoot string) error {
	return copyYAMLFilesFromProject(testRoot, projectRoot, paths.ProcessInternalConfigsDir)
}

// CopySpecIndexFromProject copies or hardlinks spec_index.json from projectRoot into testRoot if present.
func CopySpecIndexFromProject(testRoot, projectRoot string) error {
	sourcePath := filepath.Join(projectRoot, paths.ProcessInternalDir, "spec_index.json")
	targetDir := filepath.Join(testRoot, paths.ProcessInternalDir)
	if _, err := fileutil.Stat(sourcePath); fileutil.IsNotExist(err) {
		return nil
	}
	if err := fileutil.EnsureDir(targetDir); err != nil {
		return errfmt.Newf("failed to create target directory").Wrap(err)
	}
	targetPath := filepath.Join(targetDir, "spec_index.json")
	return linkOrCopyYAML(sourcePath, targetPath)
}

func copyYAMLFiles(sourceDir, targetDir string) error {
	if _, err := fileutil.Stat(sourceDir); fileutil.IsNotExist(err) {
		return nil
	}
	return filepath.Walk(sourceDir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			if fileutil.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(targetDir, rel)
		if err := fileutil.EnsureDir(filepath.Dir(targetPath)); err != nil {
			return err
		}
		return linkOrCopyYAML(path, targetPath)
	})
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
	if err := copyYAMLFilesFromProject(testRoot, projectRoot, paths.ProcessInternalDir); err != nil {
		return err
	}
	if err := copyYAMLFilesFromProject(testRoot, projectRoot, "packs"); err != nil {
		return err
	}
	// Also ensure pack-owned specs and lifecycles (packs/*/specs -> .zqk/specs/objects
	// and packs/*/lifecycles -> .zqk/specs/lifecycles) are seeded directly into the internal
	// schema plane so that hermetic test roots have full access to all pack kinds.
	dstSpecs := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir)
	dstLifecycles := filepath.Join(testRoot, paths.ProcessInternalLifecyclesDir)
	packsDir := filepath.Join(projectRoot, "packs")
	if entries, err := fileutil.ReadDir(packsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				packSpecs := filepath.Join(packsDir, e.Name(), "specs")
				_ = copyYAMLFiles(packSpecs, dstSpecs)
				packLifecycles := filepath.Join(packsDir, e.Name(), "lifecycles")
				_ = copyYAMLFiles(packLifecycles, dstLifecycles)
			}
		}
	}

	return CopySpecIndexFromProject(testRoot, projectRoot)
}
