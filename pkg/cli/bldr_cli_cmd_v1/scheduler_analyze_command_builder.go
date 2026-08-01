package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSchedulerAnalyzeCommandBuilder creates a new scheduler_analyze command
func NewSchedulerAnalyzeCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for analyze")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
