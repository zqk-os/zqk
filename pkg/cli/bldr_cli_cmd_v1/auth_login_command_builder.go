package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewAuthLoginCommandBuilder creates a new auth_login command
func NewAuthLoginCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("login")
	builder.WithShort("Authenticate with remote kernel cluster or artifact registry")
	help := clipkg.DynamicHelpBuilder("Authenticate with remote kernel cluster or artifact registry")
	help.WithDescriptionLines("Logs in to a remote cluster endpoint or artifact registry and securely caches credentials in the local keystore.")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
