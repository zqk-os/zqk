package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/interactive"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// 1. Interactive Handlers
func TestInteractiveHandlers_Comprehensive(t *testing.T) {
	ctx := pkgctx.NewSystemContext()
	server := NewServer()
	server.SetProjectRoot(t.TempDir())

	// Missing kind
	_, err := HandleCreateObjectInteractive(ctx, server, map[string]any{})
	if err == nil {
		t.Error("expected error for missing kind")
	}

	// Invalid session_id
	_, err = HandleCreateObjectInteractive(ctx, server, map[string]any{
		objects.FieldKeyKind:      "backlog_item",
		objects.FieldKeySessionID: "nonexistent-session-9999",
	})
	if err == nil {
		t.Error("expected error for nonexistent session_id")
	}

	// First call - new session creation (incomplete fields -> elicitation error)
	_, err = HandleCreateObjectInteractive(ctx, server, map[string]any{
		objects.FieldKeyKind: "backlog_item",
	})
	if err == nil {
		t.Fatal("expected elicitation error for incomplete fields")
	}
	elicErr, ok := err.(*ElicitationError)
	if !ok {
		t.Fatalf("expected ElicitationError, got %T: %v", err, err)
	}
	sessionID, ok := elicErr.Data[objects.FieldKeySessionID].(string)
	if !ok || sessionID == "" {
		t.Fatalf("expected session ID in elicitation error data, got %v", elicErr.Data)
	}

	// Second call - updating existing session with additional fields
	_, err = HandleCreateObjectInteractive(ctx, server, map[string]any{
		objects.FieldKeyKind:      "backlog_item",
		objects.FieldKeySessionID: sessionID,
		"title":                   "Test interactive backlog item",
		"description":             "Detailed description of the task",
	})
	// May still be incomplete or complete depending on schema, but exercises update path
	_ = err

	// Test functional helpers
	resApply, err := applyWithContext(ctx, "hello", func(s string) (string, error) {
		return s + "-world", nil
	}, "test_op")
	if err != nil || resApply != "hello-world" {
		t.Errorf("applyWithContext failed: %v, %s", err, resApply)
	}

	errDo := doWithContext(ctx, "target", func(s string) error {
		return nil
	}, "test_op")
	if errDo != nil {
		t.Errorf("doWithContext failed: %v", errDo)
	}

	resGet, err := getWithContext(ctx, func() (int, error) {
		return 123, nil
	}, "test_op")
	if err != nil || resGet != 123 {
		t.Errorf("getWithContext failed: %v, %d", err, resGet)
	}

	// Clean up session if still present
	interactive.GetGlobalInteractiveSessionManager().DeleteSession(sessionID)
}

// 2. Client Comprehensive
func TestMCPClient_Comprehensive(t *testing.T) {
	// Mock pipe connection
	serverReader, clientWriter := io.Pipe()
	clientReader, serverWriter := io.Pipe()

	client := NewClient(clientReader, clientWriter, NewDefaultTransport())
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	defer func() {
		_ = client.Close()
		_ = serverReader.Close()
		_ = clientWriter.Close()
		_ = clientReader.Close()
		_ = serverWriter.Close()
	}()

	client.SetTracker(nil)

	// Test mcpResponseTimeout
	t.Setenv(zqkenv.MCPTimeout().Name(), "5s")
	d := mcpResponseTimeout()
	if d != 5*time.Second {
		t.Errorf("expected 5s timeout, got %v", d)
	}

	t.Setenv(zqkenv.MCPTimeout().Name(), "10")
	d2 := mcpResponseTimeout()
	if d2 != 10*time.Second {
		t.Errorf("expected 10s timeout, got %v", d2)
	}

	// Simulate server handling initialize and tools/list in goroutine
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go func() {
		bufReader := bufio.NewReader(serverReader)
		transport := NewDefaultTransport()
		format := &MessageFormat{IsRawJSON: false}

		for {
			msg, _, err := transport.ReadMessage(bufReader)
			if err != nil {
				return
			}
			var req JSONRPCRequest
			if err := json.Unmarshal(msg, &req); err != nil {
				continue
			}

			switch req.Method {
			case "initialize":
				resp := JSONRPCResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Result: InitializeResult{
						ProtocolVersion: "2025-06-18",
						ServerInfo: struct {
							Name    string `json:"name"`
							Version string `json:"version"`
						}{Name: "mock-server", Version: "1.0"},
					},
				}
				respBytes, _ := json.Marshal(resp)
				_ = transport.WriteMessage(bufio.NewWriter(serverWriter), respBytes, format)
			case "notifications/initialized":
				// Notification, no reply
			case "tools/list":
				resp := JSONRPCResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Result: ToolsListResult{
						Tools: []Tool{{Name: "tool1", Description: "mock tool"}},
					},
				}
				respBytes, _ := json.Marshal(resp)
				_ = transport.WriteMessage(bufio.NewWriter(serverWriter), respBytes, format)
			case "tools/call":
				resp := JSONRPCResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Result: ToolCallResult{
						Content: []Content{{Type: "text", Text: "executed"}},
					},
				}
				respBytes, _ := json.Marshal(resp)
				_ = transport.WriteMessage(bufio.NewWriter(serverWriter), respBytes, format)
			}
		}
	}()

	initRes, err := client.Initialize(ctx, InitializeParams{
		ProtocolVersion: "2025-06-18",
	})
	if err != nil {
		t.Fatalf("client.Initialize failed: %v", err)
	}
	if initRes == nil || initRes.ServerInfo.Name != "mock-server" {
		t.Errorf("unexpected InitializeResult: %v", initRes)
	}

	err = client.Initialized(ctx)
	if err != nil {
		t.Fatalf("client.Initialized failed: %v", err)
	}

	toolsRes, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("client.ListTools failed: %v", err)
	}
	if len(toolsRes.Tools) != 1 || toolsRes.Tools[0].Name != "tool1" {
		t.Errorf("unexpected tools: %v", toolsRes)
	}

	callRes, err := client.CallTool(ctx, "tool1", map[string]any{})
	if err != nil {
		t.Fatalf("client.CallTool failed: %v", err)
	}
	if len(callRes) == 0 {
		t.Errorf("unexpected empty call result")
	}
}

