package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemGenerateConfigBuildersCommandBuilder creates a new system_generate_config_builders command
func NewSystemGenerateConfigBuildersCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for generate-config-builders")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
