package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewWorkflowWorkflowCommandBuilder creates a new workflow_workflow command
func NewWorkflowWorkflowCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("workflow")
	builder.WithShort("Workflow guidance commands")
	help := clipkg.DynamicHelpBuilder("Workflow guidance commands")
	help.WithDescriptionLines("Workflow commands for deterministic next-step recommendations.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Use 'workflow next' to get a single precedence-aware recommendation")
	help.WithDescriptionLines("from policy interrupts, active convergence sessions, and backlog priority routing.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Use 'workflow whats-next' for the composite JSON (priority plan + backlog counts + active/paused")
	help.WithDescriptionLines("CVS rows + compressed convergence measure) — preferred entry point for agent session start.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Test-bundle convergence measurement and CVS/agent handoff live under")
	help.WithDescriptionLines("`zqk scheduler convergence measure` (and `overseer` for coordinator trees).")
	help.AddExample("Get next recommended action", "%s workflow next")
	help.AddExample("Get machine-readable recommendation", "%s workflow next --format json")
	help.AddExample("Composite snapshot for agents (PRI + backlog + CVS + measure)", "%s workflow whats-next --format json")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
