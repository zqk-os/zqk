package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestMCPClient_LifecycleAndTransports exercises client communication, timeout, and cancellation.
func TestMCPClient_LifecycleAndTransports(t *testing.T) {
	t.Parallel()

	t.Run("send_request_and_wait_response", func(t *testing.T) {
		t.Parallel()
		in := bytes.NewBuffer(nil)
		out := bytes.NewBuffer(nil)
		mockResp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      101,
			Result:  json.RawMessage(`{"status":"ok"}`),
		}
		respBytes, _ := json.Marshal(mockResp)
		transport := &mockTransport{readResp: respBytes}

		client := NewClient(in, out, transport)
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		reqID, respCh := client.registerPendingRequest()
		if reqID <= 0 {
			t.Fatalf("expected positive request ID, got %d", reqID)
		}

		req := JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      reqID,
			Method:  "test/method",
		}
		rawReq, _ := json.Marshal(req)
		err := client.sendRequest(ctx, "test", "test_desc", reqID, rawReq)
		if err != nil {
			t.Fatalf("sendRequest failed: %v", err)
		}

		// Inject response to pending channel
		respCh <- mockResp

		resp, err := client.waitResponse(ctx, reqID, respCh, time.Second)
		if err != nil {
			t.Fatalf("waitResponse failed: %v", err)
		}
		if resp == nil || resp.ID != 101 {
			t.Errorf("unexpected response: %+v", resp)
		}
	})

	t.Run("wait_response_timeout", func(t *testing.T) {
		t.Parallel()
		client := NewClient(bytes.NewBuffer(nil), bytes.NewBuffer(nil), NewDefaultTransport())
		defer client.Close()

		respCh := make(chan JSONRPCResponse)
		ctx := context.Background()

		_, err := client.waitResponse(ctx, 999, respCh, 10*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "timeout") {
			t.Errorf("expected timeout error, got: %v", err)
		}
	})

	t.Run("wait_response_context_cancelled", func(t *testing.T) {
		t.Parallel()
		client := NewClient(bytes.NewBuffer(nil), bytes.NewBuffer(nil), NewDefaultTransport())
		defer client.Close()

		reqID, respCh := client.registerPendingRequest()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := client.waitResponse(ctx, reqID, respCh, time.Second)
		if err == nil || !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("expected context canceled error, got: %v", err)
		}
	})

	t.Run("wait_response_server_error", func(t *testing.T) {
		t.Parallel()
		client := NewClient(bytes.NewBuffer(nil), bytes.NewBuffer(nil), NewDefaultTransport())
		defer client.Close()

		reqID, respCh := client.registerPendingRequest()
		respCh <- JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      reqID,
			Error: &JSONRPCError{
				Code:    -32600,
				Message: "invalid request payload",
			},
		}

		_, err := client.waitResponse(context.Background(), reqID, respCh, time.Second)
		if err == nil || !strings.Contains(err.Error(), "invalid request payload") {
			t.Errorf("expected mcp error with message, got: %v", err)
		}
	})

	t.Run("log_to_sandbox", func(t *testing.T) {
		t.Parallel()
		client := NewClient(bytes.NewBuffer(nil), bytes.NewBuffer(nil), NewDefaultTransport())
		defer client.Close()

		// logToSandbox writes to .sandbox/mcp-client.log if .sandbox directory exists
		client.logToSandbox("test format message: %s\n", "value")
	})
}

// TestMCPServer_LifecycleAndRegistrations exercises server lifecycle, events, prompts, and tools.
func TestMCPServer_LifecycleAndRegistrations(t *testing.T) {
	t.Parallel()

	server := createTestServerWithoutRegistry()

	// Test SetEventEmitter / GetEventEmitter
	ee := NewEventEmitter(20)
	server.SetEventEmitter(ee)
	if server.GetEventEmitter() != ee {
		t.Errorf("expected event emitter to match")
	}

	// Test handleNotificationMessage
	ctx := context.Background()
	sentinel, err := server.handleNotificationMessage(ctx, "test-client", json.RawMessage(`{"method":"ping"}`))
	if sentinel != nil || err == nil {
		// Expects nil result and NotificationSentinel error
		if _, ok := err.(*NotificationSentinel); !ok {
			t.Errorf("expected NotificationSentinel, got err=%v", err)
		}
	}

	// Test RegisterTool, RegisterPrompt, RegisterResource
	server.RegisterTool("custom_tool", "A custom tool", nil, func(ctx context.Context, args map[string]any) (any, error) {
		return map[string]any{"status": "custom_done"}, nil
	})
	if _, exists := server.tools["custom_tool"]; !exists {
		t.Errorf("expected custom_tool to be registered")
	}

	// Test RegisterShutdownHook and execute
	hookExecuted := false
	server.RegisterShutdownHook(func(ctx context.Context) error {
		hookExecuted = true
		return nil
	})

	server.shutdownHookManager.OrderShutdown()
	errs := server.shutdownHookManager.NotifyHooks()
	if len(errs) > 0 {
		t.Errorf("shutdown hooks failed: %v", errs)
	}
	if !hookExecuted {
		t.Errorf("expected shutdown hook to have executed")
	}
}

