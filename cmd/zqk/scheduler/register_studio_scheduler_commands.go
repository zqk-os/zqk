package scheduler

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// registerStudioSchedulerCommands registers studio-only scheduler subcommands.
// In the open-core community candidate, this file is replaced with a no-op stub.
func registerStudioSchedulerCommands(cmd *cobra.Command) {
	if zqkenv.IsCommunityEdition {
		return
	}
	cmd.AddCommand(NewSubmitCmd())
	cmd.AddCommand(NewScanTestsCmd())
	cmd.AddCommand(NewPrintIDEPasteApplescriptCmd())
	cmd.AddCommand(NewSchedulerConvergenceCmd())
	cmd.AddCommand(NewTestFailuresCmd())
	cmd.AddCommand(NewIssuesBundleHealthCmd())
	cmd.AddCommand(NewRecordCvsOrchestrateRunCmd())
	cmd.AddCommand(NewSkipWindowCmd())
	cmd.AddCommand(NewSchedulerBundleProgressCmd())
	cmd.AddCommand(NewEscalateCmd())
}
