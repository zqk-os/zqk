package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
)

// NewEnsureCommandBuilder creates a new ensure command
func NewEnsureCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("ensure")
	builder.WithShort("Ensure the MCP daemon is running")
	help := clipkg.DynamicHelpBuilder("Ensure the MCP daemon is running")
	help.WithDescriptionLines("Starts the TCP MCP daemon if nothing is listening on --tcp.")
	help.WithDescriptionLines("Prefers ZQK_BIN, then workshop/repo stable (.zqk/bin/zqk-stable,")
	help.WithDescriptionLines("bin/zqk-stable), then tip bin/zqk, then the current executable.")
	help.WithDescriptionLines("Spawns via bin/<brand>-mcp-daemon so process lists show the daemon role.")
	help.WithDescriptionLines("Idempotent: reports the existing listener PID when already up.")
	help.AddExample("Ensure default daemon address", "%s mcp ensure")
	help.AddExample("Ensure a custom TCP address", "%s mcp ensure --tcp 127.0.0.1:9443")
	help.ExcludeFlag("columns")
	help.ExcludeFlag("ignore-scheduler-down")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.NoArgs)
	builder.AddStringFlag("tcp", "", "127.0.0.1:8443", "TCP target address for the MCP daemon")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"columns", "ignore-scheduler-down", "format", "output"})
	cmd := builder.Build()
	cli.RequireSession(cmd, false)
	return cmd
}
