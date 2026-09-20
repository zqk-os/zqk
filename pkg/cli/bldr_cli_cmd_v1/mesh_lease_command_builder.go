package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewMeshLeaseCommandBuilder creates a new mesh_lease command
func NewMeshLeaseCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("lease")
	builder.WithShort("lease command")
	help := clipkg.DynamicHelpBuilder("lease command")
	help.WithDescriptionLines("lease command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
