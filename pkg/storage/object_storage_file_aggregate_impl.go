package storage

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/storage/crud"
)

// Aggregate performs aggregations on objects matching the filter
func (f *FileObjectStorage) Aggregate(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter, aggregations []Aggregation) (*AggregateResult, error) {
	return crud.AggregateObjects(
		ctx,
		secCtx,
		storageCtx,
		filter,
		aggregations,
		f.List,
		f.checkPermission,
	)
}
