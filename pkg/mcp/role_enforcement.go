package mcp

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
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
		// If account lookup fails, send helpful notification and continue with other enforcement rules
		// This allows the server to continue operating with fallback permissions instead of failing
		// NOTE: For dynamic registration, observer agent should create the account file first,
		// then the agent can re-initialize with the account_id to get proper permissions
		if notificationSender != nil {
			// Normalize account ID for display (handle both account:username and account-username formats)
			displayAccountID := accountID
			if strings.HasPrefix(accountID, "account-") && !strings.HasPrefix(accountID, "account:") {
				// Convert account-ide-seat-01 -> ACC-1785920548450214001-7b3cc2de for display
				username := strings.TrimPrefix(accountID, "account-")
				displayAccountID = fmt.Sprintf("account:%s", username)
			}

			// Determine expected filename
			var expectedFilename string
			if strings.HasPrefix(displayAccountID, "account:") {
				username := strings.TrimPrefix(displayAccountID, "account:")
				expectedFilename = fmt.Sprintf("account-%s.yaml", username)
			} else {
				expectedFilename = fmt.Sprintf("%s.yaml", displayAccountID)
			}

			// Send helpful notification with instructions
			// Guide users to check for MCP tools, use CLI, or escalate to administrator
			accountYAMLPath := filepath.ToSlash(filepath.Join(paths.ProcessAccountsDir, expectedFilename))
			message := fmt.Sprintf(
				"⚠️ Account lookup failed for '%s'. The account object was not found at '%s'.\n\n"+
					"**To resolve this issue, try the following options in order:**\n\n"+
					"**Option 1: Check if account creation is available via MCP tools**\n"+
					"1. Use `tools/list` to see all available tools\n"+
					"2. Look for account creation tools:\n"+
					"   - Interactive tool: `%s` (with kind='account')\n"+
					"   - Or CLI-based tools: check for tools matching `*object*create*account*` or `*account*create*`\n"+
					"3. If available, use the MCP tool to create the account object\n\n"+
					"**Option 2: Use CLI directly (if MCP tools are not available or not surfaced)**\n"+
					"1. If account creation tools are not available in MCP, use the CLI directly:\n"+
					"   - Run: `%s object create account` with appropriate fields (roles, permissions, etc.)\n"+
					"   - Or manually create the file: `%s` with proper YAML structure\n\n"+
					"**Option 3: Escalate to system administrator**\n"+
					"1. If you don't have permissions to create accounts, contact your system administrator\n"+
					"2. Request that they either:\n"+
					"   - Create the account file: `%s`\n"+
					"   - Or ensure account creation functionality is registered in MCP tools (if not already available)\n"+
					"   - Or update your `account_id` to reference an existing account\n\n"+
					"**Alternative: Provide authentication credentials**\n"+
					"- Provide keystore key, username/password, or other authentication credentials to resolve the account\n\n"+
					"**Note:** Once the account file exists, re-initialize the MCP connection with the `account_id` to get proper permissions.\n"+
					"The server will continue with fallback permissions until the account is resolved. Some operations may be limited.",
				displayAccountID, accountYAMLPath,
				GetToolName("create_object_interactive"),
				GetCommandPath("object create account"),
				accountYAMLPath,
				accountYAMLPath,
			)

			// Send as both a message notification and a log message
			_ = notificationSender.SendMessageToClient(message, "account_lookup_failed", "high")                                       //nolint:errcheck // Notification failures are non-critical
			_ = notificationSender.SendLogMessage(LogLevelWarn, fmt.Sprintf("Account lookup failed: %s", err.Error()), map[string]any{ //nolint:errcheck // Notification failures are non-critical
				clientInfoAccountID: displayAccountID,
				"expected_filename": expectedFilename,
				"error":             err.Error(),
			})
		}
		// Continue with other enforcement rules (don't fail initialization)
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

	// Determine account filename
	// Accounts use account-{username}.yaml format
	var filename string
	if strings.HasPrefix(accountID, "account:") {
		username := strings.TrimPrefix(accountID, "account:")
		filename = fmt.Sprintf("account-%s.yaml", username)
	} else {
		// Try to infer from ID
		filename = fmt.Sprintf("%s.yaml", accountID)
	}

	// Try accounts directory
	accountPath := filepath.Join(projectRoot, paths.ProcessAccountsDir, filename)
	if _, err := fileutil.Stat(accountPath); fileutil.IsNotExist(err) {
		// Account not found
		return nil, nil, errfmt.Errorf("account not found: %s", accountID)
	}

	// Read account file
	data, err := fileutil.ReadFile(accountPath)
	if err != nil {
		return nil, nil, errfmt.Newf("failed to read account file").Wrap(err)
	}

	// Parse YAML
	var account struct {
		Roles       []string `yaml:"roles"`
		Permissions []string `yaml:"permissions"`
	}
	if err := yaml.Unmarshal(data, &account); err != nil {
		return nil, nil, errfmt.Newf("failed to parse account file").Wrap(err)
	}

	return account.Roles, account.Permissions, nil
}

