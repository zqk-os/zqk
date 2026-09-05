package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemStatsCommandBuilder creates a new system_stats command
func NewSystemStatsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for stats")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
