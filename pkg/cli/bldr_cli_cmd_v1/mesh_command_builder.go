package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewMeshCommandBuilder creates a new mesh command
func NewMeshCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("mesh")
	builder.WithShort("mesh command")
	help := clipkg.DynamicHelpBuilder("mesh command")
	help.WithDescriptionLines("mesh command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
