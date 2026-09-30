package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemFederateCommandBuilder creates a new system_federate command
func NewSystemFederateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("federate")
	builder.WithShort("Federate and synchronize kernel graph across distributed cluster nodes")
	help := clipkg.DynamicHelpBuilder("Federate and synchronize kernel graph across distributed cluster nodes")
	help.WithDescriptionLines("Provides cluster federation controls, peer synchronization, and distributed event coordination.")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
