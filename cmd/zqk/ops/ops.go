package ops

import (
	"github.com/spf13/cobra"
)

// NewOpsCmd creates a new ops command group
func NewOpsCmd() *cobra.Command {
	opsCmd := &cobra.Command{
		Use:   "ops",
		Short: "Operations and evaluation commands",
		Long:  "Commands for operating the ZQK system and evaluating agent skills.",
	}

	opsCmd.AddCommand(NewLevelUpCmd())

	return opsCmd
}
