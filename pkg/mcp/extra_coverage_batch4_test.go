package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// 1. Server Init Helpers & Authentication Flow Extended
func TestBatch4_ServerInitHelpers_ExtendedFlow(t *testing.T) {
	server := NewServer()
	tmpDir := t.TempDir()
	server.SetProjectRoot(tmpDir)

	ctx := context.Background()

	// 1. Local stdio with isHumanClient
	clientInfoHuman := map[string]any{}
	initParamsHuman := InitializeParams{}
	initParamsHuman.ClientInfo.Name = "vscode"
	resInfo, accID, err := server.handleAuthenticationFlow(ctx, "client-human", clientInfoHuman, initParamsHuman)
	if err != nil || accID != SystemAccountID {
		t.Errorf("expected SystemAccountID for human client, got %s, %v", accID, err)
	}
	if resInfo[clientInfoAccountID] != SystemAccountID {
		t.Errorf("expected clientInfoAccountID to be SystemAccountID, got %v", resInfo)
	}

	// 2. Local stdio with SystemAccountID clientID
	clientInfoSys := map[string]any{}
	initParamsSys := InitializeParams{}
	resInfoSys, accIDSys, err := server.handleAuthenticationFlow(ctx, SystemAccountID, clientInfoSys, initParamsSys)
	if err != nil || accIDSys != SystemAccountID {
		t.Errorf("expected SystemAccountID, got %s, %v", accIDSys, err)
	}
	_ = resInfoSys

	// 3. MultiClient mode uncredentialed caller -> returns Unauthenticated
	server.multiClient.Store(true)
	defer server.multiClient.Store(false)

	clientInfoUncred := map[string]any{}
	initParamsUncred := InitializeParams{}
	initParamsUncred.ClientInfo.Name = "remote-agent"
	_, _, err = server.handleAuthenticationFlow(ctx, "client-remote", clientInfoUncred, initParamsUncred)
	if err == nil {
		t.Error("expected error for uncredentialed caller in multiClient mode")
	}

	// 4. MultiClient mode with trusted loopback adapter (e.g. ide-ide-proxy)
	clientInfoLoop := map[string]any{}
	initParamsLoop := InitializeParams{}
	initParamsLoop.ClientInfo.Name = "ide-ide-proxy"
	resInfoLoop, accIDLoop, err := server.handleAuthenticationFlow(ctx, "ide-ide-proxy", clientInfoLoop, initParamsLoop)
	if err != nil || accIDLoop != SystemAccountID {
		t.Errorf("expected SystemAccountID for trusted loopback, got %s, %v", accIDLoop, err)
	}
	_ = resInfoLoop

	// 5. MultiClient mode with accountID supplied but no credentials -> fail-closed
	clientInfoAccOnly := map[string]any{
		clientInfoAccountID: "ACC-CUSTOM",
	}
	initParamsAccOnly := InitializeParams{}
	initParamsAccOnly.ClientInfo.Name = "external-client"
	_, _, err = server.handleAuthenticationFlow(ctx, "client-acc", clientInfoAccOnly, initParamsAccOnly)
	if err == nil {
		t.Error("expected error for accountID without credentials in multiClient mode")
	}

	// 6. configureClientCapabilities
	initParamsCap := InitializeParams{
		Capabilities: map[string]any{
			capabilityAllowedFormats: []any{"json", "yaml"},
		},
	}
	server.configureClientCapabilities(initParamsCap)

	initParamsCapStr := InitializeParams{
		Capabilities: map[string]any{
			capabilityAllowedFormats: "json,text",
		},
	}
	server.configureClientCapabilities(initParamsCapStr)

	// 7. finalizeInitialization
	initRes, err := server.finalizeInitialization("client-final", "seq-1", initParamsCap)
	if err != nil || initRes.ServerInfo.Name == "" {
		t.Errorf("finalizeInitialization failed: %v, %v", err, initRes)
	}

	// 8. elicitation helpers
	server.multiClient.Store(false)
	server.config = &ServerConfig{}
	_, _, _ = server.eliciteAuthentication(ctx, "cid", map[string]any{}, InitializeParams{})
	_, _, _ = server.eliciteAccountID(ctx, "cid", map[string]any{}, InitializeParams{})
	_, _, _ = server.eliciteCredentials(ctx, "cid", map[string]any{}, InitializeParams{})
	_ = server.initializeSecurityContext(ctx, map[string]any{
		clientInfoAccountID: "ACC-1",
		clientInfoRoles:     []string{"developer"},
	})
	_, _ = server.enforceRoleAndSecurity(map[string]any{clientInfoAccountID: "ACC-1"}, "ACC-1")
}

