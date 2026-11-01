package mcp

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/zqk-os/zqk/pkg/authcred"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NotificationSender is an interface for sending notifications
// This allows role enforcement to send helpful messages without directly depending on Server
type NotificationSender interface {
	SendMessageToClient(message string, messageType string, priority string) error
	SendLogMessage(level LogLevel, message string, fields map[string]any) error
	SendCriticalError(err error, severity string, category string, message string, instructions []string, fields map[string]any, clientID string, suppressNotification bool) error
}

var (
	roleEnforcementsTotal atomic.Int64
	roleOverridesTotal    atomic.Int64
	roleErrorsTotal       atomic.Int64
)

// GetRoleEnforcementStats returns lifetime counters for role enforcements, role overrides, and enforcement errors.
func GetRoleEnforcementStats() (enforcements, overrides, errors int64) {
	return roleEnforcementsTotal.Load(), roleOverridesTotal.Load(), roleErrorsTotal.Load()
}

// enforceRoleEnforcement applies role enforcement rules from config
// This ensures AI agents can only use roles that are allowed/enforced
// If notificationSender is provided, it will send helpful notifications on account lookup failures
func enforceRoleEnforcement(clientInfo map[string]any, config *ServerConfig, projectRoot string, notificationSender NotificationSender) (map[string]any, error) {
	if config == nil || config.MCPServer.Security.EnforcedRole == emptyValue && len(config.MCPServer.Security.AllowedRoles) == 0 && !config.MCPServer.Security.EnforceAccountRoles {
		// No enforcement configured
		return clientInfo, nil
	}
	roleEnforcementsTotal.Add(1)

	// Extract account ID
	accountID, _ := clientInfo[clientInfoAccountID].(string)
	if accountID == emptyValue {
		if id, ok := clientInfo["accountId"].(string); ok {
			accountID = id
		} else if id, ok := clientInfo["user"].(string); ok {
			accountID = id
		}
	}

	// Extract current roles
	var currentRoles []string
	switch v := clientInfo[objects.FieldKeyRoles].(type) {
	case []any:
		for _, r := range v {
			role, ok := r.(string)
			if !ok {
				continue
			}
			currentRoles = append(currentRoles, role)
		}
	case []string:
		// Handle []string directly (from tests)
		currentRoles = v
	case string:
		currentRoles = strings.Split(v, ",")
		for i, r := range currentRoles {
			currentRoles[i] = strings.TrimSpace(r)
		}
	}

	// Rule 1: Enforced role (overrides everything)
	if config.MCPServer.Security.EnforcedRole != emptyValue {
		// Force all agents to use the enforced role
		// Convert to []any for consistency with clientInfo format
		clientInfo[objects.FieldKeyRoles] = []any{config.MCPServer.Security.EnforcedRole}
		roleOverridesTotal.Add(1)
		return clientInfo, nil
	}

	// Rule 2: Enforce account roles and permissions (if account_id provided and enforce_account_roles is true)
	// DYNAMIC REGISTRATION: Account files are checked FIRST, even if not in config registry
	// This enables observer agent to create account files dynamically, and they work immediately
	// without requiring config.yaml updates
	if config.MCPServer.Security.EnforceAccountRoles && accountID != emptyValue {
		accountRoles, accountPerms, err := getAccountRolesAndPermissions(accountID, projectRoot)
		if err == nil {
			// Account file exists - use it (even if not in config registry)
			// This enables dynamic registration: observer agent can create account files,
			// and new agents can use them immediately without config updates
			// Enforce that agent can only use roles assigned to the account
			// Filter current roles to only those in account's roles
			allowedRoles := make([]string, 0)
			for _, role := range currentRoles {
				for _, accountRole := range accountRoles {
					if role == accountRole {
						allowedRoles = append(allowedRoles, role)
						break
					}
				}
			}
			if len(allowedRoles) == 0 {
				// No matching roles - use account's roles
				// Convert to []any for consistency
				rolesAny := make([]any, len(accountRoles))
				for i, role := range accountRoles {
					rolesAny[i] = role
				}
				clientInfo[objects.FieldKeyRoles] = rolesAny
				allowedRoles = accountRoles
			} else {
				// Convert to []any for consistency
				rolesAny := make([]any, len(allowedRoles))
				for i, role := range allowedRoles {
					rolesAny[i] = role
				}
				clientInfo[objects.FieldKeyRoles] = rolesAny
			}

			// SECURITY MODEL: Permissions come from roles OR account (not both)
			// - If account has roles: derive permissions from roles (ignore account permissions)
			// - If account has no roles: use explicit account permissions (required)
			var finalPermissions []string
			if len(allowedRoles) > 0 {
				// Account has roles: derive permissions from roles
				rolePerms, err := getPermissionsFromRoles(allowedRoles, projectRoot)
				if err != nil {
					// Role lookup failed - this is an error condition
					roleErrorsTotal.Add(1)
					return nil, errfmt.Errorf("failed to load permissions for roles %v: %w", allowedRoles, err)
				}
				if len(rolePerms) > 0 {
					finalPermissions = rolePerms
				} else {
					// Role lookup succeeded but returned no permissions - this shouldn't happen if roles are properly defined
					// Fallback to account permissions if available, otherwise error
					if len(accountPerms) > 0 {
						finalPermissions = accountPerms
					} else {
						roleErrorsTotal.Add(1)
						return nil, errfmt.Errorf("account %s has roles %v but roles have no permissions defined, and account has no explicit permissions", accountID, allowedRoles)
					}
				}
			} else {
				// Account has no roles: must have explicit permissions
				if len(accountPerms) == 0 {
					roleErrorsTotal.Add(1)
					return nil, errfmt.Errorf("account %s has no roles and no explicit permissions - permissions are required when no roles are assigned", accountID)
				}
				finalPermissions = accountPerms
			}

			// Enforce final permissions (SECURITY: prevent agents from claiming permissions)
			// This should always be set at this point (we error above if not)
			permsAny := make([]any, len(finalPermissions))
			for i, perm := range finalPermissions {
				permsAny[i] = perm
			}
			clientInfo[objects.FieldKeyPermissions] = permsAny

			return clientInfo, nil
		}
		// TRACK: BLI-1790155068391099000-c86bd852 / CRIT-1790155068391100000-e5a685ec —
		// EnforceAccountRoles + account lookup miss must deny (fail closed), never fall
		// open with client-claimed or residual permissions.
		roleErrorsTotal.Add(1)
		displayAccountID := accountID
		if strings.HasPrefix(accountID, "account-") && !strings.HasPrefix(accountID, "account:") {
			username := strings.TrimPrefix(accountID, "account-")
			displayAccountID = fmt.Sprintf("account:%s", username)
		}
		var expectedFilename string
		if strings.HasPrefix(displayAccountID, "account:") {
			username := strings.TrimPrefix(displayAccountID, "account:")
			expectedFilename = fmt.Sprintf("account-%s.yaml", username)
		} else {
			expectedFilename = fmt.Sprintf("%s.yaml", displayAccountID)
		}
		accountYAMLPath := filepath.ToSlash(filepath.Join(paths.ProcessAccountsDir, expectedFilename))
		if notificationSender != nil {
			message := fmt.Sprintf(
				"Account lookup failed for '%s' (fail-closed). Expected account object at '%s'.\n\n"+
					"Create the account (CLI: `%s object create account` or MCP create tools), then re-initialize with account_id.\n"+
					"Do not continue with client-claimed permissions.",
				displayAccountID, accountYAMLPath,
				GetCommandPath("object create account"),
			)
			_ = notificationSender.SendMessageToClient(message, "account_lookup_failed", "high")                                                     //nolint:errcheck // Notification failures are non-critical
			_ = notificationSender.SendLogMessage(LogLevelWarn, fmt.Sprintf("Account lookup failed (fail-closed): %s", err.Error()), map[string]any{ //nolint:errcheck // Notification failures are non-critical
				clientInfoAccountID: displayAccountID,
				"expected_filename": expectedFilename,
				"error":             err.Error(),
			})
		}
		return nil, errfmt.Errorf("account lookup failed for %s (enforce_account_roles fail-closed): %w", displayAccountID, err)
	}

	// Rule 3: Allowed roles whitelist
	if len(config.MCPServer.Security.AllowedRoles) > 0 {
		// Filter roles to only those in allowed list
		allowedRoles := make([]string, 0)
		for _, role := range currentRoles {
			for _, allowedRole := range config.MCPServer.Security.AllowedRoles {
				if role == allowedRole {
					allowedRoles = append(allowedRoles, role)
					break
				}
			}
		}
		if len(allowedRoles) == 0 {
			// No allowed roles - return error
			roleErrorsTotal.Add(1)
			return nil, errfmt.Errorf("none of the provided roles are allowed. Allowed roles: %v", config.MCPServer.Security.AllowedRoles)
		}
		// Convert to []any for consistency with clientInfo format
		rolesAny := make([]any, len(allowedRoles))
		for i, role := range allowedRoles {
			rolesAny[i] = role
		}
		clientInfo[objects.FieldKeyRoles] = rolesAny
		return clientInfo, nil
	}

	return clientInfo, nil
}