// 3. Server Trace Comprehensive
func TestServerTrace_Comprehensive(t *testing.T) {
	server := NewServer()

	// getTraceWriter when nil
	w := server.getTraceWriter()
	if w != nil {
		t.Errorf("expected nil trace writer initially, got %v", w)
	}

	// traceLogf when traceWriter is nil - error and warn fallback
	server.traceLogf("[MCP_ERROR] test error message: %s", "details")
	server.traceLogf("[MCP_WARN] test warn message: %s", "details")
	server.traceLogf("[MCP_INFO] test info message: %s", "suppressed")

	// writeTraceMessage
	var buf bytes.Buffer
	writeTraceMessage(&buf, "trace line")
	if !strings.Contains(buf.String(), "trace line") {
		t.Errorf("expected trace line in buffer, got %s", buf.String())
	}
	writeTraceMessage(nil, "noop")

	// Set traceWriter to buffer
	server.traceWriter = &buf
	server.traceLogf("[MCP_INFO] buffered message: %d", 42)
	if !strings.Contains(buf.String(), "buffered message: 42") {
		t.Errorf("expected buffered message, got %s", buf.String())
	}

	// Set traceWriter to os.Stderr
	server.traceWriter = os.Stderr
	server.traceLogf("[MCP_DEBUG] suppressed debug message")
	server.traceLogf("[MCP_INFO] suppressed info message")

	// isMCPTraceEnabled
	t.Setenv(zqkenv.MCPTrace().Name(), "false")
	if isMCPTraceEnabled(nil) {
		t.Error("expected trace disabled for false")
	}
	t.Setenv(zqkenv.MCPTrace().Name(), "0")
	if isMCPTraceEnabled(nil) {
		t.Error("expected trace disabled for 0")
	}
	t.Setenv(zqkenv.MCPTrace().Name(), "off")
	if isMCPTraceEnabled(nil) {
		t.Error("expected trace disabled for off")
	}
	t.Setenv(zqkenv.MCPTrace().Name(), "1")
	if !isMCPTraceEnabled(nil) {
		t.Error("expected trace enabled for 1")
	}
	t.Setenv(zqkenv.MCPTrace().Name(), "")
	cfg := &ServerConfig{}
	cfg.MCPServer.Trace.Enabled = true
	if !isMCPTraceEnabled(cfg) {
		t.Error("expected trace enabled from config")
	}

	// logTraceError
	logTraceError("test trace error", errors.New("err"), "key1", "val1", "key2", "val2")

	// rollingTraceConfigFromServerConfig
	rc, enabled := rollingTraceConfigFromServerConfig(nil)
	if !enabled || rc.MaxSize <= 0 || rc.MaxFiles <= 0 {
		t.Errorf("expected default rolling config, got enabled=%v, %+v", enabled, rc)
	}

	falseVal := false
	cfgDisabled := &ServerConfig{}
	cfgDisabled.MCPServer.Trace.Rolling.Enabled = &falseVal
	_, enabled2 := rollingTraceConfigFromServerConfig(cfgDisabled)
	if enabled2 {
		t.Error("expected rolling trace disabled")
	}

	// openTraceWriter
	tmpDir := t.TempDir()
	tracePath := filepath.Join(tmpDir, "test-trace.log")
	cfgTrace := &ServerConfig{}
	cfgTrace.MCPServer.Trace.File = tracePath
	cfgTrace.MCPServer.Trace.Rolling.Enabled = &falseVal
	tw, closer := openTraceWriter(cfgTrace, tmpDir)
	if tw == nil || closer == nil {
		t.Fatal("expected valid trace writer")
	}
	_ = closer.Close()

	// openClientSpecificTraceWriter
	server.SetProjectRoot(tmpDir)
	cWriter, cCloser := server.openClientSpecificTraceWriter("test-client@role-1", cfgTrace, tmpDir)
	if cWriter == nil || cCloser == nil {
		t.Fatal("expected valid client trace writer")
	}
	_ = cCloser.Close()

	// switchToClientSpecificTrace
	server.switchToClientSpecificTrace("agent-xyz")
	if server.getTraceWriter() == nil {
		t.Error("expected non-nil trace writer after switch")
	}
}

