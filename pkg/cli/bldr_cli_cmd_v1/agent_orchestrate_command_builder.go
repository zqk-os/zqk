package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewAgentOrchestrateCommandBuilder creates a new agent_orchestrate command
func NewAgentOrchestrateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("orchestrate")
	builder.WithShort("Route tasks from a priority plan to specialized sub-agents")
	help := clipkg.DynamicHelpBuilder("Route tasks from a priority plan to specialized sub-agents")
	help.WithDescriptionLines("Reads an active priority plan and routes its backlog items to specialized sub-agents (Coder, Reviewer, Observer) based on semantic content and dependencies.")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.MaximumNArgs(1))
	builder.AddStringFlag("session-id", "", "", "Optional convergence session ID to isolate execution context")
	builder.AddStringFlag("ambient-context", "", "", "Optional path to a file containing ambient context (or raw JSON payload) collected from human interfaces (IDE, terminal)")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
