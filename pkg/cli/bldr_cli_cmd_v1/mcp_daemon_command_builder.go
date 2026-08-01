package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewMcpDaemonCommandBuilder creates a new mcp_daemon command
func NewMcpDaemonCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Start the background MCP daemon")
	builder.AddStringFlag("tcp", "", "127.0.0.1:8443", "TCP listen address for the MCP daemon (proxy default target)")
	builder.AddIntFlag("port", "", 0, "Optional TCP port shorthand (binds 127.0.0.1:<port>; overrides --tcp when > 0)")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
