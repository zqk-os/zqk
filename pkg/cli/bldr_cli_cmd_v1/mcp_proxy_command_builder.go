package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewMcpProxyCommandBuilder creates a new mcp_proxy command
func NewMcpProxyCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Start the MCP proxy daemon")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
