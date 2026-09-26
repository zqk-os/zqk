package vendor

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/cmd/zqk/mcp"
	"github.com/zqk-os/zqk/cmd/zqk/scheduler"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewCursorCmd creates the cursor vendor command group
func NewCursorCmd() *cobra.Command {
	cursorCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewVendorCursorCursorCommandBuilder(), &cobra.Command{})

	cursorCmd.AddCommand(NewCursorAdapterCmd())
	cursorCmd.AddCommand(NewCursorPasteApplescriptCmd())
	return cursorCmd
}

// NewCursorAdapterCmd provides the Cursor-specific MCP adapter under vendor hierarchy
func NewCursorAdapterCmd() *cobra.Command {
	cmd := mcp.NewCursorAdapterCmd()
	cmd.Use = "adapter"
	cmd.Short = "Cursor-specific MCP adapter (stdio ↔ daemon)"
	cmd.Long = "Speaks a clean full MCP contract on stdio for Cursor while privately maintaining keepalive, reconnect, and event subscription against the TCP MCP daemon."
	return cmd
}

// NewCursorPasteApplescriptCmd provides the Cursor AppleScript paste automation utility
func NewCursorPasteApplescriptCmd() *cobra.Command {
	cmd := scheduler.NewPrintIDEPasteApplescriptCmd()
	cmd.Use = "paste-applescript"
	cmd.Aliases = []string{"paste-script"}
	cmd.Short = "Print AppleScript for Cursor chat paste automation"
	cmd.Long = "Prints the AppleScript used for Cursor IDE prompt paste automation. Respects ZQK_CURSOR_PASTE_PREFIX_STEPS."
	return cmd
}
