package job

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/cmd/zqk/scheduler"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewJobCmd creates the Job Management parent command group.
func NewJobCmd() *cobra.Command {
	jobCmd := clipkg.NewCommandBuilder("job").
		WithShort("Background scheduler job triggers, queues, history, and status").
		WithLong("Inspect and manage background scheduler jobs, queues, execution history, activity, and configuration.").
		Build()

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
