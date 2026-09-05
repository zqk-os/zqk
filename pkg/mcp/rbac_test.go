package mcp

import (
	pkgcli "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// normalizeTestCommandPath normalizes a command path by removing the root command prefix
// This is used in tests where the executable name may differ from "zqk"
func normalizeTestCommandPath(path string) string {
	path = strings.TrimSpace(path)
	parts := strings.Fields(path)
	if len(parts) <= 1 {
		return path
	}
	// Remove first word (root command) and return the rest
	return strings.Join(parts[1:], " ")
}

// normalizeTestToolName normalizes a tool name by removing the root command prefix
// Tool names are like "zqk_object_list" or "mcp.test_object_list", we want "object_list"
func normalizeTestToolName(toolName string) string {
	toolName = strings.TrimSpace(toolName)
	// Split by underscore and remove first part (root command)
	parts := strings.Split(toolName, "_")
	if len(parts) <= 1 {
		return toolName
	}
	// Remove first part and rejoin
	return strings.Join(parts[1:], "_")
}

// TestRBAC_RoleBasedCommandFiltering tests comprehensive role-based command filtering
func TestRBAC_RoleBasedCommandFiltering(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI discovery test in short mode (can hang on parallel discovery)")
	}
	t.Parallel()
	rootCmd := createComprehensiveCommandTree()

	// Discover all commands
	allCommands := DiscoverCLICommands(rootCmd)

	tests := []struct {
		name         string
		secCtx       *pkgctx.SecurityContext
		shouldSee    []string // Commands that should be visible
		shouldNotSee []string // Commands that should NOT be visible
		description  string
	}{
		{
			name:         "admin_role_sees_all",
			secCtx:       pkgctx.NewSecurityContext("account:admin", []string{"admin"}, []string{}),
			shouldSee:    []string{"object list", "object create", "object delete", "system admin", "system status"},
			shouldNotSee: []string{},
			description:  "Admin role should see all commands regardless of permissions",
		},
		{
			name:         "developer_role_sees_development_commands",
			secCtx:       pkgctx.NewSecurityContext("account:dev", []string{"developer"}, []string{"read:*", "write:backlog_item"}),
			shouldSee:    []string{"object list", "object create", "object update"},
			shouldNotSee: []string{"object delete", "system admin"},
			description:  "Developer role should see development commands but not admin commands",
		},
		{
			name:         "viewer_role_read_only",
			secCtx:       pkgctx.NewSecurityContext("ACC-1785920548450214017-87f10a62", []string{"viewer"}, []string{"read:*"}),
			shouldSee:    []string{"object list", "object get", "system status"},
			shouldNotSee: []string{"object create", "object update", "object delete", "system admin"},
			description:  "Viewer role should only see read commands",
		},
		{
			name:         "executive_role_sees_reports",
			secCtx:       pkgctx.NewSecurityContext("account:exec", []string{"executive"}, []string{"read:*"}),
			shouldSee:    []string{"object list", "reports pcs", "reports edd"},
			shouldNotSee: []string{"object create", "object delete", "system admin"},
			description:  "Executive role should see reports but not write commands",
		},
		{
			name:         "no_permissions_sees_nothing",
			secCtx:       pkgctx.NewSecurityContext("account:guest", []string{"guest"}, []string{}),
			shouldSee:    []string{}, // Unannotated commands are fail-closed (BLI-CEF-R2-REL-MCP-PERMS-FAILOPEN)
			shouldNotSee: []string{"object create", "object delete", "object update", "system admin"},
			description:  "User with no permissions should see minimal commands",
		},
		{
			name:         "multiple_roles_union",
			secCtx:       pkgctx.NewSecurityContext("account:multi", []string{"developer", "executive"}, []string{"read:*", "write:backlog_item"}),
			shouldSee:    []string{"object list", "object create", "reports pcs"},
			shouldNotSee: []string{"system admin"},
			description:  "User with multiple roles should see union of allowed commands",
		},
		{
			name:         "system_context_sees_all",
			secCtx:       pkgctx.NewSystemSecurityContext(),
			shouldSee:    []string{"object list", "object create", "object delete", "system admin"},
			shouldNotSee: []string{},
			description:  "System context should see all commands",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filteredCommands := FilterCommandsByPermissions(allCommands, tt.secCtx)

			// Build map of visible commands for quick lookup
			// Normalize paths for comparison (discovered commands include root prefix)
			visibleCommands := make(map[string]bool)
			allCommandPaths := make(map[string]bool)
			for _, cmd := range allCommands {
				// Normalize by removing first word (root command name)
				normalized := normalizeTestCommandPath(cmd.Path)
				allCommandPaths[normalized] = true
				allCommandPaths[cmd.Path] = true // Also keep original for debugging
			}
			for _, cmd := range filteredCommands {
				// Normalize by removing first word (root command name)
				// This works regardless of what GetBrandPrefix() returns
				normalized := normalizeTestCommandPath(cmd.Path)
				visibleCommands[normalized] = true
				visibleCommands[cmd.Path] = true // Also keep original for debugging
			}

			// Debug: Print all discovered commands if test fails
			if len(tt.shouldSee) > 0 && !visibleCommands[tt.shouldSee[0]] {
				t.Logf("All discovered commands: %v", getCommandPaths(allCommands))
				t.Logf("Filtered commands: %v", getCommandPaths(filteredCommands))
			}

			// Verify should-see commands are visible
			// Expected paths are already normalized (e.g., "object list"), so check directly
			for _, expectedPath := range tt.shouldSee {
				if !visibleCommands[expectedPath] {
					t.Errorf("%s: Expected to see '%s'", tt.description, expectedPath)
				}
			}

			// Verify should-not-see commands are NOT visible
			// Expected paths are already normalized, so check directly
			for _, expectedPath := range tt.shouldNotSee {
				if visibleCommands[expectedPath] {
					t.Errorf("%s: Expected NOT to see '%s'", tt.description, expectedPath)
				}
			}
		})
	}
}

