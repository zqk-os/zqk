package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSchedulerEventsCommandBuilder creates a new scheduler_events command
func NewSchedulerEventsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for events")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