// 4. CLI Bridge Command Execution Comprehensive
func TestCLIBridgeCommandExecution_Comprehensive(t *testing.T) {
	// extractCommandPath
	p1, err := extractCommandPath(map[string]any{"_command_path": "object list"})
	if err != nil || p1 != "object list" {
		t.Errorf("unexpected path: %v, %s", err, p1)
	}
	p2, err := extractCommandPath(map[string]any{"command": "system", "subcommand": "status"})
	if err != nil || p2 != "system status" {
		t.Errorf("unexpected path: %v, %s", err, p2)
	}
	_, err = extractCommandPath(map[string]any{})
	if err == nil {
		t.Error("expected error for missing path")
	}

	// buildCommandArguments & flags
	argsMap := map[string]any{
		"format":  "json",
		"verbose": true,
		"count":   10,
		"tags":    []any{"tag1", "tag2"},
	}
	server := NewServer()
	cmdArgs := buildCommandArguments("object list", argsMap, server)
	if len(cmdArgs) == 0 || cmdArgs[0] != "object" || cmdArgs[1] != "list" {
		t.Errorf("unexpected cmdArgs: %v", cmdArgs)
	}

	flags := extractFlags(argsMap, server)
	if len(flags) == 0 {
		t.Error("expected extracted flags")
	}

	posArgs := extractPositionalArguments(map[string]any{
		"kind": "backlog_item",
	}, server)
	if len(posArgs) != 1 || posArgs[0] != "backlog_item" {
		t.Errorf("unexpected positional args: %v", posArgs)
	}

	idx := findFlagStartIndex([]string{"object", "list", "--format", "json"})
	if idx != 2 {
		t.Errorf("expected flag start index 2, got %d", idx)
	}
	idxNone := findFlagStartIndex([]string{"object", "list"})
	if idxNone != 2 {
		t.Errorf("expected flag start index 2 for no flags, got %d", idxNone)
	}

	// buildCommandEnvironment
	secCtx := &pkgctx.SecurityContext{AccountID: "ACC-1", Roles: []string{"dev"}}
	env := buildCommandEnvironment(secCtx, t.TempDir(), 1234, server)
	if len(env) == 0 {
		t.Error("expected environment variables")
	}

	// filterCommandOutput, filterFallbackLogMessages, filterDebugLogObjects
	rawOut := "Warning: fallback\n{\"id\":\"OBJ-1\"}\n[DEBUG] some debug info\n"
	filtered := filterCommandOutput(rawOut)
	if filtered == "" {
		t.Error("expected non-empty filtered output")
	}

	jsonObj := extractFirstCompleteJSONObject("{\"valid\": true}\n{\"second\": 1}")
	if jsonObj != "{\"valid\": true}" {
		t.Errorf("unexpected extracted JSON: %s", jsonObj)
	}

	// parseCommandOutput
	parsedValid := parseCommandOutput("{\"key\": \"value\"}", bytes.Buffer{})
	if parsedValid["key"] != "value" {
		t.Errorf("unexpected parsed valid: %v", parsedValid)
	}

	parsedErr := parseCommandOutput("not json", *bytes.NewBufferString("err on stderr"))
	if parsedErr["success"] != false {
		t.Errorf("expected success=false on invalid JSON, got %v", parsedErr)
	}

	// buildParseErrorResult
	pErr := buildParseErrorResult("bad output", *bytes.NewBufferString("stderr"), errors.New("parse failed"))
	if pErr["success"] != false {
		t.Errorf("unexpected parse error result: %v", pErr)
	}

	// buildCommandResult & BuildCLICommandResultFromOutput
	res1 := buildCommandResult(map[string]any{"data": 1}, "object list", []string{"object", "list"}, bytes.Buffer{}, bytes.Buffer{}, nil)
	if res1["data"] != 1 {
		t.Errorf("unexpected buildCommandResult: %v", res1)
	}

	res2 := BuildCLICommandResultFromOutput("system status", []string{"system", "status"}, "{\"status\": \"ok\"}", "", nil)
	if res2["status"] != "ok" {
		t.Errorf("unexpected BuildCLICommandResultFromOutput: %v", res2)
	}
}

