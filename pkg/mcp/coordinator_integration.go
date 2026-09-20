package mcp

import (
	"context"
)

// CoordinatorIntegration provides integration with the coordination package
// This file bridges MCP metrics with the coordinator pattern without import cycles
// The actual coordinator integration is done via dependency injection from cmd/zqk/mcp

// CreateMCPMetricsRouter creates an MCPMetricsRouter for use with coordinator
// This should be called from cmd/zqk/mcp where we have access to coordination package
// Returns an interface that matches coordination.MetricsRouter to avoid import cycles
func CreateMCPMetricsRouter(metrics *MCPMetrics) interface {
	Emit(ctx context.Context, eventCtx any) error
} {
	return NewMCPMetricsRouter(metrics)
}
