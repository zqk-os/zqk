package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemTelemetryCommandBuilder creates a new system_telemetry command
func NewSystemTelemetryCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for telemetry")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
