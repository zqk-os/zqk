package utility

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

const (
	infraMetricDirectorySearch      = "directory_search"
	infraMetricConfigFileCopy       = "config_file_copy"
	infraMetricProfileCreation      = "profile_creation"
	infraMetricBootstrapConfigCopy  = "bootstrap_config_copy"
	infraMetricBootstrapConfigSkip  = "bootstrap_config_skip"
	infraFieldDepth                 = "depth"
	infraFieldFound                 = "found"
	infraFieldFilename              = "filename"
	infraFieldIsUpdate              = "is_update"
	infraFieldSourcePath            = "source_path"
	infraFieldTargetPath            = "target_path"
	infraFieldProfileName           = "profile_name"
	infraFieldFile                  = "file"
	infraFieldTarget                = "target"
	infraFieldUpdated               = "updated"
	infraTagInfrastructure          = "infrastructure"
	infraTagConfig                  = "config"
	infraTagBootstrap               = "bootstrap"
	infraTagCopy                    = "copy"
	infraTagSkip                    = "skip"
	infraTagFileCopy                = "file_copy"
	infraTagProfile                 = "profile"
	infraTagCreation                = "creation"
	infraProfilePathPkg             = "pkg"
	infraProfilePathCLI             = "cli"
	infraProfilePathProfiles        = "profiles"
	infraProfileExtendsHuman        = "human"
	infraProfileFormatTable         = "table"
	infraProfileLevelInfo           = "info"
	infraErrNoBootstrapConfigToCopy = "no bootstrap config files found to copy"
)

// getInfrastructureMetricsRecorder gets the metrics recorder for infrastructure operations
// Uses the factory pattern to get a self-registering, decoupled metrics recorder
func (sb *ScenarioBuilder) getInfrastructureMetricsRecorder() MetricRecorder {
	// For now, return no-op - can be extended to get from coordinator or factory
	// This allows the new API to be in place without requiring full implementation
	return GetNoOpMetricRecorder()
}

