package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSchedulerDumpCommandBuilder creates a new scheduler_dump command
func NewSchedulerDumpCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for dump")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