// getAccountRolesAndPermissions loads an account object and returns its roles and permissions
// This is the secure way to get account permissions - they come from the account object, not from client claims
func getAccountRolesAndPermissions(accountID, projectRoot string) (roles, permissions []string, err error) {
	if projectRoot == emptyValue {
		return nil, nil, errfmt.Errorf("project root not available")
	}
	if canon := authcred.CanonicalAccountID(projectRoot, accountID); canon != "" {
		accountID = canon
	}
	acc, err := authcred.LookupBoundAccount(projectRoot, accountID)
	if err != nil {
		return nil, nil, errfmt.Errorf("account not found: %s", accountID)
	}
	return acc.Roles, acc.Permissions, nil
}

func getPermissionsFromRoles(roleIDs []string, projectRoot string) ([]string, error) {
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf("project root not available")
	}
	result := authcred.PermissionsForAssignedRoles(authcred.NewDiskSeatDirectory(projectRoot), roleIDs)
	if len(roleIDs) > 0 && len(result) == 0 {
		return nil, errfmt.Errorf("no permissions found for roles %v - roles may not exist or have no permissions defined", roleIDs)
	}
	return result, nil
}

func validateRolesAgainstSystem(roles []string, projectRoot string) error {
	if projectRoot == emptyValue {
		return nil
	}
	catalog := authcred.NewDiskSeatDirectory(projectRoot).Roles()
	if len(catalog) == 0 {
		return nil
	}
	for _, role := range roles {
		ok := false
		for _, rec := range catalog {
			if rec.Matches(role) {
				ok = true
				break
			}
		}
		if !ok {
			return errfmt.Errorf("role '%s' does not exist in the system", role)
		}
	}
	return nil
}
