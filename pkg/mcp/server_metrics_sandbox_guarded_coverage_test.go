package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// 1. Server getters/setters and format permission tests
func TestDeep5_ServerGettersSettersAndMetrics(t *testing.T) {
	s := NewServer()

	// EventEmitter
	em := s.GetEventEmitter()
	if em == nil {
		t.Error("expected non-nil event emitter")
	}

	// AsyncConfig
	s.SetAsyncConfig(AsyncHandlerConfig{MaxConcurrent: 10, Timeout: time.Second})

	// StorageProvider
	s.SetStorageProvider(nil)

	// CallerAccountID
	if id := s.GetCallerAccountID(); id != "" {
		t.Errorf("expected empty caller account ID, got %s", id)
	}
	secCtx := pkgctx.NewSecurityContext("ACC-TESTER", []string{"developer"}, []string{"read:*"})
	s.SetSecurityContext(secCtx)
	if id := s.GetCallerAccountID(); id != "ACC-TESTER" {
		t.Errorf("expected ACC-TESTER, got %s", id)
	}

	// TraceWriter
	var buf bytes.Buffer
	s.SetTraceWriter(&buf)

	// CliInitializationContext & ProjectRoot
	if root := s.GetProjectRoot(); root != "." {
		t.Errorf("expected '.', got %s", root)
	}
	initCtx := &pkgctx.CliInitializationContext{ProjectRoot: "/tmp/project"}
	s.SetCliInitializationContext(initCtx)
	if ctxRet := s.GetCliInitializationContext(); ctxRet != initCtx {
		t.Error("initCtx mismatch")
	}
	if root := s.GetProjectRoot(); root != "/tmp/project" {
		t.Errorf("expected /tmp/project, got %s", root)
	}

	// AllowedFormats
	s.SetAllowedFormats([]string{"json", "table"})
	formats := s.GetAllowedFormats()
	if len(formats) != 2 || formats[0] != "json" {
		t.Errorf("unexpected allowed formats: %v", formats)
	}

	// CheckFormatPermission
	ctx := context.Background()

	// 1. No security context
	sNoSec := NewServer()
	allowed, reason := sNoSec.CheckFormatPermission(ctx, "json")
	if allowed || reason != "no security context" {
		t.Errorf("expected no security context error, got %v, %s", allowed, reason)
	}

	// 2. User inactive check with permission cache
	pc := NewPermissionCache(&mockSpecLoaderDeep2{})
	_ = pc.DeactivateUser("ACC-INACTIVE")
	sInactive := NewServer()
	sInactive.SetPermissionCache(pc)
	sInactive.SetSecurityContext(pkgctx.NewSecurityContext("ACC-INACTIVE", []string{"developer"}, nil))
	allowed, reason = sInactive.CheckFormatPermission(ctx, "json")
	if allowed || reason != "user is inactive" {
		t.Errorf("expected user is inactive error, got %v, %s", allowed, reason)
	}

	// 3. Allowed formats restriction
	sRestricted := NewServer()
	sRestricted.SetSecurityContext(pkgctx.NewSecurityContext("ACC-DEV", []string{"developer"}, nil))
	sRestricted.SetAllowedFormats([]string{"json"})
	allowed, _ = sRestricted.CheckFormatPermission(ctx, "yaml")
	if allowed {
		t.Error("expected yaml to be disallowed when only json is allowed")
	}

	// 4. Admin role allows all
	sAdmin := NewServer()
	sAdmin.SetSecurityContext(pkgctx.NewSecurityContext("ACC-ADMIN", []string{"admin"}, nil))
	allowed, _ = sAdmin.CheckFormatPermission(ctx, "yaml")
	if !allowed {
		t.Error("expected admin to be allowed format")
	}

	// 5. Streaming formats permission checks
	sStream := NewServer()
	sStream.SetSecurityContext(pkgctx.NewSecurityContext("ACC-USER", []string{"user"}, []string{"format:json-rpc"}))
	allowed, _ = sStream.CheckFormatPermission(ctx, "json-rpc")
	if !allowed {
		t.Error("expected json-rpc to be allowed with format:json-rpc")
	}

	sStream2 := NewServer()
	sStream2.SetSecurityContext(pkgctx.NewSecurityContext("ACC-USER", []string{"user"}, []string{"read:*"}))
	allowed, _ = sStream2.CheckFormatPermission(ctx, "stream")
	if !allowed {
		t.Error("expected stream to be allowed with read:*")
	}

	sStreamNoPerm := NewServer()
	sStreamNoPerm.SetSecurityContext(pkgctx.NewSecurityContext("ACC-USER", []string{"user"}, []string{"unrelated"}))
	allowed, reason = sStreamNoPerm.CheckFormatPermission(ctx, "stream")
	if allowed || !strings.Contains(reason, "requires") {
		t.Errorf("expected streaming permission error, got %v, %s", allowed, reason)
	}

	// 6. Default formats (table)
	allowed, _ = sStreamNoPerm.CheckFormatPermission(ctx, "table")
	if !allowed {
		t.Error("expected table format to be allowed by default")
	}

	// MCPMetrics & ShutdownRequested
	if m := s.GetMCPMetrics(); m == nil {
		t.Error("expected non-nil MCPMetrics")
	}
	snap := s.GetMCPMetricsSnapshot()
	if snap.Timestamp.IsZero() {
		t.Error("invalid snapshot timestamp")
	}
	if s.IsShutdownRequested() {
		t.Error("expected shutdown not requested yet")
	}
}

