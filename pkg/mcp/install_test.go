package mcp

import "testing"

func TestMCPServerConfig_Basic(t *testing.T) {
	cfg := mcpServerConfig{
		Command: "zqk",
		Args:    []string{"mcp", "serve"},
	}
	if cfg.Command != "zqk" {
		t.Fatalf("expected command zqk, got %s", cfg.Command)
	}
}
