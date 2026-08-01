package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewObjectPplanCurrentCommandBuilder creates a new object_pplan_current command
func NewObjectPplanCurrentCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("current")
	builder.WithShort("View backlog items for the current priority plan")
	help := clipkg.DynamicHelpBuilder("View backlog items for the current priority plan")
	help.WithDescriptionLines("View backlog items for the current priority plan.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("The current priority plan is determined by:")
	help.WithDescriptionLines("  1. If there's an in_progress plan, that's the current")
	help.WithDescriptionLines("  2. Otherwise, the active plan with the lowest active_order")
	help.WithDescriptionLines("  3. If no active_order, the most recent active plan by plan_date")
	help.AddExample("View backlog items for current plan", "%s pplan current")
	help.AddExample("View with filters", "%s pplan current --filter status!=complete")
	help.AddExample("View in JSON format", "%s pplan current --format json")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(0))
	builder.AddStringArrayFlag("filter", "", "Filter by field (format: field=value or field:value, can be used multiple times)")
	builder.AddStringFlag("sort-by", "", "", "Field to sort by")
	builder.AddBoolFlag("sort-asc", "", true, "Sort ascending (default: true)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
