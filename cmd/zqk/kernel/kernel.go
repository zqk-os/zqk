package kernel

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewKernelCmd creates the root kernel command.
func NewKernelCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewKernelCommandBuilder()
	cmd.AddCommand(newStewardCmd())
	return cmd
}
