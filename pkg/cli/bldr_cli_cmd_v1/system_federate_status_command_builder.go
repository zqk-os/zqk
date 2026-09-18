package bldr_cli_cmd_v1

import (
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
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
