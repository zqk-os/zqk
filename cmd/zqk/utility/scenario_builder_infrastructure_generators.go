package utility

import (
	"fmt"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	specbuilders "github.com/zqk-os/zqk/pkg/specbuilder/builders"
	lifecyclebuilders "github.com/zqk-os/zqk/pkg/specbuilder/lifecycle_builders"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// generateSpecs generates spec YAML files from built-in builders
// Returns the number of specs generated and any error
func (sb *ScenarioBuilder) generateSpecs(specsDir string) (int, error) {
	sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
		fmt.Sprintf("Starting spec generation: %s", specsDir),
		map[string]any{"specs_dir": specsDir})
	if sb.logger != nil {
		logging.FluentEvent(sb.logger).Info("Generating spec files from built-in builders").
			String("target_dir", specsDir).
			Log()
	}

	specGenerator := specbuilders.NewSpecGenerator(specsDir)
	if err := specGenerator.EnsureOutputDir(); err != nil {
		sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusError,
			fmt.Sprintf("Error ensuring specs output directory: %v", err),
			map[string]any{"error": err.Error()})
		return 0, errfmt.Newf("failed to ensure specs output directory").Wrap(err)
	}

	// Verify directory was created
	if _, err := fileutil.Stat(specsDir); err != nil {
		sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusError,
			fmt.Sprintf("Specs directory does not exist after creation: %v", err),
			map[string]any{"specs_dir": specsDir, "error": err.Error()})
		return 0, errfmt.Newf("specs directory does not exist after creation").Wrap(err)
	}

	// Check if any spec builders are registered
	specRegistry := specbuilders.GetGlobalRegistry()
	ontologies := specRegistry.GetAllOntologies()
	if len(ontologies) == 0 {
		sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
			"No spec builders registered - spec files will not be generated",
			map[string]any{"hint": "bldr_v2 package may need to be imported"})
		if sb.logger != nil {
			logging.FluentEvent(sb.logger).Warn("No spec builders registered - spec files will not be generated").
				String("hint", "bldr_v2 package may need to be imported").
				Log()
		}
		return 0, nil // Early return if no builders
	}

	sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
		fmt.Sprintf("Generating specs from %d registered builders...", len(ontologies)),
		map[string]any{"builder_count": len(ontologies)})
	if sb.logger != nil {
		logging.FluentEvent(sb.logger).Debug("Found spec builders").
			Int("count", len(ontologies)).
			Log()
	}

	// Generate all specs from registered builders
	// Generate each spec individually to catch errors
	specGeneratedCount := 0
	errorCount := 0
	for i, ontology := range ontologies {
		if err := specGenerator.GenerateLatestSpec(ontology); err != nil {
			errorCount++
			// Emit warning for first few errors
			if errorCount <= 3 {
				sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
					fmt.Sprintf("Failed to generate spec %s: %v", ontology, err),
					map[string]any{objects.FieldKeyOntology: ontology, "error": err.Error()})
			}
			if sb.logger != nil {
				logging.FluentEvent(sb.logger).Warn("Failed to generate spec").
					String("ontology", ontology).
					WithError(err).
					Log()
			}
			continue
		}
		specGeneratedCount++
		// Progress update every 20 specs
		if (i+1)%20 == 0 {
			sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
				fmt.Sprintf("Progress: %d/%d specs generated", i+1, len(ontologies)),
				map[string]any{"generated": i + 1, "total": len(ontologies)})
		}
	}

	// Report results
	resultMsg := fmt.Sprintf("Generated %d spec files", specGeneratedCount)
	if errorCount > 0 {
		resultMsg = fmt.Sprintf("Generated %d spec files (%d failed)", specGeneratedCount, errorCount)
	}
	sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
		resultMsg,
		map[string]any{"generated": specGeneratedCount, objects.FieldKeyFailed: errorCount, "total": len(ontologies)})

	// Count generated spec files
	specEntries, err := fileutil.ReadDir(specsDir)
	if err == nil {
		count := 0
		for _, entry := range specEntries {
			if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".yaml") || strings.HasSuffix(entry.Name(), ".yml")) {
				count++
			}
		}
		sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
			fmt.Sprintf("Generated %d spec files", count),
			map[string]any{"spec_count": count})
	}

	return specGeneratedCount, nil
}

// generateLifecycles generates lifecycle YAML files from built-in builders
// Returns the number of lifecycles generated and any error
func (sb *ScenarioBuilder) generateLifecycles(lifecyclesDir string) (int, error) {
	sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
		"Generating lifecycle files from built-in builders",
		map[string]any{"target_dir": lifecyclesDir})

	generator := lifecyclebuilders.NewLifecycleGenerator(lifecyclesDir)
	if err := generator.EnsureOutputDir(); err != nil {
		return 0, errfmt.Newf("failed to ensure lifecycles output directory").Wrap(err)
	}

	// Check if any lifecycle builders are registered
	lifecycleRegistry := lifecyclebuilders.GetGlobalRegistry()
	objectTypes := lifecycleRegistry.GetAllObjectTypes()
	if len(objectTypes) == 0 {
		logging.FluentEvent(sb.logger).Warn("No lifecycle builders registered - lifecycle files will not be generated").
			String("hint", "bldr_lifecycle_v1 package may need to be imported").
			Log()
	} else {
		logging.FluentEvent(sb.logger).Debug("Found lifecycle builders").
			Int("count", len(objectTypes)).
			Log()
	}

	// Generate all lifecycles from registered builders
	// Generate each lifecycle individually to catch errors
	lifecycleGeneratedCount := 0
	for _, objectType := range objectTypes {
		if err := generator.GenerateLatestLifecycle(objectType); err != nil {
			logging.FluentEvent(sb.logger).Warn("Failed to generate lifecycle").
				String("object_type", objectType).
				WithError(err).
				Log()
			continue
		}
		lifecycleGeneratedCount++
	}

	// Count generated lifecycle files
	lifecycleEntries, err := fileutil.ReadDir(lifecyclesDir)
	if err == nil {
		count := 0
		for _, entry := range lifecycleEntries {
			if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".yaml") || strings.HasSuffix(entry.Name(), ".yml")) {
				count++
			}
		}
		sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
			fmt.Sprintf("Generated %d lifecycle files", count),
			map[string]any{"lifecycle_count": count})
	}

	return lifecycleGeneratedCount, nil
}
