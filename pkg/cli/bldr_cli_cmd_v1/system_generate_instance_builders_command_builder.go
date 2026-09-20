package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemGenerateInstanceBuildersCommandBuilder creates a new system_generate_instance_builders command
func NewSystemGenerateInstanceBuildersCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for generate-instance-builders")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