// 2. ToolBuilder property helpers
func TestDeep5_ToolBuilderProperties(t *testing.T) {
	tb := NewToolBuilder("test_tool", "A test tool")
	tb.AddStringPropertyWithDefault("opt_str", "description", "default_val")
	tb.AddStringPropertyWithEnum("choice", "description", []string{"a", "b", "c"})
	tb.AddBooleanProperty("is_flag", "flag description")

	schema := tb.Build()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("missing properties map: %v", schema)
	}
	optProps, ok := props["opt_str"].(map[string]any)
	if !ok || optProps["default"] != "default_val" {
		t.Errorf("missing or incorrect default_val in opt_str: %v", optProps)
	}
	choiceProps, ok := props["choice"].(map[string]any)
	if !ok || choiceProps["enum"] == nil {
		t.Errorf("missing enum in choice: %v", choiceProps)
	}
	flagProps, ok := props["is_flag"].(map[string]any)
	if !ok || flagProps[objects.FieldKeyType] != "boolean" {
		t.Errorf("missing boolean in is_flag: %v", flagProps)
	}
}

// 3. ToolsSandbox GuardedFiles tests
func TestDeep5_ToolsSandbox_GuardedFiles(t *testing.T) {
	tmpDir := t.TempDir()

	s := NewServer()
	s.SetProjectRoot(tmpDir)

	// Write guarded workspace file
	err := s.writeGuardedWorkspaceFile("sub/test.txt", "hello world")
	if err != nil {
		t.Fatalf("writeGuardedWorkspaceFile failed: %v", err)
	}

	// Read guarded workspace file via server method
	data, err := s.readGuardedWorkspaceFile("sub/test.txt")
	if err != nil || string(data) != "hello world" {
		t.Fatalf("readGuardedWorkspaceFile failed: %v, data=%s", err, string(data))
	}

	// Package-level read/write functions
	origWd, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origWd) }()

	err = writeGuardedWorkspaceFile("direct.txt", "direct content")
	if err != nil {
		t.Fatalf("package writeGuardedWorkspaceFile failed: %v", err)
	}
	readData, err := readGuardedWorkspaceFile("direct.txt")
	if err != nil || string(readData) != "direct content" {
		t.Fatalf("package readGuardedWorkspaceFile failed: %v, data=%s", err, string(readData))
	}

	// Error branches
	if err := writeGuardedWorkspaceFile("", "data"); err == nil {
		t.Error("expected error for empty path")
	}
	if err := writeGuardedWorkspaceFile("/escaped/path", "data"); err == nil {
		t.Error("expected error for escaped path")
	}
}

