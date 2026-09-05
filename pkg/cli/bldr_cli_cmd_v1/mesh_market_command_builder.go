package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewMeshMarketCommandBuilder creates a new mesh_market command
func NewMeshMarketCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("market")
	builder.WithShort("market command")
	help := clipkg.DynamicHelpBuilder("market command")
	help.WithDescriptionLines("market command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
