package kernel_test

import (
	"context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/kernel"
	"github.com/lanceman/zqk/pkg/mcp"
	"github.com/lanceman/zqk/pkg/storage"
)

// MockMCPServer implements mcp.MCPServer for testing
type MockMCPServer struct {
	mcp.MCPServer
	CallToolFunc  func(ctx context.Context, params *mcp.ToolCallParams) (*mcp.ToolCallResult, error)
	ListToolsFunc func(ctx context.Context) (*mcp.ToolsListResult, error)
}

func (m *MockMCPServer) CallTool(ctx context.Context, params *mcp.ToolCallParams) (*mcp.ToolCallResult, error) {
	if m.CallToolFunc != nil {
		return m.CallToolFunc(ctx, params)
	}
	return nil, nil
}

func (m *MockMCPServer) ListTools(ctx context.Context) (*mcp.ToolsListResult, error) {
	if m.ListToolsFunc != nil {
		return m.ListToolsFunc(ctx)
	}
	return nil, nil
}

// MockStorage implements storage.ObjectStorageProvider for testing
type MockStorage struct {
	storage.ObjectStorageProvider
}

func TestKernel_ExecuteMCPTool(t *testing.T) {
	mockServer := &MockMCPServer{
		CallToolFunc: func(ctx context.Context, params *mcp.ToolCallParams) (*mcp.ToolCallResult, error) {
			if params.Name != "test_tool" {
				t.Errorf("expected tool test_tool, got %s", params.Name)
			}
			return &mcp.ToolCallResult{
				Content: []mcp.Content{
					{
						Type: "text",
						Text: "success",
					},
				},
			}, nil
		},
	}
	mockStorage := &MockStorage{}

	k := kernel.NewKernel(mockStorage, mockServer)
	secCtx := pkgctx.NewSystemSecurityContext()

	res, err := k.ExecuteMCPTool(context.Background(), secCtx, "test_tool", map[string]any{"arg1": "val1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Content) == 0 || res.Content[0].Text != "success" {
		t.Errorf("unexpected response: %+v", res)
	}
}

func TestKernel_ListMCPTools(t *testing.T) {
	mockServer := &MockMCPServer{
		ListToolsFunc: func(ctx context.Context) (*mcp.ToolsListResult, error) {
			return &mcp.ToolsListResult{
				Tools: []mcp.Tool{
					{
						Name:        "test_tool",
						Description: "A test tool",
					},
				},
			}, nil
		},
	}
	mockStorage := &MockStorage{}

	k := kernel.NewKernel(mockStorage, mockServer)
	secCtx := pkgctx.NewSystemSecurityContext()

	res, err := k.ListMCPTools(context.Background(), secCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Tools) != 1 || res.Tools[0].Name != "test_tool" {
		t.Errorf("unexpected tools: %+v", res.Tools)
	}
}
