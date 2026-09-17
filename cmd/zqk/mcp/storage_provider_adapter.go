package mcp

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	pkgmcp "github.com/lanceman/zqk/pkg/mcp"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// storageProviderAdapter adapts storage.ObjectStorageProvider to pkg/mcp.StorageProvider
// so the MCP server can list and create objects without importing storage in pkg/mcp.
type storageProviderAdapter struct {
	storage storage.ObjectStorageProvider
}

// NewStorageProviderAdapter returns a pkg/mcp.StorageProvider that delegates to the given storage.
func NewStorageProviderAdapter(s storage.ObjectStorageProvider) pkgmcp.StorageProvider {
	if s == nil {
		return nil
	}
	return &storageProviderAdapter{storage: s}
}

// List implements pkgmcp.StorageProvider.
func (a *storageProviderAdapter) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter any) (any, error) {
	lf := a.filterToListFilter(filter)
	result, err := a.storage.List(ctx, secCtx, storageCtx, lf)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"objects": result.Objects,
		"meta":    result.Meta,
	}, nil
}

// Create implements pkgmcp.StorageProvider.
func (a *storageProviderAdapter) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	return a.storage.Create(ctx, secCtx, obj)
}

// Read implements pkgmcp.StorageProvider.
func (a *storageProviderAdapter) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	return a.storage.Read(ctx, secCtx, id)
}

func (a *storageProviderAdapter) filterToListFilter(filter any) storage.ListFilter {
	lf := storage.ListFilter{}
	if m, ok := filter.(map[string]any); ok {
		if k, ok := m[objects.FieldKeyKind].(string); ok {
			lf.Kind = k
		}
		if f, ok := m["filters"].(map[string]any); ok {
			lf.Filters = f
		}
	}
	return lf
}
