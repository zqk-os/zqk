package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewAuthCommandBuilder creates a new auth command
func NewAuthCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("auth")
	builder.WithShort("auth command")
	help := clipkg.DynamicHelpBuilder("auth command")
	help.WithDescriptionLines("auth command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
