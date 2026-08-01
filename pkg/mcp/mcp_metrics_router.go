package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
)

// EventContext interface to avoid import cycle with coordination package
// This matches coordination.EventContext structure
type EventContext interface {
	GetOperationID() string
	GetOperationType() string
	GetStatus() string
	GetDuration() time.Duration
	GetError() error
	GetEventData() EventData
}

// EventData interface to avoid import cycle
type EventData interface {
	GetMetricsData() map[string]any
}

// MCPMetricsRouter routes coordinator events to MCP metrics
// This integrates MCP protocol metrics with the coordinator pattern
type MCPMetricsRouter struct {
	metrics  *MCPMetrics
	recorder any // observability.Recorder - stored as any to avoid import cycles
}

// NewMCPMetricsRouter creates a new router that records metrics in MCPMetrics
func NewMCPMetricsRouter(metrics *MCPMetrics) *MCPMetricsRouter {
	return &MCPMetricsRouter{
		metrics: metrics,
	}
}

// Emit implements coordination.MetricsRouter interface
// Extracts metrics data from EventContext and records in MCPMetrics
// Uses any to avoid import cycle with coordination package
func (r *MCPMetricsRouter) Emit(ctx context.Context, eventCtx any) error {
	if r.metrics == nil || eventCtx == nil {
		return nil // Best effort - skip if no metrics or event data
	}

	// Type assert to get event context fields
	// We use a type switch to handle both coordination.EventContext and our interface
	var operationType string
	var duration time.Duration
	var err error
	var metricsData map[string]any

	// Try to extract data using type assertion
	// We check for both our MCPEventContext and coordination.EventContext (via interface)
	if ec, ok := eventCtx.(EventContext); ok {
		operationType = ec.GetOperationType()
		duration = ec.GetDuration()
		err = ec.GetError()
		eventData := ec.GetEventData()
		if eventData != nil {
			metricsData = eventData.GetMetricsData()
		}
	} else {
		// Try to extract using reflection-like approach for coordination.EventContext
		// Use a type assertion that matches coordination.EventContext structure
		type eventContextWithFields interface {
			GetOperationType() string
			GetDuration() time.Duration
			GetError() error
			GetEventData() EventData
		}
		if ec, ok := eventCtx.(eventContextWithFields); ok {
			operationType = ec.GetOperationType()
			duration = ec.GetDuration()
			err = ec.GetError()
			eventData := ec.GetEventData()
			if eventData != nil {
				metricsData = eventData.GetMetricsData()
			}
		} else {
			// Unknown type - best effort, return nil
			return nil
		}
	}

	if metricsData == nil {
		return nil
	}

	// Use new builder-pattern API for recording metrics
	// This eliminates the large switch statement and makes it easy to add new operation types
	recorder := r.getMCPMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := r.buildMCPMetric(operationType, metricsData, duration, err)
		_ = recorder.Record(operationType, builder)
	}

	return nil
}

// getInt64 safely extracts int64 from metrics data
func getInt64(data map[string]any, key string) (int64, bool) {
	val, ok := data[key]
	if !ok {
		return 0, false
	}

	switch v := val.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
}

// MCPEventContext is a simple event context for MCP operations
// This avoids import cycle with coordination package
type MCPEventContext struct {
	OperationID   string
	OperationType string
	Status        string
	Duration      time.Duration
	Error         error
	EventData     *MCPEventData
}

// MCPEventData contains metrics data
type MCPEventData struct {
	MetricsData map[string]any
}

// GetOperationID implements EventContext interface
func (e *MCPEventContext) GetOperationID() string {
	return e.OperationID
}

// GetOperationType implements EventContext interface
func (e *MCPEventContext) GetOperationType() string {
	return e.OperationType
}

// GetStatus implements EventContext interface
func (e *MCPEventContext) GetStatus() string {
	return e.Status
}

// GetDuration implements EventContext interface
func (e *MCPEventContext) GetDuration() time.Duration {
	return e.Duration
}

// GetError implements EventContext interface
func (e *MCPEventContext) GetError() error {
	return e.Error
}

// GetEventData implements EventContext interface
func (e *MCPEventContext) GetEventData() EventData {
	return e.EventData
}

// GetMetricsData implements EventData interface
func (e *MCPEventData) GetMetricsData() map[string]any {
	return e.MetricsData
}

// BuildMCPEventContext creates an EventContext for MCP operations
// This is a helper for emitting MCP metrics via coordinator
func BuildMCPEventContext(operationID, operationType, status string, duration time.Duration, err error) *MCPEventContext {
	// Build metrics data
	metricsData := map[string]any{
		objects.FieldKeyOperationID: operationID,
		"operation_type":            operationType,
		objects.FieldKeyStatus:      status,
		"duration_ns":               duration.Nanoseconds(),
	}

	if err != nil {
		metricsData["error"] = err.Error()
	}

	return &MCPEventContext{
		OperationID:   operationID,
		OperationType: operationType,
		Status:        status,
		Duration:      duration,
		Error:         err,
		EventData: &MCPEventData{
			MetricsData: metricsData,
		},
	}
}

// BuildMCPToolCallEventContext creates an EventContext for tool call operations
func BuildMCPToolCallEventContext(operationID, toolName string, duration time.Duration, err error) *MCPEventContext {
	eventCtx := BuildMCPEventContext(operationID, "mcp_tool_call", "complete", duration, err)
	eventCtx.EventData.MetricsData["tool_name"] = toolName
	return eventCtx
}

// BuildMCPBatchToolCallEventContext creates an EventContext for batch tool call operations
func BuildMCPBatchToolCallEventContext(operationID string, batchSize, errorCount int, duration time.Duration) *MCPEventContext {
	eventCtx := BuildMCPEventContext(operationID, "mcp_batch_tool_call", "complete", duration, nil)
	eventCtx.EventData.MetricsData[objects.FieldKeyBatchSize] = batchSize
	eventCtx.EventData.MetricsData["error_count"] = errorCount
	return eventCtx
}

// BuildMCPQueueMetricsEventContext creates an EventContext for queue metrics
func BuildMCPQueueMetricsEventContext(depth, dropped, sent, errors int64) *MCPEventContext {
	operationID := fmt.Sprintf("queue_metrics_%d", time.Now().UnixNano())
	return &MCPEventContext{
		OperationID:   operationID,
		OperationType: "mcp_queue_metrics",
		Status:        "update",
		Duration:      0,
		EventData: &MCPEventData{
			MetricsData: map[string]any{
				"depth":   depth,
				"dropped": dropped,
				"sent":    sent,
				"errors":  errors,
			},
		},
	}
}
