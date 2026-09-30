package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemMetricsSchedulerHealthCommandBuilder creates a new system_metrics_scheduler_health command
func NewSystemMetricsSchedulerHealthCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("scheduler-health")
	builder.WithShort("Inspect scheduler daemon heartbeats, job queues, and executor health")
	help := clipkg.DynamicHelpBuilder("Inspect scheduler daemon heartbeats, job queues, and executor health")
	help.WithDescriptionLines("Reports health metrics, heartbeat intervals, active worker allocations, and job latency for the scheduler service.")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
