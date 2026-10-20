package job

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/cmd/zqk/scheduler"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewJobCmd creates the Job Management parent command group.
func NewJobCmd() *cobra.Command {
	jobCmd := bldr_cli_cmd_v1.NewJobCommandBuilder()

	jobCmd.AddCommand(scheduler.NewListCmd())
	jobCmd.AddCommand(scheduler.NewTriggerCmd())
	jobCmd.AddCommand(scheduler.NewActivityCmd())
	jobCmd.AddCommand(scheduler.NewHistoryCmd())
	jobCmd.AddCommand(scheduler.NewStateCmd())
	jobCmd.AddCommand(scheduler.NewConfigCmd())
	jobCmd.AddCommand(scheduler.NewClearIssuesCmd())
	jobCmd.AddCommand(scheduler.NewHealthCheckCmd())
	jobCmd.AddCommand(scheduler.NewDumpCmd())

	return jobCmd
}
