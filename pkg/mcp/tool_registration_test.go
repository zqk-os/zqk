package mcp

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// TestToolRegistrationFlow tests the exact flow that mcp-simple uses:
// 1. Register minimal tools in main()
// 2. Call initialize (which calls prepareReinitialization and registerToolsAndResources)
// 3. Verify tools are still registered
func TestToolRegistrationFlow(t *testing.T) {
	t.Parallel()
	server := NewServer()

	// Step 1: Register minimal tools (like mcp-simple does)
	server.RegisterTool("zqk_test_echo", "Test echo tool", map[string]any{
		objects.FieldKeyType: "object",
		"properties": map[string]any{
			"message": map[string]any{objects.FieldKeyType: "string"},
		},
	}, nil)

	initialCount := server.getToolCount()
	if initialCount != 1 {
		t.Fatalf("Expected 1 tool after registration, got %d", initialCount)
	}

	// Step 2: Simulate prepareReinitialization (first time - should NOT clear)
	server.prepareReinitialization()

	afterPrepareCount := server.getToolCount()
	if afterPrepareCount != 1 {
		t.Fatalf("Expected 1 tool after prepareReinitialization (first time), got %d", afterPrepareCount)
	}

	// Step 3: Simulate registerToolsAndResources (should ADD more tools)
	secCtx := server.secCtx
	if secCtx == nil {
		// Create a minimal security context for testing
		secCtx = pkgctx.NewSystemSecurityContext()
		server.SetSecurityContext(secCtx)
	}

	secCtxTyped, ok := secCtx.(*pkgctx.SecurityContext)
	if !ok {
		t.Fatalf("Security context is not the expected type")
	}

	server.registerToolsAndResources(secCtxTyped)

	finalCount := server.getToolCount()
	if finalCount < 1 {
		t.Fatalf("Expected at least 1 tool after registerToolsAndResources, got %d", finalCount)
	}

	t.Logf("Tool registration flow: initial=%d, afterPrepare=%d, final=%d", initialCount, afterPrepareCount, finalCount)

	// Verify the original tool is still there
	server.toolsMu.RLock()
	_, exists := server.tools["zqk_test_echo"]
	server.toolsMu.RUnlock()

	if !exists {
		t.Errorf("Original tool 'zqk_test_echo' was lost during registration")
	}
}

// TestApplyToolsAllowlist verifies that when an allowlist is applied, only listed tools remain.
func TestApplyToolsAllowlist(t *testing.T) {
	t.Parallel()
	server := NewServer()

	server.RegisterTool("zqk_a", "Tool A", nil, nil)
	server.RegisterTool("zqk_b", "Tool B", nil, nil)
	server.RegisterTool("zqk_c", "Tool C", nil, nil)

	if server.getToolCount() != 3 {
		t.Fatalf("expected 3 tools, got %d", server.getToolCount())
	}

	// Empty allowlist = no-op
	server.ApplyToolsAllowlist(nil)
	if server.getToolCount() != 3 {
		t.Errorf("empty allowlist should be no-op, got %d tools", server.getToolCount())
	}
	server.ApplyToolsAllowlist([]string{})
	if server.getToolCount() != 3 {
		t.Errorf("empty allowlist should be no-op, got %d tools", server.getToolCount())
	}

	// Non-empty allowlist keeps only listed tools
	server.ApplyToolsAllowlist([]string{"zqk_a", "zqk_c"})
	if server.getToolCount() != 2 {
		t.Fatalf("expected 2 tools after allowlist, got %d", server.getToolCount())
	}
	tools := server.ListTools()
	names := make(map[string]bool)
	for _, t := range tools {
		names[t.Name] = true
	}
	if !names["zqk_a"] || !names["zqk_c"] || names["zqk_b"] {
		t.Errorf("expected only zqk_a and zqk_c, got %v", names)
	}
}

func TestToolRegistration_AutoMetricsRegistration(t *testing.T) {
	server := NewServer()
	server.RegisterTool("auto_metrics_tool", "Auto metrics test tool", nil, nil)

	snapshot := server.GetMCPMetricsSnapshot()
	toolMetrics, exists := snapshot.Tools.ByTool["auto_metrics_tool"]
	if !exists {
		t.Fatalf("Expected auto_metrics_tool to be registered in metrics snapshot")
	}

	if toolMetrics.CallCount != 0 {
		t.Errorf("Expected initial CallCount=0, got %d", toolMetrics.CallCount)
	}
}
