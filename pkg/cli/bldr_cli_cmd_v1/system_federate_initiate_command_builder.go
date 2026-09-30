package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemFederateInitiateCommandBuilder creates a new system_federate_initiate command
func NewSystemFederateInitiateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("initiate")
	builder.WithShort("Initiate federation establishment with a remote peer system")
	help := clipkg.DynamicHelpBuilder("Initiate federation establishment with a remote peer system")
	help.WithDescriptionLines(
		"Start the outbound federation protocol sequence to discover, establish,",
		"and verify mutual trust and synchronization with a remote peer.",
	)
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