// TestMCPServer_BuiltinToolExecutionLifecycle exercises built-in tool execution paths.
func TestMCPServer_BuiltinToolExecutionLifecycle(t *testing.T) {
	t.Parallel()

	tmpRoot := t.TempDir()
	server := createTestServerWithoutRegistry()
	server.initCtx = &pkgctx.CliInitializationContext{
		ProjectRoot: tmpRoot,
	}

	ctx := context.Background()

	// 1. test_echo
	t.Run("test_echo", func(t *testing.T) {
		res, err := server.handleToolCallWithContext(ctx, "test_echo", map[string]any{"message": "ping 123"})
		if err != nil {
			t.Fatalf("test_echo failed: %v", err)
		}
		if res == nil {
			t.Fatal("expected non-nil result from test_echo")
		}
	})

	// 2. read_file & write_file
	t.Run("file_read_write", func(t *testing.T) {
		filePath := "sample_file.txt"
		fileContent := "hello world payload"

		// Write file
		writeRes, err := server.handleToolCallWithContext(ctx, "write_file", map[string]any{
			"path":    filePath,
			"content": fileContent,
		})
		if err != nil {
			t.Fatalf("write_file failed: %v", err)
		}
		if writeRes == nil {
			t.Fatal("expected non-nil write_file result")
		}

		// Read file back
		readRes, err := server.handleToolCallWithContext(ctx, "read_file", map[string]any{
			"path": filePath,
		})
		if err != nil {
			t.Fatalf("read_file failed: %v", err)
		}
		if readRes == nil {
			t.Fatal("expected non-nil read_file result")
		}
	})

	// 3. read_code & write_code
	t.Run("code_read_write", func(t *testing.T) {
		codePath := "sample_code.go"
		codeContent := "package sample\n\nfunc Sample() int { return 42 }\n"

		// Write code
		writeRes, err := server.handleToolCallWithContext(ctx, "write_code", map[string]any{
			"path":    codePath,
			"content": codeContent,
		})
		if err != nil {
			t.Fatalf("write_code failed: %v", err)
		}
		if writeRes == nil {
			t.Fatal("expected non-nil write_code result")
		}

		// Read code
		readRes, err := server.handleToolCallWithContext(ctx, "read_code", map[string]any{
			"path": codePath,
		})
		if err != nil {
			t.Fatalf("read_code failed: %v", err)
		}
		if readRes == nil {
			t.Fatal("expected non-nil read_code result")
		}
	})

	// 4. execute_bash
	t.Run("execute_bash_allowed_and_denied", func(t *testing.T) {
		// Allowed echo
		_, _ = server.handleToolCallWithContext(ctx, "execute_bash", map[string]any{
			"command": "echo 'safe test'",
		})

		// Denied high-risk command
		_, err := server.handleToolCallWithContext(ctx, "execute_bash", map[string]any{
			"command": "rm -rf /",
		})
		if err == nil {
			t.Error("expected error for high-risk bash command")
		}
	})

	// 5. observer_search
	t.Run("observer_search", func(t *testing.T) {
		_, _ = server.handleToolCallWithContext(ctx, "observer_search", map[string]any{
			"query": "find active plans",
		})
	})

	// 6. trigger_verification
	t.Run("trigger_verification", func(t *testing.T) {
		_, _ = server.handleToolCallWithContext(ctx, "trigger_verification", map[string]any{
			"verification_type": "unit",
		})
	})

	// 7. metrics tools
	t.Run("metrics_tools", func(t *testing.T) {
		server.RegisterTool("get_metrics", "metrics", nil, func(ctx context.Context, args map[string]any) (any, error) {
			return map[string]any{"metric": 1}, nil
		})
		server.RegisterTool("get_tool_metrics", "tool metrics", nil, func(ctx context.Context, args map[string]any) (any, error) {
			return map[string]any{"tool_metric": 2}, nil
		})

		_, err := server.handleToolCallWithContext(ctx, "get_metrics", nil)
		if err != nil {
			t.Errorf("get_metrics failed: %v", err)
		}
		_, err = server.handleToolCallWithContext(ctx, "get_tool_metrics", nil)
		if err != nil {
			t.Errorf("get_tool_metrics failed: %v", err)
		}
	})

	// 8. Custom tool dispatch
	t.Run("custom_tool_dispatch", func(t *testing.T) {
		server.RegisterTool("custom_echo", "custom echo tool", nil, func(ctx context.Context, args map[string]any) (any, error) {
			return "custom_val", nil
		})
		val, err := server.handleToolCallWithContext(ctx, "custom_echo", nil)
		if err != nil || val != "custom_val" {
			t.Errorf("custom tool failed: val=%v, err=%v", val, err)
		}
	})

	// 9. Format permission check
	t.Run("format_permission", func(t *testing.T) {
		server.allowedFormats = []string{"json"}
		server.secCtx = &pkgctx.SecurityContext{
			AccountID: "ACC-ADMIN",
			Roles:     []string{"admin"},
		}
		allowed, _ := server.CheckFormatPermission(ctx, "json")
		if !allowed {
			t.Errorf("expected json format to be allowed")
		}
		allowed, _ = server.CheckFormatPermission(ctx, "xml")
		if allowed {
			t.Errorf("expected xml format to be denied")
		}
	})
}

