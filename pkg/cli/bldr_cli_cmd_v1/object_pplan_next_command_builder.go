package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewObjectPplanNextCommandBuilder creates a new object_pplan_next command
func NewObjectPplanNextCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("next")
	builder.WithShort("Navigate to the next priority plan")
	help := clipkg.DynamicHelpBuilder("Navigate to the next priority plan")
	help.WithDescriptionLines("Navigate to the next priority plan in the sequence.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("The sequence includes all active and in_progress priority plans, sorted by:")
	help.WithDescriptionLines("  1. in_progress plans first")
	help.WithDescriptionLines("  2. Then active plans by active_order (lower first)")
	help.WithDescriptionLines("  3. Then by plan_date (descending)")
	help.AddExample("View backlog items for next plan", "%s pplan next")
	help.AddExample("View in JSON format", "%s pplan next --format json")
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
