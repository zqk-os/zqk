package mcp

import (
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
)

// SpecLoaderWrapper wraps objects.SpecLoader to implement SpecLoader interface
type SpecLoaderWrapper struct {
	// loader field removed - unused
}

// LoadSpecWithInheritance implements SpecLoader interface
func (w *SpecLoaderWrapper) LoadSpecWithInheritance(filename string) (Spec, error) {
	// Type assert and call the actual loader
	// This will be implemented in cmd/zqk/mcp where we have access to objects package
	// For now, return error - this will be fixed in the integration
	return nil, errfmt.Errorf("spec loader wrapper not fully implemented")
}

// GetResolvedFields implements Spec interface
func (w *SpecLoaderWrapper) GetResolvedFields() map[string]any {
	return nil
}

// InitializePermissionCache initializes the permission cache for the MCP server
// This should be called during server initialization
// initCtx provides CLI initialization context (project root, etc.)
// specLoader should be a *objects.SpecLoader wrapped in SpecLoaderWrapper
// loggerAdapter can be provided to enable debug logging (optional, can be nil)
// Returns the permission cache and spec access control for use in format permission checking
func InitializePermissionCache(server *Server, initCtx *pkgctx.CliInitializationContext, specLoader SpecLoader, loggerAdapter Logger) (*PermissionCache, *SpecAccessControl, error) {
	// Create permission cache with the provided spec loader
	permissionCache := NewPermissionCache(specLoader)

	// Create spec access control with the spec loader
	specAccessControl := NewSpecAccessControl(specLoader)

	// Set permission cache on spec access control for optimized access checks
	specAccessControl.SetPermissionCache(permissionCache)

	// Create logger event adapter if logger is provided and server has event emitter
	var eventLogger Logger
	if loggerAdapter != nil && server.eventEmitter != nil {
		eventLogger = NewLoggerEventAdapter(loggerAdapter, server.eventEmitter)
	} else {
		eventLogger = loggerAdapter
	}

	// Set logger if provided
	if eventLogger != nil {
		permissionCache.SetLogger(eventLogger)
		specAccessControl.SetLogger(eventLogger)
	}

	// Set event emitter on permission cache and spec access control
	if server.eventEmitter != nil {
		permissionCache.SetEventEmitter(server.eventEmitter)
		specAccessControl.SetEventEmitter(server.eventEmitter)
	}

	// Set MCP server context on permission cache and spec access control
	// This allows them to check serving state without global access
	mcpCtx := pkgctx.GetMCPServerContext()
	permissionCache.SetMCPServerContext(mcpCtx)
	specAccessControl.SetMCPServerContext(mcpCtx)

	// Set on server
	server.SetPermissionCache(permissionCache)
	server.SetSpecAccessControl(specAccessControl)

	// Build permission cache for the current security context if available
	if server.secCtx != nil {
		// Type assert to *pkgctx.SecurityContext
		if secCtx, ok := server.secCtx.(*pkgctx.SecurityContext); ok {
			// Build permission cache for this user
			if err := permissionCache.BuildPermissionCache(secCtx); err != nil {
				// Log error but don't fail initialization
				// Permission cache will be built lazily when needed
				return permissionCache, specAccessControl, nil
			}
		}
	}

	return permissionCache, specAccessControl, nil
}
