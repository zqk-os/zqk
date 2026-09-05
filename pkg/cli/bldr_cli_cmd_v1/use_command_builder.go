package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewUseCommandBuilder creates a new use command
func NewUseCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("use")
	builder.WithShort("use command")
	help := clipkg.DynamicHelpBuilder("use command")
	help.WithDescriptionLines("use command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
