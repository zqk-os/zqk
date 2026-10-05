package utility

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// NewScenarioBuilderCmd creates the scenario builder command.
// TODO(arch-cli-split): when the scenario/bundle CLI is split out, this command
// should move under that dedicated entrypoint (for example "zqk scenario build")
// so bulk scenario/test data generation lives alongside other scenario/bundle
// commands instead of under the generic utility group.
func NewScenarioBuilderCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Build test scenarios with configurable data",
		"Build test scenarios with configurable object generation.",
		"",
		"The scenario builder uses a builder pattern to generate test data:",
		"  - Specify object kinds to generate",
		"  - Set counts per kind or use defaults",
		"  - Control diversity (field variation)",
		"  - Create relationships between objects",
		"  - Distribute objects across time ranges",
	).
		AddExample("Generate 100 backlog items with default diversity", "%s utility scenario-builder --target test-scenarios/my-test --kinds backlog_item --count backlog_item=100").
		AddExample("Generate diverse test data", "%s utility scenario-builder --target test-scenarios/my-test --kinds backlog_item goal milestone --count backlog_item=500,goal=50,milestone=20 --diversity backlog_item=8,goal=5,milestone=3").
		AddExample("Generate with relationships", "%s utility scenario-builder --target test-scenarios/my-test --kinds backlog_item goal --link-probability 0.5").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewUtilityScenarioBuilderCommandBuilder(), &cobra.Command{
		Use:  "scenario-builder [flags]",
		RunE: runScenarioBuilder,
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	// Add common flags
	cli.AddCommonFlags(cmd)

	// Scenario builder flags
	cmd.Flags().String("target", "", "Target scenario directory (required)")
	cmd.Flags().String("scenario-id", "", "Load configuration from scenario object ID (alternative to --kinds)")
	cmd.Flags().String("data-file", "", "Path to YAML file containing object data to load (alternative to --kinds)")
	cmd.Flags().StringSlice("kinds", []string{}, "Object kinds to generate (e.g., backlog_item,goal,milestone)")
	cmd.Flags().StringToString("count", map[string]string{}, "Count per kind (e.g., backlog_item=100,goal=50)")
	cmd.Flags().Int("default-count", 100, "Default count for kinds not specified in --count")
	cmd.Flags().StringToString("diversity", map[string]string{}, "Diversity level per kind 1-10 (e.g., backlog_item=8,goal=5)")
	cmd.Flags().Int("default-diversity", 5, "Default diversity level (1-10)")
	cmd.Flags().Float64("link-probability", 0.3, "Probability of creating links between objects (0.0-1.0)")
	cmd.Flags().Duration("time-range", 30*24*time.Hour, "Time range for created_at timestamps")
	cmd.Flags().Bool("dry-run", false, "Show what would be generated without creating objects")
	cmd.Flags().Bool("force", false, "Force overwrite existing objects (update instead of skip on duplicate)")

	// Object copying flags
	cmd.Flags().StringSlice("object-ids", []string{}, "Object IDs to copy from main project (alternative to --kinds)")
	cmd.Flags().String("source-project", "", "Source project root (defaults to main project root)")
	cmd.Flags().String("id-prefix-override", "TEST-", "ID prefix override for copied objects (e.g., TEST-)")
	cmd.Flags().String("namespace-override", "test:scenario", "Namespace override for copied objects")
	cmd.Flags().Bool("preserve-state", false, "Preserve object state (status, timestamps) when copying")
	cmd.Flags().StringToString("field-override", map[string]string{}, "Field value overrides (e.g., status=proposed)")
	cmd.Flags().String("change-policy", "include", "Policy for handling objects that change during snapshot: reject (skip), include (use current), reconstruct (from change journal)")

	return cmd
}

//nolint:gocyclo // Function orchestrates scenario building; complexity reduced via helper functions
func runScenarioBuilder(cmd *cobra.Command, args []string) error {
	// Parse flags
	flags, err := parseScenarioBuilderFlags(cmd)
	if err != nil {
		return err
	}

	// Resolve and validate target directory
	targetDir := resolveTargetDirectory(flags.TargetDir)
	flags.TargetDir = targetDir

	// Note: We no longer validate that target directory has .zqk directory
	// The scenario builder will create the necessary directory structure if needed

	// Create scenario builder using Processor pattern
	builder, err := createScenarioBuilder(cmd, targetDir)
	if err != nil {
		return err
	}

	// Handle copying objects from project
	if len(flags.ObjectIDs) > 0 {
		return handleCopyObjects(cmd, builder, flags)
	}

	// Handle loading data from file
	if flags.DataFile != emptyValue {
		// Emit start event (as progress - subscriber handles progress, warning, error, complete)
		builder.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
			"Loading scenario data from file",
			map[string]any{"file": flags.DataFile})

		if err := handleLoadDataFile(cmd, builder, flags); err != nil {
			// Emit error event before returning
			builder.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusError,
				fmt.Sprintf("Failed to load data file: %s", err),
				map[string]any{"file": flags.DataFile, "error": err.Error()})
			return err
		}
		// Verify objects were loaded (handleLoadDataFile should have emitted success event)
		if len(builder.dataFileObjects) == 0 {
			err := errfmt.Errorf("no objects loaded from data file: %s (file may be empty or invalid YAML)", flags.DataFile)
			builder.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusError,
				err.Error(),
				map[string]any{"file": flags.DataFile})
			return err
		}
		// Emit completion event
		builder.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusComplete,
			"Successfully loaded scenario data from file",
			map[string]any{"file": flags.DataFile, "count": len(builder.dataFileObjects)})
	} else if flags.ScenarioID != emptyValue {
		// Load from scenario object if specified
		if err := handleLoadFromScenario(builder, flags.ScenarioID, targetDir); err != nil {
			return err
		}
	} else {
		// Configure from command-line flags
		if err := configureBuilderFromFlags(builder, flags); err != nil {
			return err
		}
	}

	// Handle dry run
	if flags.DryRun {
		return handleDryRun(builder, flags.Kinds)
	}

	// Build scenario
	buildErr := builder.Build(pkgctx.NewSystemContext())

	// Allow async event processing to complete (events are emitted in goroutines)
	// This ensures progress/complete events are displayed before command exits
	time.Sleep(100 * time.Millisecond)

	// Flush stdout/stderr to ensure all output is visible (OS-level buffering)
	_ = os.Stdout.Sync() //nolint:errcheck // Best effort
	_ = os.Stderr.Sync() //nolint:errcheck // Best effort

	return buildErr
}
