package mcp

import (
	"context"
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
	format, _ := args["format"].(string)
	if format == "" {
		format = "summary"
	}
	return map[string]any{
		"status": "success",
		"format": format,
		"meta":   map[string]any{"source": "GlobalToolHandlerRegistry"},
	}, nil
}
