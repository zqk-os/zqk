package storage

import (
	"context"
	"sync/atomic"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// DefaultBatchSize is the default batch size for high-volume processing jobs
const DefaultBatchSize = 5000

// BatchQueryFunc queries a batch of objects with pagination
// Returns the objects in the batch and any error
type BatchQueryFunc func(ctx context.Context, offset, limit int) ([]map[string]any, error)

// BatchProcessFunc processes a batch of objects and returns a result
// The result can be any type - it will be merged using BatchMergeFunc
type BatchProcessFunc func(batch []map[string]any) (any, []string, error)

// BatchMergeFunc merges two batch processing results into one
// Should handle nil firstResult (first batch) and return the merged result
type BatchMergeFunc func(firstResult, secondResult any) (any, error)

// BatchProcessor handles batching/pagination for high-volume processing jobs
type BatchProcessor struct {
	batchSize             int
	batchesProcessedTotal atomic.Int64
	itemsProcessedTotal   atomic.Int64
}

// NewBatchProcessor creates a new batch processor with the specified batch size
// If batchSize is 0, uses DefaultBatchSize
func NewBatchProcessor(batchSize int) *BatchProcessor {
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	return &BatchProcessor{
		batchSize: batchSize,
	}
}

// GetBatchStats returns lifetime counters for batches processed and items processed.
func (bp *BatchProcessor) GetBatchStats() (batchesProcessed, itemsProcessed int64) {
	return bp.batchesProcessedTotal.Load(), bp.itemsProcessedTotal.Load()
}

// ProcessInBatches processes objects in batches and returns the merged result
// queryFunc: Function to query a batch of objects (offset, limit)
// processFunc: Function to process each batch and return a result + IDs
// mergeFunc: Function to merge results from multiple batches
// Returns: merged result, all IDs from all batches, error
func (bp *BatchProcessor) ProcessInBatches(
	ctx context.Context,
	queryFunc BatchQueryFunc,
	processFunc BatchProcessFunc,
	mergeFunc BatchMergeFunc,
) (any, []string, error) {
	allIDs := make([]string, 0)
	var mergedResult any
	batchOffset := 0
	hasItems := false

	for {
		// Respect job timeout/cancellation so the executor can record conclusion (completed/failed) in job events file
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		// Query a batch
		batch, err := queryFunc(ctx, batchOffset, bp.batchSize)
		if err != nil {
			return nil, nil, errfmt.Errorf(ConstMiscFailedToQueryBatchOffsetDW, batchOffset, err)
		}

		if len(batch) == 0 {
			break // No more items to process
		}

		hasItems = true

		// Process this batch
		batchResult, batchIDs, err := processFunc(batch)
		if err != nil {
			return nil, nil, errfmt.Errorf(ConstMiscFailedToProcessBatchOffsetDW, batchOffset, err)
		}

		bp.batchesProcessedTotal.Add(1)
		bp.itemsProcessedTotal.Add(int64(len(batch)))

		// Merge with previous batches
		if mergedResult == nil {
			mergedResult = batchResult
		} else {
			mergedResult, err = mergeFunc(mergedResult, batchResult)
			if err != nil {
				return nil, nil, errfmt.Newf(ConstMiscFailedToMergeBatchResults).Wrap(err)
			}
		}

		allIDs = append(allIDs, batchIDs...)

		// If we got fewer items than batch size, we've reached the end
		if len(batch) < bp.batchSize {
			break
		}

		batchOffset += len(batch)
	}

	if !hasItems {
		return nil, []string{}, nil
	}

	return mergedResult, allIDs, nil
}

// BatchQueryBuilder helps build batch query functions for List operations
type BatchQueryBuilder struct {
	storage    ObjectStorageProvider
	secCtx     *pkgctx.SecurityContext
	storageCtx *pkgctx.StorageContext
	kind       string
	filters    map[string]any
	sortBy     string
	sortAsc    bool
}

// NewBatchQueryBuilder creates a new batch query builder
func NewBatchQueryBuilder(
	storage ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	kind string,
) *BatchQueryBuilder {
	return &BatchQueryBuilder{
		storage:    storage,
		secCtx:     secCtx,
		storageCtx: storageCtx,
		kind:       kind,
		filters:    make(map[string]any),
		sortBy:     "created_at",
		sortAsc:    true,
	}
}

// WithFilters adds filters to the query
func (bqb *BatchQueryBuilder) WithFilters(filters map[string]any) *BatchQueryBuilder {
	for k, v := range filters {
		bqb.filters[k] = v
	}
	return bqb
}

// WithSort sets the sort field and direction
func (bqb *BatchQueryBuilder) WithSort(sortBy string, sortAsc bool) *BatchQueryBuilder {
	bqb.sortBy = sortBy
	bqb.sortAsc = sortAsc
	return bqb
}

// BuildQueryFunc builds a BatchQueryFunc for List operations
func (bqb *BatchQueryBuilder) BuildQueryFunc(ctx context.Context) BatchQueryFunc {
	return func(queryCtx context.Context, offset, limit int) ([]map[string]any, error) {
		filter := ListFilter{
			Kind:    bqb.kind,
			Filters: bqb.filters,
			SortBy:  bqb.sortBy,
			SortAsc: bqb.sortAsc,
			Offset:  offset,
			Limit:   limit,
		}

		result, err := bqb.storage.List(queryCtx, bqb.secCtx, bqb.storageCtx, filter)
		if err != nil {
			return nil, err
		}

		return result.Objects, nil
	}
}
