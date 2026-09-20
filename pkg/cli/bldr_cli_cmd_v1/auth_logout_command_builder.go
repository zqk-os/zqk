package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewAuthLogoutCommandBuilder creates a new auth_logout command
func NewAuthLogoutCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("logout")
	builder.WithShort("logout command")
	help := clipkg.DynamicHelpBuilder("logout command")
	help.WithDescriptionLines("logout command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
