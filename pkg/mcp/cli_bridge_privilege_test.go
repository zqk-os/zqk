package mcp

import (
	"testing"

	"github.com/spf13/cobra"

	pkgcli "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// TestPrivilegeFiltering_ReadOnlyCommands tests that read-only users only see read commands
func TestPrivilegeFiltering_ReadOnlyCommands(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI discovery test in short mode (can hang on parallel discovery)")
	}
	t.Parallel()
	rootCmd := createCommandWithPermissions()

	// Discover all commands
	allCommands := DiscoverCLICommands(rootCmd)

	// Create read-only security context
	readOnlySecCtx := pkgctx.NewSecurityContext("ACC-1785920548450214017-87f10a62", []string{"viewer"}, []string{"read:*"})

	// Filter commands
	filteredCommands := FilterCommandsByPermissions(allCommands, readOnlySecCtx)

	// Verify read-only user can see list command
	// Normalize paths by removing root command name (e.g., "zqk object list" -> "object list")
	foundList := false
	for _, cmd := range filteredCommands {
		normalized := normalizeTestCommandPath(cmd.Path)
		if normalized == "object list" {
			foundList = true
			break
		}
	}
	if !foundList {
		// Debug: show all filtered command paths
		paths := make([]string, 0, len(filteredCommands))
		for _, cmd := range filteredCommands {
			paths = append(paths, cmd.Path+" (normalized: "+NormalizeCommandPath(cmd.Path)+")")
		}
		t.Errorf("read-only user should be able to see 'object list' command. Filtered commands: %v", paths)
	}

	// Verify read-only user cannot see write commands
	foundCreate := false
	foundDelete := false
	for _, cmd := range filteredCommands {
		normalized := normalizeTestCommandPath(cmd.Path)
		if normalized == "object create" {
			foundCreate = true
		}
		if normalized == "object delete" {
			foundDelete = true
		}
	}
	if foundCreate {
		t.Error("read-only user should NOT be able to see 'object create' command")
	}
	if foundDelete {
		t.Error("read-only user should NOT be able to see 'object delete' command")
	}
}

// TestPrivilegeFiltering_WritePermissions tests that users with write permissions see write commands
func TestPrivilegeFiltering_WritePermissions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI discovery test in short mode (can hang on parallel discovery)")
	}
	t.Parallel()
	rootCmd := createCommandWithPermissions()

	// Discover all commands
	allCommands := DiscoverCLICommands(rootCmd)

	// Create security context with write permissions
	writeSecCtx := pkgctx.NewSecurityContext("account:writer", []string{"developer"}, []string{"read:*", "write:backlog_item"})

	// Filter commands
	filteredCommands := FilterCommandsByPermissions(allCommands, writeSecCtx)

	// Verify user can see list command
	// Normalize paths by removing root command name
	foundList := false
	for _, cmd := range filteredCommands {
		normalized := normalizeTestCommandPath(cmd.Path)
		if normalized == "object list" {
			foundList = true
			break
		}
	}
	if !foundList {
		t.Error("user with write permissions should be able to see 'object list' command")
	}

	// Verify user can see create command
	foundCreate := false
	for _, cmd := range filteredCommands {
		normalized := normalizeTestCommandPath(cmd.Path)
		if normalized == "object create" {
			foundCreate = true
			break
		}
	}
	if !foundCreate {
		t.Error("user with write permissions should be able to see 'object create' command")
	}

	// Verify user cannot see delete command (requires delete permission)
	foundDelete := false
	for _, cmd := range filteredCommands {
		if normalizeTestCommandPath(cmd.Path) == "object delete" {
			foundDelete = true
			break
		}
	}
	if foundDelete {
		t.Error("user without delete permissions should NOT be able to see 'object delete' command")
	}
}

