package workflow

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewWorkflowCmd creates the workflow command group.
func NewWorkflowCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewWorkflowCommandBuilder()
	cmd.AddCommand(NewNextCmd())
	cmd.AddCommand(NewWhatsNextCmd())
	cmd.AddCommand(NewCheckCmd())
	cmd.AddCommand(NewVDSCmd())
	cmd.AddCommand(NewAddCmd())
	cmd.AddCommand(NewCoachCmd())
	cmd.AddCommand(NewGenTracePipelineCmd())
	cmd.AddCommand(NewLinkCmd())
	return cmd
}
