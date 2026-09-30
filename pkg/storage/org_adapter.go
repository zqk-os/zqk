package storage

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// OrganizationalStorageAdapter adapts an ObjectStorageProvider to satisfy
// the organizational domain Repository interface without coupling domain packages to storage.
type OrganizationalStorageAdapter struct {
	provider ObjectStorageProvider
}

// NewOrganizationalStorageAdapter creates a new adapter wrapping provider.
func NewOrganizationalStorageAdapter(provider ObjectStorageProvider) *OrganizationalStorageAdapter {
	return &OrganizationalStorageAdapter{provider: provider}
}

// Read delegates object retrieval to the underlying storage provider.
func (a *OrganizationalStorageAdapter) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	return a.provider.Read(ctx, secCtx, id)
}

// Create delegates object creation to the underlying storage provider.
func (a *OrganizationalStorageAdapter) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	return a.provider.Create(ctx, secCtx, obj)
}

// Update delegates object update to the underlying storage provider.
func (a *OrganizationalStorageAdapter) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	return a.provider.Update(ctx, secCtx, id, updates)
}

// ListByKind queries all objects of a given kind from the underlying storage provider.
func (a *OrganizationalStorageAdapter) ListByKind(ctx context.Context, secCtx *pkgctx.SecurityContext, kind string) ([]map[string]any, error) {
	result, err := a.provider.List(ctx, secCtx, nil, ListFilter{Kind: kind})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return []map[string]any{}, nil
	}
	return result.Objects, nil
}
