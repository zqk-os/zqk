package kernel

import (
	"context"
	"errors"
	"sort"
	"sync"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/storage"
)

// Kernel is the concrete implementation of the KnowledgeKernel interface.
type Kernel struct {
	storage.ObjectStorageProvider
	mcpServer   mcp.MCPServer
	projectRoot string
	mu          sync.RWMutex
	extensions  map[string]Extension
}

// NewKernel creates a new Knowledge Kernel instance.
func NewKernel(store storage.ObjectStorageProvider, mcpServer mcp.MCPServer) *Kernel {
	return NewKernelWithOptions(store, mcpServer, "")
}

// NewKernelWithOptions creates a new Knowledge Kernel instance with an explicit project root.
func NewKernelWithOptions(store storage.ObjectStorageProvider, mcpServer mcp.MCPServer, projectRoot string) *Kernel {
	return &Kernel{
		ObjectStorageProvider: store,
		mcpServer:             mcpServer,
		projectRoot:           projectRoot,
		extensions:            make(map[string]Extension),
	}
}

// ProjectRoot returns the workspace root path of this kernel instance.
func (k *Kernel) ProjectRoot() string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.projectRoot
}

// RegisterExtension attaches a pluggable extension to the kernel runtime.
func (k *Kernel) RegisterExtension(ext Extension) error {
	if ext == nil {
		return errfmt.Errorf("cannot register nil extension")
	}
	name := ext.Name()
	if name == "" {
		return errfmt.Errorf("extension name cannot be empty")
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if _, exists := k.extensions[name]; exists {
		return errfmt.Errorf("extension %q already registered", name)
	}
	if err := ext.Init(context.Background(), k); err != nil {
		return errfmt.Errorf("failed to initialize extension %q: %w", name, err)
	}
	k.extensions[name] = ext
	return nil
}

// GetExtension retrieves an extension by its registered name.
func (k *Kernel) GetExtension(name string) (Extension, bool) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	ext, ok := k.extensions[name]
	return ext, ok
}

// ListExtensions returns all currently registered extension names in sorted order.
func (k *Kernel) ListExtensions() []string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	names := make([]string, 0, len(k.extensions))
	for n := range k.extensions {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Shutdown cleanly stops all registered extensions and storage provider resources.
func (k *Kernel) Shutdown(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	var errs []error
	for name, ext := range k.extensions {
		if err := ext.Shutdown(ctx); err != nil {
			errs = append(errs, errfmt.Errorf("error shutting down extension %s: %w", name, err))
		}
	}
	if k.ObjectStorageProvider != nil {
		if closer, ok := k.ObjectStorageProvider.(interface{ Shutdown(context.Context) error }); ok {
			if err := closer.Shutdown(ctx); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// ExecuteMCPTool executes an MCP tool.
func (k *Kernel) ExecuteMCPTool(ctx context.Context, secCtx *pkgctx.SecurityContext, toolName string, params map[string]any) (*mcp.ToolCallResult, error) {
	req := &mcp.ToolCallParams{
		Name:      toolName,
		Arguments: params,
	}
	return k.mcpServer.CallTool(ctx, req)
}

// ListMCPTools lists available MCP tools.
func (k *Kernel) ListMCPTools(ctx context.Context, secCtx *pkgctx.SecurityContext) (*mcp.ToolsListResult, error) {
	return k.mcpServer.ListTools(ctx)
}
