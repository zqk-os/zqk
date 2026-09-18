// Extracted from bulk_delete_optimized.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"
	"fmt"
	"sync"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/when"
)

func (f *FileObjectStorage) BulkDeleteOptimized(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	ids []string,
	cascade bool,
	maxWorkers int,
) (*BulkDeleteResult, error) {
	if !IsCLIOperation(ctx, secCtx) {
		return nil, errfmt.Errorf(ErrMsgDeleteRequiresCLI)
	}

	// Defer audit events so we can bulk-create them at the end (avoids N single creates)
	ctx = WithDeferAuditEvents(ctx)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	startTime := time.Now()

	// OPTIMIZATION: Check if all objects are leaf nodes before building dependency graph
	// For leaf nodes (audit_event, metrics, etc.), we can skip expensive Read() calls
	// and dependency graph building, going straight to parallel deletion
	graph := NewDependencyGraph()
	allLeafNodes := true
	leafKinds := make(map[string]bool)

	// Quick check: sample first few IDs to infer kind from ID pattern
	// If all sampled IDs are leaf nodes, assume all are leaf nodes
	sampleSize := 10
	if len(ids) < sampleSize {
		sampleSize = len(ids)
	}

	// Load ID validator patterns if needed
	if err := f.idValidator.LoadPatterns(); err != nil {
		StorageLog(logger).Warn(LogEventStorageBulkDeleteIDPatternsFailedFullGraphWarn).WithError(err).Log()
		allLeafNodes = false
	} else {
		for i := 0; i < sampleSize; i++ {
			id := ids[i]
			// Infer kind from ID pattern (faster than Read())
			kind := f.idValidator.InferKindFromID(id)
			if kind == emptyValue {
				// Can't infer - need to check, so not all leaf nodes
				allLeafNodes = false
				break
			}
			leafKinds[kind] = true
			if !graph.IsLeafNode(kind) {
				allLeafNodes = false
				break
			}
		}

		// If all sampled are leaf nodes, check if remaining are also leaf nodes
		// (in case batch has mixed kinds)
		if allLeafNodes && len(ids) > sampleSize {
			for i := sampleSize; i < len(ids); i++ {
				id := ids[i]
				kind := f.idValidator.InferKindFromID(id)
				if kind == emptyValue || !graph.IsLeafNode(kind) {
					allLeafNodes = false
					break
				}
				leafKinds[kind] = true
			}
		}
	}

	var deletionOrder []string
	if allLeafNodes {
		// Fast path: all leaf nodes - skip dependency graph, delete directly
		kindList := make([]string, 0, len(leafKinds))
		for k := range leafKinds {
			kindList = append(kindList, k)
		}
		StorageLog(logger).Debug(LogEventStorageBulkDeleteSkipGraphAllLeavesDebug).
			ObjectCount(len(ids)).
			KindsSummary(fmt.Sprintf("%v", kindList)).
			Log()
		deletionOrder = ids // No ordering needed for leaf nodes
	} else {
		// Slow path: build dependency graph for non-leaf nodes
		StorageLog(logger).Info(LogEventStorageBulkDeleteBuildingGraphInfo).ObjectCount(len(ids)).Log()

		// Membership set for the deletion batch. A dependent inside the batch is being erased too,
		// so it constrains ordering; a dependent outside the batch would be left pointing at
		// nothing, which is the orphaning case this path has to refuse.
		deleting := make(map[string]bool, len(ids))
		for _, id := range ids {
			deleting[id] = true
		}
		orphaned := make(map[string][]string)

		// Add all nodes to graph
		for _, id := range ids {
			// Read object to get kind
			obj, err := f.Read(ctx, secCtx, id)
			if err != nil {
				// Skip if not found
				continue
			}

			kind, _ := obj[objects.FieldKeyKind].(string)
			if kind == emptyValue {
				continue
			}

			if err := graph.AddNode(id, kind); err != nil {
				StorageLog(logger).Warn(LogEventStorageBulkDeleteAddNodeFailedWarn).
					ObjectID(id).
					WithError(err).
					Log()
				continue
			}

			// If it's a leaf node, skip dependency checking
			if graph.IsLeafNode(kind) {
				continue
			}

			// Find dependents for non-leaf nodes
			dependents, err := f.findDependents(ctx, id, kind)
			if err != nil {
				// Fail closed. An unreadable reverse-reference index means we cannot tell an
				// orphaning erase from a safe one, and the previous `continue` resolved that
				// doubt in favor of deleting.
				return nil, errfmt.Errorf(
					"bulk delete refused: cannot determine dependents of %s (%s): %v", id, kind, err)
			}

			// Add dependency relationships
			for _, dependentID := range dependents {
				if deleting[dependentID] {
					if err := graph.AddDependency(dependentID, id); err != nil {
						StorageLog(logger).Warn(LogEventStorageBulkDeleteAddDependencyFailedWarn).
							ObjectID(id).
							WithError(err).
							Log()
					}
					continue
				}
				orphaned[id] = append(orphaned[id], dependentID)
			}
		}

		// Referential integrity, matching what deleteImpl has enforced on the single-object path
		// since the 2026-08-03 workstream loss. Bulk reached the same erase without it, so which
		// invariant applied depended on the entry point a caller happened to use — and the
		// retention sweep of 2026-08-24 took the path without it, orphaning 270 objects.
		if len(orphaned) > 0 && !cascade {
			if err := f.resolveBulkOrphansOrRefuse(ctx, secCtx, orphaned, deleting); err != nil {
				return nil, err
			}
		}

		// Get deletion order
		var gerr error
		deletionOrder, gerr = graph.GetDeletionOrder(ids)
		if gerr != nil {
			StorageLog(logger).Error(LogEventStorageBulkDeleteGetOrderFailedErr, gerr).Log()
			// Fallback to original order if graph fails
			deletionOrder = ids
		}
	}
	StorageLog(logger).Info(LogEventStorageBulkDeleteOrderDeterminedInfo).
		Int("total_objects", len(deletionOrder)).
		Int("leaf_nodes", len(ids)-len(deletionOrder)).
		Log()

	// Step 3: Parallel deletion with worker pool
	if maxWorkers <= 0 {
		maxWorkers = 10 // Default to 10 workers
	}
	if maxWorkers > len(deletionOrder) {
		maxWorkers = len(deletionOrder)
	}

	result := &BulkDeleteResult{
		TotalCount:    len(ids),
		SuccessCount:  0,
		FailureCount:  0,
		Errors:        make([]BulkOperationError, 0),
		DeletionOrder: deletionOrder,
		Duration:      time.Since(startTime),
	}

	// OPTIMIZATION: For leaf nodes, use one transaction and batched WAL so we do one Sync instead of N.
	// Previously we did N parallel f.Delete() which each took walMu and Sync() — with 50k deletes that serialized 50k syncs and caused bulk delete to hang.
	if allLeafNodes {
		idToKind := make(map[string]string, len(deletionOrder))
		for _, id := range deletionOrder {
			if kind := f.idValidator.InferKindFromID(id); kind != emptyValue {
				idToKind[id] = kind
			}
		}

		tx, txErr := f.BeginTransaction(ctx)
		if txErr != nil {
			return nil, errfmt.Newf(ErrMsgBeginTxBulkDelete).Wrap(txErr)
		}
		fileTx, ok := tx.(*FileObjectTransaction)
		if !ok {
			if rerr := tx.Rollback(ctx); rerr != nil {
				StorageLog(logger).Error(LogEventStorageBulkDeleteRollbackFailedErr, rerr).Log()
			}
			return nil, errfmt.Errorf(ErrMsgBulkDeleteReqFileObjTx)
		}
		batchSize := 1000
		for i, id := range deletionOrder {
			kind := idToKind[id]
			if kind != emptyValue {
				if err := fileTx.DeleteByIDAndKind(ctx, id, kind); err != nil {
					if rerr := tx.Rollback(ctx); rerr != nil {
						StorageLog(logger).Error(LogEventStorageBulkDeleteRollbackFailedErr, rerr).Log()
					}
					return nil, errfmt.Errorf(ErrMsgBulkDeleteEnqueue, id, err)
				}
			} else {
				if err := tx.Delete(ctx, secCtx, id, cascade); err != nil {
					if rerr := tx.Rollback(ctx); rerr != nil {
						StorageLog(logger).Error(LogEventStorageBulkDeleteRollbackFailedErr, rerr).Log()
					}
					return nil, errfmt.Errorf(ErrMsgBulkDeleteEnqueue, id, err)
				}
			}

			if (i+1)%batchSize == 0 {
				if err := tx.Commit(ctx); err != nil {
					return nil, errfmt.Newf(ErrMsgBulkDeleteCommit).Wrap(err)
				}
				tx, txErr = f.BeginTransaction(ctx)
				if txErr != nil {
					return nil, errfmt.Newf(ErrMsgBeginTxBulkDelete).Wrap(txErr)
				}
				fileTx = tx.(*FileObjectTransaction)
			}
		}
		// Commit any remaining operations
		if len(deletionOrder)%batchSize != 0 {
			if err := tx.Commit(ctx); err != nil {
				return nil, errfmt.Newf(ErrMsgBulkDeleteCommit).Wrap(err)
			}
		}

		result.SuccessCount = len(deletionOrder)
		result.Duration = time.Since(startTime)
		StorageLog(logger).Info(LogEventStorageBulkDeleteCompletedFastLeafInfo).
			Int("success", result.SuccessCount).
			Int("failed", result.FailureCount).
			String("duration", result.Duration.String()).
			Log()
		FlushPendingAuditEvents(ctx, f.projectRoot, f, secCtx)
		return result, nil
	}

	// Slow path: Use transaction for non-leaf nodes (need atomicity)
	tx, err := f.BeginTransaction(ctx)
	if err != nil {
		return nil, errfmt.Newf(ErrMsgBeginTx).Wrap(err)
	}

	needsRollback := true
	defer func() {
		if needsRollback {
			if rerr := tx.Rollback(ctx); rerr != nil {
				StorageLog(logger).Error(LogEventStorageBulkDeleteRollbackFailedErr, rerr).Log()
			}
		}
	}()

	// Worker pool for parallel deletions
	type deleteJob struct {
		id  string
		idx int
	}

	jobs := make(chan deleteJob, len(deletionOrder))
	results := make(chan struct {
		success bool
		err     error
		job     deleteJob
	}, len(deletionOrder))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < maxWorkers; i++ {
		workerID := i
		goroutinelabels.NewGoroutine(fmt.Sprintf(FmtBulkDeleteWorkerName, workerID), fmt.Sprintf(FmtBulkDeleteWorkerDesc, workerID, maxWorkers)).
			WithWaitGroup(&wg).
			StartWithContext(ctx, func(ctx context.Context) error {
				for {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case job, ok := <-jobs:
						if !ok {
							return nil
						}
						err := tx.Delete(ctx, secCtx, job.id, cascade)
						select {
						case results <- struct {
							success bool
							err     error
							job     deleteJob
						}{
							success: err == nil,
							err:     err,
							job:     job,
						}:
						case <-ctx.Done():
							return ctx.Err()
						}
					}
				}
			})
	}

	// Send jobs
	goroutinelabels.NewGoroutine(DescBulkDeleteJobSender, DescSendDelJobsWorkers).
		WithCleanup(func() {
			close(jobs)
		}).
		StartWithContext(ctx, func(ctx context.Context) error {
			for i, id := range deletionOrder {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case jobs <- deleteJob{id: id, idx: i}:
				}
			}
			return nil
		})

	// Collect results
	goroutinelabels.NewGoroutine(DescBulkDeleteResCollector, DescCollectDelRes).
		WithCleanup(func() {
			close(results)
		}).
		StartWithContext(ctx, func(ctx context.Context) error {
			wg.Wait()
			return nil
		})

	// Process results
	for res := range results {
		when.When(func() bool { return res.success }).Then(func() {
			result.SuccessCount++
		}).OrElse(func() {
			result.FailureCount++
			result.Errors = append(result.Errors, BulkOperationError{
				ID:      res.job.id,
				Index:   res.job.idx,
				Error:   res.err,
				Message: fmt.Sprintf(ErrMsgDeleteObjFail, res.job.id, res.err),
			})
		}).Run()
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, errfmt.Newf(ErrMsgCommitBulkDelTx).Wrap(err)
	}

	needsRollback = false
	result.Duration = time.Since(startTime)

	StorageLog(logger).Info(LogEventStorageBulkDeleteCompletedInfo).
		Int("success", result.SuccessCount).
		Int("failed", result.FailureCount).
		String("duration", result.Duration.String()).
		Log()

	FlushPendingAuditEvents(ctx, f.projectRoot, f, secCtx)
	return result, nil
}

// BulkDeleteResult contains detailed results of an optimized bulk delete operation
type BulkDeleteResult struct {
	TotalCount    int
	SuccessCount  int
	FailureCount  int
	Errors        []BulkOperationError
	DeletionOrder []string
	Duration      time.Duration
}
