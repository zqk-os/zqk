package bootstrap

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// RestoreState restores all specs from a bootstrap capture to the target directory
func RestoreState(capture *BootstrapCapture, targetDir string, force bool) error {
	// Restore object specs
	specsDir := filepath.Join(targetDir, "object_specs")
	if err := restoreDirectory(capture.ObjectSpecs, specsDir, force); err != nil {
		return errfmt.Newf("failed to restore object specs").Wrap(err)
	}

	// Restore lifecycles
	lifecyclesDir := filepath.Join(targetDir, "lifecycles")
	if err := restoreDirectory(capture.Lifecycles, lifecyclesDir, force); err != nil {
		return errfmt.Newf("failed to restore lifecycles").Wrap(err)
	}

	// Restore profiles
	profilesDir := filepath.Join(targetDir, "profile_specs")
	if err := restoreDirectory(capture.Profiles, profilesDir, force); err != nil {
		return errfmt.Newf("failed to restore profiles").Wrap(err)
	}

	// Restore config files (top-level)
	if err := restoreConfigFiles(capture.Configs, targetDir, force); err != nil {
		return errfmt.Newf("failed to restore configs").Wrap(err)
	}

	// Restore traits
	traitsDir := filepath.Join(targetDir, "traits")
	if err := restoreDirectory(capture.Traits, traitsDir, force); err != nil {
		return errfmt.Newf("failed to restore traits").Wrap(err)
	}

	return nil
}

// restoreDirectory restores files to a directory
func restoreDirectory(files map[string][]byte, targetDir string, force bool) error {
	// Create target directory
	if err := fileutil.MkdirAll(targetDir, paths.DirPerm755); err != nil {
		return errfmt.Errorf("failed to create directory %s: %w", targetDir, err)
	}

	for filename, content := range files {
		// Handle subdirectory paths (e.g., "built-in/component_lifecycle.yaml")
		filePath := filepath.Join(targetDir, filename)

		// Check if file exists
		if _, err := fileutil.Stat(filePath); err == nil && !force {
			return errfmt.Errorf("file already exists (use --force to overwrite): %s", filePath)
		}

		// Create subdirectory if needed
		if err := fileutil.MkdirAll(filepath.Dir(filePath), paths.DirPerm755); err != nil {
			return errfmt.Errorf("failed to create directory for %s: %w", filePath, err)
		}

		// Write file
		if err := fileutil.WriteFile(filePath, content, paths.FilePerm644); err != nil {
			return errfmt.Errorf("failed to write file %s: %w", filePath, err)
		}
	}

	return nil
}

// restoreConfigFiles restores config files to the target directory (top-level)
func restoreConfigFiles(files map[string][]byte, targetDir string, force bool) error {
	for filename, content := range files {
		filePath := filepath.Join(targetDir, filename)

		// Check if file exists
		if _, err := fileutil.Stat(filePath); err == nil && !force {
			return errfmt.Errorf("config file already exists (use --force to overwrite): %s", filePath)
		}

		// Write file
		if err := fileutil.WriteFile(filePath, content, paths.FilePerm644); err != nil {
			return errfmt.Errorf("failed to write config file %s: %w", filePath, err)
		}
	}

	return nil
}

// InitializeFromBootstrap initializes a directory from a bootstrap file
func InitializeFromBootstrap(bootstrapFile string, targetDir string, force bool) error {
	// Load bootstrap capture
	capture, err := LoadFromFile(bootstrapFile)
	if err != nil {
		return errfmt.Newf("failed to load bootstrap file").Wrap(err)
	}

	// Restore state
	if err := RestoreState(capture, targetDir, force); err != nil {
		return errfmt.Newf("failed to restore state").Wrap(err)
	}

	return nil
}
