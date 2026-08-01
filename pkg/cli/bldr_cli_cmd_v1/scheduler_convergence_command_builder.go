package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSchedulerConvergenceCommandBuilder creates a new scheduler_convergence command
func NewSchedulerConvergenceCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for convergence")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
