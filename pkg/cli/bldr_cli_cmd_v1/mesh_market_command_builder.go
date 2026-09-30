package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewMeshMarketCommandBuilder creates a new mesh_market command
func NewMeshMarketCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("market")
	builder.WithShort("Discover and browse available agent swarm capabilities in the mesh market")
	help := clipkg.DynamicHelpBuilder("Discover and browse available agent swarm capabilities in the mesh market")
	help.WithDescriptionLines("Lists, searches, and inspects agent services, capability advertisements, and capability leases across the swarm mesh.")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
