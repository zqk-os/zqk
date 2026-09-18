package mcp

import "github.com/zqk-os/zqk/pkg/objects"

// RegisterInteractiveTools registers interactive object creation tools
func RegisterInteractiveTools(server *Server) {
	server.RegisterTool(
		GetToolName("create_object_interactive"),
		"Interactively create an object with guided field collection. This tool uses MCP elicitation to request required fields step-by-step, ensuring all required fields are provided before object creation. Use this instead of direct YAML editing to ensure proper validation and system integrity. Example: "+GetToolName("create_object_interactive")+" with kind='backlog_item'.",
		map[string]any{
			objects.FieldKeyType: "object",
			"required":           []string{"kind"},
			"properties": map[string]any{
				objects.FieldKeyKind: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Object kind to create (e.g., 'backlog_item', 'milestone', 'criteria')",
				},
				objects.FieldKeySessionID: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Session ID for continuing an existing interactive session (optional, auto-generated if not provided)",
				},
				// Fields are provided dynamically via elicitation
				// We accept any additional fields that the client provides
			},
			"additionalProperties": true, // Allow dynamic field collection
		},
		nil, // Handler is registered in handleToolCall
	)
}
