package kernel

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/mcp"
	"github.com/lanceman/zqk/pkg/storage"
)

// Kernel is the concrete implementation of the KnowledgeKernel interface.
type Kernel struct {
	storage.ObjectStorageProvider
	mcpServer mcp.MCPServer
}

// NewKernel creates a new Knowledge Kernel instance
func NewKernel(store storage.ObjectStorageProvider, mcpServer mcp.MCPServer) *Kernel {
	return &Kernel{
		ObjectStorageProvider: store,
		mcpServer:             mcpServer,
	}
}

// ExecuteMCPTool executes an MCP tool
func (k *Kernel) ExecuteMCPTool(ctx context.Context, secCtx *pkgctx.SecurityContext, toolName string, params map[string]any) (*mcp.ToolCallResult, error) {
	// To pass secCtx to the MCP server we could attach it to the context,
	// assuming mcp.MCPServer checks the context for it.
	req := &mcp.ToolCallParams{
		Name:      toolName,
		Arguments: params,
	}
	return k.mcpServer.CallTool(ctx, req)
}

// ListMCPTools lists available MCP tools
func (k *Kernel) ListMCPTools(ctx context.Context, secCtx *pkgctx.SecurityContext) (*mcp.ToolsListResult, error) {
	return k.mcpServer.ListTools(ctx)
}