// 4. ResourceMIMEAdapters comprehensive tests
func TestDeep5_ResourceMIMEAdapters_Comprehensive(t *testing.T) {
	tmpDir := t.TempDir()

	jsonPath1 := filepath.Join(tmpDir, "test1.json")
	_ = fileutil.WriteFile(jsonPath1, []byte(`{"title": "Test JSON", "description": "JSON Desc", "version": "1.0"}`), paths.FilePerm644)

	jsonPath2 := filepath.Join(tmpDir, "test2.json")
	_ = fileutil.WriteFile(jsonPath2, []byte(`{"Title": "Alt Title", "Description": "Alt Desc", "Name": "alt_name"}`), paths.FilePerm644)

	jsonPath3 := filepath.Join(tmpDir, "test3.json")
	_ = fileutil.WriteFile(jsonPath3, []byte(`{"name": "json_name"}`), paths.FilePerm644)

	jsonInvalid := filepath.Join(tmpDir, "invalid.json")
	_ = fileutil.WriteFile(jsonInvalid, []byte(`{not json`), paths.FilePerm644)

	yamlPath1 := filepath.Join(tmpDir, "test1.yaml")
	_ = fileutil.WriteFile(yamlPath1, []byte("title: Test YAML\ndescription: YAML Desc\nauthor: alice\n"), paths.FilePerm644)

	yamlPath2 := filepath.Join(tmpDir, "test2.yaml")
	_ = fileutil.WriteFile(yamlPath2, []byte("Title: YAML Alt Title\nDescription: YAML Alt Desc\nName: yaml_alt\n"), paths.FilePerm644)

	yamlPath3 := filepath.Join(tmpDir, "test3.yaml")
	_ = fileutil.WriteFile(yamlPath3, []byte("name: yaml_name\n"), paths.FilePerm644)

	yamlInvalid := filepath.Join(tmpDir, "invalid.yaml")
	_ = fileutil.WriteFile(yamlInvalid, []byte("[\ninvalid: yaml"), paths.FilePerm644)

	// JSONAdapter
	ja := &JSONAdapter{}
	if title := ja.ExtractTitle(jsonPath1); title != "Test JSON" {
		t.Errorf("expected 'Test JSON', got %q", title)
	}
	if title := ja.ExtractTitle(jsonPath2); title != "Alt Title" {
		t.Errorf("expected 'Alt Title', got %q", title)
	}
	if title := ja.ExtractTitle(jsonPath3); title != "json_name" {
		t.Errorf("expected 'json_name', got %q", title)
	}
	if title := ja.ExtractTitle(jsonInvalid); title != "" {
		t.Errorf("expected empty title for invalid json, got %q", title)
	}
	if title := ja.ExtractTitle(filepath.Join(tmpDir, "missing.json")); title != "" {
		t.Errorf("expected empty title for missing json, got %q", title)
	}

	if desc := ja.ExtractDescription(jsonPath1); desc != "JSON Desc" {
		t.Errorf("expected 'JSON Desc', got %q", desc)
	}
	if desc := ja.ExtractDescription(jsonPath2); desc != "Alt Desc" {
		t.Errorf("expected 'Alt Desc', got %q", desc)
	}
	if desc := ja.ExtractDescription(jsonPath3); desc != "" {
		t.Errorf("expected empty desc, got %q", desc)
	}
	if desc := ja.ExtractDescription(jsonInvalid); desc != "" {
		t.Errorf("expected empty desc for invalid json, got %q", desc)
	}
	if desc := ja.ExtractDescription(filepath.Join(tmpDir, "missing.json")); desc != "" {
		t.Errorf("expected empty desc for missing json, got %q", desc)
	}

	meta := ja.ExtractMetadata(jsonPath1)
	if meta["version"] != "1.0" {
		t.Errorf("expected version 1.0, got %v", meta)
	}
	_ = ja.ExtractMetadata(jsonInvalid)
	_ = ja.ExtractMetadata(filepath.Join(tmpDir, "missing.json"))

	// YAMLAdapter
	ya := &YAMLAdapter{}
	if title := ya.ExtractTitle(yamlPath1); title != "Test YAML" {
		t.Errorf("expected 'Test YAML', got %q", title)
	}
	if title := ya.ExtractTitle(yamlPath2); title != "YAML Alt Title" {
		t.Errorf("expected 'YAML Alt Title', got %q", title)
	}
	if title := ya.ExtractTitle(yamlPath3); title != "yaml_name" {
		t.Errorf("expected 'yaml_name', got %q", title)
	}
	if title := ya.ExtractTitle(yamlInvalid); title != "" {
		t.Errorf("expected empty title for invalid yaml, got %q", title)
	}
	if title := ya.ExtractTitle(filepath.Join(tmpDir, "missing.yaml")); title != "" {
		t.Errorf("expected empty title for missing yaml, got %q", title)
	}

	if desc := ya.ExtractDescription(yamlPath1); desc != "YAML Desc" {
		t.Errorf("expected 'YAML Desc', got %q", desc)
	}
	if desc := ya.ExtractDescription(yamlPath2); desc != "YAML Alt Desc" {
		t.Errorf("expected 'YAML Alt Desc', got %q", desc)
	}
	if desc := ya.ExtractDescription(yamlPath3); desc != "" {
		t.Errorf("expected empty desc, got %q", desc)
	}
	if desc := ya.ExtractDescription(yamlInvalid); desc != "" {
		t.Errorf("expected empty desc for invalid yaml, got %q", desc)
	}
	if desc := ya.ExtractDescription(filepath.Join(tmpDir, "missing.yaml")); desc != "" {
		t.Errorf("expected empty desc for missing yaml, got %q", desc)
	}

	yMeta := ya.ExtractMetadata(yamlPath1)
	if yMeta["author"] != "alice" {
		t.Errorf("expected author alice, got %v", yMeta)
	}
	_ = ya.ExtractMetadata(yamlInvalid)
	_ = ya.ExtractMetadata(filepath.Join(tmpDir, "missing.yaml"))

	// MIME detection
	if mime := DetectMIMEType("file.json"); !strings.HasPrefix(mime, "application/json") {
		t.Errorf("expected application/json, got %s", mime)
	}
	if mime := DetectMIMEType("file.yaml"); !strings.HasPrefix(mime, "application/yaml") {
		t.Errorf("expected application/yaml, got %s", mime)
	}
	if mime := DetectMIMEType("file.yml"); !strings.HasPrefix(mime, "application/yaml") {
		t.Errorf("expected application/yaml, got %s", mime)
	}
	if mime := DetectMIMEType("file.md"); !strings.HasPrefix(mime, "text/markdown") {
		t.Errorf("expected text/markdown, got %s", mime)
	}
	if mime := DetectMIMEType("file.txt"); !strings.HasPrefix(mime, "text/plain") {
		t.Errorf("expected text/plain, got %s", mime)
	}
	if mime := DetectMIMEType("file.unknown"); mime != "application/octet-stream" {
		t.Errorf("expected application/octet-stream, got %s", mime)
	}

	// Registry
	reg := NewResourceMIMEAdapterRegistry()
	if desc := reg.ExtractDescription(jsonPath1, "application/json"); desc != "JSON Desc" {
		t.Errorf("registry json desc failed: %s", desc)
	}
	if title := reg.ExtractTitle(yamlPath1, "application/yaml"); title != "Test YAML" {
		t.Errorf("registry yaml title failed: %s", title)
	}
	if m := reg.ExtractMetadata(jsonPath1, "application/json"); len(m) == 0 {
		t.Error("registry metadata failed")
	}
	if reg.GetAdapter("unknown/mime") != nil {
		t.Error("expected nil adapter for unknown mime")
	}
}

