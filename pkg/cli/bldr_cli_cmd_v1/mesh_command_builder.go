package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewMeshCommandBuilder creates a new mesh command
func NewMeshCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("mesh")
	builder.WithShort("Manage peer-to-peer agent mesh networking, routing, and discovery")
	help := clipkg.DynamicHelpBuilder("Manage peer-to-peer agent mesh networking, routing, and discovery")
	help.WithDescriptionLines("Controls the agent mesh overlay network, peer discovery, route propagation, and swarm communication.")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
