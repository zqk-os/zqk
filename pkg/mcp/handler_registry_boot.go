package mcp

import (
	"context"

	"github.com/zqk-os/zqk/pkg/objects"
)

func init() {
	RegisterStandardHandlers(GlobalToolHandlerRegistry)
}

// RegisterStandardHandlers populates a ToolHandlerRegistry with built-in MCP tool handlers.
func RegisterStandardHandlers(registry *ToolHandlerRegistry) {
	registry.Register("echo", HandleEcho)
	registry.Register("test_echo", HandleEcho)
	registry.Register("zqk_test_echo", HandleEcho)
	registry.Register("metrics", handleMetricsAdapter)
	registry.Register("get_metrics", handleMetricsAdapter)
	registry.Register("zqk_get_metrics", handleMetricsAdapter)
}

func handleMetricsAdapter(ctx context.Context, args map[string]any) (any, error) {
	format, _ := args[objects.FieldKeyFormat].(string)
	if format == "" {
		format = "summary"
	}
	return map[string]any{
		objects.FieldKeyStatus: "success",
		objects.FieldKeyFormat: format,
		"meta":                 map[string]any{objects.FieldKeySource: "GlobalToolHandlerRegistry"},
	}, nil
}
