package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewAgentSynthesizeSkillCommandBuilder creates a new agent_synthesize_skill command
func NewAgentSynthesizeSkillCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("synthesize-skill")
	builder.WithShort("synthesize-skill command")
	help := clipkg.DynamicHelpBuilder("synthesize-skill command")
	help.WithDescriptionLines("synthesize-skill command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
