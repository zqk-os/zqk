package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewWorkflowAddCommandBuilder creates a new workflow_add command
func NewWorkflowAddCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("add")
	builder.WithShort("Add backlog items to a priority plan")
	help := clipkg.DynamicHelpBuilder("Add backlog items to a priority plan")
	help.WithDescriptionLines("Add backlog items to a priority plan.")
	help.WithDescriptionLines("If the second argument is a plan ID (e.g. starting with PLAN-), it links to that plan.")
	help.WithDescriptionLines("If it is a number, it links to the active priority plan with that active order.")
	help.WithDescriptionLines("If no plan exists, the system automatically scaffolds a new priority plan, milestone, and goal.")
	help.AddExample("Add a backlog item to the default active priority plan", "%s workflow add ITEM-SYM-901")
	help.AddExample("Add a backlog item to the priority plan with active order 3", "%s workflow add ITEM-SYM-901 3")
	help.AddExample("Add multiple backlog items to an explicit plan ID", "%s workflow add ITEM-1 ITEM-2 PLAN-EXAMPLE")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.MinimumNArgs(1))
	builder.AddStringFlag("milestone", "", "", "Explicit milestone ID to link (optional)")
	builder.AddStringFlag("priority", "", "", "Priority level (critical, high, medium, low) (optional)")
	builder.AddBoolFlag("override", "", false, "Force manual status/lifecycle changes (triggers process debt audit)")
	builder.AddStringFlag("reason-code", "", "", "Reason code required if --override is used")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
