package utility

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

// handleCopyObjects handles copying objects from project
func handleCopyObjects(cmd *cobra.Command, builder *ScenarioBuilder, flags *ScenarioBuilderFlags) error {
	_ = cmd // Reserved for future use (e.g., flag parsing)
	sourceProject := flags.SourceProject
	if sourceProject == emptyValue {
		// Default to main project root
		sourceProject = cli.ResolveProjectRoot(".")
		if sourceProject == emptyValue {
			return errfmt.Errorf("could not find source project root - specify --source-project")
		}
	}

	builder.copyConfig = &ScenarioCopyConfig{
		SourceProjectRoot: sourceProject,
		TargetDir:         flags.TargetDir,
		ObjectIDs:         flags.ObjectIDs,
		IDPrefixOverride:  flags.IDPrefixOverride,
		NamespaceOverride: flags.NamespaceOverride,
		PIIFields:         make(map[string]bool),
		FieldOverrides:    flags.FieldOverrides,
		PreserveState:     flags.PreserveState,
		ChangePolicy:      flags.ChangePolicy,
		HashMappings:      make(map[string]string),
		SnapshotHashes:    make(map[string]string),
	}

	if flags.DryRun {
		builder.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
			fmt.Sprintf("Dry run: Would copy %d objects from %s to %s", len(flags.ObjectIDs), sourceProject, flags.TargetDir),
			map[string]any{
				objects.FieldKeyObjectCount: len(flags.ObjectIDs),
				"source_project":            sourceProject,
				"target_dir":                flags.TargetDir,
			})
		// Emit individual object IDs as separate progress events
		for _, id := range flags.ObjectIDs {
			builder.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
				fmt.Sprintf("  - %s", id),
				map[string]any{"object_id": id})
		}
		return nil
	}

	return builder.Build(pkgctx.NewSystemContext())
}

// handleLoadFromScenario handles loading from scenario object
func handleLoadFromScenario(builder *ScenarioBuilder, scenarioID, targetDir string) error {
	systemCtx := pkgctx.NewSystemContext()
	if err := builder.LoadFromScenarioObject(systemCtx, scenarioID); err != nil {
		// Try to find scenario by name if ID lookup fails
		scenarioName := filepath.Base(targetDir)
		if foundID, err := builder.findScenarioByName(systemCtx, scenarioName); err == nil {
			if err := builder.LoadFromScenarioObject(systemCtx, foundID); err != nil {
				return errfmt.Newf("failed to load from scenario object").Wrap(err)
			}
			builder.scenarioID = foundID
			builder.emitCoordinatorEvent(systemCtx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
				fmt.Sprintf("Loaded configuration from scenario object %s (found by name)", foundID),
				map[string]any{"scenario_id": foundID, "found_by": "name"})
		} else {
			return errfmt.Newf("failed to load from scenario object").Wrap(err)
		}
	} else {
		builder.scenarioID = scenarioID
		builder.emitCoordinatorEvent(systemCtx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
			fmt.Sprintf("Loaded configuration from scenario object %s", scenarioID),
			map[string]any{"scenario_id": scenarioID})
	}
	return nil
}

// configureBuilderFromFlags configures builder from command-line flags
func configureBuilderFromFlags(builder *ScenarioBuilder, flags *ScenarioBuilderFlags) error {
	if len(flags.Kinds) == 0 {
		return errfmt.Errorf("--kinds is required (e.g., --kinds backlog_item,goal) or use --scenario-id to load from scenario object, or --object-ids to copy objects from project")
	}

	builder = builder.
		WithKinds(flags.Kinds...).
		WithDefaultCount(flags.DefaultCount).
		WithDefaultDiversity(flags.DefaultDiversity).
		WithLinkProbability(flags.LinkProb).
		WithTimeRange(flags.TimeRange)

	// Set counts and diversity per kind
	for kind, count := range flags.Counts {
		builder = builder.WithCount(kind, count)
	}
	for kind, div := range flags.Diversity {
		builder = builder.WithDiversity(kind, div)
	}

	return nil
}

// handleDryRun handles dry run mode
func handleDryRun(builder *ScenarioBuilder, kinds []string) error {
	systemCtx := pkgctx.NewSystemContext()
	builder.emitCoordinatorEvent(systemCtx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
		"Dry run: Would generate:",
		nil)
	genKinds := builder.config.Kinds
	if len(genKinds) == 0 {
		genKinds = kinds
	}
	for _, kind := range genKinds {
		count := builder.config.Counts[kind]
		if count == 0 {
			count = builder.config.DefaultCount
		}
		div := builder.config.Diversity[kind]
		if div == 0 {
			div = builder.config.DefaultDiversity
		}
		builder.emitCoordinatorEvent(systemCtx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
			fmt.Sprintf("  - %s: %d objects (diversity: %d)", kind, count, div),
			map[string]any{
				objects.FieldKeyKind: kind,
				"count":              count,
				"diversity":          div,
			})
	}
	return nil
}
