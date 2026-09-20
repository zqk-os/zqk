package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemFederateCommandBuilder creates a new system_federate command
func NewSystemFederateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("federate")
	builder.WithShort("federate command")
	help := clipkg.DynamicHelpBuilder("federate command")
	help.WithDescriptionLines("federate command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
