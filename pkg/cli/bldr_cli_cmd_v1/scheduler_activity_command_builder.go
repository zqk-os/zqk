package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSchedulerActivityCommandBuilder creates a new scheduler_activity command
func NewSchedulerActivityCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for activity")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
