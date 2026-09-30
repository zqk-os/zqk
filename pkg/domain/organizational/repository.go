package organizational

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// Repository defines the storage contract required by organizational domain services,
// ensuring domain logic remains pure and decoupled from infrastructure storage implementations.
type Repository interface {
	// Read retrieves an object by ID.
	Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error)
	// Create persists a new object.
	Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error
	// Update mutates an existing object.
	Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error
	// ListByKind lists all objects matching a specific kind.
	ListByKind(ctx context.Context, secCtx *pkgctx.SecurityContext, kind string) ([]map[string]any, error)
}