// TestMCPSecurity_SandboxAndPathTraversalGuards exercises boundary conditions and path security.
func TestMCPSecurity_SandboxAndPathTraversalGuards(t *testing.T) {
	t.Parallel()

	tmpRoot := t.TempDir()
	server := createTestServerWithoutRegistry()
	server.initCtx = &pkgctx.CliInitializationContext{
		ProjectRoot: tmpRoot,
	}

	t.Run("resolveSandboxPath_boundaries", func(t *testing.T) {
		// Empty path
		_, err := resolveSandboxPath(tmpRoot, "")
		if err == nil {
			t.Error("expected error on empty path")
		}

		// Clean subpath
		resolved, err := resolveSandboxPath(tmpRoot, "valid/subpath.txt")
		if err != nil {
			t.Fatalf("unexpected error on valid subpath: %v", err)
		}
		if !strings.HasPrefix(resolved, filepath.Clean(tmpRoot)) {
			t.Errorf("expected resolved path inside root: %s", resolved)
		}

		// Relative path escaping root via ..
		_, err = resolveSandboxPath(tmpRoot, "../../outside.txt")
		if err == nil {
			t.Error("expected error for path traversal via ..")
		}

		// Absolute path escaping root
		_, err = resolveSandboxPath(tmpRoot, "/etc/passwd")
		if err == nil {
			t.Error("expected error for absolute path escaping root")
		}
	})

	t.Run("isHighRiskBashCommand_evaluation", func(t *testing.T) {
		ctx := context.Background()

		highRiskCases := []string{
			"rm -rf /tmp/data",
			"git push origin main",
			"git commit -m 'feat: test'",
			"sudo apt-get install",
			"chmod 777 script.sh",
			"curl http://malicious.site",
			"wget https://malicious.site",
		}

		for _, cmd := range highRiskCases {
			if !server.isHighRiskBashCommand(ctx, cmd) {
				t.Errorf("expected command %q to be flagged high risk", cmd)
			}
		}

		safeCases := []string{
			"go test -v ./...",
			"git status",
			"echo 'hello world'",
			"ls -la",
		}

		for _, cmd := range safeCases {
			if server.isHighRiskBashCommand(ctx, cmd) {
				t.Errorf("expected command %q to be classified safe", cmd)
			}
		}
	})
}

