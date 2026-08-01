package mcp

import (
	"context"
	"fmt"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
)

// HandleEcho handles the echo tool - simple test tool that echoes back a message
// This is useful for testing MCP communication and event emission
func HandleEcho(ctx context.Context, args map[string]any) (any, error) {
	message, ok := args["message"].(string)
	if !ok {
		return nil, errfmt.Errorf("missing required parameter: message")
	}

	// Optional: emit a custom event
	// This would require access to the server's eventEmitter
	// For now, we'll just return the echo

	result := map[string]any{
		"echo":                 message,
		objects.FieldKeyStatus: "success",
		"message":              fmt.Sprintf("Echo: %s", message),
	}

	return result, nil
}

// RegisterEchoTool registers the echo tool with the server
// This is a test/debug tool for validating MCP communication
func RegisterEchoTool(server *Server) {
	server.RegisterTool(
		GetToolName("test_echo"),
		"Test tool that echoes a message back - useful for testing MCP communication and validating that the MCP server is responding correctly. Use this to verify the MCP connection is working. Example: "+GetToolName("test_echo")+" with message='Hello MCP'.",
		map[string]any{
			objects.FieldKeyType: "object",
			"properties": map[string]any{
				"message": map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Message to echo back",
				},
			},
			"required": []string{"message"},
		},
		nil, // Handler is called via switch statement in handleToolCallWithContext
	)
}