// copyBootstrapConfigFiles copies essential configuration files from the main project
// to the scenario's _internal directory. These files are required for ID validation
// and other system operations to work correctly.
// This operation is idempotent - it only copies files if the source is newer than the target
// (based on mtime comparison), preventing unnecessary copies.
func (sb *ScenarioBuilder) copyBootstrapConfigFiles(targetInternalDir string) error {
	startTime := time.Now()
	recorder := sb.getInfrastructureMetricsRecorder()

	// Find source directory (main project root)
	// Walk up from current directory to find .zqk/specs
	searchStart := time.Now()
	dir, err := fileutil.Getwd()
	if err != nil {
		return errfmt.Newf("failed to get current directory").Wrap(err)
	}

	var sourceInternalDir string
	depth := 0
	for {
		testPath := filepath.Join(dir, paths.ProcessInternalDir)
		if _, err := fileutil.Stat(testPath); err == nil {
			sourceInternalDir = testPath
			_ = recorder.Record(infraMetricDirectorySearch, NewMetricBuilder(infraMetricDirectorySearch).
				WithField(infraFieldDepth, depth).
				WithField(infraFieldFound, true).
				WithDuration(time.Since(searchStart)).
				WithTags(infraTagInfrastructure, infraMetricDirectorySearch))
			break
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root - try to find from target directory instead
			// (scenario might be in a subdirectory of the main project)
			if absTarget, err := filepath.Abs(sb.config.TargetDir); err == nil {
				// Walk up from target directory
				targetDir := filepath.Dir(absTarget)
				depth = 0
				searchStart = time.Now()
				for {
					testPath := filepath.Join(targetDir, paths.ProcessInternalDir)
					if _, err := fileutil.Stat(testPath); err == nil {
						sourceInternalDir = testPath
						_ = recorder.Record(infraMetricDirectorySearch, NewMetricBuilder(infraMetricDirectorySearch).
							WithField(infraFieldDepth, depth).
							WithField(infraFieldFound, true).
							WithDuration(time.Since(searchStart)).
							WithTags(infraTagInfrastructure, infraMetricDirectorySearch))
						break
					}
					parent := filepath.Dir(targetDir)
					if parent == targetDir {
						_ = recorder.Record(infraMetricDirectorySearch, NewMetricBuilder(infraMetricDirectorySearch).
							WithField(infraFieldDepth, depth).
							WithField(infraFieldFound, false).
							WithDuration(time.Since(searchStart)).
							WithTags(infraTagInfrastructure, infraMetricDirectorySearch))
						break
					}
					targetDir = parent
					depth++
				}
			} else {
				_ = recorder.Record(infraMetricDirectorySearch, NewMetricBuilder(infraMetricDirectorySearch).
					WithField(infraFieldDepth, depth).
					WithField(infraFieldFound, false).
					WithDuration(time.Since(searchStart)).
					WithTags(infraTagInfrastructure, infraMetricDirectorySearch))
			}
			break
		}
		dir = parent
		depth++
	}

	if sourceInternalDir == emptyValue {
		return errfmt.Errorf("cannot find source bootstrap directory (%s)", paths.ProcessInternalDir)
	}

	// Required config files to copy (essential for ID validation and system operation)
	// Map filename to reload handler function
	requiredFiles := map[string]func() error{
		"id_prefixes_config.yaml": func() error {
			// Reset global config to force reload on next access
			validation.ResetGlobalIDPrefixesConfig()
			return nil
		},
		"kind_mappings_config.yaml": func() error {
			objects.ResetGlobalKindMappingsConfig()
			return nil
		},
		"namespaces_config.yaml": func() error {
			// Reset namespaces config if it has a reset function
			return nil
		},
		"paths_config.yaml": func() error {
			// Reset paths config if it has a reset function
			validation.ResetGlobalPathsConfig()
			return nil
		},
	}

	// Copy each required file (idempotent - only if source is newer)
	copiedCount := 0
	updatedCount := 0
	for filename, reloadHandler := range requiredFiles {
		sourcePath := filepath.Join(sourceInternalDir, filename)
		targetPath := filepath.Join(targetInternalDir, filename)

		// Check if source exists
		sourceInfo, err := fileutil.Stat(sourcePath)
		if fileutil.IsNotExist(err) {
			logging.FluentEvent(sb.logger).Debug("Bootstrap source file not found, skipping").
				String(infraFieldFile, filename).
				Log()
			continue
		}
		if err != nil {
			return errfmt.Errorf("failed to stat source file %s: %w", filename, err)
		}

		// Check if target exists and compare mtime (idempotent check)
		targetInfo, err := fileutil.Stat(targetPath)
		needsCopy := false
		if fileutil.IsNotExist(err) {
			// Target doesn't exist - need to copy
			needsCopy = true
		} else if err != nil {
			return errfmt.Errorf("failed to stat target file %s: %w", filename, err)
		} else {
			// Both exist - check if source is newer
			if sourceInfo.ModTime().After(targetInfo.ModTime()) {
				needsCopy = true
			}
		}

		if !needsCopy {
			logging.FluentEvent(sb.logger).Debug("Bootstrap config file up to date, skipping").
				String(infraFieldFile, filename).
				Log()
			_ = recorder.Record(infraMetricBootstrapConfigSkip, NewMetricBuilder(infraMetricBootstrapConfigSkip).
				WithField(infraFieldFilename, filename).
				WithTags(infraTagInfrastructure, infraTagConfig, infraTagBootstrap, infraTagSkip))
			continue
		}

		// Copy file
		copyStart := time.Now()
		if err := sb.copyConfigFile(sourcePath, targetPath); err != nil {
			copyDuration := time.Since(copyStart)
			_ = recorder.Record(infraMetricBootstrapConfigCopy, NewMetricBuilder(infraMetricBootstrapConfigCopy).
				WithField(infraFieldFilename, filename).
				WithField(infraFieldIsUpdate, targetInfo != nil).
				WithDuration(copyDuration).
				WithError(err).
				WithTags(infraTagInfrastructure, infraTagConfig, infraTagBootstrap, infraTagCopy))
			return errfmt.Errorf("failed to copy bootstrap file %s: %w", filename, err)
		}
		copyDuration := time.Since(copyStart)

		copiedCount++
		isUpdate := targetInfo != nil
		if isUpdate {
			updatedCount++
		}
		_ = recorder.Record(infraMetricBootstrapConfigCopy, NewMetricBuilder(infraMetricBootstrapConfigCopy).
			WithField(infraFieldFilename, filename).
			WithField(infraFieldIsUpdate, isUpdate).
			WithDuration(copyDuration).
			WithTags(infraTagInfrastructure, infraTagConfig, infraTagBootstrap, infraTagCopy))

		logging.FluentEvent(sb.logger).Debug("Copied bootstrap config file").
			String(infraFieldFile, filename).
			String(infraFieldTarget, targetPath).
			Bool(infraFieldUpdated, targetInfo != nil).
			Log()

		// Emit event via coordinator if available (for file change detection)
		if sb.coordinator != nil {
			eventType := "config_file_copied"
			if targetInfo != nil {
				eventType = "config_file_updated"
			}
			sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, eventType,
				fmt.Sprintf("Config file %s %s", filename, map[bool]string{true: "updated", false: "copied"}[targetInfo != nil]),
				map[string]any{
					infraFieldFile:         filename,
					objects.FieldKeySource: sourcePath,
					objects.FieldKeyTarget: targetPath,
					"mtime":                sourceInfo.ModTime(),
					"reloaded":             false, // Will be reloaded on next access
				})

			// Trigger reload handler to reset global config (will reload on next access)
			if reloadHandler != nil {
				if err := reloadHandler(); err != nil {
					logging.FluentEvent(sb.logger).Warn("Failed to trigger config reload").
						String("file", filename).
						WithError(err).
						Log()
				}
			}
		}
	}

	if copiedCount == 0 {
		totalDuration := time.Since(startTime)
		_ = recorder.Record(infraMetricBootstrapConfigCopy, NewMetricBuilder(infraMetricBootstrapConfigCopy).
			WithDuration(totalDuration).
			WithError(errors.New(infraErrNoBootstrapConfigToCopy)).
			WithTags(infraTagInfrastructure, infraTagConfig, infraTagBootstrap, infraTagCopy))
		return errors.New(infraErrNoBootstrapConfigToCopy)
	}

	// Total operation duration could be recorded here if needed
	_ = time.Since(startTime) // Total duration available for future metrics

	return nil
}