// TestPrivilegeFiltering_AdminRole tests that admin role bypasses permission checks
func TestPrivilegeFiltering_AdminRole(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI discovery test in short mode (can hang on parallel discovery)")
	}
	t.Parallel()
	rootCmd := createCommandWithPermissions()

	// Discover all commands
	allCommands := DiscoverCLICommands(rootCmd)

	// Create admin security context (no specific permissions, but has admin role)
	adminSecCtx := pkgctx.NewSecurityContext("account:admin", []string{"admin"}, []string{})

	// Filter commands
	filteredCommands := FilterCommandsByPermissions(allCommands, adminSecCtx)

	// Admin should see all commands regardless of permissions
	if len(filteredCommands) < len(allCommands) {
		t.Errorf("admin should see all commands, got %d out of %d", len(filteredCommands), len(allCommands))
	}

	// Verify admin can see all command types (use normalizeTestCommandPath: test root Use is "zqk", executable may differ)
	foundList := false
	foundCreate := false
	foundDelete := false
	for _, cmd := range filteredCommands {
		if normalizeTestCommandPath(cmd.Path) == "object list" {
			foundList = true
		}
		if normalizeTestCommandPath(cmd.Path) == "object create" {
			foundCreate = true
		}
		if normalizeTestCommandPath(cmd.Path) == "object delete" {
			foundDelete = true
		}
	}
	if !foundList {
		t.Error("admin should be able to see 'object list' command")
	}
	if !foundCreate {
		t.Error("admin should be able to see 'object create' command")
	}
	if !foundDelete {
		t.Error("admin should be able to see 'object delete' command")
	}
}

// TestPrivilegeFiltering_RoleRequirements tests that commands with role requirements are filtered correctly
func TestPrivilegeFiltering_RoleRequirements(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI discovery test in short mode (can hang on parallel discovery)")
	}
	t.Parallel()
	rootCmd := createCommandWithRoles()

	// Discover all commands
	allCommands := DiscoverCLICommands(rootCmd)

	// Create security context with developer role (not admin)
	devSecCtx := pkgctx.NewSecurityContext("account:dev", []string{"developer"}, []string{"read:*", "write:*"})

	// Filter commands
	filteredCommands := FilterCommandsByPermissions(allCommands, devSecCtx)
	if len(allCommands) == 0 {
		t.Fatal("allCommands is empty!")
	}

	// Developer should see regular commands (use normalizeTestCommandPath: test root Use is "zqk", executable may differ)
	foundList := false
	for _, cmd := range filteredCommands {
		if normalizeTestCommandPath(cmd.Path) == "object list" {
			foundList = true
			break
		}
	}
	if !foundList {
		t.Errorf("developer should be able to see 'object list' command. got: %v", filteredCommands)
	}

	// Developer should NOT see admin-only commands
	foundAdminCmd := false
	for _, cmd := range filteredCommands {
		if normalizeTestCommandPath(cmd.Path) == "system admin" {
			foundAdminCmd = true
			break
		}
	}
	if foundAdminCmd {
		t.Error("developer should NOT be able to see admin-only commands")
	}

	// Admin should see admin-only commands
	adminSecCtx := pkgctx.NewSecurityContext("account:admin", []string{"admin"}, []string{})
	adminFiltered := FilterCommandsByPermissions(allCommands, adminSecCtx)
	foundAdminCmd = false
	for _, cmd := range adminFiltered {
		if normalizeTestCommandPath(cmd.Path) == "system admin" {
			foundAdminCmd = true
			break
		}
	}
	if !foundAdminCmd {
		t.Error("admin should be able to see admin-only commands")
	}
}

