package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewAmbientWaveCommandBuilder creates a new ambient wave command
func NewAmbientWaveCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("wave")
	builder.WithShort("Run overnight metrics wave rollup")
	help := clipkg.DynamicHelpBuilder("Run overnight metrics wave rollup")
	help.WithDescriptionLines("Gathers command_metric, scheduler_health_metric, audit_aggregation_metric, and system-check GhostRef/error/warn rollups.")
	help.WithDescriptionLines("Lands the digests into agent_feed and ambient/whats-next with a ranked next_admin_action.")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(true, nil)
	return builder.Build()
}
