package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSchedulerEventsHealthCommandBuilder creates a new scheduler_events_health command
func NewSchedulerEventsHealthCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("health")
	builder.WithShort("health command")
	help := clipkg.DynamicHelpBuilder("health command")
	help.WithDescriptionLines("health command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
