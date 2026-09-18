package kernel

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/storage"
)

// KnowledgeKernel defines the interface for the central knowledge base.
// It integrates object storage with MCP tool execution capabilities for
// the multi-agent orchestration pipeline.
type KnowledgeKernel interface {
	storage.ObjectStorageProvider

	// ExecuteMCPTool executes an MCP tool by name with the given parameters
	ExecuteMCPTool(ctx context.Context, secCtx *pkgctx.SecurityContext, toolName string, params map[string]any) (*mcp.ToolCallResult, error)

	// ListMCPTools lists available MCP tools within the knowledge kernel
	ListMCPTools(ctx context.Context, secCtx *pkgctx.SecurityContext) (*mcp.ToolsListResult, error)
}