// 2. Server Handlers Auth Comprehensive
func TestBatch4_ServerHandlersAuth_AllBranches(t *testing.T) {
	server := NewServer()
	tmpDir := t.TempDir()
	server.SetProjectRoot(tmpDir)
	ctx := context.Background()

	// 1. Missing project root
	serverNoRoot := NewServer()
	_, _, _, err := serverNoRoot.validateCredentialsAndResolveAccount(ctx, map[string]any{})
	if err == nil {
		t.Error("expected error when project root missing")
	}

	// 2. No credentials provided
	_, _, _, err = server.validateCredentialsAndResolveAccount(ctx, map[string]any{})
	if err == nil {
		t.Error("expected error when no credentials provided")
	}

	// 3. Keystore Key ID authentication
	_, _, _, _ = server.validateCredentialsAndResolveAccount(ctx, map[string]any{
		clientInfoKeystoreKeyID: "KEY-12345",
	})

	// 4. Username/password authentication - missing password
	_, _, _, err = server.validateCredentialsAndResolveAccount(ctx, map[string]any{
		clientInfoUsername: "testuser",
	})
	if err == nil || !strings.Contains(err.Error(), "password required") {
		t.Errorf("expected password required error, got %v", err)
	}

	// 5. Username/password authentication - with password
	_, _, _, _ = server.validateCredentialsAndResolveAccount(ctx, map[string]any{
		clientInfoUsername: "testuser",
		clientInfoPassword: "secretpassword",
	})

	// 6. OAuth token authentication
	_, _, _, _ = server.validateCredentialsAndResolveAccount(ctx, map[string]any{
		clientInfoOAuthTok: "oauth-tok-12345",
	})

	// 7. Personal Access Token (PAT) authentication
	_, _, _, _ = server.validateCredentialsAndResolveAccount(ctx, map[string]any{
		clientInfoPersonalAccessToken: "pat-secret-token",
	})

	// 8. Individual validators directly
	_, _, _, _ = server.validateKeystoreKey(ctx, "KEY-1", tmpDir)
	_, _, _, _ = server.validateOAuthToken(ctx, "TOK-1", tmpDir)
	_, _, _, _ = server.validatePersonalAccessToken(ctx, "PAT-1", tmpDir)
	_, _, _, _ = server.validateUsernamePassword(ctx, "user", "pass", tmpDir)
	_, _ = server.loadAccountObject("ACC-1", tmpDir)
	_, _ = server.loadAccountByEmail("user@example.com", tmpDir)
}

// 3. Logging Channel Extended Flow
func TestBatch4_LoggingChannel_AllLevelsAndGuards(t *testing.T) {
	server := NewServer()
	server.SetProjectRoot(t.TempDir())

	// Normal flow
	_ = server.SendLogDebug("debug msg", map[string]any{"source": "test"})
	_ = server.SendLogInfo("info msg", map[string]any{"source": "test"})
	_ = server.SendLogWarn("warn msg", map[string]any{"source": "test"})
	_ = server.SendLogError("error msg", map[string]any{"source": "test"})
	_ = server.SendProgressLog("test-op", 0.75, "in progress")
	_ = server.SendLogMessage(LogLevelInfo, "info msg 2", nil)

	// Guard 0: Shutdown flag set
	server.shutdownFlag.Store(1)
	_ = server.SendLogMessage(LogLevelError, "shutdown error", map[string]any{"reason": "terminating"})
	_ = server.SendLogMessage(LogLevelInfo, "shutdown info", nil)

	// Guard 1: Shutdown context cancelled
	server2 := NewServer()
	server2.shutdownCancel()
	_ = server2.SendLogMessage(LogLevelWarn, "context cancelled warn", map[string]any{"flag": 1})
}

