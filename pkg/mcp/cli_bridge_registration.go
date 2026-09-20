package mcp

import (
	"strings"

	"github.com/spf13/cobra"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
)

// RegisterCLIToolsWithRootCommandAndConfig registers CLI commands as MCP tools with config security enforcement
func RegisterCLIToolsWithRootCommandAndConfig(server *Server, rootCmd *cobra.Command, secCtx *pkgctx.SecurityContext, projectRoot string, config *ServerConfig) error {
	if rootCmd == nil {
		return NewMissingParameterError("rootCmd", "RegisterCLIToolsWithRootCommandAndConfig")
	}

	// Discover and filter commands
	allCommands := DiscoverCLICommands(rootCmd)
	filteredCommands := FilterCommandsByPermissions(allCommands, secCtx)
	filteredCommands = filterCommandsByConfig(filteredCommands, config, secCtx)

	// Register executable commands as MCP tools
	registeredCount := registerExecutableCommands(server, filteredCommands)

	// Log registration details
	logCLIToolRegistration(server, secCtx, len(allCommands), len(filteredCommands), registeredCount)

	return nil
}

// registerExecutableCommands registers only executable (leaf) commands as MCP tools
func registerExecutableCommands(server *Server, commands []*DiscoveredCommand) int {
	registeredCount := 0
	for _, cmd := range commands {
		if isExecutableCommand(cmd) {
			tool := ConvertCommandToMCPTool(cmd)
			server.RegisterTool(tool.Name, tool.Description, tool.InputSchema, nil)
			registeredCount++
		}
	}
	return registeredCount
}

// isExecutableCommand checks if a command is executable (has RunE or Run handler)
func isExecutableCommand(cmd *DiscoveredCommand) bool {
	return cmd.Command != nil && (cmd.Command.RunE != nil || cmd.Command.Run != nil)
}

// logCLIToolRegistration logs CLI tool registration details to trace file
func logCLIToolRegistration(server *Server, secCtx *pkgctx.SecurityContext, discoveredCount, filteredCount, registeredCount int) {
	if server.getTraceWriter() == nil {
		return // No trace logging available
	}

	rolesStr, permsStr := formatSecurityContextForLogging(secCtx)
	logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
	logging.Fluent(logger).Debug("CLI tool registration").
		EmitComponent("mcp_cli_bridge").
		String("trace", "true").
		Int("discovered", discoveredCount).
		Int("filtered", filteredCount).
		Int("registered", registeredCount).
		String("roles", rolesStr).
		String("permissions", permsStr).
		Log()
}

// formatSecurityContextForLogging formats security context for logging
func formatSecurityContextForLogging(secCtx *pkgctx.SecurityContext) (rolesStr, permsStr string) {
	if secCtx == nil {
		return "none", "none"
	}

	if len(secCtx.Roles) > 0 {
		rolesStr = strings.Join(secCtx.Roles, ",")
	} else {
		rolesStr = "none"
	}

	if len(secCtx.Permissions) > 0 {
		permsStr = strings.Join(secCtx.Permissions, ",")
	} else {
		permsStr = "none"
	}

	return rolesStr, permsStr
}
