package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSchedulerHistoryCommandBuilder creates a new scheduler_history command
func NewSchedulerHistoryCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for history")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
