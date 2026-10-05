package system

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"

	"github.com/zqk-os/zqk/pkg/objects"
)

// NewSnapshotScenarioCmd creates the snapshot-scenario command.
// TODO(arch-cli-split): this scenario-focused command should be moved under a
// dedicated scenario CLI group (e.g. "zqk scenario snapshot") so that system
// maintenance commands and scenario/test workflows are clearly separated.
func NewSnapshotScenarioCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Capture system state snapshot for test scenario creation",
		"Capture a timestamp-based snapshot of system state for test scenario creation.",
		"",
		"The snapshot capture system:",
		"  - Records a precise timestamp when snapshot is initiated",
		"  - Buffers write operations during capture (non-blocking)",
		"  - Captures critical metadata atomically (object IDs, mtimes, hashes)",
		"  - Reconstructs state from change journal if objects are modified",
		"  - Applies scenario adaptors to transform baseline state",
	).
		AddExample("Capture specific objects to test scenario", "%s system snapshot-scenario --target test-scenarios/scheduler-optimization --object-ids SCH-001,SCH-002,SCH-003 --change-policy include").
		AddExample("Capture all scheduler jobs with adaptors", "%s system snapshot-scenario --target test-scenarios/scheduler-test --kinds scheduler_job --adaptors id-remap,field-override --id-prefix TEST- --field-override status=active").
		AddExample("Dry run to preview what would be captured", "%s system snapshot-scenario --object-ids SCH-001,SCH-002 --dry-run").
		AddExample("Capture with change journal reconstruction", "%s system snapshot-scenario --target test-scenarios/exact-state --object-ids SCH-001,SCH-002 --change-policy reconstruct").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemSnapshotScenarioCommandBuilder(), &cobra.Command{
		Use:  "snapshot-scenario [flags]",
		RunE: runSnapshotScenario,
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	// Add common flags
	cli.AddCommonFlags(cmd)

	// Snapshot capture flags
	cmd.Flags().String("target", "", "Target scenario directory (default: test-scenarios/snapshot-{timestamp})")
	cmd.Flags().StringSlice("object-ids", []string{}, "Object IDs to capture (comma-separated)")
	cmd.Flags().StringSlice("kinds", []string{}, "Object kinds to capture (comma-separated)")
	cmd.Flags().String("query-preset", "", "Query preset name from snapable trait (e.g., active_only, recent)")
	cmd.Flags().String("change-policy", "include", "Policy for changes: reject, include, reconstruct")

	// Scenario adaptor flags
	cmd.Flags().StringSlice("adaptors", []string{}, "Scenario adaptors: id-remap,field-override,pii-filter,namespace,state-preserve")
	cmd.Flags().String("id-prefix", "TEST-", "ID prefix override for adaptors (default: TEST-)")
	cmd.Flags().String(objects.KindNamespace, "test:scenario", "Namespace override for adaptors")
	cmd.Flags().StringToString("field-override", map[string]string{}, "Field overrides (key=value, comma-separated)")
	cmd.Flags().Bool("exclude-pii", true, "Exclude PII fields (default: true)")
	cmd.Flags().Bool("preserve-state", false, "Preserve object state (status, timestamps)")

	// Compression flags
	cmd.Flags().Bool("compress", false, "Store snapshot in compressed format (.csnap file)")
	cmd.Flags().String("compressed-output", "", "Path for compressed snapshot file (default: {target}/snapshot.csnap)")

	// Execution flags
	cmd.Flags().Bool("dry-run", false, "Show what would be captured without creating snapshot")
	// Note: --verbose is provided by cli.AddCommonFlags()

	return cmd
}

// runSnapshotScenario is implemented in snapshot_scenario_impl.go
