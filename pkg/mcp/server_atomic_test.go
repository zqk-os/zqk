package mcp

import (
	"testing"
)

func TestServer_InitializedAtomic(t *testing.T) {
	t.Parallel()

	var nilServer *Server
	if nilServer.IsInitialized() {
		t.Fatalf("expected nil server IsInitialized to return false")
	}

	server := NewServer()
	if server.IsInitialized() {
		t.Fatalf("expected initial server IsInitialized to return false")
	}

	server.SetInitialized(true)
	if !server.IsInitialized() {
		t.Fatalf("expected server IsInitialized to return true after SetInitialized(true)")
	}

	server.SetInitialized(false)
	if server.IsInitialized() {
		t.Fatalf("expected server IsInitialized to return false after SetInitialized(false)")
	}
}
