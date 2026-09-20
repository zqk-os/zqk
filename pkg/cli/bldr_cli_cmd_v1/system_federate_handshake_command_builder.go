package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemFederateHandshakeCommandBuilder creates a new system_federate_handshake command
func NewSystemFederateHandshakeCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("handshake")
	builder.WithShort("handshake command")
	help := clipkg.DynamicHelpBuilder("handshake command")
	help.WithDescriptionLines("handshake command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
