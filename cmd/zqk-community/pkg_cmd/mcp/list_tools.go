package mcp

import (
	"io"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/mcp"
	"github.com/spf13/cobra"
)

const listToolsWithCommandFlag = "with-command"

func NewListToolsCmd() *cobra.Command {
	var withCommand bool

	helpBuilder := clipkg.DynamicHelpBuilder(
		"List MCP tool names",
		"List the exact MCP tool names that would be exposed by the server (after allowlist). "+
			"Use these names in mcp_server.tools.allowlist in .zqk/mcp/config.yaml.",
	).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewMcpListToolsCommandBuilder(), &cobra.Command{
		Use: "list-tools",
	})
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		return runListTools(cmd, cli.CommandOutputWriter(cmd, nil), withCommand)
	})
	cmd.Flags().BoolVar(&withCommand, listToolsWithCommandFlag, false, "when set, print the CLI command path for each tool when available (e.g. object list)")

	helpBuilder.ApplyToCommand(cmd)
	return cmd
}

func runListTools(cmd *cobra.Command, out io.Writer, withCommand bool) error {
	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	rootCmd := cmd.Root()
	secCtx := pkgctx.NewSystemSecurityContext()
	config, _ := mcp.LoadMCPConfig(initCtx.GetProjectRoot()) //nolint:errcheck

	tools := mcp.ListExposedTools(initCtx, rootCmd, config, secCtx)
	for _, t := range tools {
		line := t.Name
		if withCommand {
			if path := commandPathFromTool(&t); path != emptyValue {
				line = t.Name + "  # " + path
			}
		}
		_, _ = io.WriteString(out, line+"\n")
	}
	return nil
}

// commandPathFromTool extracts the CLI command path from a tool's input schema if present
// (e.g. _command_path default "object list"). Returns empty string if not found.
func commandPathFromTool(t *mcp.Tool) string {
	if t.InputSchema == nil {
		return ""
	}
	schema, ok := t.InputSchema.(map[string]any)
	if !ok {
		return ""
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return ""
	}
	cp, ok := props["_command_path"].(map[string]any)
	if !ok {
		return ""
	}
	if def, ok := cp["default"].(string); ok && def != emptyValue {
		return mcp.NormalizeCommandPath(def)
	}
	if c, ok := cp["const"].(string); ok && c != emptyValue {
		return mcp.NormalizeCommandPath(c)
	}
	return ""
}
