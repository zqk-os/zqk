package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewMcpSvcIDEAdapterCommandBuilder creates a new mcp_svc_ide_adapter command
func NewMcpSvcIDEAdapterCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("ide-adapter")
	builder.WithShort("IDE-specific MCP adapter (stdio ↔ daemon)")
	help := clipkg.DynamicHelpBuilder("IDE-specific MCP adapter (stdio ↔ daemon)")
	help.WithDescriptionLines("Speaks a clean full MCP contract on stdio for IDE while privately")
	help.WithDescriptionLines("maintaining keepalive, reconnect, and event subscription against the TCP")
	help.WithDescriptionLines("MCP daemon. Other IDEs should use mcp daemon / mcp serve directly.")
	help.AddExample("IDE entrypoint (default daemon)", "%s mcp ide-adapter --tcp 127.0.0.1:8443")
	help.ExcludeFlag("columns")
	help.ExcludeFlag("ignore-scheduler-down")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.NoArgs)
	builder.AddStringFlag("tcp", "", "127.0.0.1:8443", "TCP address of the underlying MCP daemon")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"columns", "ignore-scheduler-down", "format", "output"})
	cmd := builder.Build()
	cli.RequireSession(cmd, false)
	return cmd
}
