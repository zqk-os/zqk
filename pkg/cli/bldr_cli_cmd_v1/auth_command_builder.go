package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewAuthCommandBuilder creates a new auth command
func NewAuthCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("auth")
	builder.WithShort("Manage authentication credentials, tokens, and identity profiles")
	help := clipkg.DynamicHelpBuilder("Manage authentication credentials, tokens, and identity profiles")
	help.WithDescriptionLines("Configures user authentication tokens, active credentials, and security contexts for the knowledge kernel.")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
