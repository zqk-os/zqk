package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemGenerateRoutingBuildersCommandBuilder creates a new system_generate_routing_builders command
func NewSystemGenerateRoutingBuildersCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for generate-routing-builders")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
