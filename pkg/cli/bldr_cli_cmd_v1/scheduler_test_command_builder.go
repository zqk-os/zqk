package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSchedulerTestCommandBuilder creates a new scheduler_test command
func NewSchedulerTestCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for test")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