// 5. ClientMetrics CompressMetrics full execution
func TestDeep5_ClientMetrics_CompressMetrics(t *testing.T) {
	tmpDir := t.TempDir()
	metricsPath := filepath.Join(tmpDir, "client-metrics.json")

	store, err := NewClientMetricsStore(metricsPath, context.Background())
	if err != nil {
		t.Fatalf("NewClientMetricsStore failed: %v", err)
	}
	defer store.Close()

	// Empty compress
	if err := store.CompressMetrics(24 * time.Hour); err != nil {
		t.Errorf("compress on empty store failed: %v", err)
	}

	// Add metrics older than 24 hours
	oldTime := time.Now().Add(-72 * time.Hour)
	store.metrics["seq-old-1"] = &ClientSequenceMetrics{
		SequenceID:  "seq-old-1",
		ClientID:    "client-1",
		FirstEvent:  oldTime,
		LastEvent:   oldTime.Add(time.Hour),
		TotalEvents: 5,
	}
	store.metrics["seq-old-2"] = &ClientSequenceMetrics{
		SequenceID:  "seq-old-2",
		ClientID:    "client-2",
		FirstEvent:  oldTime,
		LastEvent:   oldTime.Add(2 * time.Hour),
		TotalEvents: 10,
	}
	// Add fresh metric that shouldn't be archived
	store.metrics["seq-fresh"] = &ClientSequenceMetrics{
		SequenceID:  "seq-fresh",
		ClientID:    "client-3",
		FirstEvent:  time.Now(),
		LastEvent:   time.Now(),
		TotalEvents: 1,
	}

	// Compress metrics
	if err := store.CompressMetrics(24 * time.Hour); err != nil {
		t.Fatalf("CompressMetrics failed: %v", err)
	}

	// Verify old sequences archived and fresh remains
	if store.metrics["seq-fresh"] == nil {
		t.Error("expected fresh sequence to remain in memory")
	}
	if store.metrics["seq-old-1"] != nil || store.metrics["seq-old-2"] != nil {
		t.Error("expected old sequences to be archived and removed")
	}

	// Test second compression to merge into existing archive
	store.metrics["seq-old-3"] = &ClientSequenceMetrics{
		SequenceID:  "seq-old-3",
		ClientID:    "client-4",
		FirstEvent:  oldTime,
		LastEvent:   oldTime.Add(time.Hour),
		TotalEvents: 20,
	}
	if err := store.CompressMetrics(24 * time.Hour); err != nil {
		t.Fatalf("second CompressMetrics failed: %v", err)
	}
	if store.metrics["seq-old-3"] != nil {
		t.Error("expected seq-old-3 to be archived")
	}
}