// 5. Client Metrics Extended
func TestClientMetrics_Extended(t *testing.T) {
	tmpDir := t.TempDir()
	metricsPath := filepath.Join(tmpDir, "client_metrics.json")

	store, err := NewClientMetricsStore(metricsPath, context.Background())
	if err != nil {
		t.Fatalf("NewClientMetricsStore failed: %v", err)
	}
	defer store.Close()

	// Record events
	_ = store.RecordEvent("seq-1", "client-1", "init", map[string]any{"duration": 100})
	_ = store.RecordEvent("seq-1", "client-1", "tool_call", map[string]any{"tool": "echo"})
	_ = store.RecordEvent("seq-2", "client-2", "init", map[string]any{"duration": 200})

	// GetSequenceMetrics
	m1, err := store.GetSequenceMetrics("seq-1")
	if err != nil || m1 == nil || m1.ClientID != "client-1" {
		t.Errorf("unexpected sequence metrics: %v, %v", err, m1)
	}

	// Nonexistent sequence
	_, err = store.GetSequenceMetrics("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent sequence")
	}

	// GetAllMetrics
	all, err := store.GetAllMetrics()
	if err != nil || len(all) != 2 {
		t.Errorf("unexpected GetAllMetrics: %v, count=%d", err, len(all))
	}

	// GetMetricsByClientID
	byClient, err := store.GetMetricsByClientID("client-1")
	if err != nil || len(byClient) != 1 {
		t.Errorf("unexpected GetMetricsByClientID: %v, count=%d", err, len(byClient))
	}

	// UpdateClientID
	err = store.UpdateClientID("seq-1", "client-1-updated")
	if err != nil {
		t.Errorf("UpdateClientID failed: %v", err)
	}

	// Save and Load
	err = store.Save()
	if err != nil {
		t.Errorf("Save failed: %v", err)
	}

	err = store.Load()
	if err != nil {
		t.Errorf("Load failed: %v", err)
	}

	// CompressMetrics
	err = store.CompressMetrics(1 * time.Hour)
	if err != nil {
		t.Errorf("CompressMetrics failed: %v", err)
	}

	// Periodic compression ticker
	ctx, cancel := context.WithCancel(context.Background())
	ticker := store.StartPeriodicCompression(ctx, 100*time.Millisecond, 1*time.Hour)
	time.Sleep(50 * time.Millisecond)
	cancel()
	ticker.Stop()

	// Clone functions
	clonedSeq := cloneClientSequenceMetrics(m1)
	if clonedSeq.SequenceID != m1.SequenceID {
		t.Errorf("clone failed: %v", clonedSeq)
	}
	clonedMap := cloneClientSequenceMetricsMap(all)
	if len(clonedMap) != len(all) {
		t.Errorf("clone map failed: %v", clonedMap)
	}
}

type testCoordinatorTimeoutManager struct {
	ctx context.Context
}

func (m *testCoordinatorTimeoutManager) GetServerContext() context.Context {
	return m.ctx
}

func (m *testCoordinatorTimeoutManager) ResetTimeout() {}

// 6. Serve Coordinator Extended
func TestServeCoordinator_Extended(t *testing.T) {
	server := NewServer()
	proc := NewMessageProcessor(server, nil, NewDefaultTransport())
	builder := NewServerLifecycleBuilder(server)
	sc := NewServeCoordinator(server, proc, builder)

	validCtx, validCancel := context.WithCancel(context.Background())
	defer validCancel()
	sc.SetTimeoutManager(&testCoordinatorTimeoutManager{ctx: validCtx})
	iters, timeouts := sc.GetServeCoordinatorStats()
	if iters != 0 || timeouts != 0 {
		t.Errorf("unexpected stats: %d, %d", iters, timeouts)
	}

	// getValidServerContext
	sCtx := sc.getValidServerContext(false, nil)
	if sCtx == nil {
		t.Error("expected non-nil server context")
	}

	// getTimeoutChannel
	cancelCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := sc.getTimeoutChannel(cancelCtx)
	if ch == nil {
		t.Error("expected non-nil timeout channel")
	}

	// log methods and reset methods
	sc.logWaitingState()
	sc.logReadResult(nil)
	sc.logReadResult(errors.New("simulated error"))
	sc.resetServerState()
	sc.resetTraceWriter()

	// getTimeoutDuration
	td := sc.getTimeoutDuration()
	if td == "" {
		t.Error("expected non-empty timeout duration")
	}

	// handleClientDisconnect, handleContextError, handleExplicitShutdown
	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	_ = sc.handleClientDisconnect(writer)
	_ = sc.handleContextError(context.Canceled, writer, false, nil)
	_ = sc.handleExplicitShutdown()
	_ = sc.endConnectionOnly("test reason", writer)
}

// 7. Server Init Helpers Extended
func TestServerInitHelpers_Extended(t *testing.T) {
	server := NewServer()
	server.SetProjectRoot(t.TempDir())

	// extractClientInfoFromInitParams
	initParams := InitializeParams{
		ProtocolVersion: "2025-06-18",
		Capabilities: map[string]any{
			clientInfoAccountID:      "ACC-123",
			objects.FieldKeyUsername: "agent_tester",
		},
	}
	initParams.ClientInfo.Name = "claude-desktop"
	initParams.ClientInfo.Version = "1.2.3"

	cInfo := extractClientInfoFromInitParams(initParams)
	if cInfo[clientInfoAccountID] != "ACC-123" || cInfo[clientInfoName] != "claude-desktop" {
		t.Errorf("unexpected extracted info: %v", cInfo)
	}

	// resolveAccountIDFromRegistry
	server.config = createTestServerConfigForRegistry(false, map[string]AgentConfig{
		"claude-desktop": {AccountID: "ACC-123"},
	})
	accID := server.resolveAccountIDFromRegistry("claude-desktop", cInfo)
	if accID != "ACC-123" {
		t.Errorf("expected ACC-123, got %s", accID)
	}

	// setupClientID
	clientID := server.setupClientID(initParams)
	if clientID == "" {
		t.Error("expected non-empty client ID")
	}

	// recordConnectionEvent
	server.recordConnectionEvent("seq-conn-1", clientID, initParams)

	// enabledAuthStrategiesOrDefault
	strats := server.enabledAuthStrategiesOrDefault(context.Background(), clientID)
	if len(strats) == 0 {
		t.Error("expected default auth strategies")
	}

	// configureClientCapabilities
	server.configureClientCapabilities(initParams)

	// prepareReinitialization
	server.prepareReinitialization()

	// finalizeInitialization
	res, err := server.finalizeInitialization(clientID, "seq-1", initParams)
	if err != nil || res.ProtocolVersion == "" {
		t.Errorf("finalizeInitialization failed: %v, %v", err, res)
	}
}

