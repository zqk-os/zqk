package kernel_test

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/kernel"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/storage"
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
	ShutdownFunc func(ctx context.Context) error
}

func (m *MockStorage) Shutdown(ctx context.Context) error {
	if m.ShutdownFunc != nil {
		return m.ShutdownFunc(ctx)
	}
	return nil
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

type testExtension struct {
	name        string
	initCalled  bool
	closeCalled bool
}

func (e *testExtension) Name() string { return e.name }
func (e *testExtension) Init(ctx context.Context, k kernel.KnowledgeKernel) error {
	e.initCalled = true
	return nil
}
func (e *testExtension) Shutdown(ctx context.Context) error {
	e.closeCalled = true
	return nil
}

func TestKernel_ExtensionLifecycle(t *testing.T) {
	mockServer := &MockMCPServer{}
	mockStorage := &MockStorage{}

	k := kernel.NewKernelWithOptions(mockStorage, mockServer, "/test/workspace")
	if k.ProjectRoot() != "/test/workspace" {
		t.Errorf("expected project root /test/workspace, got %s", k.ProjectRoot())
	}

	ext := &testExtension{name: "studio-plugin"}
	if err := k.RegisterExtension(ext); err != nil {
		t.Fatalf("RegisterExtension failed: %v", err)
	}
	if !ext.initCalled {
		t.Error("expected Init to be called on RegisterExtension")
	}

	// Duplicate registration must fail
	if err := k.RegisterExtension(ext); err == nil {
		t.Error("expected error on duplicate registration, got nil")
	}

	// Lookup
	retrieved, ok := k.GetExtension("studio-plugin")
	if !ok || retrieved != ext {
		t.Errorf("expected to retrieve extension, got %v, ok=%v", retrieved, ok)
	}

	// List
	names := k.ListExtensions()
	if len(names) != 1 || names[0] != "studio-plugin" {
		t.Errorf("expected [studio-plugin], got %v", names)
	}

	// Shutdown
	if err := k.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
	if !ext.closeCalled {
		t.Error("expected Shutdown to be called on extension")
	}
}

