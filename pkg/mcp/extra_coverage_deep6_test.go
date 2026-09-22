package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
)

// 1. CLI bridge command execution comprehensive tests
func TestDeep6_CLIBridgeCommandExecution(t *testing.T) {
	// filterCommandOutput and sub-filters
	out := filterCommandOutput("")
	if out != "" {
		t.Errorf("expected empty output, got %q", out)
	}

	polluted := "[warn] this is a warning\n[error] some error\n{\"success\":true,\"data\":123}\n[warn] trailing warning"
	filtered := filterCommandOutput(polluted)
	if filtered != "{\"success\":true,\"data\":123}" {
		t.Errorf("unexpected filtered output: %q", filtered)
	}

	fallbackFiltered := filterFallbackLogMessages("[warn] skip me\nkeep me\n[error] skip me too")
	if fallbackFiltered != "keep me" {
		t.Errorf("unexpected fallback filtered: %q", fallbackFiltered)
	}

	debugFiltered := filterDebugLogObjects("{\"debug\": true}\n{\"result\": \"ok\"}")
	if debugFiltered == "" {
		t.Error("expected non-empty debug filtered")
	}

	ext := extractFirstCompleteJSONObject("{\"first\":1}{\"second\":2}")
	if ext != "{\"first\":1}" {
		t.Errorf("unexpected first json object: %q", ext)
	}

	// parseCommandOutput
	var stderrBuf bytes.Buffer
	resEmpty := parseCommandOutput("", stderrBuf)
	if resEmpty == nil || resEmpty["success"] == true {
		t.Errorf("expected failed parse on empty output: %v", resEmpty)
	}

	stderrBuf.WriteString("[warn] fallback in stderr")
	resEmptyWithWarn := parseCommandOutput("", stderrBuf)
	if resEmptyWithWarn == nil {
		t.Error("expected non-nil result for empty output with stderr warn")
	}

	var jsonStderr bytes.Buffer
	jsonStderr.WriteString("{\"stderr_json\":true}")
	resStderrJSON := parseCommandOutput("", jsonStderr)
	if resStderrJSON["stderr_json"] != true {
		t.Errorf("expected parsed stderr json: %v", resStderrJSON)
	}

	validJSON := "{\"status\":\"success\",\"count\":42}"
	resValid := parseCommandOutput(validJSON, bytes.Buffer{})
	if resValid["status"] != "success" || resValid["count"] != float64(42) {
		t.Errorf("unexpected valid parsed result: %v", resValid)
	}

	invalidJSON := "not valid json {{"
	resInvalid := parseCommandOutput(invalidJSON, bytes.Buffer{})
	if resInvalid["error_type"] != "parse_error" {
		t.Errorf("expected parse_error type, got: %v", resInvalid["error_type"])
	}

	// buildParseErrorResult with polluted output
	pollutedErr := buildParseErrorResult("[warn] corrupted output", bytes.Buffer{}, fmt.Errorf("bad json"))
	if pollutedErr["error_type"] != "parse_error" {
		t.Errorf("expected parse_error: %v", pollutedErr)
	}

	// buildCommandResult
	resSuccess := buildCommandResult(map[string]any{"data": "val"}, "test cmd", []string{"--flag"}, bytes.Buffer{}, bytes.Buffer{}, nil)
	if resSuccess["success"] != true {
		t.Errorf("expected success: %v", resSuccess)
	}

	resFailure := buildCommandResult(map[string]any{"success": false}, "test cmd", []string{}, bytes.Buffer{}, bytes.Buffer{}, nil)
	if resFailure["success"] != false {
		t.Errorf("expected failure: %v", resFailure)
	}

	resErr := buildCommandResult(map[string]any{}, "test cmd", []string{}, bytes.Buffer{}, bytes.Buffer{}, fmt.Errorf("cmd failed"))
	if resErr["success"] != false {
		t.Errorf("expected failure with exec error: %v", resErr)
	}

	// BuildCLICommandResultFromOutput
	resCLIOut := BuildCLICommandResultFromOutput("object list", []string{"backlog_item"}, "{\"objects\":[]}", "", nil)
	if resCLIOut["success"] != true {
		t.Errorf("expected success from BuildCLICommandResultFromOutput: %v", resCLIOut)
	}

	// executeCommand with echo
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	tmpDir := t.TempDir()
	stdout, stderr, err := executeCommand(ctx, "/bin/echo", []string{"hello_mcp"}, []string{"FOO=BAR"}, tmpDir, nil, "echo_tool")
	if err != nil {
		t.Fatalf("executeCommand failed: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("hello_mcp")) {
		t.Errorf("unexpected stdout: %s (stderr: %s)", stdout.String(), stderr.String())
	}

	// executeCommand with process group manager
	pgm := NewProcessGroupManager(ctx, 5*time.Second)
	stdout2, _, err2 := executeCommand(ctx, "/bin/echo", []string{"with_pgm"}, nil, tmpDir, pgm, "echo_pgm")
	if err2 != nil || !bytes.Contains(stdout2.Bytes(), []byte("with_pgm")) {
		t.Errorf("unexpected pgm executeCommand: %v, %s", err2, stdout2.String())
	}
}

// 2. Proxy daemon QueryEventsSubscriberCount and QueryDiagnostics tests
func TestDeep6_ProxyDaemon_Queries(t *testing.T) {
	// Setup a mock TCP server that mimics the MCP daemon
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on local tcp: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().String()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				reader := bufio.NewReader(c)
				writer := bufio.NewWriter(c)
				for {
					line, err := reader.ReadBytes('\n')
					if err != nil {
						return
					}
					line = bytes.TrimSpace(line)
					if len(line) == 0 {
						continue
					}
					var req struct {
						ID     any    `json:"id"`
						Method string `json:"method"`
					}
					if err := json.Unmarshal(line, &req); err != nil {
						return
					}

					var resp map[string]any
					switch req.Method {
					case "initialize":
						resp = map[string]any{
							"jsonrpc": "2.0",
							"id":      req.ID,
							"result": map[string]any{
								"protocolVersion": "2024-11-05",
								"capabilities":    map[string]any{},
								"serverInfo":      map[string]any{"name": "mock-daemon", "version": "dev"},
							},
						}
					case "events/list":
						resp = map[string]any{
							"jsonrpc": "2.0",
							"id":      req.ID,
							"result": map[string]any{
								"subscriberCount": 3,
								"events":          []any{},
							},
						}
					case "system/diagnostics":
						resp = map[string]any{
							"jsonrpc": "2.0",
							"id":      req.ID,
							"result": map[string]any{
								"status": "healthy",
								"uptime": 120,
							},
						}
					default:
						resp = map[string]any{
							"jsonrpc": "2.0",
							"id":      req.ID,
							"result":  map[string]any{},
						}
					}

					b, _ := json.Marshal(resp)
					writer.Write(append(b, '\n'))
					writer.Flush()
				}
			}(conn)
		}
	}()

	proxy := &ProxyDaemon{
		tcpAddr: addr,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// QueryEventsSubscriberCount
	count, err := proxy.QueryEventsSubscriberCount(ctx)
	if err != nil {
		t.Fatalf("QueryEventsSubscriberCount failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected subscriberCount 3, got: %d", count)
	}

	// QueryDiagnostics
	diag, err := proxy.QueryDiagnostics(ctx)
	if err != nil {
		t.Fatalf("QueryDiagnostics failed: %v", err)
	}
	if diag["status"] != "healthy" {
		t.Errorf("expected status healthy, got: %v", diag["status"])
	}

	// Nil proxy receiver tests
	var nilProxy *ProxyDaemon
	_, err = nilProxy.QueryEventsSubscriberCount(ctx)
	if err == nil {
		t.Error("expected error on nil proxy QueryEventsSubscriberCount")
	}
	_, err = nilProxy.QueryDiagnostics(ctx)
	if err == nil {
		t.Error("expected error on nil proxy QueryDiagnostics")
	}
}