// 8. Server Handlers Auth Extended
func TestServerHandlersAuth_Extended(t *testing.T) {
	server := NewServer()
	tmpDir := t.TempDir()
	server.SetProjectRoot(tmpDir)

	// hasCredentials
	if hasCredentials(map[string]any{}) {
		t.Error("expected false for empty map")
	}
	if !hasCredentials(map[string]any{"password": "secret"}) {
		t.Error("expected true when password present")
	}
	if !hasCredentials(map[string]any{clientInfoKeystoreKeyID: "key-1"}) {
		t.Error("expected true when key_id present")
	}
	if !hasCredentials(map[string]any{clientInfoOAuthTok: "tok-1"}) {
		t.Error("expected true when token present")
	}
	if !hasCredentials(map[string]any{clientInfoPersonalAccessToken: "pat-1"}) {
		t.Error("expected true when pat present")
	}

	// extractRolesFromAccount
	acc := map[string]any{
		objects.FieldKeyRoles: []any{"admin", "developer"},
	}
	roles := extractRolesFromAccount(acc)
	if len(roles) != 2 || roles[0] != "admin" {
		t.Errorf("unexpected roles: %v", roles)
	}

	// extractPermissionsFromAccount
	accPerm := map[string]any{
		objects.FieldKeyPermissions: []any{"read:*", "write:*"},
	}
	perms := extractPermissionsFromAccount(accPerm)
	if len(perms) != 2 || perms[0] != "read:*" {
		t.Errorf("unexpected perms: %v", perms)
	}

	// isStrategyStatusActive
	_ = server.isStrategyStatusActive(objects.ObjectStatusActive)
	if server.isStrategyStatusActive("unknown_status_never_exists") {
		t.Error("expected unknown status not to be active")
	}

	// normalizeCredentialHashForPAT
	norm := normalizeCredentialHashForPAT("pat_hash_123")
	if norm != "pat_hash_123" {
		t.Errorf("unexpected norm: %s", norm)
	}

	// validateUsernamePassword on missing accounts dir
	_, _, _, err := server.validateUsernamePassword(context.Background(), "user", "pass", tmpDir)
	if err == nil {
		t.Error("expected error for missing accounts dir")
	}

	// validateKeystoreKey on missing keystore
	_, _, _, err = server.validateKeystoreKey(context.Background(), "key-1", tmpDir)
	if err == nil {
		t.Error("expected error for missing keystore")
	}

	// validateOAuthToken on missing keystore
	_, _, _, err = server.validateOAuthToken(context.Background(), "tok-1", tmpDir)
	if err == nil {
		t.Error("expected error for missing oauth keystore")
	}

	// validatePersonalAccessToken on missing keystore
	_, _, _, err = server.validatePersonalAccessToken(context.Background(), "pat-1", tmpDir)
	if err == nil {
		t.Error("expected error for missing pat keystore")
	}

	// loadAccountObject missing
	_, err = server.loadAccountObject("ACC-NONEXISTENT", tmpDir)
	if err == nil {
		t.Error("expected error for nonexistent account")
	}

	// loadAccountByEmail missing
	_, err = server.loadAccountByEmail("test@example.com", tmpDir)
	if err == nil {
		t.Error("expected error for nonexistent account by email")
	}
}

// 9. Role Prompt Renderer Comprehensive
func TestRolePromptRenderer_Comprehensive(t *testing.T) {
	pCtx := &ProjectContext{
		Mission: &MissionContext{ID: "M-1", Title: "Mission 1"},
	}
	promptGen := NewRoleAwarePromptGenerator(pCtx, []string{"viewer"}).
		WithStorageProvider(&testStorageProvider{})
	guidanceGen := NewRoleGuidanceGenerator(&testStorageProvider{})
	secCtx := &pkgctx.SecurityContext{Roles: []string{"viewer"}}

	renderer := NewRolePromptRenderer(guidanceGen, promptGen, secCtx)
	if renderer == nil {
		t.Fatal("expected non-nil renderer")
	}

	// GetStandardPromptNames
	names := GetStandardPromptNames()
	if len(names) == 0 {
		t.Error("expected non-empty standard prompt names")
	}

	// RenderPrompt
	_, _ = renderer.RenderPrompt(context.Background(), "big_picture")
	_, _ = renderer.RenderPrompt(context.Background(), "execution_context")
	_, _ = renderer.RenderPrompt(context.Background(), "my_role")
	_, found := renderer.RenderPrompt(context.Background(), "nonexistent_prompt_name")
	if found {
		t.Error("expected nonexistent prompt not to be found")
	}

	// substituteToolNames
	sub := renderer.substituteToolNames("Run {tool:create_object} now")
	if sub == "" {
		t.Error("expected non-empty substitution")
	}
}