// 6. ProxyDaemon QueryEventsSubscriberCount and QueryDiagnostics tests
func TestDeep5_ProxyDaemon_Queries(t *testing.T) {
	ctx := context.Background()

	// Nil receiver checks
	var nilPD *ProxyDaemon
	if _, err := nilPD.QueryEventsSubscriberCount(ctx); err == nil {
		t.Error("expected error for nil QueryEventsSubscriberCount")
	}
	if _, err := nilPD.QueryDiagnostics(ctx); err == nil {
		t.Error("expected error for nil QueryDiagnostics")
	}

	// Mock server
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	pd := NewProxyDaemon(ln.Addr().String(), nil)

	// Server goroutine answering requests
	goroutinelabels.NewGoroutine("mock_proxy_server", "mock mcp proxy server loop").StartSimple(func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			cConn := conn
			goroutinelabels.NewGoroutine("mock_proxy_conn", "mock mcp proxy conn handler").StartSimple(func() {
				defer cConn.Close()
				br := bufio.NewReader(cConn)
				for {
					line, err := br.ReadBytes('\n')
					if err != nil {
						return
					}
					line = bytes.TrimSpace(line)
					if len(line) == 0 {
						continue
					}
					var req map[string]any
					if json.Unmarshal(line, &req) != nil {
						return
					}
					id := req["id"]
					method := req["method"]

					switch method {
					case "initialize":
						resp := fmt.Sprintf(`{"jsonrpc":"2.0","id":%v,"result":{"protocolVersion":"2024-11-05"}}`+"\n", id)
						_, _ = cConn.Write([]byte(resp))
					case "events/list":
						resp := fmt.Sprintf(`{"jsonrpc":"2.0","id":%v,"result":{"subscriberCount":4}}`+"\n", id)
						_, _ = cConn.Write([]byte(resp))
					case "system/diagnostics":
						resp := fmt.Sprintf(`{"jsonrpc":"2.0","id":%v,"result":{"status":"healthy","uptime":42}}`+"\n", id)
						_, _ = cConn.Write([]byte(resp))
					}
				}
			})
		}
	})

	// Test subscriber count
	count, err := pd.QueryEventsSubscriberCount(ctx)
	if err != nil {
		t.Errorf("QueryEventsSubscriberCount failed: %v", err)
	}
	if count != 4 {
		t.Errorf("expected 4 subscribers, got %d", count)
	}

	// Test diagnostics
	diag, err := pd.QueryDiagnostics(ctx)
	if err != nil {
		t.Errorf("QueryDiagnostics failed: %v", err)
	}
	if diag["status"] != "healthy" {
		t.Errorf("expected healthy status, got %v", diag)
	}
}

