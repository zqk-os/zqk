package mcp

import (
	"io"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/mcp"
)

const listToolsWithCommandFlag = "with-command"

func NewListToolsCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewMcpSvcListToolsCommandBuilder()
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		var flags clipkg.FlagBag
		withCommand := flags.Bool(cmd, listToolsWithCommandFlag)
		if err := flags.Err(); err != nil {
			return err
		}
		return runListTools(cmd, cli.CommandOutputWriter(cmd, nil), withCommand)
	})
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
