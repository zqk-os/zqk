package swarm

import (
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/spf13/cobra"
)

// NewSwarmCmd creates the swarm command group.
// TRACK: BLI-1785886166768774000-c429c155
func NewSwarmCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSwarmCommandBuilder()
	cmd.AddCommand(NewStatusCmd())
	cmd.AddCommand(NewInitCmd())
	return cmd
}
