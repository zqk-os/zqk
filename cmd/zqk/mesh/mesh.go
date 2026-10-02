package mesh

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/federation"
)

// NewMeshCmd creates the mesh command group
func NewMeshCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewMeshCommandBuilder()

	cmd.AddCommand(NewMarketCmd())
	cmd.AddCommand(NewAdvertiseCmd())
	cmd.AddCommand(NewLeaseCmd())

	return cmd
}

func resolveLocalKernelID(projectRoot string) (string, error) {
	idManager := federation.NewIdentityManager(projectRoot)
	return idManager.GetKernelID()
}
