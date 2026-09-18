package mcp

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/idebridge"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// HandleIdeBridgeRequest appends a zqk_ide_bridge_v1 control event for the
// installed IDE bridge extension (extensions/zqk-ide-bridge).
func HandleIdeBridgeRequest(ctx context.Context, server *Server, args map[string]any, projectRoot string) (any, error) {
	_ = ctx
	command, _ := args[objects.FieldKeyCommand].(string)
	if command == "" {
		return nil, errfmt.Errorf("missing required parameter: command")
	}
	requestID, _ := args["request_id"].(string)

	var callArgs []any
	if raw, ok := args["args"]; ok && raw != nil {
		switch v := raw.(type) {
		case []any:
			callArgs = v
		default:
			return nil, errfmt.Errorf("args must be a JSON array when provided")
		}
	}

	if server != nil {
		if root := server.GetProjectRoot(); root != "" {
			projectRoot = root
		}
	}
	if projectRoot == "" {
		cwd, err := fileutil.Getwd()
		if err != nil {
			return nil, errfmt.Errorf("failed to get working directory: %w", err)
		}
		projectRoot = cwd
	}

	path, err := idebridge.AppendRequest(projectRoot, command, requestID, callArgs)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		objects.FieldKeyStatus:  "queued",
		"schema":                idebridge.SchemaV1,
		objects.FieldKeyCommand: command,
		"request_id":            requestID,
		"control_path":          path,
		objects.FieldKeyNote:    "Extension zqk-ide-bridge must be installed and watching this JSONL; enable matching zqkIdeBridge.capabilities.",
	}, nil
}

// RegisterIdeBridgeTool registers the IDE bridge control-bus tool.
func RegisterIdeBridgeTool(server *Server) {
	server.RegisterTool(
		GetToolName("ide_bridge_request"),
		"Queue a stable zqk.* IDE command on the local zqk-ide-bridge control bus (reload MCP, new chat, mode switch, etc.). Does not call IDE private command ids — only zqk.* commands the extension maps. Requires the zqk-ide-bridge extension installed in the IDE.",
		map[string]any{
			objects.FieldKeyType: "object",
			"required":           []string{"command"},
			"properties": map[string]any{
				objects.FieldKeyCommand: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Stable extension command, e.g. zqk.mcp.reloadClient, zqk.chat.new, zqk.mode.plan",
				},
				"request_id": map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Optional correlation id for extension Output logs",
				},
				"args": map[string]any{
					objects.FieldKeyType:        "array",
					objects.FieldKeyDescription: "Optional executeCommand args array (usually empty for zqk.* wrappers)",
					"items":                     map[string]any{},
				},
			},
		},
		nil,
	)
}
