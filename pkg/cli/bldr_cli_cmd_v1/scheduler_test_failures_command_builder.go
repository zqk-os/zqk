package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSchedulerTestFailuresCommandBuilder creates a new scheduler_test_failures command
func NewSchedulerTestFailuresCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for test-failures")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
