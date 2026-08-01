package mcp_harness

import (
	"os"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/mcp"
)

// Harness represents a standardized wrapper around an MCP server
// designed to host isolated standalone utility services within the monorepo.
// Following the Infrastructure Demarcation Strategy v1.0, these services
// run externally to the main ZQK binary to avoid polluting the core CLI
// with heavy subkernel logic (e.g., A/V processing, complex ML tasks).
type Harness struct {
	Name    string
	Version string
	server  *mcp.Server
	logger  *logging.EventLogger
}

// HarnessOptions configures the utility MCP harness
type HarnessOptions struct {
	Name        string
	Version     string
	ProjectRoot string
}

// NewHarness creates a new standardized utility MCP server harness.
func NewHarness(opts HarnessOptions) *Harness {
	server := mcp.NewServer()

	// Initialize security context and configuration context
	secCtx := pkgctx.NewSystemSecurityContext()
	server.SetSecurityContext(secCtx)

	root := opts.ProjectRoot
	if root == "" {
		if wd, err := os.Getwd(); err == nil {
			root = wd
		} else {
			root = "."
		}
	}

	initCtx := &pkgctx.CliInitializationContext{
		ProjectRoot: root,
	}
	server.SetCliInitializationContext(initCtx)

	// Create logger
	ctx := pkgctx.NewSystemContext()
	logger := logging.NewEventLogger(ctx)

	return &Harness{
		Name:    opts.Name,
		Version: opts.Version,
		server:  server,
		logger:  logger,
	}
}

// ToolRegistration specifies a tool to be registered on the harness
type ToolRegistration struct {
	Name        string
	Description string
	InputSchema any
	Handler     mcp.ToolHandler
}

// RegisterTool registers a specialized tool specific to this utility subkernel.
func (h *Harness) RegisterTool(tool ToolRegistration) {
	h.server.RegisterTool(tool.Name, tool.Description, tool.InputSchema, tool.Handler)
}

// Serve starts the MCP server over stdio
func (h *Harness) Serve() error {
	logging.FluentEvent(h.logger).Info("Starting MCP utility harness").
		String("name", h.Name).
		String("version", h.Version).
		Log()

	return h.server.Serve()
}
