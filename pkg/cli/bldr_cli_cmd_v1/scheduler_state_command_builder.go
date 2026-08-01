package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSchedulerStateCommandBuilder creates a new scheduler_state command
func NewSchedulerStateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("state")
	builder.WithShort("Summarize scheduler state-file health")
	help := clipkg.DynamicHelpBuilder("Summarize scheduler state-file health")
	help.WithDescriptionLines("Summarize scheduler execution state files under .zqk/scheduler/state.")
	help.WithDescriptionLines("Shows counts by state and highlights stale in_progress/deferred entries.")
	help.AddExample("Show state summary with default stale threshold", "%s scheduler state")
	help.AddExample("Use a 2 hour stale threshold", "%s scheduler state --stale-after 2h")
	help.AddExample("Move legacy flat state YAML into per-job directories (one-time cleanup)", "%s scheduler state --migrate")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("stale-after", "", "30m", "Mark in_progress/deferred entries older than this as stale")
	builder.AddBoolFlag("migrate", "", false, "Move legacy flat *.yaml state files into per-job subdirectories under .zqk/scheduler/state/")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
