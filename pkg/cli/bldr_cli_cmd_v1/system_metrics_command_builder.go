package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemMetricsCommandBuilder creates a new system_metrics command
func NewSystemMetricsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for metrics")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
