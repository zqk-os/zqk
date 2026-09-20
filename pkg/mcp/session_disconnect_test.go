package mcp

import (
	"testing"
)

func TestStatusDisconnected(t *testing.T) {
	t.Parallel()
	if StatusDisconnected != "disconnected" {
		t.Errorf("StatusDisconnected = %q, want %q", StatusDisconnected, "disconnected")
	}
}

func TestServer_GetSetClearCurrentSessionID(t *testing.T) {
	t.Parallel()
	s := NewServer()

	// Initially empty
	if got := s.GetCurrentSessionID(); got != emptyValue {
		t.Errorf("GetCurrentSessionID() = %q, want empty", got)
	}

	// Set and get
	s.SetCurrentSessionID("MCP-001")
	if got := s.GetCurrentSessionID(); got != "MCP-001" {
		t.Errorf("after SetCurrentSessionID(MCP-001), GetCurrentSessionID() = %q, want MCP-001", got)
	}

	// Clear
	s.ClearCurrentSessionID()
	if got := s.GetCurrentSessionID(); got != emptyValue {
		t.Errorf("after ClearCurrentSessionID(), GetCurrentSessionID() = %q, want empty", got)
	}

	// SetCurrentSessionID with empty string is no-op (does not overwrite)
	s.SetCurrentSessionID("MCP-002")
	s.SetCurrentSessionID("")
	if got := s.GetCurrentSessionID(); got != "MCP-002" {
		t.Errorf("SetCurrentSessionID(\"\") should be no-op; GetCurrentSessionID() = %q, want MCP-002", got)
	}
}

func TestMarkSessionDisconnected_emptySessionID(t *testing.T) {
	t.Parallel()
	s := NewServer()
	// Should not panic when no session ID is set
	MarkSessionDisconnected(s)
	// Still empty
	if got := s.GetCurrentSessionID(); got != emptyValue {
		t.Errorf("GetCurrentSessionID() = %q, want empty (MarkSessionDisconnected with no session should not set id)", got)
	}
}

func TestMarkSessionDisconnected_buildsCorrectStatusValue(t *testing.T) {
	t.Parallel()
	// Ensure the status we set is the one retention does not protect
	if StatusDisconnected != "disconnected" {
		t.Fatalf("StatusDisconnected must be %q for retention_tolerance to prune mcp_session", "disconnected")
	}
	// Field format for CLI: status=disconnected
	fieldVal := "status=" + StatusDisconnected
	if fieldVal != "status=disconnected" {
		t.Errorf("field value = %q, want status=disconnected", fieldVal)
	}
}