// 4. Role Enforcement All Rules
func TestBatch4_RoleEnforcement_AllRules(t *testing.T) {
	tmpDir := t.TempDir()
	server := NewServer()
	server.SetProjectRoot(tmpDir)

	// Rule 1: EnforcedRole overrides everything
	cfgEnforced := &ServerConfig{}
	cfgEnforced.MCPServer.Security.EnforcedRole = "observer"
	clientInfo := map[string]any{
		objects.FieldKeyRoles: []any{"admin"},
	}
	res1, err := enforceRoleEnforcement(clientInfo, cfgEnforced, tmpDir, server)
	if err != nil {
		t.Fatalf("enforceRoleEnforcement failed: %v", err)
	}
	roles1, ok := res1[objects.FieldKeyRoles].([]any)
	if !ok || len(roles1) != 1 || roles1[0] != "observer" {
		t.Errorf("expected enforced role observer, got %v", res1[objects.FieldKeyRoles])
	}

	// Rule 2: AllowedRoles filters current roles
	cfgAllowed := &ServerConfig{}
	cfgAllowed.MCPServer.Security.AllowedRoles = []string{"viewer", "developer"}
	clientInfoAllowed := map[string]any{
		objects.FieldKeyRoles: []string{"developer", "superuser"},
	}
	res2, err := enforceRoleEnforcement(clientInfoAllowed, cfgAllowed, tmpDir, server)
	if err != nil {
		t.Fatalf("enforceRoleEnforcement allowed failed: %v", err)
	}
	_ = res2

	// Rule 3: EnforceAccountRoles
	cfgAcc := &ServerConfig{}
	cfgAcc.MCPServer.Security.EnforceAccountRoles = true
	clientInfoAcc := map[string]any{
		clientInfoAccountID:   "ACC-TEST",
		objects.FieldKeyRoles: "developer,admin",
	}
	res3, err := enforceRoleEnforcement(clientInfoAcc, cfgAcc, tmpDir, server)
	if err != nil {
		t.Fatalf("enforceRoleEnforcement account failed: %v", err)
	}
	_ = res3
}

// 5. Server Tool Execution & CLI Bridge Execution Extended
func TestBatch4_ServerToolExecution_Comprehensive(t *testing.T) {
	server := NewServer()
	server.SetProjectRoot(t.TempDir())
	ctx := context.Background()

	// Register builtins
	RegisterEchoTool(server)

	// Execute valid registered tool
	res, err := server.handleToolCallWithContext(ctx, "echo", map[string]any{
		"message": "hello world",
	})
	if err != nil {
		t.Errorf("handleToolCallWithContext echo failed: %v", err)
	}
	_ = res

	// Execute nonexistent tool
	_, err = server.handleToolCallWithContext(ctx, "nonexistent_tool_12345", map[string]any{})
	if err == nil {
		t.Error("expected error for nonexistent tool")
	}

	// executeCLICommandWithContext directly
	_, _ = server.executeCLICommandWithContext(ctx, map[string]any{
		"command": "system",
	})

	// BootstrapCLITools
	_ = server.BootstrapCLITools()
}

