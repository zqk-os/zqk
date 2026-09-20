package mcp

import (
	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// ListExposedTools returns the list of tools that would be exposed by the MCP server
// for the given init context, root command, config, and security context. Used for
// discovery (e.g. "zqk mcp list-tools") so users can see exact tool names for
// allowlist config. Does not start the server or register resources.
func ListExposedTools(
	initCtx *pkgctx.CliInitializationContext,
	rootCmd any,
	config *ServerConfig,
	secCtx *pkgctx.SecurityContext,
) []Tool {
	s := NewServer()
	s.SetCliInitializationContext(initCtx)
	s.SetRootCommand(rootCmd)
	s.SetConfig(config)
	s.SetSecurityContext(secCtx)
	s.registerToolsOnly(secCtx)
	return s.ListTools()
}
