package ops

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewOpsCmd creates a new ops command group
func NewOpsCmd() *cobra.Command {
	opsCmd := bldr_cli_cmd_v1.NewOpsCommandBuilder()
	opsCmd.AddCommand(NewLevelUpCmd())
	return opsCmd
}