// Helper to get command paths for debugging
func getCommandPaths(commands []*DiscoveredCommand) []string {
	paths := make([]string, 0, len(commands))
	for _, cmd := range commands {
		paths = append(paths, cmd.Path)
	}
	return paths
}

// TestRBAC_PermissionBasedFiltering tests permission-based command filtering
func TestRBAC_PermissionBasedFiltering(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI discovery test in short mode (can hang on parallel discovery)")
	}
	t.Parallel()
	rootCmd := createComprehensiveCommandTree()
	allCommands := DiscoverCLICommands(rootCmd)

	tests := []struct {
		name         string
		secCtx       *pkgctx.SecurityContext
		shouldSee    []string
		shouldNotSee []string
		description  string
	}{
		{
			name:         "wildcard_read_sees_read_commands",
			secCtx:       pkgctx.NewSecurityContext("account:reader", []string{"viewer"}, []string{"read:*"}),
			shouldSee:    []string{"object list", "object get"},
			shouldNotSee: []string{"object create", "object delete"},
			description:  "Wildcard read permission should see all read commands",
		},
		{
			name:         "specific_write_permission",
			secCtx:       pkgctx.NewSecurityContext("account:writer", []string{"developer"}, []string{"read:*", "write:backlog_item"}),
			shouldSee:    []string{"object list", "object create"},
			shouldNotSee: []string{"object delete"},
			description:  "Specific write permission should see write commands for that kind",
		},
		{
			name:         "wildcard_write_sees_all_write",
			secCtx:       pkgctx.NewSecurityContext("account:writer", []string{"developer"}, []string{"read:*", "write:*"}),
			shouldSee:    []string{"object list", "object create", "object update"},
			shouldNotSee: []string{"object delete"},
			description:  "Wildcard write permission should see all write commands",
		},
		{
			name:         "delete_permission_required",
			secCtx:       pkgctx.NewSecurityContext("account:deleter", []string{"developer"}, []string{"read:*", "write:*", "delete:backlog_item"}),
			shouldSee:    []string{"object list", "object create", "object delete"},
			shouldNotSee: []string{},
			description:  "Delete permission required to see delete commands",
		},
		{
			name:         "no_permissions_no_write",
			secCtx:       pkgctx.NewSecurityContext("account:none", []string{"guest"}, []string{}),
			shouldSee:    []string{}, // Only commands without permission requirements
			shouldNotSee: []string{"object create", "object update", "object delete"},
			description:  "No permissions should not see write commands",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filteredCommands := FilterCommandsByPermissions(allCommands, tt.secCtx)

			// Normalize paths for comparison
			visibleCommands := make(map[string]bool)
			for _, cmd := range filteredCommands {
				normalized := normalizeTestCommandPath(cmd.Path)
				visibleCommands[normalized] = true
			}

			for _, expectedPath := range tt.shouldSee {
				if !visibleCommands[expectedPath] {
					t.Errorf("%s: Expected to see '%s'", tt.description, expectedPath)
				}
			}

			for _, expectedPath := range tt.shouldNotSee {
				if visibleCommands[expectedPath] {
					t.Errorf("%s: Expected NOT to see '%s'", tt.description, expectedPath)
				}
			}
		})
	}
}

