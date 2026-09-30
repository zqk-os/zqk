package mcp

import (
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestMCPSpecIntegration_Startup verifies that arbitrary mcp_spec objects
// from storage are automatically loaded and applied during server initialization.
func TestMCPSpecIntegration_Startup(t *testing.T) {
	specObject := map[string]any{
		objects.FieldKeyID:     "MCPSPEC-custom",
		objects.FieldKeyKind:   objects.KindMcpSpec,
		objects.FieldKeyName:   "custom_integration_spec",
		objects.FieldKeyStatus: objects.ObjectStatusActive,
		objects.FieldKeySpec: "name: custom_integration_spec\n" +
			"prompts:\n" +
			"  - name: test_custom_prompt\n" +
			"    description: A prompt from a custom mcp_spec object\n",
		objects.FieldKeyCreatedAt: time.Now().Format(time.RFC3339),
		objects.FieldKeyUpdatedAt: time.Now().Format(time.RFC3339),
	}

	server := NewServer()
	server.storageProvider = &recordingMCPSpecStorage{
		result: map[string]any{"objects": []map[string]any{specObject}},
	}
	secCtx := pkgctx.NewSystemSecurityContext()

	// Initialize the server (which should call registerToolsAndResources internally
	// or we can call it directly for this test).
	server.registerToolsAndResources(secCtx)

	// Verify that the prompt from the custom spec was registered.
	_, ok := server.prompts["test_custom_prompt"]
	if !ok {
		t.Fatalf("Expected prompt 'test_custom_prompt' to be registered from custom mcp_spec object during startup, but it was not. Integration is missing.")
	}
}
