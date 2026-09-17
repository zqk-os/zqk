package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterProjectContextTool(t *testing.T) {
	server := NewServer()
	RegisterProjectContextTool(server)

	tools := server.ListTools()
	toolNames := make([]string, len(tools))
	for i, tool := range tools {
		toolNames[i] = tool.Name
	}
	name := GetToolName("get_project_context")
	assert.Contains(t, toolNames, name)

	for _, tool := range tools {
		if tool.Name == name {
			require.NotNil(t, tool.Handler)
			return
		}
	}
	t.Fatalf("registered tool %q missing handler", name)
}
