package mcp_harness

import "testing"

func TestNewHarness_Defaults(t *testing.T) {
	t.Parallel()
	h := NewHarness(HarnessOptions{Name: "test-harness", Version: "0.0.1"})
	if h == nil {
		t.Fatal("NewHarness returned nil")
	}
	if h.Name != "test-harness" || h.Version != "0.0.1" {
		t.Fatalf("got name=%q version=%q", h.Name, h.Version)
	}
	if h.server == nil {
		t.Fatal("expected MCP server")
	}
	h.RegisterTool(ToolRegistration{
		Name:        "noop",
		Description: "test tool",
	})
}
