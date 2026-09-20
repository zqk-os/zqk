package utility

import (
	"github.com/zqk-os/zqk/pkg/datacell"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"io"
	"path/filepath"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
)

// setupInfrastructure generates required infrastructure files from built-in builders
// This allows the scenario to work independently without requiring the main project.
// Lifecycles are generated from built-in lifecycle builders, ensuring the CLI can
// establish itself without any external references.
func (sb *ScenarioBuilder) setupInfrastructure() error {
	// Ensure directory structure exists
	processDir := datacell.ProcessPrimaryDir(sb.config.TargetDir)
	internalDir := filepath.Join(processDir, "_internal")
	lifecyclesDir := filepath.Join(internalDir, "lifecycles")
	specsDir := filepath.Join(internalDir, "object_specs")

	if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create lifecycles directory").Wrap(err)
	}
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create specs directory").Wrap(err)
	}

	// Generate spec YAML files from built-in builders (must be done first)
	specGeneratedCount, err := sb.generateSpecs(specsDir)
	if err != nil {
		return err
	}
	if specGeneratedCount == 0 && sb.logger != nil {
		logging.FluentEvent(sb.logger).Warn("No specs generated; continuing so lifecycles can still be written").
			String("hint", "bldr_v2 package may need to be imported").
			Log()
	}

	// Copy bootstrap configuration files (id_prefixes_config.yaml, etc.)
	// These are required for ID validation and other system operations
	// Use the same bootstrap extraction logic as system init
	if err := sb.copyBootstrapConfigFiles(internalDir); err != nil {
		// Log warning but don't fail - system can fall back to defaults
		sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
			"Failed to copy bootstrap config files, system will use defaults",
			map[string]any{
				"target_dir": internalDir,
				"error":      err.Error(),
			})
	} else {
		sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
			"Copied bootstrap config files",
			map[string]any{"target_dir": internalDir})
		if sb.logger != nil {
			logging.FluentEvent(sb.logger).Debug("Copied bootstrap config files").
				String("target_dir", internalDir).
				Log()
		}
	}

	// Copy scenario_builder profile to target's CLI profiles directory
	// This preserves the profile for the scenario so it can bootstrap itself independently
	// The profile is needed for proper logging configuration during scenario execution
	if err := sb.copyScenarioBuilderProfile(sb.config.TargetDir); err != nil {
		// Log warning but don't fail - profile loader can walk up to find it, but scenario won't be self-contained
		sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
			"Failed to copy scenario_builder profile, scenario will depend on main project",
			map[string]any{
				"target_dir": sb.config.TargetDir,
				"error":      err,
			})
	} else {
		logging.FluentEvent(sb.logger).Debug("Copied scenario_builder profile").
			String("target_dir", sb.config.TargetDir).
			Log()
	}

	// Setup config file watcher to detect changes and trigger reloads
	// This enables event-driven reloads when config files are modified
	if sb.coordinator != nil && sb.logger != nil {
		// Use logger's underlying Logger interface for watcher
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		watcher := NewConfigFileWatcher(sb.config.TargetDir, sb.coordinator, logger)
		watcher.SetupDefaultWatchers()
		// Start watching in background (non-blocking)
		ctx := pkgctx.NewSystemContext()
		if err := watcher.Start(ctx); err != nil {
			logging.FluentEvent(sb.logger).Warn("Failed to start config file watcher").
				WithError(err).
				Log()
		} else {
			logging.FluentEvent(sb.logger).Debug("Started config file watcher").
				String("target_dir", sb.config.TargetDir).
				Log()
		}
	}

	// Generate lifecycle YAML files from built-in builders
	lifecycleGeneratedCount, err := sb.generateLifecycles(lifecyclesDir)
	if err != nil {
		return err
	}

	logging.FluentEvent(sb.logger).Debug("Infrastructure setup complete - specs and lifecycles generated from built-in builders").
		String("target_dir", sb.config.TargetDir).
		Int("specs_generated", specGeneratedCount).
		Int("lifecycles_generated", lifecycleGeneratedCount).
		Log()

	return nil
}

// copyFile copies a single file from source to target
func (sb *ScenarioBuilder) copyFile(source, target string) error {
	// Ensure target directory exists
	if err := fileutil.MkdirAll(filepath.Dir(target), paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create target directory").Wrap(err)
	}

	sourceFile, err := fileutil.Open(source)
	if err != nil {
		return errfmt.Newf("failed to open source file").Wrap(err)
	}
	defer sourceFile.Close()

	targetFile, err := fileutil.Create(target)
	if err != nil {
		return errfmt.Newf("failed to create target file").Wrap(err)
	}
	defer targetFile.Close()

	if _, err := io.Copy(targetFile, sourceFile); err != nil {
		return errfmt.Newf("failed to copy file content").Wrap(err)
	}

	// Preserve file permissions
	sourceInfo, err := sourceFile.Stat()
	if err == nil {
		_ = fileutil.Chmod(target, sourceInfo.Mode()) //nolint:errcheck // Best effort to preserve permissions
	}

	return nil
}

// copyDirectory recursively copies a directory from source to target
func (sb *ScenarioBuilder) copyDirectory(source, target string) error {
	sourceInfo, err := validateSourceDirectory(source)
	if err != nil {
		return err
	}

	if err := fileutil.MkdirAll(target, sourceInfo.Mode()); err != nil {
		return errfmt.Newf("failed to create target directory").Wrap(err)
	}

	return filepath.Walk(source, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}

		if shouldSkipFile(info, relPath) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		targetPath := filepath.Join(target, relPath)
		return sb.copyFileOrDirectory(path, targetPath, info)
	})
}

// copyBootstrapConfigFiles, copyScenarioBuilderProfile, copyConfigFile are defined in scenario_builder_infrastructure_config.go
