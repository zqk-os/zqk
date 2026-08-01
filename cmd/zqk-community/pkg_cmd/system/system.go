package system

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/spf13/cobra"
)

// NewSystemCmd creates a new system command group for ZQK Community Edition
func NewSystemCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"System operations (health, validation, and maintenance)",
		"System operations for health checks, validation, and maintenance.",
		"",
		"This command group provides operations for system-level management:",
		"  Common operations: check (object health and compliance), align (strategic alignment)",
		"  Specialized operations: init, status, validate, sync",
	).
		AddExample("Common operations", "%s system check").
		AddExample("Strategic alignment", "%s system align --gaps").
		AddExample("Specialized operations", "%s system init").
		AddExample("Show status", "%s system status").
		AddExample("Validate objects", "%s system validate").
		AddExample("Sync system", "%s system sync")

	systemCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemCommandBuilder(), &cobra.Command{
		Use: "system",
	})
	systemCmd.PersistentPreRunE = runSystemSchedulerGuard

	// Apply help builder to command
	helpBuilder.ApplyToCommand(systemCmd)
	systemCmd.PersistentFlags().Bool("allow-degraded", false, "Allow scheduler-dependent commands to run when scheduler daemon is not running")

	systemCmd.AddCommand(NewCheckCmd())
	systemCmd.AddCommand(NewMetricsCmd())
	systemCmd.AddCommand(NewRetentionToleranceCmd())
	systemCmd.AddCommand(NewCompactJournalCmd())
	systemCmd.AddCommand(NewCleanupDuplicatesCmd())

	systemCmd.AddCommand(NewSnapshotScenarioCmd())
	systemCmd.AddCommand(NewSnapshotExpandCmd())
	systemCmd.AddCommand(NewSnapshotVerifyCmd())
	systemCmd.AddCommand(NewSnapshotCmd())
	systemCmd.AddCommand(NewStateCommitCmd())
	systemCmd.AddCommand(NewStateDiffCmd())
	systemCmd.AddCommand(NewStateRestoreCmd())

	systemCmd.AddCommand(NewRecoverCASCmd())
	systemCmd.AddCommand(NewRepairCASCorruptionCmd())
	systemCmd.AddCommand(NewSyncCASIndexCmd())
	systemCmd.AddCommand(NewStreamGCCmd())

	// Specialized operations
	systemCmd.AddCommand(NewInitCmd())
	systemCmd.AddCommand(NewStartHereCmd())
	systemCmd.AddCommand(NewStatusCmd())
	systemCmd.AddCommand(NewValidateCmd())
	systemCmd.AddCommand(NewValidateScenarioCmd())
	systemCmd.AddCommand(NewSyncCmd())

	// Configuration
	systemCmd.AddCommand(NewConfigGetCmd())

	// Daemons

	return systemCmd
}