// 7. ServerInitHelpers Role and Security validation tests
func TestDeep5_ServerInitHelpers_RolesAndAuth(t *testing.T) {
	s := NewServer()
	s.config = &ServerConfig{}
	s.config.MCPServer.Security.ValidateRoles = true

	// validateRolesAgainstSystem
	clientInfoMap := map[string]any{
		clientInfoRoles: []any{"developer"},
	}
	_ = s.validateRolesAgainstSystem(clientInfoMap)

	clientInfoString := map[string]any{
		clientInfoRoles: "developer, admin",
	}
	_ = s.validateRolesAgainstSystem(clientInfoString)

	clientInfoEmpty := map[string]any{}
	if err := s.validateRolesAgainstSystem(clientInfoEmpty); err != nil {
		t.Errorf("expected nil error for empty roles, got %v", err)
	}

	// enabledAuthStrategiesOrDefault
	ctxAuth := context.Background()
	enabled := s.enabledAuthStrategiesOrDefault(ctxAuth, "client-1")
	if len(enabled) == 0 {
		t.Error("expected non-empty auth strategies")
	}

	// initializeSecurityContext
	ctx := context.Background()
	clientInfoSec := map[string]any{
		clientInfoAccountID: "ACC-TEST",
		clientInfoRoles:     []any{"developer"},
		clientInfoSessionID: "MCP-001",
	}
	secCtx := s.initializeSecurityContext(ctx, clientInfoSec)
	if secCtx == nil || secCtx.AccountID != "ACC-TEST" {
		t.Errorf("initializeSecurityContext failed: %v", secCtx)
	}
	if s.GetCurrentSessionID() != "MCP-001" {
		t.Errorf("expected session MCP-001, got %s", s.GetCurrentSessionID())
	}
}

// 8. ServerTCP ServeTLS error paths
func TestDeep5_ServerTCP_ServeTLS(t *testing.T) {
	s := NewServer()

	// Non-loopback
	if err := s.ServeTLS("192.168.1.100:8080", "cert.pem", "key.pem"); err == nil {
		t.Error("expected error for non-loopback addr")
	}

	// Missing cert or key
	if err := s.ServeTLS("127.0.0.1:0", "", "key.pem"); err == nil {
		t.Error("expected error for missing cert")
	}
	if err := s.ServeTLS("127.0.0.1:0", "cert.pem", ""); err == nil {
		t.Error("expected error for missing key")
	}

	// Non-existent cert and key
	if err := s.ServeTLS("127.0.0.1:0", "missing.crt", "missing.key"); err == nil {
		t.Error("expected error for non-existent cert files")
	}
}

// 9. ToolsWorkflow Extractors comprehensive tests
func TestDeep5_ToolsWorkflow_Extractors(t *testing.T) {
	// extractWhatsNextLeadPlan
	plan, ok, err := extractWhatsNextLeadPlan(map[string]any{
		"data": map[string]any{
			workflowKeyPriorityPlan: map[string]any{"id": "PRI-DATA"},
		},
	})
	if err != nil || !ok || plan == nil {
		t.Errorf("extractWhatsNextLeadPlan data nest failed: %v, %v", ok, err)
	}

	planStr, ok, err := extractWhatsNextLeadPlan(`{"priority_plan":{"id":"PRI-JSON"}}`)
	if err != nil || !ok || planStr == nil {
		t.Errorf("extractWhatsNextLeadPlan json string failed: %v, %v", ok, err)
	}

	_, _, err = extractWhatsNextLeadPlan(`{not json`)
	if err == nil {
		t.Error("expected error for invalid json string")
	}

	_, _, err = extractWhatsNextLeadPlan(12345)
	if err == nil {
		t.Error("expected error for non-map/string type")
	}

	// extractWorkflowResultItem
	item, ok, err := extractWorkflowResultItem(map[string]any{
		workflowKeyObjects: []map[string]any{{"id": "BLI-MAP"}},
	})
	if err != nil || !ok || item == nil {
		t.Errorf("extractWorkflowResultItem []map failed: %v, %v", ok, err)
	}

	itemSlice, ok, err := extractWorkflowResultItem([]any{"item1", "item2"})
	if err != nil || !ok || itemSlice != "item1" {
		t.Errorf("extractWorkflowResultItem []any failed: %v, %v", ok, err)
	}

	_, ok, _ = extractWorkflowResultItem([]any{})
	if ok {
		t.Error("expected false for empty []any")
	}

	itemSliceMap, ok, err := extractWorkflowResultItem([]map[string]any{{"id": "ITEM-MAP"}})
	if err != nil || !ok || itemSliceMap == nil {
		t.Errorf("extractWorkflowResultItem []map[string]any failed: %v, %v", ok, err)
	}

	_, ok, _ = extractWorkflowResultItem([]map[string]any{})
	if ok {
		t.Error("expected false for empty []map[string]any")
	}

	itemStr, ok, err := extractWorkflowResultItem(`{"objects":["str-item"]}`)
	if err != nil || !ok || itemStr != "str-item" {
		t.Errorf("extractWorkflowResultItem json string failed: %v, %v", ok, err)
	}

	_, ok, _ = extractWorkflowResultItem(`{"other":1}`)
	if ok {
		t.Error("expected false for json without objects")
	}

	_, _, err = extractWorkflowResultItem(`{not json`)
	if err == nil {
		t.Error("expected error for invalid json in extractWorkflowResultItem")
	}

	_, _, err = extractWorkflowResultItem(true)
	if err == nil {
		t.Error("expected error for boolean in extractWorkflowResultItem")
	}
}

