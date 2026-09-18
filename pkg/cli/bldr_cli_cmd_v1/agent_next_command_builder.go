package bldr_cli_cmd_v1

import (
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewAgentNextCommandBuilder creates a new agent_next command
func NewAgentNextCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("next")
	builder.WithShort("Push an agent task to its next lifecycle state")
	help := clipkg.DynamicHelpBuilder("Push an agent task to its next lifecycle state")
	help.WithDescriptionLines("Simplifies the state machine transition for swarm workers. Evaluates the task and transitions to the next state (e.g., pending_verification) via auto-transitions. Can optionally register a callback to wake the agent if validation fails.")
	help.AddExample("Transition task to next state", "%s agent next ATK-123")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("on-validation-failure", "", "", "Action to take if validation fails (e.g., wake)")
	builder.WithCommonFlagsDefault(cli.AddCommonFlags)
	cmd := builder.Build()
	return cmd
}
