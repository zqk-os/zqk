package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemHealthDataCommandBuilder creates a new system_health_data command
func NewSystemHealthDataCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for health-data")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
