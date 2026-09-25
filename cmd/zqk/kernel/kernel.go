package kernel

import (
	"github.com/spf13/cobra"
)

// NewKernelCmd creates the root kernel command.
func NewKernelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kernel",
		Short: "Knowledge Kernel governance, steward, and runtime lifecycle",
	}
	cmd.AddCommand(newStewardCmd())
	return cmd
}
