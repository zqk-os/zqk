package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemMetricsSchedulerHealthCommandBuilder creates a new system_metrics_scheduler_health command
func NewSystemMetricsSchedulerHealthCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("scheduler-health")
	builder.WithShort("scheduler-health command")
	help := clipkg.DynamicHelpBuilder("scheduler-health command")
	help.WithDescriptionLines("scheduler-health command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
