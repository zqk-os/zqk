package mcp

import (
	"fmt"
	"slices"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// hasPermission checks if security context has permission for a command.
// This is called during command registration filtering (FilterCommandsByPermissions).
//
// Unannotated commands (no roles and no permissions) are denied unless the
// caller has an explicit allow: admin role or wildcard permission.
func hasPermission(cmd *DiscoveredCommand, secCtx *pkgctx.SecurityContext) bool {
	if secCtx == nil {
		return false
	}

	// Explicit allow: admin or wildcard bypasses annotation requirements.
	if hasAdminRole(secCtx) || hasWildcardPermission(secCtx) {
		return true
	}

	if len(cmd.Roles) == 0 && len(cmd.Permissions) == 0 {
		return false
	}

	// Check role requirements
	if len(cmd.Roles) > 0 && !hasRequiredRole(cmd.Roles, secCtx.Roles) {
		return false
	}

	// Check permission requirements
	if len(cmd.Permissions) > 0 && !hasRequiredPermission(cmd.Permissions, secCtx.Permissions) {
		return false
	}

	return true
}

// hasAdminRole checks if the security context has admin role
func hasAdminRole(secCtx *pkgctx.SecurityContext) bool {
	return slices.Contains(secCtx.Roles, "admin")
}

// hasWildcardPermission checks if the security context has wildcard permission
func hasWildcardPermission(secCtx *pkgctx.SecurityContext) bool {
	return slices.Contains(secCtx.Permissions, "*")
}

// hasRequiredRole checks if the user has at least one of the required roles
func hasRequiredRole(requiredRoles []string, userRoles []string) bool {
	for _, requiredRole := range requiredRoles {
		if slices.Contains(userRoles, requiredRole) {
			return true
		}
	}
	return false
}

// hasRequiredPermission checks if the user has at least one of the required permissions
// Supports exact matches and pattern matching (e.g., "read:*" matches "read:backlog_item")
func hasRequiredPermission(requiredPerms []string, userPerms []string) bool {
	for _, requiredPerm := range requiredPerms {
		if matchesAnyPermission(requiredPerm, userPerms) {
			return true
		}
	}
	return false
}

// matchesAnyPermission checks if a required permission matches any user permission
// Supports exact matches and wildcard pattern matching
func matchesAnyPermission(requiredPerm string, userPerms []string) bool {
	for _, userPerm := range userPerms {
		if userPerm == "*" {
			return true // Wildcard matches everything
		}
		if userPerm == requiredPerm {
			return true // Exact match
		}
		// Check pattern match (e.g., "read:*" matches "read:backlog_item")
		if matchesPermissionPattern(requiredPerm, userPerm) {
			return true
		}
	}
	return false
}

// matchesPermissionPattern checks if a permission matches a pattern
// Pattern format: "operation:kind" (e.g., "read:backlog_item")
// User permission can be "operation:*" to match all kinds for that operation
func matchesPermissionPattern(requiredPerm string, userPerm string) bool {
	if !strings.Contains(requiredPerm, ":") {
		return false // Not a pattern permission
	}

	parts := strings.Split(requiredPerm, ":")
	if len(parts) != 2 {
		return false // Invalid pattern format
	}

	op := parts[0]
	kind := parts[1]

	// User has "read:*" and command requires "read:backlog_item"
	if userPerm == fmt.Sprintf("%s:*", op) {
		return true
	}

	// User has "read:backlog_item" and command requires "read:backlog_item"
	if userPerm == fmt.Sprintf("%s:%s", op, kind) {
		return true
	}

	return false
}
