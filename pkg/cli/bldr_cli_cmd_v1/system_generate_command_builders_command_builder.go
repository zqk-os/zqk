package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemGenerateCommandBuildersCommandBuilder creates a new system_generate_command_builders command
func NewSystemGenerateCommandBuildersCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for generate-command-builders")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
