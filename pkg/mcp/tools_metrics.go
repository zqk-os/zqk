package mcp

import (
	"context"
	"fmt"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// RegisterMetricsTools registers MCP tools for accessing metrics
func RegisterMetricsTools(server *Server) {
	// Register metrics tool
	server.RegisterTool(
		GetToolName("get_metrics"),
		"Get comprehensive MCP protocol metrics snapshot",
		map[string]any{
			objects.FieldKeyType: "object",
			"properties": map[string]any{
				objects.FieldKeyFormat: map[string]any{
					objects.FieldKeyType:        "string",
					"enum":                      []string{"json", "summary"},
					objects.FieldKeyDescription: "Output format: 'json' for full metrics, 'summary' for human-readable summary",
					"default":                   "summary",
				},
			},
		},
		func(ctx context.Context, args map[string]any) (any, error) {
			return HandleGetMetrics(server, args)
		},
	)

	// Register per-tool metrics tool
	server.RegisterTool(
		GetToolName("get_tool_metrics"),
		"Get metrics for a specific tool",
		map[string]any{
			objects.FieldKeyType: "object",
			"properties": map[string]any{
				"tool_name": map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Name of the tool to get metrics for",
				},
			},
			"required": []string{"tool_name"},
		},
		func(ctx context.Context, args map[string]any) (any, error) {
			return HandleGetToolMetrics(server, args)
		},
	)
}

// HandleGetMetrics handles the get_metrics built-in tool
func HandleGetMetrics(server *Server, args map[string]any) (any, error) {
	// Get format (default to summary)
	format := "summary"
	if f, ok := args[objects.FieldKeyFormat].(string); ok && f != emptyValue {
		format = f
	}

	snapshot := server.GetMCPMetricsSnapshot()

	if format == "json" {
		// Return full JSON
		return snapshot, nil
	}

	// Return human-readable summary
	return formatMetricsSummary(snapshot), nil
}

// HandleGetToolMetrics handles the get_tool_metrics built-in tool
func HandleGetToolMetrics(server *Server, args map[string]any) (any, error) {
	toolName, ok := args["tool_name"].(string)
	if !ok || toolName == emptyValue {
		return nil, errfmt.Errorf("tool_name is required")
	}

	snapshot := server.GetMCPMetricsSnapshot()

	toolMetrics, exists := snapshot.Tools.ByTool[toolName]
	if !exists {
		return map[string]any{
			"tool_name": toolName,
			"found":     false,
			"message":   fmt.Sprintf("No metrics found for tool '%s'", toolName),
		}, nil
	}

	var errorRate float64
	if toolMetrics.CallCount > 0 {
		errorRate = float64(toolMetrics.ErrorCount) / float64(toolMetrics.CallCount)
	}

	return map[string]any{
		"tool_name":               toolName,
		"found":                   true,
		"call_count":              toolMetrics.CallCount,
		"average_duration":        toolMetrics.AverageDuration.String(),
		"error_count":             toolMetrics.ErrorCount,
		objects.FieldKeyErrorRate: errorRate,
		"last_called":             zqktime.FormatLayoutUTC(toolMetrics.LastCalled, zqktime.LayoutDateTimeMillis),
	}, nil
}

