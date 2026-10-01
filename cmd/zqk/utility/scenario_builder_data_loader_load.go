package utility

import (
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/safepath"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)


// handleLoadDataFile handles loading objects from a YAML data file
func handleLoadDataFile(cmd any, builder *ScenarioBuilder, flags *ScenarioBuilderFlags) error {
	// Resolve data file path (may be relative to project root or current directory)
	dataFilePath := flags.DataFile
	if !filepath.IsAbs(dataFilePath) {
		// Try to resolve relative to project root first
		projectRoot := builder.projectRoot
		if projectRoot != emptyValue {
			if safePath, sErr := safepath.JoinUnderRoot(projectRoot, dataFilePath); sErr == nil {
				if _, err := fileutil.Stat(safePath); err == nil {
					dataFilePath = safePath
				}
			}
		}
		// If still not found, try current working directory
		if _, err := fileutil.Stat(dataFilePath); fileutil.IsNotExist(err) {
			if wd, err := fileutil.Getwd(); err == nil {
				if safePath, sErr := safepath.JoinUnderRoot(wd, dataFilePath); sErr == nil {
					if _, err := fileutil.Stat(safePath); err == nil {
						dataFilePath = safePath
					}
				}
			}
		}
	}


	// Emit start event
	builder.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
		fmt.Sprintf("Loading data file: %s", dataFilePath),
		map[string]any{"file": dataFilePath})

	// Read the data file
	data, err := fileutil.ReadFile(dataFilePath)
	if err != nil {
		builder.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusError,
			fmt.Sprintf("Failed to read data file: %s", err),
			map[string]any{"file": dataFilePath, "error": err.Error()})
		return errfmt.Errorf("failed to read data file %s: %w", dataFilePath, err)
	}

	// Parse YAML array
	var objects []map[string]any
	if err := yaml.Unmarshal(data, &objects); err != nil {
		builder.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusError,
			fmt.Sprintf("Failed to parse YAML data file: %s", err),
			map[string]any{"file": dataFilePath, "error": err.Error()})
		return errfmt.Newf("failed to parse YAML data file").Wrap(err)
	}

	// Build dependency graph and group objects by layers (for parallel processing)
	objectLayers, err := builder.buildDependencyGraphAndSort(objects)
	if err != nil {
		builder.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusError,
			fmt.Sprintf("Failed to build dependency graph: %s", err),
			map[string]any{"file": dataFilePath, "error": err.Error()})
		return errfmt.Newf("failed to build dependency graph").Wrap(err)
	}

	// Flatten layers into a single list for backward compatibility
	// Also store layers separately for parallel processing
	sortedObjects := make([]map[string]any, 0, len(objects))
	for _, layer := range objectLayers {
		sortedObjects = append(sortedObjects, layer...)
	}
	builder.dataFileObjects = sortedObjects
	builder.dataFileObjectLayers = objectLayers
	// Store force flag
	builder.force = flags.Force

	// Emit success event via coordinator
	builder.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
		"Loaded objects from data file",
		map[string]any{
			"file":  dataFilePath,
			"count": len(objects),
			"force": flags.Force,
		})

	return nil
}
