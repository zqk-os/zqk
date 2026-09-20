package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewWorkflowTreeCommandBuilder creates a new workflow_tree command
func NewWorkflowTreeCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("tree")
	builder.WithShort("Show hierarchical view of a priority plan and its dependents")
	help := clipkg.DynamicHelpBuilder("Show hierarchical view of a priority plan and its dependents")
	help.WithDescriptionLines("Renders a tree-like hierarchical representation of a priority plan, its goals, requirements,")
	help.WithDescriptionLines("and backlog items, showing their titles and statuses.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("To prevent massive graphs, traversal is bounded by depth and summaries are rolled up.")
	help.AddExample("Show dependency tree for the active priority plan", "%s workflow tree")
	help.AddExample("Show dependency tree with recursive depth of 2", "%s workflow tree --depth 2")
	help.AddExample("Show dependency tree for a specific plan", "%s workflow tree --priority-plan [REDACTED-ID]")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(0))
	builder.AddStringFlag("priority-plan", "", "", "Priority plan ID (PRI-*). When empty, resolves the active or in_progress plan.")
	builder.AddIntFlag("depth", "", 3, "Maximum recursive depth to traverse dependencies.")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
