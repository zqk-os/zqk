package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSchedulerHealthCheckCommandBuilder creates a new scheduler_health_check command
func NewSchedulerHealthCheckCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for health-check")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