// TestPrivilegeFiltering_WildcardPermissions tests wildcard permission matching
func TestPrivilegeFiltering_WildcardPermissions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI discovery test in short mode (can hang on parallel discovery)")
	}
	t.Parallel()
	rootCmd := createCommandWithPermissions()

	// Discover all commands
	allCommands := DiscoverCLICommands(rootCmd)

	// Create security context with wildcard read permission
	wildcardSecCtx := pkgctx.NewSecurityContext("account:wildcard", []string{"viewer"}, []string{"read:*"})

	// Filter commands
	filteredCommands := FilterCommandsByPermissions(allCommands, wildcardSecCtx)

	// User with "read:*" should see read commands (use normalizeTestCommandPath: test root Use is "zqk", executable may differ)
	foundList := false
	for _, cmd := range filteredCommands {
		if normalizeTestCommandPath(cmd.Path) == "object list" {
			foundList = true
			break
		}
	}
	if !foundList {
		t.Error("user with 'read:*' should be able to see 'object list' command")
	}

	// User with "read:*" should NOT see write commands
	foundCreate := false
	for _, cmd := range filteredCommands {
		if normalizeTestCommandPath(cmd.Path) == "object create" {
			foundCreate = true
			break
		}
	}
	if foundCreate {
		t.Error("user with only 'read:*' should NOT be able to see 'object create' command")
	}
}

// TestPrivilegeFiltering_NoPermissions tests that users with no permissions see minimal commands
func TestPrivilegeFiltering_NoPermissions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI discovery test in short mode (can hang on parallel discovery)")
	}
	t.Parallel()
	rootCmd := createCommandWithPermissions()

	// Discover all commands
	allCommands := DiscoverCLICommands(rootCmd)

	// Create security context with no permissions
	noPermsSecCtx := pkgctx.NewSecurityContext("account:guest", []string{"guest"}, []string{})

	// Filter commands
	filteredCommands := FilterCommandsByPermissions(allCommands, noPermsSecCtx)

	// User with no permissions should see no unannotated commands (fail-closed).
	// But commands with permission requirements should be filtered out (use normalizeTestCommandPath)
	foundCreate := false
	foundDelete := false
	for _, cmd := range filteredCommands {
		if normalizeTestCommandPath(cmd.Path) == "object create" {
			foundCreate = true
		}
		if normalizeTestCommandPath(cmd.Path) == "object delete" {
			foundDelete = true
		}
	}
	if foundCreate {
		t.Error("user with no permissions should NOT be able to see 'object create' command")
	}
	if foundDelete {
		t.Error("user with no permissions should NOT be able to see 'object delete' command")
	}
}

// TestMCPToolRegistration_PrivilegeFiltering tests that MCP tools are filtered by privileges
func TestMCPToolRegistration_PrivilegeFiltering(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI discovery test in short mode (can hang on parallel discovery)")
	}
	t.Parallel()
	rootCmd := createCommandWithPermissions()

	// Create server
	server := NewServer()

	// Test with read-only user
	readOnlySecCtx := pkgctx.NewSecurityContext("ACC-1785920548450214017-87f10a62", []string{"viewer"}, []string{"read:*"})
	err := RegisterCLIToolsWithRootCommand(server, rootCmd, readOnlySecCtx, "")
	if err != nil {
		t.Fatalf("failed to register CLI tools: %v", err)
	}

	// Get registered tools
	toolsList := server.ListTools()

	// Verify read-only user can see list tool (tool names use brand prefix, e.g. zqk_object_list or mcp.test_object_list)
	foundListTool := false
	for _, tool := range toolsList {
		if normalizeTestToolName(tool.Name) == "object_list" {
			foundListTool = true
			break
		}
	}
	if !foundListTool {
		t.Error("read-only user should be able to see 'object_list' tool (e.g. cli_object_list)")
	}

	// Verify read-only user cannot see write tools
	foundCreateTool := false
	foundDeleteTool := false
	for _, tool := range toolsList {
		if normalizeTestToolName(tool.Name) == "object_create" {
			foundCreateTool = true
		}
		if normalizeTestToolName(tool.Name) == "object_delete" {
			foundDeleteTool = true
		}
	}
	if foundCreateTool {
		t.Error("read-only user should NOT be able to see 'object_create' tool")
	}
	if foundDeleteTool {
		t.Error("read-only user should NOT be able to see 'object_delete' tool")
	}
}

