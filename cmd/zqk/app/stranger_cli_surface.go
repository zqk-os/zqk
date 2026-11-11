package app

import pkgmcp "github.com/zqk-os/zqk/pkg/mcp"

// shouldRegisterStudioPackCLI reports whether priority_plan shortcuts (pplan)
// and agent/CAP/MMORCH orchestrate discovery may register on the root command.
// Default stranger/community surface is impermeable; opt-in mirrors MCP.
// TRACK: BLI-1794392273289214000-f82837c4
func shouldRegisterStudioPackCLI() bool {
	return pkgmcp.IsStudioPackToolsEnabled(nil)
}