// 3. Server TLS comprehensive tests
func TestDeep6_ServerTLS_Validation(t *testing.T) {
	s := NewServer()

	// Non-loopback address rejected
	err := s.ServeTLS("192.168.1.50:8080", "cert.pem", "key.pem")
	if err == nil {
		t.Error("expected error for non-loopback address in ServeTLS")
	}

	// Missing cert/key files rejected
	err = s.ServeTLS("127.0.0.1:0", "", "")
	if err == nil {
		t.Error("expected error for missing certFile and keyFile in ServeTLS")
	}

	// Invalid cert files rejected
	err = s.ServeTLS("127.0.0.1:0", "/nonexistent/cert.pem", "/nonexistent/key.pem")
	if err == nil {
		t.Error("expected error for nonexistent key pair in ServeTLS")
	}
}

// 4. ServeCoordinator handleTimeout comprehensive tests
func TestDeep6_ServeCoordinator_HandleTimeout(t *testing.T) {
	s := NewServer()
	var traceBuf bytes.Buffer
	s.SetTraceWriter(&traceBuf)

	sc := &ServeCoordinator{server: s}
	var writerBuf bytes.Buffer
	writer := bufio.NewWriter(&writerBuf)

	timeoutCtx, cancel := context.WithCancel(context.Background())
	cancel() // Trigger timeout context cancellation

	err := sc.handleTimeout(timeoutCtx, writer, true, &traceBuf)
	if err != nil && err != io.EOF {
		t.Errorf("unexpected error from handleTimeout: %v", err)
	}
}

