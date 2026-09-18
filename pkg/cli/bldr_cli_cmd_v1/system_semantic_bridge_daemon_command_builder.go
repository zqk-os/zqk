package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemSemanticBridgeDaemonCommandBuilder creates a new system_semantic_bridge_daemon command
func NewSystemSemanticBridgeDaemonCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for semantic-bridge-daemon")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
