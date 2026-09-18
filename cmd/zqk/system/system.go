package system

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewSystemCmd creates a new system command group
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
	systemCmd.AddCommand(NewCheckPolicyCmd())
	systemCmd.AddCommand(NewExportGateCmd())
	systemCmd.AddCommand(NewMetricsCmd())
	systemCmd.AddCommand(NewAuditReportCmd())
	systemCmd.AddCommand(NewImprovementReportCmd())
	systemCmd.AddCommand(NewEnsureRetentionJobsCmd())
	systemCmd.AddCommand(NewRetentionToleranceCmd())
	// Deprecated migration commands removed - use 'system migrate' with migration specs instead
	systemCmd.AddCommand(NewMigrateCasCmd())
	systemCmd.AddCommand(NewCompactJournalCmd())
	// - migrate-audit-buckets: Use 'system migrate' with migration spec
	// - migrate-lifecycles: Use 'system migrate lifecycle-files-to-objects.yaml'
	systemCmd.AddCommand(NewCleanupDuplicatesCmd())
	systemCmd.AddCommand(NewQuarantineReportCmd())
	systemCmd.AddCommand(NewCleanupQuarantineCmd())
	systemCmd.AddCommand(NewHealthDataCmd())

	systemCmd.AddCommand(NewDashboardCmd())
	systemCmd.AddCommand(NewSnapshotScenarioCmd())
	systemCmd.AddCommand(NewSnapshotExpandCmd())
	systemCmd.AddCommand(NewSnapshotVerifyCmd())
	systemCmd.AddCommand(NewSnapshotCmd())
	systemCmd.AddCommand(NewStateCommitCmd())
	systemCmd.AddCommand(NewStateDiffCmd())
	systemCmd.AddCommand(NewStateRestoreCmd())
	systemCmd.AddCommand(NewKernelIntegrityCmd())
	// Integrity readers were implemented and specced but never registered, so they
	// reported nothing and no gate noticed the silence.
	// TRACK: PRI-STABILIZE-FAILCLOSED-READS-001
	systemCmd.AddCommand(NewObjectCountReportCmd())
	systemCmd.AddCommand(NewObjectHygieneScanCmd())
	systemCmd.AddCommand(NewTruthSentinelCmd())
	// Developer and Codegen commands (only available in zqk-admin).
	// spec-origination / update-specs belong here: they write object specs and
	// require generate-* follow-up. They are not first-run product verbs.
	isAdminBinary := false
	if len(os.Args) > 0 && (strings.HasSuffix(os.Args[0], "zqk-admin") || strings.HasSuffix(os.Args[0], "zqk-admin.exe")) {
		isAdminBinary = true
	}

	if isAdminBinary {
		codegenCmds := []*cobra.Command{
			NewUpdateSpecsCmd(),
			NewSpecOriginationCmd(),
			NewCliHooksCmd(),
			NewValidateCommandSpecsCmd(),
			NewValidateAgentRulesCmd(),
			NewGenerateInstanceBuildersCmd(),
			NewGenerateAgentConfigsCmd(),
			NewGenerateLifecycleBuildersCmd(),
			NewGenerateProfileBuildersCmd(),
			NewGenerateConfigBuildersCmd(),
			NewGenerateAPIBuildersCmd(),
			NewGenerateCommandBuildersCmd(),
			NewGenerateSpecIndexCmd(),
			NewGenerateFieldKeysCmd(),
			NewGeneratePipelineOutcomeKeysCmd(),
			NewHydrateGraphCmd(),
			NewSyncGlossaryFromSpecsCmd(),
			NewSyncIDPrefixesFromSpecsCmd(),
			NewRepairYAMLCmd(),
		}
		for _, c := range codegenCmds {
			systemCmd.AddCommand(c)
		}
	}
	systemCmd.AddCommand(NewRecoverCASCmd())
	systemCmd.AddCommand(NewRepairCASCorruptionCmd())
	systemCmd.AddCommand(NewSyncCASIndexCmd())
	systemCmd.AddCommand(NewFederateCmd())
	systemCmd.AddCommand(NewPromptBuilderCmd())
	systemCmd.AddCommand(NewTelemetryCmd())
	systemCmd.AddCommand(NewCleanBranchesCmd())
	systemCmd.AddCommand(NewPolicyInterruptsCmd())
	systemCmd.AddCommand(NewStreamGCCmd())
	systemCmd.AddCommand(NewDiskUsageCmd())

	// Specialized operations
	systemCmd.AddCommand(NewInitCmd())
	systemCmd.AddCommand(NewStartHereCmd())
	systemCmd.AddCommand(NewQuickstartCmd())
	systemCmd.AddCommand(NewAgentOnboardCmd())
	systemCmd.AddCommand(NewSeedDefaultAgentSeatingCmd())
	systemCmd.AddCommand(NewRebuildGraphProjectionCmd())
	systemCmd.AddCommand(NewStatusCmd())
	systemCmd.AddCommand(NewShutdownCmd())
	systemCmd.AddCommand(NewStartCmd())
	systemCmd.AddCommand(NewWhoamiCmd())
	systemCmd.AddCommand(NewValidateCmd())
	systemCmd.AddCommand(NewAlignCmd())
	systemCmd.AddCommand(NewAuditMilestonesCmd())
	systemCmd.AddCommand(NewValidateScenarioCmd())
	systemCmd.AddCommand(NewSyncCmd())
	systemCmd.AddCommand(NewAuditStreamCmd())
	systemCmd.AddCommand(NewSyncGitHooksCmd())
	systemCmd.AddCommand(NewOrchestrateBatchCmd())
	systemCmd.AddCommand(NewSealSkillCmd())
	systemCmd.AddCommand(NewGitCmd())

	// Internal commands (hidden from help)
	systemCmd.AddCommand(NewAutoFixBatchCmd())
	systemCmd.AddCommand(NewAutoFixProcessPendingCmd())
	systemCmd.AddCommand(NewConfigGetCmd())

	// Daemons
	systemCmd.AddCommand(NewSemanticBridgeDaemonCmd())
	systemCmd.AddCommand(NewFSWatcherDaemonCmd())
	systemCmd.AddCommand(NewEmergencyManagerCmd())

	// Studio-only operations
	registerStudioCommands(systemCmd)

	return systemCmd
}