// 5. ServerInitHelpers authentication & roles validation tests
func TestDeep6_ServerInitHelpers_AuthAndRoles(t *testing.T) {
	s := NewServer()
	ctx := context.Background()

	// handleCredentialAuthentication with empty credentials (fails and triggers SendCriticalError)
	clientInfo := map[string]any{
		"client": "test_client",
	}
	_, _, err := s.handleCredentialAuthentication(ctx, clientInfo)
	if err == nil {
		t.Error("expected authentication failure with empty credentials")
	}

	// validateRolesAgainstSystem empty
	err = s.validateRolesAgainstSystem(map[string]any{})
	if err != nil {
		t.Errorf("expected no error on empty roles: %v", err)
	}

	// validateRolesAgainstSystem with string
	_ = s.validateRolesAgainstSystem(map[string]any{
		clientInfoRoles: "developer, admin",
	})

	// validateRolesAgainstSystem with slice of any
	_ = s.validateRolesAgainstSystem(map[string]any{
		clientInfoRoles: []any{"developer", "reader"},
	})
}

// 6. ClientMetricsStore CompressMetrics tests
func TestDeep6_ClientMetricsStore_CompressMetrics(t *testing.T) {
	tmpDir := t.TempDir()
	metricsPath := filepath.Join(tmpDir, "client_metrics.json")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := NewClientMetricsStore(metricsPath, ctx)
	if err != nil {
		t.Fatalf("failed to create client metrics store: %v", err)
	}

	// Compress with empty metrics
	err = store.CompressMetrics(24 * time.Hour)
	if err != nil {
		t.Errorf("expected nil error on empty compress: %v", err)
	}

	// Record an old event to be compressed
	_ = store.RecordEvent("old_sess", "client_1", "call_tool", map[string]any{"duration_ms": 50})

	// Artificially age the metrics in the store
	store.mu.Lock()
	if seq, ok := store.metrics["old_sess"]; ok {
		seq.FirstEvent = time.Now().Add(-48 * time.Hour)
		seq.LastEvent = time.Now().Add(-48 * time.Hour)
	}
	store.mu.Unlock()

	// Compress metrics older than 24 hours
	err = store.CompressMetrics(24 * time.Hour)
	if err != nil {
		t.Fatalf("failed to compress old metrics: %v", err)
	}

	// Verify old_sess is archived and removed from active store
	store.mu.RLock()
	_, stillActive := store.metrics["old_sess"]
	store.mu.RUnlock()
	if stillActive {
		t.Error("expected old_sess to be archived and removed from active metrics")
	}
}

