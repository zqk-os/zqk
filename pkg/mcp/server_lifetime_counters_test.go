package mcp

import (
	"context"
	"testing"
)

func TestServer_LifetimeCounters(t *testing.T) {
	var s *Server
	reqNil, errNil, shutNil := s.GetServerStats()
	if reqNil != 0 || errNil != 0 || shutNil != 0 {
		t.Fatalf("expected nil server stats (0, 0, 0), got req=%d err=%d shut=%d", reqNil, errNil, shutNil)
	}

	server := NewServer()
	reqInit, errInit, shutInit := server.GetServerStats()
	if reqInit != 0 || errInit != 0 || shutInit != 0 {
		t.Fatalf("expected new server stats (0, 0, 0), got req=%d err=%d shut=%d", reqInit, errInit, shutInit)
	}

	// Register a dummy tool
	handler := func(ctx context.Context, args map[string]any) (any, error) {
		return map[string]any{"status": "ok"}, nil
	}
	server.RegisterTool("test_ping", "A dummy test tool", nil, handler)

	// Call registered tool -> success (req=1, err=0, shut=0)
	ctx := context.Background()
	if _, err := server.HandleToolCall(ctx, "test_ping", nil); err != nil {
		t.Fatalf("expected tool call success, got err: %v", err)
	}

	req1, err1, shut1 := server.GetServerStats()
	if req1 != 1 || err1 != 0 || shut1 != 0 {
		t.Errorf("expected stats (1, 0, 0) after success, got req=%d err=%d shut=%d", req1, err1, shut1)
	}

	// Call unregistered tool -> error (req=2, err=1, shut=0)
	if _, err := server.HandleToolCall(ctx, "unknown_tool", nil); err == nil {
		t.Errorf("expected error for unknown tool call, got nil")
	}

	req2, err2, shut2 := server.GetServerStats()
	if req2 != 2 || err2 != 1 || shut2 != 0 {
		t.Errorf("expected stats (2, 1, 0) after failure, got req=%d err=%d shut=%d", req2, err2, shut2)
	}

	// Trigger shutdown sequence -> (req=2, err=1, shut=1)
	server.shutdownSequence("test shutdown")
	req3, err3, shut3 := server.GetServerStats()
	if req3 != 2 || err3 != 1 || shut3 != 1 {
		t.Errorf("expected stats (2, 1, 1) after shutdown, got req=%d err=%d shut=%d", req3, err3, shut3)
	}
}
