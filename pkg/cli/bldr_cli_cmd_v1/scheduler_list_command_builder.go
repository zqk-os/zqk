package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSchedulerListCommandBuilder creates a new scheduler_list command
func NewSchedulerListCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for list")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
