package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSchedulerHealthCommandBuilder creates a new scheduler_health command
func NewSchedulerHealthCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for health")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
