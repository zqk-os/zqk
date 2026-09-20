package convergence

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/cmd/zqk/scheduler"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewConvergenceCmd creates top-level zqk convergence command with nest-* veneer subcommands
func NewConvergenceCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewConvergenceCommandBuilder(), &cobra.Command{
		Use:   "convergence",
		Short: "Convergence measurement & nest management",
		Long:  "Top-level convergence command suite for nested CVS management (nest-spawn, nest-link, nest-status).",
	})

	// Add nest-spawn subcommand
	nestSpawnCmd := &cobra.Command{
		Use:   "nest-spawn",
		Short: "Spawn nested child CVS under parent",
		RunE:  scheduler.RunConvergenceNestSpawn,
	}
	nestSpawnCmd.Flags().String("parent-session-id", "", "Parent CVS session ID")
	nestSpawnCmd.Flags().String("title", "", "Child session title")
	nestSpawnCmd.Flags().String("hypothesis", "", "Child session hypothesis")
	nestSpawnCmd.Flags().String("desired-end-state", "", "Child session desired end state")
	nestSpawnCmd.Flags().String("next-action", "", "Next action")
	nestSpawnCmd.Flags().String("current-phase", "", "Current phase")
	nestSpawnCmd.Flags().Int("max-depth", 0, "Maximum tree depth")
	cmd.AddCommand(nestSpawnCmd)

	// Add nest-link subcommand
	nestLinkCmd := &cobra.Command{
		Use:   "nest-link",
		Short: "Link existing child CVS under parent",
		RunE:  scheduler.RunConvergenceNestLink,
	}
	nestLinkCmd.Flags().String("parent-session-id", "", "Parent CVS session ID")
	nestLinkCmd.Flags().String("child-session-id", "", "Child CVS session ID")
	nestLinkCmd.Flags().String("coordinator-session-id", "", "Coordinator CVS session ID")
	nestLinkCmd.Flags().Int("max-depth", 0, "Maximum tree depth")
	cmd.AddCommand(nestLinkCmd)

	// Add nest-status subcommand
	nestStatusCmd := &cobra.Command{
		Use:   "nest-status",
		Short: "BFS nest tree status for a parent CVS",
		RunE:  scheduler.RunConvergenceNestStatus,
	}
	nestStatusCmd.Flags().String("parent-session-id", "", "Parent CVS session ID")
	nestStatusCmd.Flags().Int("max-depth", 0, "Maximum tree depth")
	cmd.AddCommand(nestStatusCmd)

	// Add record-cvs-orchestrate-run subcommand
	cmd.AddCommand(scheduler.NewRecordCvsOrchestrateRunCmd())

	// Add convergence measure / overseer subcommands
	schedConv := scheduler.NewSchedulerConvergenceCmd()
	for _, sub := range schedConv.Commands() {
		cmd.AddCommand(sub)
	}

	return cmd
}
