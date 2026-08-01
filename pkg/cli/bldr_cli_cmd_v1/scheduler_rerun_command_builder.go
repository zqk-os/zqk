package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSchedulerRerunCommandBuilder creates a new scheduler_rerun command
func NewSchedulerRerunCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for rerun")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