// TestMCPSession_LeaseManagementAndConcurrency tests session leases under concurrent stress.
func TestMCPSession_LeaseManagementAndConcurrency(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()

	// SessionLeaseExpired unit tests
	t.Run("lease_expiration_checks", func(t *testing.T) {
		if !SessionLeaseExpired("", 30*time.Minute, now) {
			t.Error("expected empty lastActivity to be expired")
		}
		if !SessionLeaseExpired("invalid-date-format", 30*time.Minute, now) {
			t.Error("expected unparseable timestamp to be expired")
		}
		if SessionLeaseExpired(now.Format(time.RFC3339), 0, now) {
			t.Error("expected ttl <= 0 to not be expired")
		}
		past := now.Add(-40 * time.Minute).Format(time.RFC3339)
		if !SessionLeaseExpired(past, 30*time.Minute, now) {
			t.Error("expected 40 min old activity to be expired with 30 min TTL")
		}
		recent := now.Add(-5 * time.Minute).Format(time.RFC3339)
		if SessionLeaseExpired(recent, 30*time.Minute, now) {
			t.Error("expected 5 min old activity to be valid with 30 min TTL")
		}
	})

	// TouchSessionLastActivity & GetSessionLastActivityAt
	t.Run("touch_session_activity", func(t *testing.T) {
		TouchSessionLastActivity(nil) // nil safe check

		server := createTestServerWithoutRegistry()
		server.SetCurrentSessionID("MCP-SESSION-TEST-001")

		TouchSessionLastActivity(server)
		activityAt := server.GetSessionLastActivityAt()
		if activityAt.IsZero() {
			t.Error("expected non-zero session last activity")
		}

		if server.CurrentSessionLeaseExpired(10 * time.Minute) {
			t.Error("expected current session lease to be valid")
		}
	})

	// Concurrency stress test on session leases
	t.Run("concurrent_session_lease_stress", func(t *testing.T) {
		server := createTestServerWithoutRegistry()
		server.SetCurrentSessionID("MCP-SESSION-CONCURRENT-001")

		var wg sync.WaitGroup
		concurrencyCount := 10

		for i := 0; i < concurrencyCount; i++ {
			wg.Add(1)
			goroutinelabels.NewGoroutine("mcp_test", fmt.Sprintf("lease_worker_%d", i)).
				StartSimple(func() {
					defer wg.Done()
					for j := 0; j < 50; j++ {
						TouchSessionLastActivity(server)
						_ = server.GetSessionLastActivityAt()
						_ = server.CurrentSessionLeaseExpired(30 * time.Minute)
					}
				})
		}
		wg.Wait()
	})
}

// TestMCPAuthentication_SessionLifecycle tests mcp_sessions creation and update.
func TestMCPAuthentication_SessionLifecycle(t *testing.T) {
	t.Parallel()

	tmpRoot := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(tmpRoot)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create process dir: %v", err)
	}
	sessionsDir := datacell.CellCASPrimaryDir(tmpRoot, "mcp_sessions")
	if err := fileutil.MkdirAll(sessionsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create sessions dir: %v", err)
	}

	server := createTestServerWithoutRegistry()
	server.initCtx = &pkgctx.CliInitializationContext{
		ProjectRoot: tmpRoot,
	}

	ctx := context.Background()

	// 1. Create authentication session
	sessionID, err := server.createAuthenticationSession(ctx, "client-001", "agent-worker", "ACC-AGENT-1")
	if err != nil {
		t.Fatalf("createAuthenticationSession failed: %v", err)
	}
	if sessionID == "" {
		t.Fatal("expected non-empty session ID")
	}

	// Verify session file exists
	sessionFile := filepath.Join(sessionsDir, fmt.Sprintf("%s.yaml", sessionID))
	content, err := fileutil.ReadFile(sessionFile)
	if err != nil {
		t.Fatalf("failed to read created session file: %v", err)
	}
	if !strings.Contains(string(content), "mcp_session") {
		t.Errorf("expected session file to contain kind mcp_session, got: %s", string(content))
	}

	// 2. Update authentication session
	secCtx := &pkgctx.SecurityContext{
		AccountID:   "ACC-AGENT-1",
		Roles:       []string{"agent_role"},
		Permissions: []string{"read", "write"},
	}
	if err := server.updateAuthenticationSession(ctx, sessionID, secCtx, nil); err != nil {
		t.Fatalf("updateAuthenticationSession failed: %v", err)
	}

	// 3. Negative case: update non-existent session
	err = server.updateAuthenticationSession(ctx, "MCP-NON-EXISTENT", secCtx, nil)
	if err == nil {
		t.Error("expected error updating non-existent session")
	}

	// 4. Negative case: invalid project root
	serverInvalid := createTestServerWithoutRegistry()
	serverInvalid.initCtx = &pkgctx.CliInitializationContext{
		ProjectRoot: filepath.Join(tmpRoot, "pkg", "mcp"),
	}
	_, err = serverInvalid.createAuthenticationSession(ctx, "c1", "name", "acc")
	if err == nil {
		t.Error("expected error with invalid project root inside pkg/mcp")
	}
}

