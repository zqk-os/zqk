package mcp

import (
	"path/filepath"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// createTestServerWithoutRegistry creates a test server without agent registry
// Use this for tests that don't need registry enforcement
func createTestServerWithoutRegistry() *Server {
	server := NewServer()
	server.eventEmitter = NewEventEmitter(100)

	// Create test config without agent registry
	config := &ServerConfig{}
	config.MCPServer.Security.RequireAccountID = false
	config.MCPServer.Security.EnforceAccountRoles = false
	config.MCPServer.Security.ValidateRoles = false

	server.config = config
	server.initCtx = &pkgctx.CliInitializationContext{
		ProjectRoot: filepath.Join("..", ".."),
	}

	return server
}

// getRegisteredAccountID removed - unused test helper
