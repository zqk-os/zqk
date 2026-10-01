package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemFederateStatusCommandBuilder creates a new system_federate_status command
func NewSystemFederateStatusCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("status")
	builder.WithShort("Show the status of the federated sovereign mesh")
	help := clipkg.DynamicHelpBuilder("Show the status of the federated sovereign mesh")
	help.WithDescriptionLines("Displays all established federated connections, their trust levels, ")
	help.WithDescriptionLines("and shared capabilities.")
	help.AddExample("Show federated mesh status", "%s system federate status")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlagsDefault(cli.AddCommonFlags)
	cmd := builder.Build()
	return cmd
}