// 10. MCP Spec Generator & Builder Comprehensive
func TestMCPSpecGenerator_Comprehensive(t *testing.T) {
	server := NewServer()
	gen := NewMCPSpecGenerator(server)
	if gen == nil {
		t.Fatal("expected non-nil spec generator")
	}

	// Register prompt spec
	promptSpec := PromptSpec{
		Name:        "custom_prompt",
		Description: "Custom prompt description",
		Template:    "Hello {{name}}",
		Arguments: []PromptArgSpec{
			{Name: "name", Description: "user name", Required: true},
		},
	}
	err := gen.RegisterPrompt(promptSpec)
	if err != nil {
		t.Errorf("RegisterPrompt failed: %v", err)
	}

	// Register resource spec
	resSpec := ResourceSpec{
		URI:         "spec://custom_resource",
		Name:        "Custom Resource",
		Description: "Custom resource description",
		MimeType:    "text/plain",
	}
	err = gen.registerResource(resSpec)
	if err != nil {
		t.Errorf("registerResource failed: %v", err)
	}

	// Register schema handler spec
	shSpec := SchemaHandlerSpec{
		URIPattern: "schema://custom_sh",
		Handler:    "handleSchemaRegistry",
	}
	err = gen.registerSchemaHandler(shSpec)
	if err != nil {
		t.Errorf("registerSchemaHandler failed: %v", err)
	}

	// buildInputSchema
	toolSpec := ToolSpec{
		Name:        "custom_tool",
		Description: "Tool description",
		Properties: map[string]PropertySpec{
			"param1": {Type: "string", Description: "p1 desc"},
			"param2": {Type: "integer", Description: "p2 desc"},
		},
		Required: []string{"param1"},
	}
	schema := gen.buildInputSchema(toolSpec)
	if schema["type"] != "object" {
		t.Errorf("unexpected input schema: %v", schema)
	}

	// GenerateFromSpec
	fullSpec := &MCPSpec{
		Name:           "test-spec",
		Prompts:        []PromptSpec{promptSpec},
		Resources:      []ResourceSpec{resSpec},
		SchemaHandlers: []SchemaHandlerSpec{shSpec},
	}
	err = gen.GenerateFromSpec(fullSpec)
	if err != nil {
		t.Errorf("GenerateFromSpec failed: %v", err)
	}

	err = gen.GenerateFromSpecs([]*MCPSpec{fullSpec})
	if err != nil {
		t.Errorf("GenerateFromSpecs failed: %v", err)
	}

	// MCPSpecLoader
	loader := NewMCPSpecLoader()
	if loader == nil {
		t.Fatal("expected non-nil spec loader")
	}

	tmpDir := t.TempDir()
	specYAML := `name: test-spec
prompts:
  - name: test_spec_prompt
    description: Test
    template: Template text
`
	specFile := filepath.Join(tmpDir, "spec.yaml")
	_ = fileutil.WriteSecureFile(specFile, []byte(specYAML))

	loaded, err := loader.LoadSpec(specFile)
	if err != nil || loaded == nil || len(loaded.Prompts) != 1 {
		t.Errorf("LoadSpec failed: %v, %v", err, loaded)
	}

	specs, err := loader.LoadSpecs(tmpDir)
	if err != nil || len(specs) != 1 {
		t.Errorf("LoadSpecs directory failed: %v, count=%d", err, len(specs))
	}
}

// 11. Tools Workflow Comprehensive
func TestToolsWorkflow_Comprehensive(t *testing.T) {
	secCtx := pkgctx.NewSystemSecurityContext()

	// IsStudioPackToolsEnabled
	enabled := IsStudioPackToolsEnabled(secCtx)
	_ = enabled

	server := NewServer()
	RegisterWorkflowTools(server, secCtx)

	// workflowExecContext
	wCtx, cancel := workflowExecContext(context.Background(), server, 1*time.Second)
	if wCtx == nil {
		t.Error("expected non-nil workflow context")
	}
	cancel()

	// HandleGetCurrentPriorityPlan with empty server
	_, _ = HandleGetCurrentPriorityPlan(context.Background(), server, map[string]any{})
	_, _ = HandleGetPriorityPlanItems(context.Background(), server, map[string]any{})
	_, _ = HandleGetCurrentBacklogItem(context.Background(), server, map[string]any{})
	_, _ = HandleGetNextBacklogItem(context.Background(), server, map[string]any{})

	// extractWhatsNextLeadPlan
	lead, ok, err := extractWhatsNextLeadPlan(map[string]any{
		"priority_plan": map[string]any{"id": "PRI-1", "title": "Plan 1"},
	})
	if err != nil || !ok || lead == nil {
		t.Errorf("extractWhatsNextLeadPlan failed: %v, %v, %v", err, ok, lead)
	}

	// extractWorkflowResultItem
	item, ok, err := extractWorkflowResultItem(map[string]any{
		"objects": []any{map[string]any{"id": "BLI-1"}},
	})
	if err != nil || !ok || item == nil {
		t.Errorf("extractWorkflowResultItem failed: %v, %v, %v", err, ok, item)
	}
}