// TestRBAC_MCPToolRegistration tests MCP tool registration based on RBAC
func TestRBAC_MCPToolRegistration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI discovery test in short mode (can hang on parallel discovery)")
	}
	t.Parallel()
	rootCmd := createComprehensiveCommandTree()

	tests := []struct {
		name          string
		secCtx        *pkgctx.SecurityContext
		shouldHave    []string // Tool names that should be registered
		shouldNotHave []string // Tool names that should NOT be registered
		description   string
	}{
		{
			name:          "admin_sees_all_tools",
			secCtx:        pkgctx.NewSecurityContext("account:admin", []string{"admin"}, []string{}),
			shouldHave:    []string{GetToolName("object_list"), GetToolName("object_create"), GetToolName("object_delete")},
			shouldNotHave: []string{},
			description:   "Admin should see all MCP tools",
		},
		{
			name:          "viewer_sees_read_tools_only",
			secCtx:        pkgctx.NewSecurityContext("ACC-1785920548450214017-87f10a62", []string{"viewer"}, []string{"read:*"}),
			shouldHave:    []string{GetToolName("object_list"), GetToolName("object_get")},
			shouldNotHave: []string{GetToolName("object_create"), GetToolName("object_delete")},
			description:   "Viewer should only see read tools",
		},
		{
			name:          "developer_sees_write_tools",
			secCtx:        pkgctx.NewSecurityContext("account:dev", []string{"developer"}, []string{"read:*", "write:backlog_item"}),
			shouldHave:    []string{GetToolName("object_list"), GetToolName("object_create")},
			shouldNotHave: []string{GetToolName("object_delete")},
			description:   "Developer should see write tools but not delete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := NewServer()
			err := RegisterCLIToolsWithRootCommand(server, rootCmd, tt.secCtx, "")
			if err != nil {
				t.Fatalf("Failed to register CLI tools: %v", err)
			}

			toolsList := server.ListTools()
			toolMap := make(map[string]bool)
			for _, tool := range toolsList {
				toolMap[tool.Name] = true
			}

			// Debug: show all registered tools if test fails
			if len(tt.shouldHave) > 0 && !toolMap[tt.shouldHave[0]] {
				allToolNames := make([]string, 0, len(toolsList))
				for _, tool := range toolsList {
					allToolNames = append(allToolNames, tool.Name)
				}
				t.Logf("All registered tools: %v", allToolNames)
			}

			for _, toolName := range tt.shouldHave {
				// Tool names from ConvertCommandToMCPTool use sanitizeToolName which includes root prefix
				// So "zqk object list" becomes "zqk_object_list" (or "mcp.test_object_list" in tests)
				// We need to check for the actual registered name, not the expected GetToolName result
				// The actual name is based on the command path which includes the root command
				found := false
				for registeredName := range toolMap {
					// Check if the registered tool matches (normalize by removing root prefix)
					normalizedRegistered := normalizeTestToolName(registeredName)
					normalizedExpected := normalizeTestToolName(toolName)
					if normalizedRegistered == normalizedExpected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("%s: Expected tool matching '%s' to be registered", tt.description, toolName)
				}
			}

			for _, toolName := range tt.shouldNotHave {
				// Check if any registered tool matches the expected pattern
				found := false
				for registeredName := range toolMap {
					normalizedRegistered := normalizeTestToolName(registeredName)
					normalizedExpected := normalizeTestToolName(toolName)
					if normalizedRegistered == normalizedExpected {
						found = true
						break
					}
				}
				if found {
					t.Errorf("%s: Expected tool matching '%s' NOT to be registered", tt.description, toolName)
				}
			}
		})
	}
}

