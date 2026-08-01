package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSchedulerSubmitCommandBuilder creates a new scheduler_submit command
func NewSchedulerSubmitCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for submit")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
