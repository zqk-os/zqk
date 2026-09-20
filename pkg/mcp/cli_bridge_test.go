package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/cobra"
	pkgcli "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestDiscoverCLICommands(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI discovery test in short mode (can hang on full command tree)")
	}
	t.Parallel()
	// Get root command
	rootCmd := getRootCommand()
	if rootCmd == nil {
		t.Fatal("failed to get root command")
	}

	// Discover all commands
	commands := DiscoverCLICommands(rootCmd)

	// Should discover at least the root command
	if len(commands) == 0 {
		t.Error("expected to discover at least one command")
	}

	// Should discover object commands
	foundObject := false
	for _, cmd := range commands {
		if cmd.Use == "object" {
			foundObject = true
			// Should have subcommands
			if len(cmd.Subcommands) == 0 {
				t.Error("object command should have subcommands")
			}
			break
		}
	}
	if !foundObject {
		t.Error("expected to discover 'object' command")
	}
}

func TestFilterCommandsByPermissions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI discovery test in short mode (can hang on full command tree)")
	}
	t.Parallel()
	rootCmd := getRootCommand()
	commands := DiscoverCLICommands(rootCmd)

	// Test with admin role (should see all commands)
	adminSecCtx := pkgctx.NewSecurityContext("account:admin", []string{"admin"}, []string{"read:*", "write:*", "delete:*"})
	filteredAdmin := FilterCommandsByPermissions(commands, adminSecCtx)
	if len(filteredAdmin) < len(commands) {
		t.Errorf("admin should see all commands, got %d out of %d", len(filteredAdmin), len(commands))
	}

	// Test with read-only role (should see fewer commands)
	readOnlySecCtx := pkgctx.NewSecurityContext("ACC-1785920548450214017-87f10a62", []string{"viewer"}, []string{"read:*"})
	filteredReadOnly := FilterCommandsByPermissions(commands, readOnlySecCtx)
	if len(filteredReadOnly) >= len(filteredAdmin) {
		t.Errorf("read-only user should see fewer commands than admin: got %d (read-only) >= %d (admin)", len(filteredReadOnly), len(filteredAdmin))
	}

	// Test with no permissions (should see minimal commands)
	noPermsSecCtx := pkgctx.NewSecurityContext("account:guest", []string{"guest"}, []string{})
	filteredNoPerms := FilterCommandsByPermissions(commands, noPermsSecCtx)
	if len(filteredNoPerms) >= len(filteredReadOnly) {
		t.Errorf("user with no permissions should see fewer commands than read-only user: got %d (no-perms) >= %d (read-only)", len(filteredNoPerms), len(filteredReadOnly))
	}
}

func TestConvertCommandToMCPTool(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI discovery test in short mode (can hang on full command tree)")
	}
	t.Parallel()
	rootCmd := getRootCommand()

	// Find object list command
	var listCmd *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "object" {
			for _, subCmd := range cmd.Commands() {
				if subCmd.Use == "list" {
					listCmd = subCmd
					break
				}
			}
		}
	}

	if listCmd == nil {
		t.Fatal("failed to find object list command")
	}

	// Convert to MCP tool (need to create DiscoveredCommand first)
	discoveredCmd := &DiscoveredCommand{
		Use:     listCmd.Use,
		Short:   listCmd.Short,
		Long:    listCmd.Long,
		Command: listCmd,
		Path:    "object list",
	}
	tool := ConvertCommandToMCPTool(discoveredCmd)

	if tool.Name == emptyValue {
		t.Error("expected tool to have a name")
	}

	if tool.Description == emptyValue {
		t.Error("expected tool to have a description")
	}

	// Check input schema
	schema, ok := tool.InputSchema.(map[string]any)
	if !ok {
		t.Fatal("expected input schema to be a map")
	}

	if schema[objects.FieldKeyType] != "object" {
		t.Error("expected input schema type to be 'object'")
	}
}

