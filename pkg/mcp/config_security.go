package mcp

import (
	"fmt"
	"slices"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/mcp/mcp_helpers"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// isCommandAllowed checks if a command is allowed based on config security settings
// Admin users bypass the exposed_commands whitelist but still respect blocked_commands
func isCommandAllowed(commandPath string, config *ServerConfig, secCtx any) (bool, string) {
	return isCommandAllowedWithConfig(commandPath, config, secCtx)
}

// isCommandAllowedWithConfig checks if a command is allowed with config support for command group normalization
func isCommandAllowedWithConfig(commandPath string, config *ServerConfig, secCtx any) (bool, string) {
	if config == nil {
		// No config = allow all (backward compatibility)
		return true, ""
	}

	// Check blocked commands first (blacklist takes precedence - even for admin)
	for _, blocked := range config.MCPServer.BlockedCommands {
		if matchesCommandPatternWithConfig(commandPath, blocked, config) {
			return false, fmt.Sprintf("command '%s' is blocked by config (pattern: %s)", commandPath, blocked)
		}
	}

	// Check if user has admin role - if so, bypass exposed_commands whitelist
	hasAdminRole := false
	if secCtx != nil {
		if secCtxTyped, ok := secCtx.(*pkgctx.SecurityContext); ok {
			hasAdminRole = slices.Contains(secCtxTyped.Roles, "admin")
			// Also check for wildcard permission "*" which grants admin-like access
			if !hasAdminRole {
				hasAdminRole = slices.Contains(secCtxTyped.Permissions, "*")
			}
		}
	}

	// If exposed_commands is specified, it's a whitelist (unless user is admin)
	if len(config.MCPServer.ExposedCommands) > 0 && !hasAdminRole {
		allowed := false
		for _, exposed := range config.MCPServer.ExposedCommands {
			if matchesCommandPatternWithConfig(commandPath, exposed, config) {
				allowed = true
				break
			}
		}
		if !allowed {
			return false, fmt.Sprintf("command '%s' is not in exposed_commands list", commandPath)
		}
	}

	return true, ""
}

// isWriteOperationAllowed checks if a write operation is allowed
func isWriteOperationAllowed(commandPath string, config *ServerConfig, secCtx any) (bool, string) {
	return isWriteOperationAllowedWithConfig(commandPath, config, secCtx)
}

// isWriteOperationAllowedWithConfig checks if a write operation is allowed with config support
func isWriteOperationAllowedWithConfig(commandPath string, config *ServerConfig, secCtx any) (bool, string) {
	if config == nil {
		// No config = allow all (backward compatibility)
		return true, ""
	}

	// Check if this is a write operation
	isWriteOp := false
	for _, writeOp := range config.MCPServer.WriteOperations {
		if matchesCommandPatternWithConfig(commandPath, writeOp, config) {
			isWriteOp = true
			break
		}
	}

	// If it's a write operation and require_write_permission is true, check permissions
	if isWriteOp && config.MCPServer.Security.RequireWritePermission {
		// Check if security context has write permissions
		if secCtx == nil {
			return false, "write operation requires security context with write permissions"
		}
		// Check if secCtx is a SecurityContext and has write permissions
		if secCtxTyped, ok := secCtx.(*pkgctx.SecurityContext); ok {
			// Check for admin role (bypasses permission checks)
			hasWritePerm := slices.Contains(secCtxTyped.Roles, "admin")
			// Check for write permissions
			if !hasWritePerm {
				hasWritePerm = slices.Contains(secCtxTyped.Permissions, "*") ||
					slices.Contains(secCtxTyped.Permissions, "write:*") ||
					slices.ContainsFunc(secCtxTyped.Permissions, func(perm string) bool {
						return strings.HasPrefix(perm, "write:")
					})
			}
			if !hasWritePerm {
				return false, "write operation requires write permissions (write:* or write:<kind>)"
			}
		}
	}

	// If it's not in write_operations but looks like a write operation, check if it's allowed
	// Write operations typically include: create, update, delete, bulk_*
	if !isWriteOp && isLikelyWriteOperation(commandPath) {
		// Check if write operations are explicitly required
		if config.MCPServer.Security.RequireWritePermission {
			return false, fmt.Sprintf("write operation '%s' must be explicitly enabled in write_operations", commandPath)
		}
	}

	return true, ""
}

// matchesCommandPattern checks if a command path matches a pattern
// Supports wildcards: "system git *" matches "system git analyze", "system git status", etc.
// Also handles root command prefix (e.g., "<executable> object list" matches pattern "object list")
func matchesCommandPattern(commandPath, pattern string) bool {
	return matchesCommandPatternWithConfig(commandPath, pattern, nil)
}

// matchesCommandPatternWithConfig checks if a command path matches a pattern with config support
func matchesCommandPatternWithConfig(commandPath, pattern string, config *ServerConfig) bool {
	// Normalize both paths by removing first word (root command prefix)
	// This makes pattern matching independent of executable name
	normalizedPath := normalizeCommandPathWithConfig(commandPath, config)
	normalizedPattern := normalizeCommandPathWithConfig(pattern, config)

	// Exact match
	if normalizedPath == normalizedPattern {
		return true
	}

	// Wildcard pattern matching
	if strings.HasSuffix(normalizedPattern, " *") {
		prefix := strings.TrimSuffix(normalizedPattern, " *")
		return strings.HasPrefix(normalizedPath, prefix+" ")
	}

	// Check if pattern is a prefix (e.g., "object" matches "object list", "object get", etc.)
	// But only if it's followed by a space or is the exact match
	if strings.HasPrefix(normalizedPath, normalizedPattern) {
		// Check if it's followed by a space or is exact match
		if len(normalizedPath) == len(normalizedPattern) || normalizedPath[len(normalizedPattern)] == ' ' {
			return true
		}
	}

	return false
}

// normalizeCommandPath removes the root command prefix from a command path
// e.g., "zqk object list" -> "object list", "object list" -> "object list" (idempotent)
// This allows config patterns to omit the root command name
// Works by removing the first word only if there are multiple words
// Uses configurable command groups from ServerConfig if available, otherwise defaults
func normalizeCommandPath(path string) string {
	return normalizeCommandPathWithConfig(path, nil)
}

// normalizeCommandPathWithConfig removes the root command prefix from a command path
// Uses command groups from config if provided, otherwise uses defaults
func normalizeCommandPathWithConfig(path string, config *ServerConfig) string {
	path = strings.TrimSpace(path)
	parts := strings.Fields(path)
	if len(parts) <= 1 {
		return path
	}

	// Get command groups from config or use defaults
	commandGroups := getCommandGroups(config)
	firstWord := parts[0]
	for _, group := range commandGroups {
		if firstWord == group {
			// Already normalized, return as-is
			return path
		}
	}
	// Remove first word (root command) and return the rest
	return strings.Join(parts[1:], " ")
}

// getCommandGroups returns the list of command groups from config or defaults
func getCommandGroups(config *ServerConfig) []string {
	if config != nil && len(config.MCPServer.CLI.CommandGroups) > 0 {
		return config.MCPServer.CLI.CommandGroups
	}
	// Default command groups (backward compatibility)
	return []string{"object", "system", "reports", "graph", "keystore", "scheduler"}
}

// isLikelyWriteOperation checks if a command path looks like a write operation
func isLikelyWriteOperation(commandPath string) bool {
	writeKeywords := []string{
		"create", "update", "delete", "bulk_create", "bulk_update", "bulk_delete",
		"sync", "init", "register", "enable", "disable", "flush",
	}

	lowerPath := strings.ToLower(commandPath)
	for _, keyword := range writeKeywords {
		if strings.Contains(lowerPath, keyword) {
			return true
		}
	}
	return false
}

// filterCommandsByConfig filters commands based on config security settings
func filterCommandsByConfig(commands []*DiscoveredCommand, config *ServerConfig, secCtx any) []*DiscoveredCommand {
	if config == nil {
		// No config = allow all (backward compatibility)
		return commands
	}

	var filtered []*DiscoveredCommand

	// New logic for AliasMode: default to true to protect clients from huge tool catalogs (except in Community Edition)
	aliasMode := !zqkenv.IsCommunityEdition
	if config.MCPServer.Tools.AliasMode != nil {
		aliasMode = *config.MCPServer.Tools.AliasMode
	}

	allowlist := config.MCPServer.Tools.Allowlist

	// First, apply config filtering (blocked, exposed, write_operations)
	for _, cmd := range commands {
		// Check if command is allowed (passes config for command group normalization)
		allowed, _ := isCommandAllowedWithConfig(cmd.Path, config, secCtx)
		if !allowed {
			// Skip blocked commands
			continue
		}

		// Check if write operation is allowed
		writeAllowed, _ := isWriteOperationAllowedWithConfig(cmd.Path, config, secCtx)
		if !writeAllowed {
			// Skip write operations that aren't allowed
			continue
		}

		// Filter subcommands recursively (this part remains the same)
		filteredSubcommands := []*DiscoveredCommand{}
		for _, subCmd := range cmd.Subcommands {
			subAllowed, _ := isCommandAllowedWithConfig(subCmd.Path, config, secCtx)
			subWriteAllowed, _ := isWriteOperationAllowedWithConfig(subCmd.Path, config, secCtx)
			if subAllowed && subWriteAllowed {
				filteredSubcommands = append(filteredSubcommands, subCmd)
			}
		}
		cmd.Subcommands = filteredSubcommands

		filtered = append(filtered, cmd)
	}

	// Now, apply alias mode filtering if enabled and allowlist is provided
	if aliasMode && len(allowlist) > 0 {
		finalFiltered := make([]*DiscoveredCommand, 0)
		for _, cmd := range filtered { // 'filtered' is the result of previous filtering
			toolName := mcp_helpers.SanitizeToolName(cmd.Path) // Use the same sanitization as ConvertCommandToMCPTool
			if slices.Contains(allowlist, toolName) {
				finalFiltered = append(finalFiltered, cmd)
			}
		}
		return finalFiltered
	}

	return filtered
}
