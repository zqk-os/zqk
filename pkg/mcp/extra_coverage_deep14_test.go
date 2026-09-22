package mcp

import (
	"bufio"
	"bytes"
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// 1. Permission format helpers and string utilities
func TestDeep14_PermissionFormatHelpers_Comprehensive(t *testing.T) {
	s := NewServer()

	// Default operations
	ops := s.getPermissionOperations()
	if len(ops) < 4 {
		t.Errorf("expected at least 4 default ops, got: %d", len(ops))
	}

	// Operations from config
	s.config = &ServerConfig{}
	s.config.MCPServer.Security.PermissionOperations = []string{"read", "write"}
	opsConfig := s.getPermissionOperations()
	if len(opsConfig) != 2 {
		t.Errorf("expected 2 ops from config, got: %d", len(opsConfig))
	}

	// getObjectKindsForExamples
	kinds := s.getObjectKindsForExamples()
	if len(kinds) == 0 {
		t.Log("note: no kinds returned in mock environment")
	}

	// generatePermissionFormatExamples
	_ = s.generatePermissionFormatExamples()

	// Utility functions: capitalizeFirst and min
	if c := capitalizeFirst(""); c != "" {
		t.Errorf("expected empty string, got %s", c)
	}
	if c := capitalizeFirst("read"); c != "Read" {
		t.Errorf("expected Read, got %s", c)
	}
	if m := min(5, 3); m != 3 {
		t.Errorf("expected 3, got %d", m)
	}
	if m := min(2, 7); m != 2 {
		t.Errorf("expected 2, got %d", m)
	}
}

// 2. ServerLifecycleBuilder ApplyAsyncConfig comprehensive tests
func TestDeep14_ServerLifecycleBuilder_ApplyAsyncConfig(t *testing.T) {
	s := NewServer()
	var traceBuf bytes.Buffer
	s.SetTraceWriter(&traceBuf)

	// Valid async config with duration
	cfgValid := &ServerConfig{}
	cfgValid.MCPServer.Async.MaxConcurrent = 15
	cfgValid.MCPServer.Async.Timeout = "25s"

	builder1 := &ServerLifecycleBuilder{server: s, config: cfgValid}
	builder1.ApplyAsyncConfig()
	if s.asyncConfig.MaxConcurrent != 15 || s.asyncConfig.Timeout != 25*time.Second {
		t.Errorf("unexpected async config: %v", s.asyncConfig)
	}

	// Invalid async timeout duration
	cfgInvalid := &ServerConfig{}
	cfgInvalid.MCPServer.Async.Timeout = "not-a-duration"
	builder2 := &ServerLifecycleBuilder{server: s, config: cfgInvalid}
	builder2.ApplyAsyncConfig()

	// Empty async timeout
	cfgEmpty := &ServerConfig{}
	builder3 := &ServerLifecycleBuilder{server: s, config: cfgEmpty}
	builder3.ApplyAsyncConfig()
}

// 3. Server HandleToolCall public wrapper and notification routing
func TestDeep14_Server_PublicHandleToolCallAndNotifications(t *testing.T) {
	s := NewServer()
	ctx := context.Background()

	// Public HandleToolCall with actor context
	res, err := s.HandleToolCall(ctx, "test_echo", map[string]any{
		"message": "echo_public",
		"_meta": map[string]any{
			"account_id": "ACC-CALLER",
		},
	})
	if err != nil || res == nil {
		t.Errorf("HandleToolCall failed: %v", err)
	}

	// HandleToolCall error counter
	_, _ = s.HandleToolCall(ctx, "unknown_tool", map[string]any{})

	// CheckFormatPermission streaming permissions
	secAdmin := pkgctx.NewSecurityContext("ACC-ADMIN", []string{"admin"}, []string{"*"})
	s.SetSecurityContext(secAdmin)
	allowedStream, _ := s.CheckFormatPermission(ctx, "json-rpc")
	if !allowedStream {
		t.Error("expected json-rpc streaming format allowed for admin")
	}

	secViewer := pkgctx.NewSecurityContext("ACC-VIEWER", []string{"viewer"}, []string{"read:status"})
	s.SetSecurityContext(secViewer)
	allowedViewer, _ := s.CheckFormatPermission(ctx, "json-rpc")
	if allowedViewer {
		t.Error("expected streaming format denied for viewer without stream:* or read:*")
	}

	secStreamer := pkgctx.NewSecurityContext("ACC-STREAM", []string{"developer"}, []string{"stream:*"})
	s.SetSecurityContext(secStreamer)
	allowedExplicit, _ := s.CheckFormatPermission(ctx, "stream")
	if !allowedExplicit {
		t.Error("expected explicit stream allowed")
	}

	// SendNotificationToClient
	// 1. Unknown client
	err = s.SendNotificationToClient("unknown_client", "test/notify", map[string]any{"data": 1})
	if err == nil {
		t.Error("expected error for unknown client notification")
	}

	// 2. Known client with queue
	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	format := &MessageFormat{IsRawJSON: true}
	queue := NewMessageQueue(writer, format, DefaultQueueConfig())

	s.clientsMu.Lock()
	s.clients["client_active"] = &ClientConnection{
		ID:     "client_active",
		Writer: writer,
		Format: format,
		Queue:  queue,
	}
	s.clientsMu.Unlock()

	err = s.SendNotificationToClient("client_active", "test/notify", map[string]any{"hello": "client"})
	if err != nil {
		t.Errorf("expected successful notification delivery to active client: %v", err)
	}
}
