package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewAgentBuildPromptCommandBuilder creates a new agent_build_prompt command
func NewAgentBuildPromptCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("build-prompt")
	builder.WithShort("Generate a fully policy-injected prompt for an agent task")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
