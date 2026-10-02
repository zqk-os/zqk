package mcp

import (
	"context"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/storage"
)

const emptyValue = ""

// mcpMetricsRouterAdapter adapts MCP metrics router to coordination.MetricsRouter
type mcpMetricsRouterAdapter struct {
	mcpRouter interface {
		Emit(ctx context.Context, eventCtx any) error
	}
}

func (a *mcpMetricsRouterAdapter) Emit(ctx context.Context, eventCtx *coordination.EventContext) error {
	// Convert coordination.EventContext to any for MCP router
	return a.mcpRouter.Emit(ctx, eventCtx)
}

// SetupMCPCoordinatorIntegration sets up coordinator integration for MCP server
// This bridges MCP metrics with the coordinator pattern
func SetupMCPCoordinatorIntegration(
	server *mcp.Server,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
) {
	coordinator := CreateMCPCoordinatorWithRouters(projectRoot, storageProvider, server.GetMCPMetrics())

	// Set coordinator on global coordinator (if not already set)
	// This allows other parts of the system to use it
	coordination.SetGlobalCoordinator(coordinator)

	// Create adapter with coordinator (wrapped to match mcp.EventCoordinator interface)
	coordinatorAdapter := &coordinatorEventAdapter{coordinator: coordinator}
	mcpAdapter := mcp.NewMCPServerAdapterWithCoordinator(server, coordinatorAdapter)

	// Store adapter on server for coordinator integration
	// Note: This requires adding an adapter field to Server struct
	// For now, coordinator is available via global coordinator
	_ = mcpAdapter
}

// coordinatorEventAdapter adapts coordination.EventCoordinator to mcp.EventCoordinator
type coordinatorEventAdapter struct {
	coordinator coordination.EventCoordinator
}

func (a *coordinatorEventAdapter) Emit(ctx context.Context, eventCtx any) error {
	switch
	// Convert any to coordination.EventContext
	v := eventCtx.(type) {
	case *coordination.EventContext:
		return a.coordinator.Emit(ctx, v)
	}

	// If not a coordination.EventContext, try to convert via type assertion
	// This handles mcp.MCPEventContext and other types

	// Best effort - return nil if we can't convert
	return nil
}

// CreateMCPCoordinatorWithRouters creates a coordinator configured for MCP operations
// This is a helper that sets up all the routers needed for MCP
func CreateMCPCoordinatorWithRouters(
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	mcpMetrics *mcp.MCPMetrics,
) coordination.EventCoordinator {
	// Create MCP metrics router
	mcpRouter := mcp.CreateMCPMetricsRouter(mcpMetrics)

	// Create adapter to match coordination.MetricsRouter interface
	mcpMetricsRouter := &mcpMetricsRouterAdapter{mcpRouter: mcpRouter}

	// Create coordinator with MCP metrics router
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		AuditRouter:       coordination.NewStorageAuditRouter(projectRoot, storageProvider),
		MetricsRouter:     mcpMetricsRouter, // MCP metrics router adapter
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	return coordinator
}
