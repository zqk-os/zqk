package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSchedulerCommandBuilder creates a new scheduler command
func NewSchedulerCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("scheduler")
	builder.WithShort("Manage background scheduler daemon, jobs, and recurrent tasks")
	help := clipkg.DynamicHelpBuilder("Manage background scheduler daemon, jobs, and recurrent tasks")
	help.WithDescriptionLines("Controls the background scheduler process, scheduled maintenance jobs, and automated task execution pipelines.")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
