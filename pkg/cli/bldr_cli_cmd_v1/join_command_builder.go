package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewJoinCommandBuilder creates a new join command
func NewJoinCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("join")
	builder.WithShort("join command")
	help := clipkg.DynamicHelpBuilder("join command")
	help.WithDescriptionLines("join command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
