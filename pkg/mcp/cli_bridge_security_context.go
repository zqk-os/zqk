package mcp

import (
	"slices"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

// InitializeSecurityContextFromMCP initializes security context from MCP client info
// This allows MCP clients to provide their identity, roles, and permissions
func InitializeSecurityContextFromMCP(clientInfo map[string]any) *pkgctx.SecurityContext {
	// Extract account ID, roles, and permissions
	accountID := extractAccountID(clientInfo)
	roles := extractRolesFromClientInfo(clientInfo)
	permissions := extractPermissionsFromClientInfo(clientInfo)

	// Apply admin role auto-grant
	if slices.Contains(roles, "admin") && len(permissions) == 0 {
		permissions = []string{"read:*", "write:*", "delete:*"}
	}

	// Apply defaults based on what's provided
	roles, permissions = applySecurityContextDefaults(accountID, roles, permissions)

	return pkgctx.NewSecurityContext(accountID, roles, permissions)
}

// extractAccountID is a convenience wrapper around ExtractAccountID.
// Kept for backward compatibility with existing code.
func extractAccountID(clientInfo map[string]any) string {
	return ExtractAccountID(clientInfo)
}

// extractRolesFromClientInfo is a convenience wrapper around ExtractRolesFromClientInfo.
// Kept for backward compatibility with existing code.
func extractRolesFromClientInfo(clientInfo map[string]any) []string {
	return ExtractRolesFromClientInfo(clientInfo)
}

// extractPermissionsFromClientInfo is a convenience wrapper around ExtractPermissionsFromClientInfo.
// Kept for backward compatibility with existing code.
func extractPermissionsFromClientInfo(clientInfo map[string]any) []string {
	return ExtractPermissionsFromClientInfo(clientInfo)
}

// applySecurityContextDefaults applies default roles and permissions based on what's provided
// IMPORTANT: This function is ONLY called for MCP server connections
// All MCP connections are AI agents, not human clients
func applySecurityContextDefaults(accountID string, roles []string, permissions []string) ([]string, []string) {
	// Check if it's a system account
	if isSystemAccount(accountID) {
		return []string{"admin"}, []string{"read:*", "write:*", "delete:*"}
	}

	// MCP connections are always automated (not human)
	// Default to viewer (read-only) for unknown/automated accounts
	if len(roles) == 0 && len(permissions) == 0 {
		return []string{"viewer"}, []string{"read:*"}
	}

	// If roles provided but no permissions, default based on client type
	if len(roles) > 0 && len(permissions) == 0 && !slices.Contains(roles, "admin") {
		// Automated clients (including MCP) get read-only
		return roles, []string{"read:*"}
	}

	// If permissions provided but no roles, default based on client type
	if len(roles) == 0 && len(permissions) > 0 {
		// Automated clients (including MCP) default to viewer role
		return []string{"viewer"}, permissions
	}

	return roles, permissions
}

// isSystemAccount checks if an account ID represents a system account
func isSystemAccount(accountID string) bool {
	return accountID == pkgctx.SystemAccountID || strings.HasPrefix(accountID, "system:") || strings.HasPrefix(accountID, pkgctx.SystemAccountID)
}
