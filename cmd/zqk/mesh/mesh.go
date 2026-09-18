package mesh

import (
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/spf13/cobra"
)

// NewMeshCmd creates the mesh command group
func NewMeshCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewMeshCommandBuilder()

	cmd.AddCommand(NewMarketCmd())
	cmd.AddCommand(NewAdvertiseCmd())
	cmd.AddCommand(NewLeaseCmd())

	return cmd
}
