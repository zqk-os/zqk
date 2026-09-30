package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewAuthLogoutCommandBuilder creates a new auth_logout command
func NewAuthLogoutCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("logout")
	builder.WithShort("Log out and invalidate current authentication credentials")
	help := clipkg.DynamicHelpBuilder("Log out and invalidate current authentication credentials")
	help.WithDescriptionLines(
		"Clear stored session tokens, credentials, and cached authentication state",
		"for the current user or agent session.",
	)
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
