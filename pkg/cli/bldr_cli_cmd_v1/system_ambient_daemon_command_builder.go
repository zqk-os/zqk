package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemAmbientDaemonCommandBuilder creates a new system_ambient_daemon command
func NewSystemAmbientDaemonCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for ambient-daemon")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
