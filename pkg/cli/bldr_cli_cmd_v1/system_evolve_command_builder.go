package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemEvolveCommandBuilder creates a new system_evolve command
func NewSystemEvolveCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("evolve")
	builder.WithShort("evolve command")
	help := clipkg.DynamicHelpBuilder("evolve command")
	help.WithDescriptionLines("evolve command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