// 6. CLI Bridge Functions Comprehensive
func TestBatch4_CLIBridge_Comprehensive(t *testing.T) {
	// Lifetime stats
	disc := GetCLIBridgeStats()
	_ = disc

	// sanitizeToolName
	name := sanitizeToolName("zqk system status")
	if name != "zqk_system_status" {
		t.Errorf("unexpected sanitized name: %s", name)
	}

	// isUnsafeCLIBridgeBinary & acceptCLIBridgeBinary
	if !isUnsafeCLIBridgeBinary("/path/to/test.test") {
		t.Error("expected test binary to be unsafe")
	}
	if !isUnsafeCLIBridgeBinary("/path/to/go-build123/bin") {
		t.Error("expected go-build bin to be unsafe")
	}
	if isUnsafeCLIBridgeBinary("/usr/local/bin/zqk") {
		t.Error("expected real zqk bin to be safe")
	}

	_, _ = acceptCLIBridgeBinary("/usr/local/bin/zqk")
	_ = findFullZqkBinary()

	// logMCPCLIDiagnostic
	logMCPCLIDiagnostic("/usr/local/bin/zqk", "/project", []string{"status"}, 10*time.Millisecond, "", nil)
	logMCPCLIDiagnostic("/usr/local/bin/zqk", "/project", []string{"status"}, 10*time.Millisecond, "some stderr", errors.New("exec error"))

	// formatSecurityContextForLogging
	secCtx := &pkgctx.SecurityContext{
		AccountID:   "ACC-1",
		Roles:       []string{"dev"},
		Permissions: []string{"read:*"},
	}
	rStr, pStr := formatSecurityContextForLogging(secCtx)
	if !strings.Contains(rStr, "dev") || !strings.Contains(pStr, "read:*") {
		t.Errorf("unexpected security context format: %s, %s", rStr, pStr)
	}

	// DiscoverCLICommands with custom root command
	rootCmd := &cobra.Command{Use: "zqk"}
	subCmd := &cobra.Command{
		Use:   "testcmd",
		Short: "A test command",
		Run:   func(cmd *cobra.Command, args []string) {},
	}
	subCmd.Annotations = map[string]string{
		"mcp.permissions": "read:test",
		"mcp.roles":       "developer",
	}
	rootCmd.AddCommand(subCmd)

	commands := DiscoverCLICommands(rootCmd)
	if len(commands) == 0 {
		t.Error("expected discovered commands")
	}

	filtered := FilterCommandsByPermissions(commands, secCtx)
	_ = filtered

	server := NewServer()
	server.SetProjectRoot(t.TempDir())
	_ = RegisterCLIToolsWithRootCommand(server, rootCmd, secCtx, server.GetProjectRoot())
	_ = RegisterCLIToolsWithRootCommandAndConfig(server, rootCmd, secCtx, server.GetProjectRoot(), &ServerConfig{})
}

// 7. MCP Spec Builder Fluent API Comprehensive
func TestBatch4_MCPSpecBuilder_Comprehensive(t *testing.T) {
	builder := NewMCPSpecBuilder("custom-spec").
		Description("A comprehensive custom MCP spec").
		Version("2.0.0")

	// PromptSpecBuilder
	prompt := NewPromptSpecBuilder("analyze_code", "Analyzes source code").
		AddArgument("file_path", "Path to source file", true).
		AddArgument("depth", "Analysis depth", false).
		SetTemplate("Analyze file: {{file_path}}").
		SetTemplateRef("templates/analyze.tmpl").
		AddVariable("model", "default").
		Build()
	builder.AddPrompt(prompt)

	// ResourceSpecBuilder
	resource := NewResourceSpecBuilder("doc://arch", "Architecture Guide", "High level system architecture").
		SetMimeType("text/markdown").
		SetCategory("documentation").
		SetPriority("high").
		AddTag("arch").
		AddTags("design", "system").
		AddMetadata("version", "1").
		Build()
	builder.AddResource(resource)

	// ToolSpecBuilder
	tool := NewToolSpecBuilder("run_linter", "Executes codebase linting").
		AddProperty("ruleset", PropertySpec{Type: "string", Description: "Lint ruleset"}).
		AddStringProperty("path", "Directory path").
		AddStringPropertyWithDefault("severity", "Minimum severity level", "warning").
		AddBooleanProperty("fix", "Auto fix issues").
		AddNumberProperty("max_issues", "Maximum issues to report").
		MarkRequired("path").
		SetHandler("handleLinter").
		Build()
	builder.AddTool(tool)

	// ToolGroup
	builder.AddToolGroup(ToolGroupSpec{
		Name:        "analysis_tools",
		Description: "Code analysis and linting tools",
		Tools:       []string{"run_linter"},
		Tags:        []string{"quality"},
	})

	// ResourceDiscovery
	builder.SetResourceDiscovery(ResourceDiscoverySpec{
		Enabled:   true,
		BasePaths: []string{"docs/"},
		Patterns:  []string{"*.md"},
	})

	spec := builder.Build()
	if spec == nil || spec.Name != "custom-spec" {
		t.Fatalf("unexpected built spec: %v", spec)
	}
	if len(spec.Prompts) != 1 || len(spec.Resources) != 1 || len(spec.Tools) != 1 {
		t.Errorf("unexpected spec contents: %v", spec)
	}
}