// TestRBAC_SecurityContextInitialization tests security context initialization from MCP client info
func TestRBAC_SecurityContextInitialization(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI discovery test in short mode (can hang on parallel discovery)")
	}
	t.Parallel()
	tests := []struct {
		name          string
		clientInfo    map[string]any
		expectedID    string
		expectedRoles []string
		expectedPerms []string
		description   string
	}{
		{
			name: "full_client_info",
			clientInfo: map[string]any{
				clientInfoAccountID:         "ACC-1785920548450214003-23d25bd5",
				objects.FieldKeyRoles:       []any{"developer"},
				objects.FieldKeyPermissions: []any{"read:*", "write:backlog_item"},
			},
			expectedID:    "ACC-1785920548450214003-23d25bd5",
			expectedRoles: []string{"developer"},
			expectedPerms: []string{"read:*", "write:backlog_item"},
			description:   "Full client info should initialize security context correctly",
		},
		{
			name: "no_account_id_defaults_to_viewer",
			clientInfo: map[string]any{
				objects.FieldKeyRoles: []any{"viewer"},
			},
			expectedID:    "",
			expectedRoles: []string{"viewer"},
			expectedPerms: []string{"read:*"},
			description:   "No account ID should default to viewer (read-only) for security",
		},
		{
			name: "no_roles_defaults_to_viewer",
			clientInfo: map[string]any{
				clientInfoAccountID: "account:user",
			},
			expectedID:    "account:user",
			expectedRoles: []string{"viewer"},
			expectedPerms: []string{"read:*"},
			description:   "No roles should default to viewer with read-only",
		},
		{
			name: "admin_role_auto_grants_permissions",
			clientInfo: map[string]any{
				clientInfoAccountID:   "account:admin",
				objects.FieldKeyRoles: []any{"admin"},
			},
			expectedID:    "account:admin",
			expectedRoles: []string{"admin"},
			expectedPerms: []string{"read:*", "write:*", "delete:*"},
			description:   "Admin role should automatically grant all permissions",
		},
		{
			name: "comma_separated_roles",
			clientInfo: map[string]any{
				clientInfoAccountID:   "account:multi",
				objects.FieldKeyRoles: "developer,executive",
			},
			expectedID:    "account:multi",
			expectedRoles: []string{"developer", "executive"},
			expectedPerms: []string{"read:*"},
			description:   "Comma-separated roles should be parsed correctly",
		},
		{
			name: "comma_separated_permissions",
			clientInfo: map[string]any{
				clientInfoAccountID:         "account:user",
				objects.FieldKeyPermissions: "read:*,write:backlog_item",
			},
			expectedID:    "account:user",
			expectedRoles: []string{"viewer"},
			expectedPerms: []string{"read:*", "write:backlog_item"},
			description:   "Comma-separated permissions should be parsed correctly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secCtx := InitializeSecurityContextFromMCP(tt.clientInfo)

			if secCtx.AccountID != tt.expectedID {
				t.Errorf("%s: Expected AccountID '%s', got '%s'", tt.description, tt.expectedID, secCtx.AccountID)
			}

			if len(secCtx.Roles) != len(tt.expectedRoles) {
				t.Errorf("%s: Expected %d roles, got %d: %v", tt.description, len(tt.expectedRoles), len(secCtx.Roles), secCtx.Roles)
			} else {
				roleMap := make(map[string]bool)
				for _, r := range secCtx.Roles {
					roleMap[r] = true
				}
				for _, expectedRole := range tt.expectedRoles {
					if !roleMap[expectedRole] {
						t.Errorf("%s: Expected role '%s' not found in %v", tt.description, expectedRole, secCtx.Roles)
					}
				}
			}

			if len(secCtx.Permissions) != len(tt.expectedPerms) {
				t.Errorf("%s: Expected %d permissions, got %d: %v", tt.description, len(tt.expectedPerms), len(secCtx.Permissions), secCtx.Permissions)
			} else {
				permMap := make(map[string]bool)
				for _, p := range secCtx.Permissions {
					permMap[p] = true
				}
				for _, expectedPerm := range tt.expectedPerms {
					if !permMap[expectedPerm] {
						t.Errorf("%s: Expected permission '%s' not found in %v", tt.description, expectedPerm, secCtx.Permissions)
					}
				}
			}
		})
	}
}