// 10. RoleAwarePrompts formatPermissionGroup and UpgradeInstructions
func TestDeep5_RoleAwarePrompts_FormatAndUpgrade(t *testing.T) {
	generator := NewRoleAwarePromptGenerator(&ProjectContext{}, []string{"developer"})

	// formatPermissionGroup
	strWildcard := generator.formatPermissionGroup("read", []string{"read:*"})
	if !strings.Contains(strWildcard, "read:*") {
		t.Errorf("expected wildcard text, got: %s", strWildcard)
	}

	strFew := generator.formatPermissionGroup("write", []string{"write:bli", "write:req"})
	if !strings.Contains(strFew, "write:bli") || !strings.Contains(strFew, "write:req") {
		t.Errorf("expected few items text, got: %s", strFew)
	}

	strMany := generator.formatPermissionGroup("manage", []string{"p1", "p2", "p3", "p4", "p5"})
	if !strings.Contains(strMany, "... and 2 more") {
		t.Errorf("expected ellipsis text for many items, got: %s", strMany)
	}

	// generateAccessUpgradeInstructions
	resEmpty := generator.generateAccessUpgradeInstructions()
	if resEmpty != "" {
		t.Errorf("expected empty for generator without guidanceGenerator, got %q", resEmpty)
	}
}

// 11. MessageProcessor registerNewClient and ensureClientQueue branches
func TestDeep5_MessageProcessor_QueueAndClient(t *testing.T) {
	s := NewServer()
	s.maxClients = 1
	mp := NewMessageProcessor(s, s.setupHandlers(), &mockTransport{})

	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	format := &MessageFormat{IsRawJSON: true}

	// Register first client
	mp.registerNewClient("client-1", writer, format)
	if len(s.clients) != 1 {
		t.Errorf("expected 1 client, got %d", len(s.clients))
	}

	// Register second client when maxClients = 1 (should be rejected)
	mp.registerNewClient("client-2", writer, format)
	if len(s.clients) != 1 {
		t.Errorf("expected client limit enforced, got %d", len(s.clients))
	}

	// Multi-client mode
	s.multiClient.Store(true)
	s.maxClients = 0
	mp.registerNewClient("client-3", writer, format)
	if s.clients["client-3"] == nil {
		t.Error("expected client-3 registered in multi-client mode")
	}

	// ensureClientQueue
	conn := &ClientConnection{
		ID:     "client-test",
		Writer: writer,
		Format: format,
	}

	// multiClient true
	mp.ensureClientQueue(conn, writer, format)
	if conn.Queue == nil {
		t.Error("expected queue created in multiClient mode")
	}

	// multiClient false with active queue
	s.multiClient.Store(false)
	q := NewMessageQueue(writer, format, s.getQueueConfig())
	conn.Queue = q
	mp.ensureClientQueue(conn, writer, format)
	if conn.Queue != q {
		t.Error("expected existing active queue preserved")
	}

	// multiClient false with abandoned queue
	q.Abandon()
	mp.ensureClientQueue(conn, writer, format)
	if conn.Queue == q {
		t.Error("expected new queue created after abandon")
	}
}
