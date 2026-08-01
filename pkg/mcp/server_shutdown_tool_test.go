package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/stretchr/testify/assert"
)

func TestServerShutdownTool(t *testing.T) {
	// Setup a new server instance
	server := NewServer()

	// Register the shutdown tool explicitly for the test
	RegisterCommonTools(server)

	// Context with timeout to avoid hanging
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Call the server_shutdown tool
	args := map[string]any{
		objects.FieldKeyReason: "testing shutdown tool",
	}

	result, err := server.handleToolCallWithContext(ctx, GetToolName("server_shutdown"), args)

	// Check results
	assert.NoError(t, err)
	assert.NotNil(t, result)

	resMap, ok := result.(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "success", resMap[objects.FieldKeyStatus])
	assert.Equal(t, "Shutdown requested: testing shutdown tool", resMap["message"])

	// The server should now have shutdown requested
	assert.True(t, server.IsShutdownRequested())
	assert.Equal(t, "testing shutdown tool", server.shutdownReason)
}
