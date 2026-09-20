package mcp

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

// ExtractAccountID extracts account ID from client info with fallback options.
// Tries multiple field names: "account_id", "accountId", "user".
// Returns empty string if not found (security: no default to system context).
func ExtractAccountID(clientInfo map[string]any) string {
	// Try primary field name
	if accountID, ok := clientInfo[clientInfoAccountID].(string); ok && accountID != emptyValue {
		return accountID
	}

	// Try alternative field names
	if id, ok := clientInfo["accountId"].(string); ok && id != emptyValue {
		return id
	}
	if id, ok := clientInfo["user"].(string); ok && id != emptyValue {
		return id
	}

	// SECURITY: Don't default to system context (admin) when no account_id provided
	// Instead, default to viewer (read-only) to prevent unauthorized admin access
	return ""
}

// ExtractRoles extracts roles from a map, handling both array and comma-separated string formats.
// Used for extracting roles from client info, account objects, etc.
// Returns empty slice if not found or invalid format.
func ExtractRoles(data map[string]any) []string {
	switch v := data[objects.FieldKeyRoles].(type) {
	case []any:
		roles := make([]string, 0, len(v))
		for _, r := range v {
			if role, ok := r.(string); ok {
				roles = append(roles, role)
			}
		}
		return roles
	case string:
		roles := strings.Split(v, ",")
		for i, r := range roles {
			roles[i] = strings.TrimSpace(r)
		}
		return roles
	default:
		return []string{}
	}
}

// ExtractPermissions extracts permissions from a map, handling both array and comma-separated string formats.
// Used for extracting permissions from client info, account objects, etc.
// Returns empty slice if not found or invalid format.
func ExtractPermissions(data map[string]any) []string {
	switch v := data[objects.FieldKeyPermissions].(type) {
	case []any:
		permissions := make([]string, 0, len(v))
		for _, p := range v {
			if perm, ok := p.(string); ok {
				permissions = append(permissions, perm)
			}
		}
		return permissions
	case string:
		permissions := strings.Split(v, ",")
		for i, p := range permissions {
			permissions[i] = strings.TrimSpace(p)
		}
		return permissions
	default:
		return []string{}
	}
}

// ExtractRolesFromClientInfo extracts roles from MCP client info.
// This is a convenience wrapper around ExtractRoles for client info specifically.
func ExtractRolesFromClientInfo(clientInfo map[string]any) []string {
	return ExtractRoles(clientInfo)
}

// ExtractPermissionsFromClientInfo extracts permissions from MCP client info.
// This is a convenience wrapper around ExtractPermissions for client info specifically.
func ExtractPermissionsFromClientInfo(clientInfo map[string]any) []string {
	return ExtractPermissions(clientInfo)
}

// ExtractRolesFromAccount extracts roles from account object.
// This is a convenience wrapper around ExtractRoles for account objects specifically.
func ExtractRolesFromAccount(account map[string]any) []string {
	return ExtractRoles(account)
}

// ExtractPermissionsFromAccount extracts permissions from account object.
// This is a convenience wrapper around ExtractPermissions for account objects specifically.
func ExtractPermissionsFromAccount(account map[string]any) []string {
	return ExtractPermissions(account)
}

// ExtractPrivilegeTags extracts privilege tags from an object's tags field.
// Privilege tags are tags starting with "access:" or "privilege:".
// Returns nil if tags field is missing or invalid.
func ExtractPrivilegeTags(obj map[string]any) []string {
	tags, ok := obj[objects.FieldKeyTags].([]any)
	if !ok {
		return nil
	}

	privilegeTags := make([]string, 0)
	for _, tag := range tags {
		tagStr, ok := tag.(string)
		if !ok {
			continue
		}

		// Check for access: or privilege: prefix
		if after, found := strings.CutPrefix(tagStr, "access:"); found {
			// Extract tag (e.g., "access:team-alpha" -> "team-alpha")
			privilegeTags = append(privilegeTags, after)
		} else if after, found := strings.CutPrefix(tagStr, "privilege:"); found {
			// Extract tag (e.g., "privilege:confidential" -> "confidential")
			privilegeTags = append(privilegeTags, after)
		}
	}

	return privilegeTags
}

// ExtractAccessPermissions extracts access permissions from a permissions slice.
// Access permissions are those starting with "access:".
func ExtractAccessPermissions(permissions []string) []string {
	accessPerms := make([]string, 0)
	for _, perm := range permissions {
		if strings.HasPrefix(perm, "access:") {
			accessPerms = append(accessPerms, perm)
		}
	}
	return accessPerms
}
