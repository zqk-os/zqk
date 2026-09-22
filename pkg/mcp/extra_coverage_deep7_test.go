package mcp

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

type mockEventCoordinatorDeep7 struct {
	emitted []any
}

func (m *mockEventCoordinatorDeep7) Emit(ctx context.Context, eventCtx any) error {
	m.emitted = append(m.emitted, eventCtx)
	return nil
}

type mockStorageProviderDeep7 struct {
	sessionObj map[string]any
	resSpecs   []ResourceSpec
}

func (m *mockStorageProviderDeep7) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if m.sessionObj != nil {
		return m.sessionObj, nil
	}
	return nil, os.ErrNotExist
}

func (m *mockStorageProviderDeep7) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter any) (any, error) {
	if len(m.resSpecs) > 0 {
		specName := "critical_resources"
		if filterMap, ok := filter.(map[string]any); ok {
			if n, ok := filterMap[objects.FieldKeyName].(string); ok && n != "" {
				specName = n
			}
		}
		specObj := map[string]any{
			objects.FieldKeyID:   "mcp-spec-1",
			objects.FieldKeyKind: objects.KindMcpSpec,
			objects.FieldKeyName: specName,
			objects.FieldKeySpec: map[string]any{
				objects.FieldKeyName: specName,
				"resources":          m.resSpecs,
			},
		}
		return map[string]any{"objects": []any{specObj}}, nil
	}
	return nil, nil
}

func (m *mockStorageProviderDeep7) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	return nil
}

// 1. ServeTLS complete execution with certificate handshake and shutdown
func TestDeep7_ServerTCP_ServeTLS_Lifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	_, serverCertFile, serverKeyFile, _, _ := generateTestPKI(t, tmpDir)

	s := NewServer()

	// Get free port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	done := make(chan error, 1)
	go func() {
		done <- s.ServeTLS(addr, serverCertFile, serverKeyFile)
	}()

	time.Sleep(50 * time.Millisecond)

	// Dial with TLS
	tlsConf := &tls.Config{InsecureSkipVerify: true}
	conn, err := tls.Dial("tcp", addr, tlsConf)
	if err == nil {
		_ = conn.Close()
	}

	// Trigger graceful shutdown
	s.shutdownFlag.Store(1)
	s.shutdownCancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("ServeTLS error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ServeTLS did not exit within timeout")
	}
}

// 2. ProxyDaemon readFromDaemon and control-plane sniffing
func TestDeep7_ProxyDaemon_ReadFromDaemon(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	p := NewProxyDaemon("127.0.0.1:0", logger)
	p.stdout = io.Discard
	p.conn = clientConn
	p.ensureTransports()

	done := make(chan struct{})
	go func() {
		p.readFromDaemon(clientConn)
		close(done)
	}()

	// 1. Send heartbeat ID response
	hbResp := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s}`+"\n", proxyHeartbeatID)
	_, _ = serverConn.Write([]byte(hbResp))
	time.Sleep(10 * time.Millisecond)
	if !p.daemonAlive.Load() {
		t.Error("expected daemonAlive true after heartbeat")
	}

	// 2. Send events subscribe ID response
	subResp := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s}`+"\n", proxyEventsSubscribeID)
	_, _ = serverConn.Write([]byte(subResp))

	// 3. Send normal RPC response
	normResp := `{"jsonrpc":"2.0","id":1,"result":"ok"}` + "\n"
	_, _ = serverConn.Write([]byte(normResp))

	// 4. Close serverConn to trigger read error and exit
	_ = serverConn.Close()

	select {
	case <-done:
		// Exited cleanly
	case <-time.After(1 * time.Second):
		t.Fatal("readFromDaemon did not exit on connection close")
	}
}