// Helper function to create test ServerConfig
func createTestServerConfig(exposedCommands, blockedCommands, writeOperations []string, requireWritePermission bool) *ServerConfig {
	config := &ServerConfig{}
	config.MCPServer.ExposedCommands = exposedCommands
	config.MCPServer.BlockedCommands = blockedCommands
	config.MCPServer.WriteOperations = writeOperations
	config.MCPServer.Security.RequireWritePermission = requireWritePermission
	return config
}

// TestRBAC_ConfigBasedFiltering tests config-based command filtering
func TestRBAC_ConfigBasedFiltering(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI discovery test in short mode (can hang on parallel discovery)")
	}
	t.Parallel()
	rootCmd := createComprehensiveCommandTree()
	allCommands := DiscoverCLICommands(rootCmd)

	// Create security context with write permissions
	secCtx := pkgctx.NewSecurityContext("account:dev", []string{"developer"}, []string{"read:*", "write:backlog_item"})

	tests := []struct {
		name         string
		config       *ServerConfig
		shouldSee    []string
		shouldNotSee []string
		description  string
	}{
		{
			name:         "exposed_commands_whitelist",
			config:       createTestServerConfig([]string{"object list", "object get"}, []string{}, []string{}, false),
			shouldSee:    []string{"object list", "object get"},
			shouldNotSee: []string{"object create", "object delete"},
			description:  "Only exposed commands should be visible",
		},
		{
			name:         "blocked_commands_blacklist",
			config:       createTestServerConfig([]string{"object list", "object create", "object delete"}, []string{"object delete"}, []string{"object create"}, false),
			shouldSee:    []string{"object list", "object create"},
			shouldNotSee: []string{"object delete"},
			description:  "Blocked commands should not be visible even if exposed",
		},
		{
			name:         "write_operations_requires_permission",
			config:       createTestServerConfig([]string{"object list", "object create"}, []string{}, []string{"object create"}, true),
			shouldSee:    []string{"object list", "object create"},
			shouldNotSee: []string{},
			description:  "Write operations should be visible if user has write permission",
		},
		{
			name:         "write_operations_no_permission",
			config:       createTestServerConfig([]string{"object list", "object create"}, []string{}, []string{"object create"}, true),
			shouldSee:    []string{"object list"},
			shouldNotSee: []string{"object create"},
			description:  "Write operations should not be visible if user lacks write permission",
		},
		{
			name:         "write_operations_with_permission",
			config:       createTestServerConfig([]string{"object list", "object create"}, []string{}, []string{"object create"}, true),
			shouldSee:    []string{"object list", "object create"},
			shouldNotSee: []string{},
			description:  "Write operations should be visible if user has write permission",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use appropriate security context for the test
			testSecCtx := secCtx
			if tt.name == "write_operations_no_permission" {
				testSecCtx = pkgctx.NewSecurityContext("ACC-1785920548450214017-87f10a62", []string{"viewer"}, []string{"read:*"})
			}

			// Filter by permissions first
			filteredCommands := FilterCommandsByPermissions(allCommands, testSecCtx)

			// Then filter by config (pass secCtx as any for compatibility)
			var secCtxInterface any = testSecCtx
			configFiltered := filterCommandsByConfig(filteredCommands, tt.config, secCtxInterface)

			// Normalize paths for comparison
			visibleCommands := make(map[string]bool)
			for _, cmd := range configFiltered {
				normalized := normalizeTestCommandPath(cmd.Path)
				visibleCommands[normalized] = true
			}

			// Debug: show what happened if test fails
			if len(tt.shouldSee) > 0 && !visibleCommands[tt.shouldSee[0]] {
				permissionFilteredPaths := make([]string, 0, len(filteredCommands))
				for _, cmd := range filteredCommands {
					permissionFilteredPaths = append(permissionFilteredPaths, normalizeTestCommandPath(cmd.Path))
				}
				configFilteredPaths := make([]string, 0, len(configFiltered))
				for _, cmd := range configFiltered {
					configFilteredPaths = append(configFilteredPaths, normalizeTestCommandPath(cmd.Path))
				}
				t.Logf("After permission filter (%d): %v", len(filteredCommands), permissionFilteredPaths)
				t.Logf("After config filter (%d): %v", len(configFiltered), configFilteredPaths)
				t.Logf("Config exposed_commands: %v", tt.config.MCPServer.ExposedCommands)
			}

			for _, expectedPath := range tt.shouldSee {
				if !visibleCommands[expectedPath] {
					t.Errorf("%s: Expected to see '%s'", tt.description, expectedPath)
				}
			}

			for _, expectedPath := range tt.shouldNotSee {
				if visibleCommands[expectedPath] {
					t.Errorf("%s: Expected NOT to see '%s'", tt.description, expectedPath)
				}
			}
		})
	}
}

