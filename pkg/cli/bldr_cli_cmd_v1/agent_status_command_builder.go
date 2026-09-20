package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewAgentStatusCommandBuilder creates a new agent_status command
func NewAgentStatusCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("status")
	builder.WithShort("status command")
	help := clipkg.DynamicHelpBuilder("status command")
	help.WithDescriptionLines("status command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
