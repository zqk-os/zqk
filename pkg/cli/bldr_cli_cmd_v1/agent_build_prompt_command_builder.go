package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewAgentBuildPromptCommandBuilder creates a new agent_build_prompt command
func NewAgentBuildPromptCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("build-prompt")
	builder.WithShort("Generate a fully policy-injected prompt for an agent task")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
