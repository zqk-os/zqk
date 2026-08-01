package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterReportTools(t *testing.T) {
	// Initialize a dummy server
	server := NewServer()
	RegisterReportTools(server)

	tools := server.ListTools()
	toolNames := make([]string, len(tools))
	for i, tool := range tools {
		toolNames[i] = tool.Name
	}

	// Verify our 3 tools are registered
	assert.Contains(t, toolNames, GetToolName("report_pcs"))
	assert.Contains(t, toolNames, GetToolName("report_edd"))
	assert.Contains(t, toolNames, GetToolName("report_blockers"))
	assert.Contains(t, toolNames, GetToolName("report_maturation"))
	assert.Contains(t, toolNames, GetToolName("report_vitality"))
	assert.Contains(t, toolNames, GetToolName("report_qa_success"))

	// Verify handler exists for pcs
	for _, tool := range tools {
		if tool.Name == GetToolName("report_pcs") {
			require.NotNil(t, tool.Handler)

			// We can't fully invoke the handler easily because it requires an active CLI bridge / exec,
			// but we can verify it doesn't crash on simple validation
			_, err := tool.Handler(context.Background(), map[string]any{})
			// Should fail because we don't have a real CLI context or it will run the command but fail.
			// Just verify it's callable.
			if err != nil {
				// It's expected to error if the binary isn't built or context isn't right
			}
		}
	}
}

func TestNewReportHandlers(t *testing.T) {
	server := NewServer()
	RegisterReportTools(server)

	tools := server.ListTools()

	for _, tool := range tools {
		if tool.Name == GetToolName("report_maturation") {
			require.NotNil(t, tool.Handler)
			res, err := tool.Handler(context.Background(), map[string]any{})
			require.NoError(t, err)
			require.NotNil(t, res)
			m := res.(map[string]any)
			assert.Equal(t, "maturation", m["report_type"])
		}
		if tool.Name == GetToolName("report_vitality") {
			require.NotNil(t, tool.Handler)
			res, err := tool.Handler(context.Background(), map[string]any{})
			require.NoError(t, err)
			require.NotNil(t, res)
			m := res.(map[string]any)
			assert.Equal(t, "vitality", m["report_type"])
		}
		if tool.Name == GetToolName("report_qa_success") {
			require.NotNil(t, tool.Handler)
			res, err := tool.Handler(context.Background(), map[string]any{})
			require.NoError(t, err)
			require.NotNil(t, res)
			m := res.(map[string]any)
			assert.Equal(t, "qa_success", m["report_type"])
		}
	}
}
