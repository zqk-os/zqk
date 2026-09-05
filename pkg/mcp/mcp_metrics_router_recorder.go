package mcp

import (
	"time"

	"github.com/lanceman/zqk/pkg/observability"
)

// getMCPMetricsRecorder gets a metrics recorder for MCP operations
// This is in a separate file to help manage import cycles
func (r *MCPMetricsRouter) getMCPMetricsRecorder() observability.Recorder {
	if r.recorder == nil {
		r.recorder = observability.GetNoOpRecorder()
	}
	if recorder, ok := r.recorder.(observability.Recorder); ok {
		return recorder
	}
	return observability.GetNoOpRecorder()
}

// buildMCPMetric builds a metric for MCP operations using the builder pattern
// This eliminates the need for a large switch statement and makes it easy to add new operation types
func (r *MCPMetricsRouter) buildMCPMetric(operationType string, metricsData map[string]any, duration time.Duration, err error) observability.Builder {
	builder := observability.NewBuilder(operationType).
		WithDuration(duration).
		WithTags("mcp", operationType)

	// Add all fields from metricsData to the builder
	for k, v := range metricsData {
		builder = builder.WithField(k, v)
	}

	// Add error if present
	if err != nil {
		builder = builder.WithError(err)
	}

	// Add operation-specific fields based on operation type
	switch operationType {
	case "mcp_tool_call":
		toolName, _ := metricsData["tool_name"].(string)
		if toolName != emptyValue {
			builder = builder.WithField("tool_name", toolName)
		}
	case "mcp_batch_tool_call":
		// batch_size and error_count are already in metricsData, so they're added above
	case "mcp_queue_metrics":
		// depth, dropped, sent, errors are already in metricsData
	case "mcp_concurrent_operation":
		// delta is already in metricsData
	}

	return builder
}
