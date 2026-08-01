package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewObjectCountCommandBuilder creates a new object_count command
func NewObjectCountCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("count")
	builder.WithShort("Count objects by kind")
	help := clipkg.DynamicHelpBuilder("Count objects by kind")
	help.WithDescriptionLines("Count objects by kind with optional filtering.")
	help.WithDescriptionLines("This command reports the total system object count (all objects).")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("When no kind is specified, counts all object kinds.")
	help.AddExample("Count all backlog items", "%s count backlog_item")
	help.AddExample("Count with filtering", "%s count backlog_item --filter status=exploring")
	help.AddExample("Count all kinds", "%s count")
	help.AddExample("Count with grouping", "%s count backlog_item --group-by status")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.MaximumNArgs(1))
	builder.AddBoolFlag("allow-degraded", "", false, "Allow running when scheduler daemon is not running (results may be partial/stale)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