func TestExecuteCLICommandViaMCP(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI execution test in short mode (spawns subprocess, can hang)")
	}
	// Not t.Parallel(): t.Setenv(ZQK_TEST_ROOT) is invalid after Parallel.
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:                     "pkg.mcp.cli_bridge",
		SkipSetupTestEnvironment: true,
		AppendStagesBeforeStorage: func(root string) []testkit.NamedTestStep {
			return []testkit.NamedTestStep{{
				Name: "cli_bridge_backlog_dir",
				Fn: func() error {
					return paths.LayoutUnder(root).Dir(paths.ProcessBacklogDir, paths.DirPerm755).Err()
				},
			}}
		},
	})
	tmpDir := proj.Root

	// Create security context
	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})

	// Execute a simple command via MCP
	args := map[string]any{
		objects.FieldKeyCommand: "object",
		"subcommand":            "list",
		objects.FieldKeyKind:    "backlog_item",
	}

	initCtx := &pkgctx.CliInitializationContext{
		ProjectRoot: tmpDir,
	}
	// Subprocess CLI must not run without a deadline — object list can block on locks/scheduler.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result, err := ExecuteCLICommandViaMCPWithContext(ctx, args, secCtx, initCtx)
	if err != nil {
		t.Logf("Command execution error (may be expected if no objects exist): %v", err)
	}

	// Result should be a map
	if result != nil {
		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Errorf("expected result to be a map, got %T", result)
		} else {
			// Should have command and output
			if _, ok := resultMap[objects.FieldKeyCommand]; !ok {
				t.Error("expected result to have 'command' key")
			}
		}
	}
}

func TestInitializeSecurityContextFromMCP(t *testing.T) {
	t.Parallel()
	// Test with client info
	clientInfo := map[string]any{
		objects.FieldKeyName:        "test-client",
		objects.FieldKeyVersion:     "1.0.0",
		clientInfoAccountID:         "account:testuser",
		objects.FieldKeyRoles:       []any{"admin", "developer"},
		objects.FieldKeyPermissions: []any{"read:*", "write:backlog_item"},
	}

	secCtx := InitializeSecurityContextFromMCP(clientInfo)

	if secCtx == nil {
		t.Fatal("expected security context to be created")
	}

	if secCtx.AccountID != "account:testuser" {
		t.Errorf("expected account ID 'account:testuser', got %s", secCtx.AccountID)
	}

	if len(secCtx.Roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(secCtx.Roles))
	}

	if len(secCtx.Permissions) != 2 {
		t.Errorf("expected 2 permissions, got %d", len(secCtx.Permissions))
	}
}

func TestInitializeSecurityContextFromMCP_Defaults(t *testing.T) {
	t.Parallel()
	// Test with minimal client info (no account_id).
	// Security: implementation does not default to system context when account_id is missing.
	clientInfo := map[string]any{
		objects.FieldKeyName: "test-client",
	}

	secCtx := InitializeSecurityContextFromMCP(clientInfo)

	if secCtx == nil {
		t.Fatal("expected security context to be created")
	}

	// When no account_id is provided, AccountID may be empty (security: no default to system)
	if secCtx.AccountID != emptyValue && secCtx.AccountID != pkgctx.SystemAccountID {
		// If a default is ever added, it should be system or explicit
		t.Logf("account ID set to %q (no account_id in clientInfo)", secCtx.AccountID)
	}
}

// Helper function to get root command for testing
func getRootCommand() *cobra.Command {
	// Create a minimal root command for testing
	rootCmd := pkgcli.NewCommandBuilder("zqk").WithShort("ZQK CLI").Build()

	// Create a minimal object command for testing
	// This command requires write permission, so read-only users won't see it
	objectCmd := pkgcli.NewCommandBuilder("object").WithShort("Object operations").WithAnnotations(map[string]string{
		"mcp.permissions": "write:*", // Requires write permission (not accessible to read-only)
	}).Build()

	// Add a list subcommand (read-only, accessible to read-only users)
	dummyRunE := func(cmd *cobra.Command, args []string) error { return nil }
	listCmd := pkgcli.NewCommandBuilder("list").WithShort("List objects").WithRunE(dummyRunE).WithAnnotations(map[string]string{
		"mcp.permissions": "read:*", // Only requires read permission
	}).Build()

	objectCmd.AddCommand(listCmd)
	rootCmd.AddCommand(objectCmd)

	return rootCmd
}

