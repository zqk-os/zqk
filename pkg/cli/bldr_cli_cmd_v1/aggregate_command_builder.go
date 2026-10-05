package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
)

// NewAggregateCommandBuilder creates a new aggregate command
func NewAggregateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("aggregate")
	builder.WithShort("Aggregate diagnostics.jsonl into scheduler-metrics-summary.json")
	help := clipkg.DynamicHelpBuilder("Aggregate diagnostics.jsonl into scheduler-metrics-summary.json")
	help.WithDescriptionLines("Reads .zqk/scheduler/diagnostics.jsonl (JSONL), aggregates job execution events")
	help.WithDescriptionLines("(completed/failed, duration), and writes .zqk/scheduler/scheduler-metrics-summary.json.")
	help.WithDescriptionLines("Use this periodically (e.g. via a scheduler job) to keep a summary for performance")
	help.WithDescriptionLines("analysis and reporting. See docs/archive/system_health/SCHEDULER_EVENTS_AND_METRICS.md.")
	help.AddExample("Aggregate scheduler events into metrics summary", "%s scheduler events aggregate")
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
