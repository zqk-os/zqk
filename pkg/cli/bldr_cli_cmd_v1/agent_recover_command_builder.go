package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewAgentRecoverCommandBuilder creates a new agent_recover command
func NewAgentRecoverCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("recover")
	builder.WithShort("Recover failed agent tasks")
	help := clipkg.DynamicHelpBuilder("Recover failed agent tasks")
	help.WithDescriptionLines("Queries the Knowledge Kernel for agent_task objects in error state, extracts verification feedback, injects it into the task context, and resets the task to planned status.")
	help.WithDescriptionLines("This allows the standard zqk agent orchestrate command to pick up and re-dispatch the tasks cleanly.")
	help.AddExample("Recover tasks for a plan", "%s agent recover <priority-plan-id>")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlagsDefault(cli.AddCommonFlags)
	cmd := builder.Build()
	return cmd
}