// TestRBAC_EdgeCases tests edge cases and boundary conditions
func TestRBAC_EdgeCases(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI discovery test in short mode (can hang on parallel discovery)")
	}
	t.Parallel()
	rootCmd := createComprehensiveCommandTree()
	allCommands := DiscoverCLICommands(rootCmd)

	tests := []struct {
		name        string
		secCtx      *pkgctx.SecurityContext
		description string
		validate    func(t *testing.T, filtered []*DiscoveredCommand)
	}{
		{
			name:        "empty_roles_empty_permissions",
			secCtx:      pkgctx.NewSecurityContext("account:empty", []string{}, []string{}),
			description: "Empty roles and permissions should see minimal commands",
			validate: func(t *testing.T, filtered []*DiscoveredCommand) {
				// Should not see any commands with permission requirements
				for _, cmd := range filtered {
					if len(cmd.Permissions) > 0 {
						t.Errorf("Command '%s' requires permissions but user has none", cmd.Path)
					}
				}
			},
		},
		{
			name:        "nil_security_context",
			secCtx:      nil,
			description: "Nil security context should not panic",
			validate: func(t *testing.T, filtered []*DiscoveredCommand) {
				// Should handle nil gracefully
				if filtered == nil {
					t.Error("Filtered commands should not be nil")
				}
			},
		},
		{
			name:        "multiple_roles_one_matches",
			secCtx:      pkgctx.NewSecurityContext("account:multi", []string{"viewer", "developer"}, []string{"read:*"}),
			description: "Multiple roles where one matches should allow access",
			validate: func(t *testing.T, filtered []*DiscoveredCommand) {
				// Should see commands allowed by any role
				found := false
				for _, cmd := range filtered {
					if normalizeTestCommandPath(cmd.Path) == "object list" {
						found = true
						break
					}
				}
				if !found {
					t.Error("Should see 'object list' with viewer role")
				}
			},
		},
		{
			name:        "wildcard_permission_matches_all",
			secCtx:      pkgctx.NewSecurityContext("account:wildcard", []string{"developer"}, []string{"*"}),
			description: "Wildcard permission '*' should match all operations",
			validate: func(t *testing.T, filtered []*DiscoveredCommand) {
				// Should see all commands (except parent commands without RunE)
				// Parent commands (like "object") are included but don't have RunE
				// So we check that we see at least the leaf commands
				leafCommands := 0
				for _, cmd := range allCommands {
					if cmd.Command != nil && (cmd.Command.RunE != nil || cmd.Command.Run != nil) {
						leafCommands++
					}
				}
				filteredLeafCommands := 0
				for _, cmd := range filtered {
					if cmd.Command != nil && (cmd.Command.RunE != nil || cmd.Command.Run != nil) {
						filteredLeafCommands++
					}
				}
				if filteredLeafCommands < leafCommands {
					t.Errorf("Wildcard permission should see all leaf commands, got %d out of %d", filteredLeafCommands, leafCommands)
				}
			},
		},
		{
			name:        "partial_wildcard_permission",
			secCtx:      pkgctx.NewSecurityContext("account:partial", []string{"developer"}, []string{"read:*", "write:*"}),
			description: "Partial wildcard permissions should match correctly",
			validate: func(t *testing.T, filtered []*DiscoveredCommand) {
				// Should see read and write commands but not delete
				hasList := false
				hasCreate := false
				hasDelete := false
				for _, cmd := range filtered {
					normalized := normalizeTestCommandPath(cmd.Path)
					if normalized == "object list" {
						hasList = true
					}
					if normalized == "object create" {
						hasCreate = true
					}
					if normalized == "object delete" {
						hasDelete = true
					}
				}
				if !hasList {
					t.Error("Should see 'object list' with read:*")
				}
				if !hasCreate {
					t.Error("Should see 'object create' with write:*")
				}
				if hasDelete {
					t.Error("Should NOT see 'object delete' without delete permission")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Handle nil security context
			var filtered []*DiscoveredCommand
			if tt.secCtx == nil {
				// hasPermission should handle nil gracefully
				filtered = []*DiscoveredCommand{}
				for _, cmd := range allCommands {
					if hasPermission(cmd, nil) {
						filtered = append(filtered, cmd)
					}
				}
			} else {
				filtered = FilterCommandsByPermissions(allCommands, tt.secCtx)
			}

			tt.validate(t, filtered)
		})
	}
}