// 8. Server Handlers List Comprehensive
func TestBatch4_ServerHandlersList_Comprehensive(t *testing.T) {
	server := NewServer()
	tmpDir := t.TempDir()
	server.SetProjectRoot(tmpDir)
	ctx := context.Background()

	// 1. handleToolsList
	RegisterEchoTool(server)
	toolsList, err := server.handleToolsList(ctx, "tools/list", nil)
	if err != nil || toolsList == nil {
		t.Errorf("handleToolsList failed: %v, %v", err, toolsList)
	}

	// 2. handleResourcesList & handleResourcesGet
	// Create a real file in tmpDir so file:// reading works
	docPath := filepath.Join(tmpDir, "readme.txt")
	_ = fileutil.WriteSecureFile(docPath, []byte("Hello resource"))
	fileURI := "file://" + docPath
	server.RegisterResource(fileURI, "Readme", "Test file", "text/plain")

	resList, err := server.handleResourcesList(ctx, "resources/list", nil)
	if err != nil || resList == nil {
		t.Errorf("handleResourcesList failed: %v, %v", err, resList)
	}

	getRes, err := server.handleResourcesGet(ctx, "resources/read", json.RawMessage(`{"uri": "`+fileURI+`"}`))
	if err != nil || getRes == nil {
		t.Errorf("handleResourcesGet failed: %v, %v", err, getRes)
	}

	// 3. handlePromptsList & handlePromptsGet
	promptsList, err := server.handlePromptsList(ctx, "prompts/list", nil)
	if err != nil || promptsList == nil {
		t.Errorf("handlePromptsList failed: %v, %v", err, promptsList)
	}

	// Register a spec prompt to test handlePromptsGet
	gen := NewMCPSpecGenerator(server)
	_ = gen.RegisterPrompt(PromptSpec{
		Name:        "greeting",
		Description: "Greets someone",
		Template:    "Hello {{name}}",
	})
	promptRes, err := server.handlePromptsGet(ctx, "prompts/get", json.RawMessage(`{"name": "greeting", "arguments": {"name": "Alice"}}`))
	if err != nil || promptRes == nil {
		t.Errorf("handlePromptsGet failed: %v, %v", err, promptRes)
	}

	// 4. handleRootsList
	rootsList, err := server.handleRootsList(ctx, "roots/list", nil)
	if err != nil || rootsList == nil {
		t.Errorf("handleRootsList failed: %v, %v", err, rootsList)
	}
}

// 9. Tools Workflow Comprehensive
func TestBatch4_ToolsWorkflow_Comprehensive(t *testing.T) {
	server := NewServer()
	server.SetProjectRoot(t.TempDir())
	secCtx := pkgctx.NewSystemSecurityContext()
	RegisterWorkflowTools(server, secCtx)

	ctx := context.Background()

	// workflowExecContext
	execCtx, cancel := workflowExecContext(ctx, server, 5*time.Second)
	defer cancel()
	if execCtx == nil {
		t.Fatal("expected non-nil workflow exec context")
	}

	// HandleGetCurrentPriorityPlan
	_, _ = HandleGetCurrentPriorityPlan(ctx, server, map[string]any{})

	// HandleGetPriorityPlanItems
	_, _ = HandleGetPriorityPlanItems(ctx, server, map[string]any{"plan_id": "PRI-1"})
	_, _ = HandleGetPriorityPlanItems(ctx, server, map[string]any{})

	// HandleGetCurrentBacklogItem
	_, _ = HandleGetCurrentBacklogItem(ctx, server, map[string]any{})

	// HandleGetNextBacklogItem
	_, _ = HandleGetNextBacklogItem(ctx, server, map[string]any{"explore": false})
	_, _ = HandleGetNextBacklogItem(ctx, server, map[string]any{"explore": true})
}

// 10. Permission Format Helpers
func TestBatch4_PermissionFormatHelpers(t *testing.T) {
	server := NewServer()

	ops := server.getPermissionOperations()
	if len(ops) == 0 {
		t.Error("expected non-empty operations")
	}

	kinds := server.getObjectKindsForExamples()
	if len(kinds) == 0 {
		t.Error("expected non-empty kinds")
	}

	examples := server.generatePermissionFormatExamples()
	if len(examples) == 0 {
		t.Error("expected non-empty examples")
	}

	if min(3, 9) != 3 || min(10, 4) != 4 {
		t.Error("min calculation incorrect")
	}
	if capitalizeFirst("alpha") != "Alpha" || capitalizeFirst("") != "" {
		t.Error("capitalizeFirst failed")
	}
}