// getPermissionsFromRoles loads role objects and extracts their permissions
// Returns union of all permissions from all roles
func getPermissionsFromRoles(roleIDs []string, projectRoot string) ([]string, error) {
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf("project root not available")
	}

	rolesDir := filepath.Join(projectRoot, paths.ProcessRolesDir)
	if _, err := fileutil.Stat(rolesDir); fileutil.IsNotExist(err) {
		return nil, errfmt.Errorf("roles directory not found")
	}

	// Collect all permissions from all roles (union)
	allPermissions := make(map[string]bool)

	for _, roleID := range roleIDs {
		// Try to find role file (format: ROL-XXX.yaml or role-{roleID}.yaml)
		var rolePath string

		// Try ROL-XXX.yaml format first
		if strings.HasPrefix(roleID, "ROL-") {
			rolePath = filepath.Join(rolesDir, roleID+".yaml")
		} else {
			// Try role-{roleID}.yaml format
			rolePath = filepath.Join(rolesDir, fmt.Sprintf("role-%s.yaml", roleID))
		}

		// Check if file exists
		if _, err := fileutil.Stat(rolePath); fileutil.IsNotExist(err) {
			// Try to find by role_id field
			entries, err := fileutil.ReadDir(rolesDir)
			if err != nil {
				continue
			}

			found := false
			for _, entry := range entries {
				if !strings.HasSuffix(entry.Name(), ".yaml") {
					continue
				}

				checkPath := filepath.Join(rolesDir, entry.Name())
				data, err := fileutil.ReadFile(checkPath)
				if err != nil {
					continue
				}

				var role struct {
					RoleID string `yaml:"role_id"`
				}
				if err := yaml.Unmarshal(data, &role); err != nil {
					continue
				}

				if role.RoleID == roleID {
					rolePath = checkPath
					found = true
					break
				}
			}

			if !found {
				// Role not found - skip
				continue
			}
		}

		// Read and parse role file (may have been read already during search, but read again for permissions)
		data, err := fileutil.ReadFile(rolePath)
		if err != nil {
			continue
		}

		var role struct {
			Permissions []string `yaml:"permissions"`
		}
		if err := yaml.Unmarshal(data, &role); err != nil {
			// Log parsing error but continue to next role
			continue
		}

		// Add permissions to set (union)
		if len(role.Permissions) == 0 {
			// Role has no permissions defined - this is a problem
			continue
		}
		for _, perm := range role.Permissions {
			if perm != emptyValue {
				allPermissions[perm] = true
			}
		}
	}

	// Convert map to slice
	result := make([]string, 0, len(allPermissions))
	for perm := range allPermissions {
		result = append(result, perm)
	}

	// If we were given roles but found no permissions, that's an error
	if len(roleIDs) > 0 && len(result) == 0 {
		return nil, errfmt.Errorf("no permissions found for roles %v - roles may not exist or have no permissions defined", roleIDs)
	}

	return result, nil
}

// validateRolesAgainstSystem validates that roles exist in the system
// This checks against role objects in the system
func validateRolesAgainstSystem(roles []string, projectRoot string) error {
	if projectRoot == emptyValue {
		// Can't validate without project root
		return nil
	}

	rolesDir := filepath.Join(projectRoot, paths.ProcessRolesDir)
	if _, err := fileutil.Stat(rolesDir); fileutil.IsNotExist(err) {
		// Roles directory doesn't exist - skip validation
		return nil
	}

	// Load all role files
	entries, err := fileutil.ReadDir(rolesDir)
	if err != nil {
		// Can't read directory - skip validation
		return nil
	}

	// Build map of valid role IDs
	validRoles := make(map[string]bool)
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		rolePath := filepath.Join(rolesDir, entry.Name())
		data, err := fileutil.ReadFile(rolePath)
		if err != nil {
			continue
		}

		var role struct {
			RoleID string `yaml:"role_id"`
		}
		if err := yaml.Unmarshal(data, &role); err != nil {
			continue
		}

		if role.RoleID != emptyValue {
			validRoles[role.RoleID] = true
		}
	}

	// Validate provided roles
	for _, role := range roles {
		if !validRoles[role] {
			return errfmt.Errorf("role '%s' does not exist in the system", role)
		}
	}

	return nil
}
