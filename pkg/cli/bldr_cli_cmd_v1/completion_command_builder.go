package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewCompletionCommandBuilder creates a new completion command
func NewCompletionCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("completion")
	builder.WithShort("completion command")
	help := clipkg.DynamicHelpBuilder("completion command")
	help.WithDescriptionLines("completion command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