// TestRBAC_CommandAnnotationParsing tests parsing of command annotations
func TestRBAC_CommandAnnotationParsing(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		annotations   map[string]string
		expectedPerms []string
		expectedRoles []string
		description   string
	}{
		{
			name: "single_permission",
			annotations: map[string]string{
				"mcp.permissions": "read:*",
			},
			expectedPerms: []string{"read:*"},
			expectedRoles: []string{},
			description:   "Single permission should be parsed correctly",
		},
		{
			name: "multiple_permissions",
			annotations: map[string]string{
				"mcp.permissions": "read:*,write:backlog_item",
			},
			expectedPerms: []string{"read:*", "write:backlog_item"},
			expectedRoles: []string{},
			description:   "Multiple permissions should be parsed correctly",
		},
		{
			name: "single_role",
			annotations: map[string]string{
				"mcp.roles": "admin",
			},
			expectedPerms: []string{},
			expectedRoles: []string{"admin"},
			description:   "Single role should be parsed correctly",
		},
		{
			name: "multiple_roles",
			annotations: map[string]string{
				"mcp.roles": "admin,developer",
			},
			expectedPerms: []string{},
			expectedRoles: []string{"admin", "developer"},
			description:   "Multiple roles should be parsed correctly",
		},
		{
			name: "both_permissions_and_roles",
			annotations: map[string]string{
				"mcp.permissions": "read:*",
				"mcp.roles":       "developer",
			},
			expectedPerms: []string{"read:*"},
			expectedRoles: []string{"developer"},
			description:   "Both permissions and roles should be parsed",
		},
		{
			name: "whitespace_trimmed",
			annotations: map[string]string{
				"mcp.permissions": "read:*, write:backlog_item ",
				"mcp.roles":       " admin , developer ",
			},
			expectedPerms: []string{"read:*", "write:backlog_item"},
			expectedRoles: []string{"admin", "developer"},
			description:   "Whitespace should be trimmed from permissions and roles",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := pkgcli.NewCommandBuilder("test").WithAnnotations(tt.annotations).Build()

			perms := extractPermissions(cmd)
			roles := extractRoles(cmd)

			if len(perms) != len(tt.expectedPerms) {
				t.Errorf("%s: Expected %d permissions, got %d: %v", tt.description, len(tt.expectedPerms), len(perms), perms)
			} else {
				for i, expected := range tt.expectedPerms {
					if perms[i] != expected {
						t.Errorf("%s: Expected permission[%d]='%s', got '%s'", tt.description, i, expected, perms[i])
					}
				}
			}

			if len(roles) != len(tt.expectedRoles) {
				t.Errorf("%s: Expected %d roles, got %d: %v", tt.description, len(tt.expectedRoles), len(roles), roles)
			} else {
				for i, expected := range tt.expectedRoles {
					if roles[i] != expected {
						t.Errorf("%s: Expected role[%d]='%s', got '%s'", tt.description, i, expected, roles[i])
					}
				}
			}
		})
	}
}