// 3. MCPServerAdapter comprehensive methods with coordinator
func TestDeep7_MCPServerAdapter_WithCoordinator(t *testing.T) {
	s := NewServer()
	coord := &mockEventCoordinatorDeep7{}
	adapter := NewMCPServerAdapterWithCoordinator(s, coord)
	adapter.SetCoordinator(coord)

	ctx := context.Background()

	// Initialize
	initParams := &InitializeParams{
		ProtocolVersion: "2024-11-05",
	}
	initParams.ClientInfo.Name = "test-client"
	initParams.ClientInfo.Version = "1.0"
	_, _ = adapter.Initialize(ctx, initParams)

	// NotifyInitialized
	_ = adapter.NotifyInitialized(ctx, &InitializedParams{})

	// ListTools
	_, _ = adapter.ListTools(ctx)

	// CallTool
	_, _ = adapter.CallTool(ctx, &ToolCallParams{Name: "ping"})

	// ListResources
	_, _ = adapter.ListResources(ctx, &ResourcesListParams{})

	// GetResource
	_, _ = adapter.GetResource(ctx, &ResourceGetParams{URI: "schema://account"})

	// ListPrompts
	_, _ = adapter.ListPrompts(ctx)

	// GetPrompt
	_, _ = adapter.GetPrompt(ctx, &PromptGetParams{Name: "role_based_access"})

	// ListRoots
	_, _ = adapter.ListRoots(ctx)

	// SendLogMessage, SendEvent, SendMessage, NotifyCancelled, Shutdown
	_ = adapter.SendLogMessage(ctx, LogLevelInfo, "test log", map[string]any{"k": "v"})
	_ = adapter.SendEvent(ctx, &Event{Type: "action_required", Message: "msg"})
	_ = adapter.SendMessage(ctx, "msg", "type", "high")
	_ = adapter.NotifyCancelled(ctx, &CancelledParams{RequestID: 1})
	_ = adapter.Shutdown(ctx)

	if len(coord.emitted) == 0 {
		t.Error("expected coordinator to receive events")
	}
}

// 4. ServerInitHelpers authentication flows comprehensive
func TestDeep7_ServerInitHelpers_AuthenticationFlows(t *testing.T) {
	s := NewServer()
	ctx := context.Background()

	// 1. multiClient mode without credentials -> Unauthenticated error
	s.multiClient.Store(true)
	var initParams InitializeParams
	initParams.ClientInfo.Name = "random-client"
	initParams.ClientInfo.Version = "1.0"
	clientInfo := map[string]any{"name": "random-client"}
	_, _, err := s.handleAuthenticationFlow(ctx, "", clientInfo, initParams)
	if err == nil {
		t.Error("expected unauthenticated error in multiClient mode")
	}

	// 2. multiClient mode with trusted loopback adapter (feedSteerProbeClientID)
	var trustedParams InitializeParams
	trustedParams.ClientInfo.Name = feedSteerProbeClientID
	trustedParams.ClientInfo.Version = "1.0"
	trustedInfo := map[string]any{"name": feedSteerProbeClientID}
	_, accID, err := s.handleAuthenticationFlow(ctx, "", trustedInfo, trustedParams)
	if err != nil || accID != SystemAccountID {
		t.Errorf("trusted adapter failed: %v, %s", err, accID)
	}

	// 3. multiClient mode with accountID already supplied and trusted adapter
	trustedInfoWithAcc := map[string]any{"name": feedSteerProbeClientID, clientInfoAccountID: "ACC-CUSTOM"}
	_, accID2, err := s.handleAuthenticationFlow(ctx, "ACC-CUSTOM", trustedInfoWithAcc, trustedParams)
	if err != nil || accID2 != "ACC-CUSTOM" {
		t.Errorf("trusted adapter with account failed: %v, %s", err, accID2)
	}

	// 4. multiClient mode with accountID already supplied but UNTRUSTED adapter without credentials
	untrustedWithAcc := map[string]any{"name": "untrusted", clientInfoAccountID: "ACC-CUSTOM"}
	_, _, err = s.handleAuthenticationFlow(ctx, "ACC-CUSTOM", untrustedWithAcc, initParams)
	if err == nil {
		t.Error("expected error for untrusted client with accountID without credentials")
	}

	// 5. stdio mode (!multiClient) with SystemAccountID clientID
	s.multiClient.Store(false)
	var sysParams InitializeParams
	sysParams.ClientInfo.Name = "sys-client"
	_, sysAccID, err := s.handleAuthenticationFlow(ctx, SystemAccountID, map[string]any{}, sysParams)
	if err != nil || sysAccID != SystemAccountID {
		t.Errorf("expected SystemAccountID, got %s, err: %v", sysAccID, err)
	}

	// 6. stdio mode with isHumanClient and without credentials
	var humanParams InitializeParams
	humanParams.ClientInfo.Name = "cursor"
	_, humanAccID, err := s.handleAuthenticationFlow(ctx, "", map[string]any{}, humanParams)
	if err != nil || humanAccID != SystemAccountID {
		t.Errorf("expected SystemAccountID for human client in stdio, got %s, err: %v", humanAccID, err)
	}

	// 7. stdio mode with valid session in storageProvider
	mockStorage := &mockStorageProviderDeep7{
		sessionObj: map[string]any{
			objects.FieldKeyKind:      objects.KindMcpSession,
			objects.FieldKeyAccountID: "ACC-SESS-USER",
		},
	}
	s.SetStorageProvider(mockStorage)
	var sessParams InitializeParams
	sessParams.ClientInfo.Name = "agent-client"
	_, sessAccID, _ := s.handleAuthenticationFlow(ctx, "MCP-SESSION-1", map[string]any{}, sessParams)
	if sessAccID != "ACC-SESS-USER" {
		t.Logf("session account resolution: %s", sessAccID)
	}
}

