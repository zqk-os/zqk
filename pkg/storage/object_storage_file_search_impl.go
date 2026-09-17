package storage

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/storage/crud"
)

// Search performs a text search across objects
func (f *FileObjectStorage) Search(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query SearchQuery) (*SearchResult, error) {
	return crud.SearchObjects(
		ctx,
		secCtx,
		storageCtx,
		query,
		f.List,
		f.checkPermission,
	)
}