// 12. Message Processor Extended
func TestMessageProcessor_Extended(t *testing.T) {
	server := NewServer()
	router := NewMethodRouter()
	router.RegisterFunc("test/method", func(ctx context.Context, method string, params json.RawMessage) (any, error) {
		return map[string]any{"status": "ok"}, nil
	})
	proc := NewMessageProcessor(server, router, NewDefaultTransport())

	// isBrokenPipeError
	if !isBrokenPipeError(errors.New("broken pipe")) {
		t.Error("expected true for broken pipe")
	}
	if !isBrokenPipeError(errors.New("connection reset by peer")) {
		t.Error("expected true for connection reset")
	}
	if isBrokenPipeError(errors.New("regular error")) {
		t.Error("expected false for regular error")
	}

	// buildResponse
	req := &JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "test/method"}
	resp := proc.buildResponse(req, map[string]any{"result": "val"}, nil)
	if resp == nil || resp.ID != 1 || resp.Error != nil {
		t.Errorf("unexpected buildResponse: %v", resp)
	}

	errResp := proc.buildResponse(req, nil, errors.New("something went wrong"))
	if errResp == nil || errResp.Error == nil || errResp.Error.Code != InternalError {
		t.Errorf("unexpected error response: %v", errResp)
	}

	// handleNotification
	notifReq := &JSONRPCRequest{JSONRPC: "2.0", Method: "notifications/test"}
	err := proc.handleNotification(context.Background(), notifReq)
	_ = err // Unregistered notification returns error or nil, exercises code path

	// handleParseError
	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	format := &MessageFormat{IsRawJSON: false}
	err = proc.handleParseError(errors.New("parse failed"), format, writer)
	if err != nil {
		t.Errorf("handleParseError returned error: %v", err)
	}
	_ = writer.Flush()
	if buf.Len() == 0 {
		t.Error("expected error response written to buffer")
	}

	// GetMessageProcessorStats
	pCount, eCount := proc.GetMessageProcessorStats()
	if pCount < 0 || eCount < 0 {
		t.Errorf("unexpected stats: %d, %d", pCount, eCount)
	}
}

// 13. Handler & Metrics Router Comprehensive
func TestHandlerAndMetricsRouter_Comprehensive(t *testing.T) {
	router := NewMethodRouter()
	router.RegisterFunc("ping", func(ctx context.Context, method string, params json.RawMessage) (any, error) {
		return "pong", nil
	})

	res, err := router.Handle(context.Background(), "ping", nil)
	if err != nil || res != "pong" {
		t.Errorf("Handle ping failed: %v, %v", err, res)
	}

	// Unregistered method without default handler
	_, err = router.Handle(context.Background(), "unknown_method", nil)
	if err == nil {
		t.Error("expected method not found error")
	}

	// With default handler
	router.SetDefaultHandler(HandlerFunc(func(ctx context.Context, method string, params json.RawMessage) (any, error) {
		return "default", nil
	}))
	resDef, err := router.Handle(context.Background(), "unknown_method", nil)
	if err != nil || resDef != "default" {
		t.Errorf("expected default handler result: %v, %v", err, resDef)
	}

	// Middleware Chain
	mw1 := func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, method string, params json.RawMessage) (any, error) {
			return next.Handle(ctx, method, params)
		})
	}
	chained := Chain(mw1)
	if chained == nil {
		t.Fatal("expected non-nil chained middleware")
	}

	// TraceMiddleware
	var traceBuf bytes.Buffer
	tmw := TraceMiddleware(&traceBuf)
	wrappedHandler := tmw(router)
	_, _ = wrappedHandler.Handle(context.Background(), "ping", nil)

	// MCPMetricsRouter & Context builders
	metrics := NewMCPMetrics()
	mr := NewMCPMetricsRouter(metrics)
	if mr == nil {
		t.Fatal("expected non-nil metrics router")
	}

	evt1 := BuildMCPEventContext("op-1", "mcp_initialize", "complete", 10*time.Millisecond, nil)
	if evt1.GetOperationID() != "op-1" || evt1.GetOperationType() != "mcp_initialize" || evt1.GetStatus() != "complete" {
		t.Errorf("unexpected evt1: %+v", evt1)
	}
	if evt1.GetDuration() != 10*time.Millisecond || evt1.GetError() != nil {
		t.Errorf("unexpected duration/err: %v, %v", evt1.GetDuration(), evt1.GetError())
	}
	_ = evt1.GetEventData()

	err = mr.Emit(context.Background(), evt1)
	if err != nil {
		t.Errorf("mr.Emit evt1 failed: %v", err)
	}

	evtTool := BuildMCPToolCallEventContext("op-2", "echo", 5*time.Millisecond, nil)
	_ = mr.Emit(context.Background(), evtTool)

	evtBatch := BuildMCPBatchToolCallEventContext("op-3", 5, 0, 20*time.Millisecond)
	_ = mr.Emit(context.Background(), evtBatch)

	evtQueue := BuildMCPQueueMetricsEventContext(10, 0, 100, 1)
	_ = mr.Emit(context.Background(), evtQueue)

	// Helper getInt64
	val, ok := getInt64(map[string]any{"num": int64(42)}, "num")
	if !ok || val != 42 {
		t.Errorf("getInt64 int64 failed: %v, %d", ok, val)
	}
	valInt, ok := getInt64(map[string]any{"num": 42}, "num")
	if !ok || valInt != 42 {
		t.Errorf("getInt64 int failed: %v, %d", ok, valInt)
	}
	valFloat, ok := getInt64(map[string]any{"num": float64(42)}, "num")
	if !ok || valFloat != 42 {
		t.Errorf("getInt64 float64 failed: %v, %d", ok, valFloat)
	}
	_, okNone := getInt64(map[string]any{}, "missing")
	if okNone {
		t.Error("expected false for missing key")
	}
}