// Helper function to create a comprehensive command tree for testing
func createComprehensiveCommandTree() *cobra.Command {
	rootCmd := pkgcli.NewCommandBuilder("zqk").WithShort("ZQK CLI").Build()

	// Object commands
	objectCmd := pkgcli.NewCommandBuilder("object").WithShort("Object operations").Build()

	listCmd := pkgcli.NewCommandBuilder("list").WithShort("List objects").WithAnnotations(map[string]string{
		"mcp.permissions": "read:*",
	}).Build()

	getCmd := pkgcli.NewCommandBuilder("get").WithShort("Get object").WithAnnotations(map[string]string{
		"mcp.permissions": "read:*",
	}).Build()

	createCmd := pkgcli.NewCommandBuilder("create").WithShort("Create object").WithAnnotations(map[string]string{
		"mcp.permissions": "write:backlog_item",
	}).Build()

	updateCmd := pkgcli.NewCommandBuilder("update").WithShort("Update object").WithAnnotations(map[string]string{
		"mcp.permissions": "write:backlog_item",
	}).Build()

	deleteCmd := pkgcli.NewCommandBuilder("delete").WithShort("Delete object").WithAnnotations(map[string]string{
		"mcp.permissions": "delete:backlog_item",
	}).Build()

	objectCmd.AddCommand(listCmd, getCmd, createCmd, updateCmd, deleteCmd)

	// System commands
	systemCmd := pkgcli.NewCommandBuilder("system").WithShort("System operations").Build()

	statusCmd := pkgcli.NewCommandBuilder("status").WithShort("System status").WithAnnotations(map[string]string{
		"mcp.permissions": "read:*",
	}).Build()

	adminCmd := pkgcli.NewCommandBuilder("admin").WithShort("Admin operations").WithAnnotations(map[string]string{
		"mcp.roles": "admin",
	}).Build()

	systemCmd.AddCommand(statusCmd, adminCmd)

	// Reports commands
	reportsCmd := pkgcli.NewCommandBuilder("reports").WithShort("Reports").Build()

	pcsCmd := pkgcli.NewCommandBuilder("pcs").WithShort("Project Confidence Score").WithAnnotations(map[string]string{
		"mcp.permissions": "read:*",
	}).Build()

	eddCmd := pkgcli.NewCommandBuilder("edd").WithShort("Effort Distribution").WithAnnotations(map[string]string{
		"mcp.permissions": "read:*",
	}).Build()

	reportsCmd.AddCommand(pcsCmd, eddCmd)

	rootCmd.AddCommand(objectCmd, systemCmd, reportsCmd)

	return rootCmd
}
