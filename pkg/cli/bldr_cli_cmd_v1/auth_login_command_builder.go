package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewAuthLoginCommandBuilder creates a new auth_login command
func NewAuthLoginCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("login")
	builder.WithShort("login command")
	help := clipkg.DynamicHelpBuilder("login command")
	help.WithDescriptionLines("login command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
