package app

import pkgmcp "github.com/zqk-os/zqk/pkg/mcp"

// shouldRegisterStudioPackCLI reports whether priority_plan shortcuts (pplan)
// and agent/CAP/MMORCH orchestrate discovery may register on the root command.
// Default stranger/community surface is impermeable; opt-in mirrors MCP.
// TRACK: stranger-cli-surface (open-core)
func shouldRegisterStudioPackCLI() bool {
	return pkgmcp.IsStudioPackToolsEnabled(nil)
}
