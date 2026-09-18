package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemSchedulerHealthCommandBuilder creates a new system_scheduler_health command
func NewSystemSchedulerHealthCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for scheduler-health")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
