package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewInternalCommandBuilder creates a new internal command
func NewInternalCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("internal")
	builder.WithShort("internal command")
	help := clipkg.DynamicHelpBuilder("internal command")
	help.WithDescriptionLines("internal command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