// copyScenarioBuilderProfile creates the scenario_builder profile programmatically
// and writes it to the target scenario's pkg/cli/profiles/ directory.
// This ensures the scenario can bootstrap itself independently without walking up to the main project.
func (sb *ScenarioBuilder) copyScenarioBuilderProfile(targetDir string) error {
	startTime := time.Now()
	recorder := sb.getInfrastructureMetricsRecorder()

	targetProfilesDir := filepath.Join(targetDir, infraProfilePathPkg, infraProfilePathCLI, infraProfilePathProfiles)
	targetProfilePath := filepath.Join(targetProfilesDir, ScenarioBuilderProfileName+".yaml")

	// Create target directory
	if err := fileutil.MkdirAll(targetProfilesDir, paths.DirPerm755); err != nil {
		duration := time.Since(startTime)
		_ = recorder.Record(infraMetricProfileCreation, NewMetricBuilder(infraMetricProfileCreation).
			WithField(infraFieldProfileName, ScenarioBuilderProfileName).
			WithDuration(duration).
			WithError(err).
			WithTags(infraTagInfrastructure, infraTagProfile, infraTagCreation))
		return errfmt.Newf("failed to create target profiles directory").Wrap(err)
	}

	// Check if target exists and is up to date (idempotent check)
	// Since we're generating it programmatically, we'll always write it
	// but we could add version checking in the future

	// Build CLI profile structure programmatically
	profile := map[string]any{
		objects.FieldKeySchemaVersion: objects.InitialFieldVersion,
		objects.FieldKeyNamespaceID:   paths.CLINamespaceID, // Use dynamic namespace ID from paths package
		objects.FieldKeyName:          ScenarioBuilderProfileName,
		objects.FieldKeyExtends:       infraProfileExtendsHuman,
		objects.FieldKeyDescription: `Scenario builder profile with Info-level logging for progress visibility.
Extends human profile but overrides logging level to show info messages.`,
		objects.FieldKeySpec: map[string]any{
			objects.FieldKeyFlags: map[string]any{
				objects.FieldKeyFormat: infraProfileFormatTable,
			},
			objects.FieldKeyFormat: infraProfileFormatTable,
			"quiet":                false,
			"verbose":              false,
			"logging": map[string]any{
				"level": infraProfileLevelInfo,
			},
		},
	}

	// Write profile to YAML file
	data, err := yaml.Marshal(profile)
	if err != nil {
		duration := time.Since(startTime)
		_ = recorder.Record(infraMetricProfileCreation, NewMetricBuilder(infraMetricProfileCreation).
			WithField(infraFieldProfileName, ScenarioBuilderProfileName).
			WithDuration(duration).
			WithError(err).
			WithTags(infraTagInfrastructure, infraTagProfile, infraTagCreation))
		return errfmt.Errorf("failed to marshal %s profile: %w", ScenarioBuilderProfileName, err)
	}

	//nolint:gosec // G306: 0600 is appropriate for readable config files
	if err := fileutil.WriteFile(targetProfilePath, data, paths.FilePerm644); err != nil {
		duration := time.Since(startTime)
		_ = recorder.Record(infraMetricProfileCreation, NewMetricBuilder(infraMetricProfileCreation).
			WithField(infraFieldProfileName, ScenarioBuilderProfileName).
			WithDuration(duration).
			WithError(err).
			WithTags(infraTagInfrastructure, infraTagProfile, infraTagCreation))
		return errfmt.Errorf("failed to write %s profile: %w", ScenarioBuilderProfileName, err)
	}

	duration := time.Since(startTime)
	_ = recorder.Record(infraMetricProfileCreation, NewMetricBuilder(infraMetricProfileCreation).
		WithField(infraFieldProfileName, ScenarioBuilderProfileName).
		WithDuration(duration).
		WithTags(infraTagInfrastructure, infraTagProfile, infraTagCreation))

	logging.FluentEvent(sb.logger).Debug(fmt.Sprintf("Created %s profile", ScenarioBuilderProfileName)).
		String("target", targetProfilePath).
		Log()

	return nil
}