// TestMCPToolRegistration_AdminSeesAll tests that admin sees all tools
func TestMCPToolRegistration_AdminSeesAll(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI discovery test in short mode (can hang on parallel discovery)")
	}
	t.Parallel()
	rootCmd := createCommandWithPermissions()

	// Create server
	server := NewServer()

	// Test with admin user
	adminSecCtx := pkgctx.NewSecurityContext("account:admin", []string{"admin"}, []string{})
	err := RegisterCLIToolsWithRootCommand(server, rootCmd, adminSecCtx, "")
	if err != nil {
		t.Fatalf("failed to register CLI tools: %v", err)
	}

	// Get registered tools
	toolsList := server.ListTools()

	// Admin should see all tools (tool names use brand prefix; compare normalized name)
	foundListTool := false
	foundCreateTool := false
	foundDeleteTool := false
	for _, tool := range toolsList {
		if normalizeTestToolName(tool.Name) == "object_list" {
			foundListTool = true
		}
		if normalizeTestToolName(tool.Name) == "object_create" {
			foundCreateTool = true
		}
		if normalizeTestToolName(tool.Name) == "object_delete" {
			foundDeleteTool = true
		}
	}
	if !foundListTool {
		t.Error("admin should be able to see 'object_list' tool (e.g. cli_object_list)")
	}
	if !foundCreateTool {
		t.Error("admin should be able to see 'object_create' tool")
	}
	if !foundDeleteTool {
		t.Error("admin should be able to see 'object_delete' tool")
	}
}

// Helper function to create commands with permission annotations
func createCommandWithPermissions() *cobra.Command {
	rootCmd := pkgcli.NewCommandBuilder("zqk").WithShort("ZQK CLI").WithAnnotations(map[string]string{"mcp.roles": "developer"}).Build()

	objectCmd := pkgcli.NewCommandBuilder("object").WithShort("Object operations").WithAnnotations(map[string]string{"mcp.roles": "developer"}).Build()

	dummyRunE := func(cmd *cobra.Command, args []string) error { return nil }

	// List command - requires read permission
	listCmd := pkgcli.NewCommandBuilder("list").WithShort("List objects").WithRunE(dummyRunE).WithAnnotations(map[string]string{
		"mcp.permissions": "read:*",
	}).Build()

	// Create command - requires write permission
	createCmd := pkgcli.NewCommandBuilder("create").WithShort("Create object").WithRunE(dummyRunE).WithAnnotations(map[string]string{
		"mcp.permissions": "write:backlog_item",
	}).Build()

	// Delete command - requires delete permission
	deleteCmd := pkgcli.NewCommandBuilder("delete").WithShort("Delete object").WithRunE(dummyRunE).WithAnnotations(map[string]string{
		"mcp.permissions": "delete:backlog_item",
	}).Build()

	objectCmd.AddCommand(listCmd)
	objectCmd.AddCommand(createCmd)
	objectCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(objectCmd)

	return rootCmd
}

// Helper function to create commands with role annotations
func createCommandWithRoles() *cobra.Command {
	rootCmd := pkgcli.NewCommandBuilder("zqk").WithShort("ZQK CLI").WithAnnotations(map[string]string{"mcp.roles": "developer"}).Build()

	objectCmd := pkgcli.NewCommandBuilder("object").WithShort("Object operations").WithAnnotations(map[string]string{"mcp.roles": "developer"}).Build()

	dummyRunE := func(cmd *cobra.Command, args []string) error { return nil }

	// List command - no role requirement
	listCmd := pkgcli.NewCommandBuilder("list").WithShort("List objects").WithRunE(dummyRunE).WithAnnotations(map[string]string{"mcp.roles": "developer"}).Build()

	systemCmd := pkgcli.NewCommandBuilder("system").WithShort("System operations").WithAnnotations(map[string]string{"mcp.roles": "developer"}).Build()

	// Admin command - requires admin role
	adminCmd := pkgcli.NewCommandBuilder("admin").WithShort("Admin operations").WithRunE(dummyRunE).WithAnnotations(map[string]string{
		"mcp.roles": "admin",
	}).Build()

	objectCmd.AddCommand(listCmd)
	systemCmd.AddCommand(adminCmd)
	rootCmd.AddCommand(objectCmd)
	rootCmd.AddCommand(systemCmd)

	return rootCmd
}