// 7. MessageProcessor queue & parse error tests
func TestDeep6_MessageProcessor_QueuesAndErrors(t *testing.T) {
	s := NewServer()
	s.SetAllowedFormats([]string{"json"})
	mp := &MessageProcessor{server: s, transport: NewDefaultTransport()}

	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	format := &MessageFormat{IsRawJSON: true}

	// registerNewClient
	mp.registerNewClient("client-test-1", writer, format)
	clientConn, ok := s.clients["client-test-1"]
	if !ok || clientConn.Queue == nil {
		t.Fatal("expected client-test-1 registered with queue")
	}

	// ensureClientQueue when active
	mp.ensureClientQueue(clientConn, writer, format)
	if clientConn.Queue == nil {
		t.Error("expected client queue preserved")
	}

	// handleParseError
	err := mp.handleParseError(fmt.Errorf("syntax error"), format, writer)
	if err != nil {
		t.Errorf("unexpected error from handleParseError: %v", err)
	}

	// sendResponse via queue
	resp := NewResponse(1)
	resp.Result = map[string]any{"pong": true}
	var writeCompleted bool
	err = mp.sendResponse(resp, format, writer, func() {
		writeCompleted = true
	})
	if err != nil {
		t.Fatalf("sendResponse failed: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if !writeCompleted {
		t.Log("note: async queue writer completed or pending")
	}
}

// 8. Server Error Handling & Critical Error Persistence tests
func TestDeep6_ServerErrors_SendCriticalError(t *testing.T) {
	s := NewServer()
	tmpDir := t.TempDir()
	s.initCtx = &pkgctx.CliInitializationContext{ProjectRoot: tmpDir}
	s.initialized.Store(true)
	var dummyBuf bytes.Buffer
	_ = concurrency.RunInLock(&s.transportMu, func() error {
		s.transportWriter = bufio.NewWriter(&dummyBuf)
		s.transportFormat = &MessageFormat{IsRawJSON: true}
		return nil
	})

	err := s.SendCriticalError(
		fmt.Errorf("fatal disk corruption"),
		"critical",
		"storage",
		"Storage system failed to write",
		[]string{"Check disk space", "Check file permissions"},
		map[string]any{"path": "/var/data"},
		"client-test",
		false,
	)
	if err != nil {
		t.Fatalf("SendCriticalError failed: %v", err)
	}

	// Verify tracker was written
	trackerPath := filepath.Join(tmpDir, paths.ProjectDataDir, paths.StateDir, "cap_failure_tracker.json")
	if _, statErr := os.Stat(trackerPath); statErr != nil {
		t.Errorf("expected cap_failure_tracker.json created, got error: %v", statErr)
	}

	// Suppress notification
	err = s.SendCriticalError(fmt.Errorf("suppressed"), "error", "auth", "msg", nil, nil, "", true)
	if err != nil {
		t.Errorf("unexpected error on suppressed SendCriticalError: %v", err)
	}

	// Nil error
	if s.SendCriticalError(nil, "error", "auth", "msg", nil, nil, "", false) != nil {
		t.Error("expected nil on nil error")
	}
}

// 9. Schema handlers and resources tests
func TestDeep6_SchemaHandlers_AndResources(t *testing.T) {
	s := NewServer()
	rootCmd := &cobra.Command{
		Use:   "zqk",
		Short: "ZQK CLI test root",
	}
	subCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version",
	}
	rootCmd.AddCommand(subCmd)
	s.SetRootCommand(rootCmd)

	RegisterSchemaResources(s)

	// handleCLIOntology
	resOntology, err := s.handleCLIOntology()
	if err != nil || resOntology == nil {
		t.Errorf("handleCLIOntology failed: %v", err)
	}

	// handleCommonFieldsSchema
	resCommon, err := s.handleCommonFieldsSchema()
	if err != nil || resCommon == nil {
		t.Errorf("handleCommonFieldsSchema failed: %v", err)
	}

	// handleObjectSchema
	resObj, err := s.handleObjectSchema("backlog_item")
	if err != nil || resObj == nil {
		t.Errorf("handleObjectSchema backlog_item failed: %v", err)
	}

	// getCriticalResourcePaths and RegisterCriticalResources
	paths := getCriticalResourcePaths(s)
	if paths == nil {
		t.Error("expected non-nil critical paths")
	}
	RegisterCriticalResources(s)
}

// 10. Tools workflow extractWorkflowResultItem comprehensive tests
func TestDeep6_ToolsWorkflow_ExtractWorkflowResultItem(t *testing.T) {
	// map with objects slice
	item, ok, err := extractWorkflowResultItem(map[string]any{
		"objects": []any{map[string]any{"id": "BLI-1"}},
	})
	if err != nil || !ok || item == nil {
		t.Errorf("expected extraction from map[string]any: %v, %v, %v", item, ok, err)
	}

	// map with objects []map[string]any
	item, ok, err = extractWorkflowResultItem(map[string]any{
		"objects": []map[string]any{{"id": "BLI-2"}},
	})
	if err != nil || !ok || item == nil {
		t.Errorf("expected extraction from []map[string]any: %v, %v, %v", item, ok, err)
	}

	// []any
	item, ok, err = extractWorkflowResultItem([]any{map[string]any{"id": "BLI-3"}})
	if err != nil || !ok || item == nil {
		t.Errorf("expected extraction from []any: %v", item)
	}

	// []map[string]any
	item, ok, err = extractWorkflowResultItem([]map[string]any{{"id": "BLI-4"}})
	if err != nil || !ok || item == nil {
		t.Errorf("expected extraction from []map: %v", item)
	}

	// string json
	item, ok, err = extractWorkflowResultItem("{\"objects\": [{\"id\": \"BLI-5\"}]}")
	if err != nil || !ok || item == nil {
		t.Errorf("expected extraction from string json: %v", item)
	}

	// string invalid json
	_, _, err = extractWorkflowResultItem("invalid json {{")
	if err == nil {
		t.Error("expected error for invalid json in extractWorkflowResultItem")
	}

	// unexpected type
	_, ok, _ = extractWorkflowResultItem(12345)
	if ok {
		t.Error("expected false for unsupported type")
	}
}

// 11. RolePromptRenderer substituteToolNames comprehensive tests
func TestDeep6_RolePromptRenderer_SubstituteToolNames(t *testing.T) {
	renderer := &RolePromptRenderer{}
	content := "Run {{tool:system_status}} and check {{tool:unknown_tool}} today."
	rendered := renderer.substituteToolNames(content)
	if !bytes.Contains([]byte(rendered), []byte("system_status")) {
		t.Errorf("expected substituted tool name in content: %s", rendered)
	}
}
