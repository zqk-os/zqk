package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemFederateInitiateCommandBuilder creates a new system_federate_initiate command
func NewSystemFederateInitiateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("initiate")
	builder.WithShort("initiate command")
	help := clipkg.DynamicHelpBuilder("initiate command")
	help.WithDescriptionLines("initiate command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