// TestMCPWakerStream_LifecycleAndShutdown tests AttachZeroIdleWakerStream and shutdown hook.
func TestMCPWakerStream_LifecycleAndShutdown(t *testing.T) {
	t.Parallel()

	// Nil server check
	nilStreamer := AttachZeroIdleWakerStream(nil, "")
	if nilStreamer != nil {
		t.Error("expected nil streamer for nil server")
	}

	tmpRoot := t.TempDir()
	server := createTestServerWithoutRegistry()
	server.initCtx = &pkgctx.CliInitializationContext{
		ProjectRoot: tmpRoot,
	}

	streamer := AttachZeroIdleWakerStream(server, tmpRoot, WithPollInterval(50*time.Millisecond))
	if streamer != nil {
		defer streamer.Stop()
		if !streamer.IsRunning() {
			t.Error("expected streamer to be running")
		}

		streamed, dropped := streamer.Stats()
		_ = streamed
		_ = dropped

		// Trigger shutdown hooks on server to ensure streamer clean stop
		server.shutdownHookManager.OrderShutdown()
		_ = server.shutdownHookManager.NotifyHooks()
		if streamer.IsRunning() {
			t.Error("expected streamer to stop after server shutdown hooks")
		}
	}
}

// TestMCPObserver_RequestGuidance tests handleObserverRequestGuidanceTool.
func TestMCPObserver_RequestGuidance(t *testing.T) {
	t.Parallel()

	server := createTestServerWithoutRegistry()
	server.SetProjectRoot(t.TempDir())
	ctx := context.Background()

	// Missing query should error
	_, err := server.handleObserverRequestGuidanceTool(ctx, map[string]any{})
	if err == nil {
		t.Error("expected error when query is empty")
	}

	// Query provided
	_, err = server.handleObserverRequestGuidanceTool(ctx, map[string]any{
		"query": "how to resolve dependency cycle",
	})
	if err != nil {
		t.Errorf("unexpected error on guidance query: %v", err)
	}
}

// TestMCPSpecAdapter_Coverage tests ObjectsSpecLoaderAdapter methods.
func TestMCPSpecAdapter_Coverage(t *testing.T) {
	t.Parallel()

	// Nil loader should return nil
	ac := NewSpecAccessControlFromObjectsLoader(nil)
	if ac != nil {
		t.Error("expected nil SpecAccessControl from nil loader")
	}

	adapter := &ObjectsSpecLoaderAdapter{Loader: nil}
	spec, err := adapter.LoadSpecWithInheritance("sample.yaml")
	if spec != nil || err != nil {
		t.Errorf("expected nil, nil for nil loader, got spec=%v, err=%v", spec, err)
	}

	objSpec := &objectsSpecAdapter{spec: nil}
	if fields := objSpec.GetResolvedFields(); fields != nil {
		t.Errorf("expected nil fields for nil spec, got %v", fields)
	}
}

// TestMCPPermissionFormat_Helpers exercises format examples and helpers.
func TestMCPPermissionFormat_Helpers(t *testing.T) {
	t.Parallel()

	server := createTestServerWithoutRegistry()

	ops := server.getPermissionOperations()
	if len(ops) == 0 {
		t.Error("expected default operations")
	}

	kinds := server.getObjectKindsForExamples()
	_ = kinds

	examples := server.generatePermissionFormatExamples()
	if !strings.Contains(examples, "operation:resource") {
		t.Errorf("expected examples to contain operation:resource format, got: %s", examples)
	}

	// Helper tests
	if capitalizeFirst("") != "" {
		t.Errorf("expected empty string on capitalizeFirst")
	}
	if capitalizeFirst("read") != "Read" {
		t.Errorf("expected 'Read', got '%s'", capitalizeFirst("read"))
	}
	if min(3, 7) != 3 || min(10, 5) != 5 {
		t.Errorf("min function returned incorrect value")
	}
}

// TestMCPTransport_ReadWriteMessages tests transport message reading and writing.
func TestMCPTransport_ReadWriteMessages(t *testing.T) {
	t.Parallel()

	transport := NewDefaultTransport()

	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)

	msg := []byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	format := &MessageFormat{IsRawJSON: true}

	if err := transport.WriteMessage(writer, msg, format); err != nil {
		t.Fatalf("WriteMessage failed: %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	reader := bufio.NewReader(&buf)
	readData, readFormat, err := transport.ReadMessage(reader)
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}
	if !bytes.Equal(readData, msg) {
		t.Errorf("expected read data to match written: got %s, want %s", string(readData), string(msg))
	}
	if readFormat == nil || !readFormat.IsRawJSON {
		t.Errorf("expected format IsRawJSON to be true")
	}
}