// 14. Server Lifecycle Builder Comprehensive
func TestServerLifecycleBuilder_Comprehensive(t *testing.T) {
	server := NewServer()
	var inBuf, outBuf bytes.Buffer

	builder := NewServerLifecycleBuilder(server).
		WithReaderAndWriter(&inBuf, &outBuf).
		LoadConfig().
		ApplyAsyncConfig().
		ApplyEventEmitterConfig().
		ApplyRateLimitConfig().
		InitializeClientMetrics().
		InitializeTraceLogging().
		MarkServing().
		LoadMCPSpecs().
		SetupTransport().
		SetupHandlers().
		Build()

	if builder.GetReader() == nil {
		t.Error("expected non-nil reader")
	}
	if builder.GetWriter() == nil {
		t.Error("expected non-nil writer")
	}
	if builder.GetTransport() == nil {
		t.Error("expected non-nil transport")
	}
	if builder.GetHandler() == nil {
		t.Error("expected non-nil handler")
	}
	if builder.GetBaseContext() == nil {
		t.Error("expected non-nil base context")
	}
	_ = builder.IsTraceEnabled()
	_ = builder.GetTraceWriter()
	_ = builder.GetConfig()

	builder.Cleanup()
}

// 15. Server TCP Comprehensive
func TestServerTCP_Comprehensive(t *testing.T) {
	if !IsLoopbackAddr("127.0.0.1:8080") {
		t.Error("expected 127.0.0.1:8080 to be loopback")
	}
	if !IsLoopbackAddr("localhost:8080") {
		t.Error("expected localhost:8080 to be loopback")
	}
	if !IsLoopbackAddr("[::1]:8080") {
		t.Error("expected [::1]:8080 to be loopback")
	}
	if IsLoopbackAddr("192.168.1.50:8080") {
		t.Error("expected 192.168.1.50:8080 not to be loopback")
	}
	if IsLoopbackAddr("example.com:80") {
		t.Error("expected example.com:80 not to be loopback")
	}

	server := NewServer()
	err := server.ServeTLS("127.0.0.1:0", "nonexistent.cert", "nonexistent.key")
	if err == nil {
		t.Error("expected error for nonexistent cert/key")
	}

	err = server.ServeMTLS("127.0.0.1:0", "nonexistent.cert", "nonexistent.key", "nonexistent.ca")
	if err == nil {
		t.Error("expected error for nonexistent mTLS certs")
	}
}

// 16. Proxy Daemon Extended
func TestProxyDaemon_Extended(t *testing.T) {
	// isConnRefused
	if isConnRefused(nil) {
		t.Error("expected false for nil error")
	}
	if !isConnRefused(errors.New("connection refused")) {
		t.Error("expected true for connection refused")
	}
	if isConnRefused(errors.New("other error")) {
		t.Error("expected false for other error")
	}

	// StampIDEInitializeClientInfo & stampIDEInitializeClientInfo
	rawInit := []byte(`{"jsonrpc":"2.0","method":"initialize","params":{"clientInfo":{"name":"test"}}}`)
	stamped := StampIDEInitializeClientInfo(rawInit)
	if len(stamped) == 0 || !strings.Contains(string(stamped), "test") {
		t.Errorf("unexpected stamped result: %s", string(stamped))
	}

	notJSON := []byte("not-json-content")
	stampedNotJSON := stampIDEInitializeClientInfo(notJSON)
	if string(stampedNotJSON) != string(notJSON) {
		t.Errorf("expected unchanged message for non-json, got %s", string(stampedNotJSON))
	}

	humanClientInit := []byte(`{"params":{"clientInfo":{"name":"vscode"}}}`)
	stampedHuman := stampIDEInitializeClientInfo(humanClientInit)
	if string(stampedHuman) != string(humanClientInit) {
		t.Errorf("expected unchanged message for human client, got %s", string(stampedHuman))
	}

	// ProxyDaemon replyDaemonUnavailable & methods
	p := NewProxyDaemon("127.0.0.1:9999", logging.GetLoggerFromProfile(""))
	p.ensureTransports()

	format := &MessageFormat{IsRawJSON: false}
	p.replyDaemonUnavailable([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`), format)

	// QueryDiagnostics & QueryEventsSubscriberCount
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, _ = p.QueryDiagnostics(ctx)
	_, _ = p.QueryEventsSubscriberCount(ctx)
	_ = p.PublishDaemonEvent(ctx, "test event", "agent-1", "evt-1")

	// Start with immediate cancel
	ctxStart, cancelStart := context.WithCancel(context.Background())
	cancelStart()
	_ = p.Start(ctxStart)
}
