package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewMeshAdvertiseCommandBuilder creates a new mesh_advertise command
func NewMeshAdvertiseCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("advertise")
	builder.WithShort("advertise command")
	help := clipkg.DynamicHelpBuilder("advertise command")
	help.WithDescriptionLines("advertise command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
