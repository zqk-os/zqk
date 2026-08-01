package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewObjectSplanShowCommandBuilder creates a new object_splan_show command
func NewObjectSplanShowCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("show")
	builder.WithShort("Show a strategic plan by ID or the first plan")
	help := clipkg.DynamicHelpBuilder("Show a strategic plan by ID or the first plan")
	help.WithDescriptionLines("Display a strategic plan. If id is provided, show that plan; otherwise list")
	help.WithDescriptionLines("strategic_plan and show the first (e.g. STRAT-PLAN-001).")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Respects field-level read permissions (ITEM-642). Output format via --format.")
	help.AddExample("Show primary strategic plan", "%s object splan show")
	help.AddExample("Show plan by ID", "%s object splan show STRAT-PLAN-001")
	help.AddExample("Show in JSON format", "%s object splan show --format json")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.MaximumNArgs(1))
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