// copyConfigFile copies a single config file from source to target
// Preserves mtime from source so file watchers can detect changes correctly
func (sb *ScenarioBuilder) copyConfigFile(sourcePath, targetPath string) error {
	startTime := time.Now()
	recorder := sb.getInfrastructureMetricsRecorder()
	sourceFile, err := fileutil.Open(sourcePath)
	if err != nil {
		duration := time.Since(startTime)
		_ = recorder.Record("config_file_copy", NewMetricBuilder("config_file_copy").
			WithField("source_path", sourcePath).
			WithField("target_path", targetPath).
			WithDuration(duration).
			WithError(err).
			WithTags("infrastructure", "config", "file_copy"))
		return errfmt.Newf("failed to open source file").Wrap(err)
	}
	defer sourceFile.Close()

	// Get source file info to preserve mtime
	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		duration := time.Since(startTime)
		_ = recorder.Record("config_file_copy", NewMetricBuilder("config_file_copy").
			WithField("source_path", sourcePath).
			WithField("target_path", targetPath).
			WithDuration(duration).
			WithError(err).
			WithTags("infrastructure", "config", "file_copy"))
		return errfmt.Newf("failed to stat source file").Wrap(err)
	}

	// Ensure target directory exists
	if err := fileutil.MkdirAll(filepath.Dir(targetPath), paths.DirPerm755); err != nil {
		duration := time.Since(startTime)
		_ = recorder.Record("config_file_copy", NewMetricBuilder("config_file_copy").
			WithField("source_path", sourcePath).
			WithField("target_path", targetPath).
			WithDuration(duration).
			WithError(err).
			WithTags("infrastructure", "config", "file_copy"))
		return errfmt.Newf("failed to create target directory").Wrap(err)
	}

	targetFile, err := fileutil.Create(targetPath)
	if err != nil {
		duration := time.Since(startTime)
		_ = recorder.Record("config_file_copy", NewMetricBuilder("config_file_copy").
			WithField("source_path", sourcePath).
			WithField("target_path", targetPath).
			WithDuration(duration).
			WithError(err).
			WithTags("infrastructure", "config", "file_copy"))
		return errfmt.Newf("failed to create target file").Wrap(err)
	}
	defer targetFile.Close()

	if _, err := io.Copy(targetFile, sourceFile); err != nil {
		duration := time.Since(startTime)
		_ = recorder.Record("config_file_copy", NewMetricBuilder("config_file_copy").
			WithField("source_path", sourcePath).
			WithField("target_path", targetPath).
			WithDuration(duration).
			WithError(err).
			WithTags("infrastructure", "config", "file_copy"))
		return errfmt.Newf("failed to copy file content").Wrap(err)
	}

	// Preserve mtime from source so file watchers can detect changes
	// This ensures the watcher sees the file as "new" or "updated" based on source mtime
	if err := fileutil.Chtimes(targetPath, sourceInfo.ModTime(), sourceInfo.ModTime()); err != nil {
		// Log warning but don't fail - mtime preservation is best effort
		logging.FluentEvent(sb.logger).Debug("Failed to preserve mtime on copied file").
			String("target", targetPath).
			WithError(err).
			Log()
	}

	duration := time.Since(startTime)
	_ = recorder.Record("config_file_copy", NewMetricBuilder("config_file_copy").
		WithField("source_path", sourcePath).
		WithField("target_path", targetPath).
		WithDuration(duration).
		WithTags("infrastructure", "config", "file_copy"))

	return nil
}