// formatMetricsSummary formats metrics as a human-readable summary
func formatMetricsSummary(snapshot MetricsSnapshot) string {
	var summary string

	summary += fmt.Sprintf("MCP Protocol Metrics (as of %s)\n\n", zqktime.FormatLayoutUTC(snapshot.Timestamp, zqktime.LayoutDateTimeSpace))

	// Lifecycle
	summary += "=== Lifecycle ===\n"
	summary += fmt.Sprintf("Initialize: %d calls, %d errors, avg %v\n",
		snapshot.Lifecycle.InitializeCount,
		snapshot.Lifecycle.InitializeErrors,
		snapshot.Lifecycle.InitializeDuration.Average)
	summary += fmt.Sprintf("Shutdown: %d calls, avg %v\n\n",
		snapshot.Lifecycle.ShutdownCount,
		snapshot.Lifecycle.ShutdownDuration.Average)

	// Tools
	summary += "=== Tools ===\n"
	summary += fmt.Sprintf("List: %d calls, %d errors, avg %v, p95 %v\n",
		snapshot.Tools.ListCount,
		snapshot.Tools.ListErrors,
		snapshot.Tools.ListDuration.Average,
		snapshot.Tools.ListDuration.P95)
	summary += fmt.Sprintf("Call: %d calls, %d errors, avg %v, p95 %v\n",
		snapshot.Tools.CallCount,
		snapshot.Tools.CallErrors,
		snapshot.Tools.CallDuration.Average,
		snapshot.Tools.CallDuration.P95)
	if len(snapshot.Tools.ByTool) > 0 {
		summary += "\nTop Tools by Call Count:\n"
		// Sort tools by call count (simplified - just show first 10)
		count := 0
		for toolName, toolMetrics := range snapshot.Tools.ByTool {
			if count >= 10 {
				break
			}
			summary += fmt.Sprintf("  %s: %d calls, avg %v, %d errors\n",
				toolName,
				toolMetrics.CallCount,
				toolMetrics.AverageDuration,
				toolMetrics.ErrorCount)
			count++
		}
	}
	summary += "\n"

	// Resources
	summary += "=== Resources ===\n"
	summary += fmt.Sprintf("List: %d calls, %d errors, avg %v\n",
		snapshot.Resources.ListCount,
		snapshot.Resources.ListErrors,
		snapshot.Resources.ListDuration.Average)
	summary += fmt.Sprintf("Get: %d calls, %d errors, avg %v, p95 %v\n\n",
		snapshot.Resources.GetCount,
		snapshot.Resources.GetErrors,
		snapshot.Resources.GetDuration.Average,
		snapshot.Resources.GetDuration.P95)

	// Prompts
	summary += "=== Prompts ===\n"
	summary += fmt.Sprintf("List: %d calls, %d errors, avg %v\n",
		snapshot.Prompts.ListCount,
		snapshot.Prompts.ListErrors,
		snapshot.Prompts.ListDuration.Average)
	summary += fmt.Sprintf("Get: %d calls, %d errors, avg %v\n\n",
		snapshot.Prompts.GetCount,
		snapshot.Prompts.GetErrors,
		snapshot.Prompts.GetDuration.Average)

	// Notifications
	summary += "=== Notifications ===\n"
	summary += fmt.Sprintf("Log Messages: %d sent, %d dropped\n",
		snapshot.Notifications.LogMessageCount,
		snapshot.Notifications.LogMessageDropped)
	summary += fmt.Sprintf("Events: %d\n", snapshot.Notifications.EventCount)
	summary += fmt.Sprintf("Messages: %d\n\n", snapshot.Notifications.MessageCount)

	// Async
	summary += "=== Async Operations ===\n"
	summary += fmt.Sprintf("Async Tool Calls: %d, %d errors\n",
		snapshot.Async.ToolCallCount,
		snapshot.Async.ToolCallErrors)
	summary += fmt.Sprintf("Batch Tool Calls: %d batches, %d errors\n\n",
		snapshot.Async.BatchCallCount,
		snapshot.Async.BatchCallErrors)

	// Queue
	summary += "=== Message Queue ===\n"
	summary += fmt.Sprintf("Current Depth: %d\n", snapshot.Queue.Depth)
	summary += fmt.Sprintf("Total Sent: %d\n", snapshot.Queue.Sent)
	summary += fmt.Sprintf("Total Dropped: %d\n", snapshot.Queue.Dropped)
	summary += fmt.Sprintf("Total Errors: %d\n", snapshot.Queue.Errors)
	if snapshot.Queue.Sent+snapshot.Queue.Dropped > 0 {
		dropRate := float64(snapshot.Queue.Dropped) / float64(snapshot.Queue.Sent+snapshot.Queue.Dropped)
		summary += fmt.Sprintf("Drop Rate: %.2f%%\n", dropRate*100)
	}
	summary += "\n"

	// Concurrency
	summary += "=== Concurrency ===\n"
	summary += fmt.Sprintf("Current: %d operations\n", snapshot.Concurrency.Current)
	summary += fmt.Sprintf("Peak: %d operations\n", snapshot.Concurrency.Max)

	return summary
}