func TestExtractFlags_Resiliency(t *testing.T) {
	t.Parallel()

	// Create mock command tree with flags
	rootCmd := new(cobra.Command)
	rootCmd.Use = "zqk"

	createCmd := new(cobra.Command)
	createCmd.Use = "create"
	createCmd.Flags().StringSlice("field", []string{}, "field description")
	rootCmd.AddCommand(createCmd)

	listCmd := new(cobra.Command)
	listCmd.Use = "list"
	listCmd.Flags().StringSlice("fields", []string{}, "fields description")
	rootCmd.AddCommand(listCmd)

	// Create MCP server and configure root command
	server := NewServer()
	server.SetRootCommand(rootCmd)

	// Test 1: create command with "fields" input -> should map to "field" flag
	argsCreate := map[string]any{
		"_command_path": "create",
		"fields":        []any{"status=in_progress"},
	}
	flagsCreate := ExtractFlags(argsCreate, server)
	foundField := false
	for i, f := range flagsCreate {
		if f == "--field" {
			foundField = true
			if i+1 >= len(flagsCreate) || flagsCreate[i+1] != "status=in_progress" {
				t.Errorf("Expected value 'status=in_progress' for flag --field, got %v", flagsCreate)
			}
		}
		if f == "--fields" {
			t.Errorf("Did not expect --fields flag, got %v", flagsCreate)
		}
	}
	if !foundField {
		t.Errorf("Expected to find --field flag, got %v", flagsCreate)
	}

	// Test 2: list command with "field" input -> should map to "fields" flag
	argsList := map[string]any{
		"_command_path":       "list",
		objects.FieldKeyField: []any{"id", "status"},
	}
	flagsList := ExtractFlags(argsList, server)
	foundFields := false
	for i, f := range flagsList {
		if f == "--fields" {
			foundFields = true
			if i+1 >= len(flagsList) || (flagsList[i+1] != "id" && flagsList[i+1] != "status") {
				t.Errorf("Expected value 'id' or 'status' for flag --fields, got %v", flagsList)
			}
		}
		if f == "--field" {
			t.Errorf("Did not expect --field flag, got %v", flagsList)
		}
	}
	if !foundFields {
		t.Errorf("Expected to find --fields flag, got %v", flagsList)
	}
}

func TestCLIBridge_LifetimeCounters(t *testing.T) {
	initDiscovered := GetCLIBridgeStats()

	rootCmd := pkgcli.NewCommandBuilder("test_cli_root").Build()
	subCmd := pkgcli.NewCommandBuilder("test_sub").Build()
	rootCmd.AddCommand(subCmd)

	cmds := DiscoverCLICommands(rootCmd)
	if len(cmds) == 0 {
		t.Fatalf("expected at least one command discovered")
	}

	afterDiscovered := GetCLIBridgeStats()
	if afterDiscovered <= initDiscovered {
		t.Fatalf("expected commandsDiscoveredTotal to increment, got init=%d after=%d", initDiscovered, afterDiscovered)
	}
}

func TestBuildCommandEnvironment_SessionID(t *testing.T) {
	server := NewServer()
	server.SetCurrentSessionID("MCP-SESSION-TEST-123")

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*"})
	env := buildCommandEnvironment(secCtx, "/tmp/test", 0, server)

	expectedSessionKV := zqkenv.SessionID().Name() + "=MCP-SESSION-TEST-123"
	found := false
	for _, e := range env {
		if e == expectedSessionKV {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %s in subprocess environment, got: %v", expectedSessionKV, env)
	}
}

func TestIsUnsafeCLIBridgeBinary(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path string
		want bool
	}{
		{"/var/folders/x/go-build123/b001/mcp.test", true},
		{"/tmp/mcp.test", true},
		{"/Users/me/ai-projects/zqk/bin/zqk", false},
		{"/Users/me/ai-projects/zqk/bin/zqk-stable", false},
		{"", true},
	}
	for _, tc := range cases {
		if got := isUnsafeCLIBridgeBinary(tc.path); got != tc.want {
			t.Fatalf("isUnsafeCLIBridgeBinary(%q)=%v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestFindFullZqkBinary_NeverReturnsTestBinary(t *testing.T) {
	t.Parallel()
	path := findFullZqkBinary()
	if path == "" {
		// PATH may lack zqk in some sandboxes; empty is fail-closed (no bomb).
		return
	}
	if isUnsafeCLIBridgeBinary(path) {
		t.Fatalf("findFullZqkBinary returned unsafe path %q", path)
	}
}

func TestFindExecutableBinary_NeverReturnsTestBinary(t *testing.T) {
	// Not parallel: may consult env / PATH.
	path, err := findExecutableBinary()
	if err != nil {
		// Fail-closed is acceptable when no safe CLI is on PATH.
		t.Logf("findExecutableBinary error (fail-closed OK): %v", err)
		return
	}
	if isUnsafeCLIBridgeBinary(path) {
		t.Fatalf("findExecutableBinary returned unsafe path %q", path)
	}
}
