package test

import (
	bldr "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/spf13/cobra"
)

// NewTestCmd creates the top-level `test` command group.
func NewTestCmd() *cobra.Command {
	cmd := bldr.NewTestCommandBuilder()
	cmd.AddCommand(NewRunCmd())
	cmd.AddCommand(NewDiscoverCmd())
	cmd.AddCommand(NewDashboardCmd())
	cmd.AddCommand(NewBindCmd())
	cmd.AddCommand(NewCheckContaminationCmd())
	cmd.AddCommand(NewIdentifyParallelCmd())
	cmd.AddCommand(NewStreamCmd())
	return cmd
}
