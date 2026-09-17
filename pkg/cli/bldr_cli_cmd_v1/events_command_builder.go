package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewEventsCommandBuilder creates a new events command
func NewEventsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("events")
	builder.WithShort("Scheduler events (health view over metrics summary)")
	help := clipkg.DynamicHelpBuilder("Scheduler events (health view over metrics summary)")
	help.WithDescriptionLines("Commands for viewing scheduler events and health. The health subcommand shows")
	help.WithDescriptionLines("a health view over scheduler-metrics-summary (failures and slow jobs). The")
	help.WithDescriptionLines("summary is updated automatically by the SCH-evag timer job.")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