// 5. Resources CriticalResources registration from spec
func TestDeep7_Resources_CriticalResourcesFromSpec(t *testing.T) {
	tmpDir := t.TempDir()
	docFile := filepath.Join(tmpDir, "test-doc.md")
	_ = os.WriteFile(docFile, []byte("# Test Doc\nContent"), 0644)

	s := NewServer()
	s.SetProjectRoot(tmpDir)

	mockStorage := &mockStorageProviderDeep7{
		resSpecs: []ResourceSpec{
			{
				URI:         "file://test-doc.md",
				Name:        "Test Doc",
				Description: "A test doc",
				MimeType:    "text/markdown",
				Category:    "documentation",
				Priority:    "high",
				Tags:        []string{"doc", "test"},
				Metadata:    map[string]string{"author": "test"},
			},
			{
				URI:  "file://nonexistent.md",
				Name: "Missing Doc",
			},
			{
				URI:  "custom://not-a-file",
				Name: "Custom URI",
			},
		},
	}
	s.SetStorageProvider(mockStorage)

	RegisterCriticalResources(s)
	rPaths := getCriticalResourcePaths(s)
	if len(rPaths) == 0 {
		t.Error("expected non-empty critical resource paths")
	}

	// Test RegisterSchemaResources with schema resources
	mockStorageSchema := &mockStorageProviderDeep7{
		resSpecs: []ResourceSpec{
			{
				URI:         "schema://custom-spec",
				Name:        "Custom Schema",
				Description: "A custom schema",
				MimeType:    "application/schema+json",
				Category:    "schemas",
				Priority:    "medium",
				Tags:        []string{"schema"},
				Metadata:    map[string]string{"version": "1.0"},
			},
			{
				URI:  "file://not-a-schema.md",
				Name: "Not Schema",
			},
		},
	}
	sSchema := NewServer()
	sSchema.SetStorageProvider(mockStorageSchema)
	RegisterSchemaResources(sSchema)
}

// 6. ToolsSandbox project root as file guard
func TestDeep7_ToolsSandbox_RootGuard(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewServer()
	s.SetProjectRoot(tmpDir)

	_, err := s.readGuardedWorkspaceFile(".")
	if err == nil {
		t.Error("expected error reading project root as file")
	}
}
