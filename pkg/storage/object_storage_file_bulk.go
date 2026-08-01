package storage

import (
	"context"
	"fmt"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// BulkCreate creates multiple objects atomically using a transaction
func (f *FileObjectStorage) BulkCreate(ctx context.Context, secCtx *pkgctx.SecurityContext, objList []map[string]any) (*BulkResult, error) {
	ctx = WithBulkCreateDeferFlush(ctx)
	defer func() {
		if f.projectRoot != emptyValue {
			q := GetListingIndexWriteQueueForProjectRoot(f.projectRoot)
			_ = q.FlushKindContext(context.Background(), objects.KindAuditEvent, IndexFlushAfterCreateTimeout)
			_ = q.FlushKindContext(context.Background(), objects.KindSchedulerJob, IndexFlushAfterCreateTimeout)
		}
	}()

	result := &BulkResult{
		TotalCount: len(objList),
		Results:    make([]map[string]any, 0),
		Errors:     make([]BulkOperationError, 0),
	}

	// Auto-detect cascade mode
	var originalCacheChecker func(string) (string, bool)
	if val := cacheChecker.Load(); val != nil {
		originalCacheChecker = val.(func(string) (string, bool))
	}
	defer func() {
		// Ensure original cacheChecker is always restored on exit
		if originalCacheChecker != nil {
			cacheChecker.Store(originalCacheChecker)
		} else {
			cacheChecker.Store((func(string) (string, bool))(nil))
		}
	}()

	if originalCacheChecker == nil && cacheOperationHandler != nil {
		newChecker := func(objectID string) (string, bool) { return "", false }
		cacheChecker.Store(newChecker)
	}

	// Process in chunks to cap peak memory usage (RECOMP-BULK-001)
	const createChunkSize = 500
	for i := 0; i < len(objList); i += createChunkSize {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}

		end := i + createChunkSize
		if end > len(objList) {
			end = len(objList)
		}
		chunk := objList[i:end]

		// Transaction per chunk
		tx, err := f.BeginTransaction(ctx)
		if err != nil {
			return result, errfmt.Newf(ConstStreamFailedToBeginTransactionForChunkStartingAtInt, i).Wrap(err)
		}

		// Track if chunk needs rollback
		chunkNeedsRollback := true
		func() {
			defer func() {
				if chunkNeedsRollback {
					var _err_83257517 = tx.Rollback(ctx)
					if _err_83257517 != nil {
						logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83257517).Log()
					}
				}
			}()

			for j, obj := range chunk {
				kind, ok := obj[objects.FieldKeyKind].(string)
				if !ok || kind == emptyValue {
					result.FailureCount++
					result.Errors = append(result.Errors, BulkOperationError{
						Index:   i + j,
						Error:   errfmt.Errorf(ConstStreamObjectMissingKindField),
						Message: fmt.Sprintf(ConstStreamObjectAtIndexIntMissingKindField, i+j),
					})
					continue
				}

				_, err := f.ensureObjectID(ctx, obj, kind)
				if err != nil {
					result.FailureCount++
					result.Errors = append(result.Errors, BulkOperationError{
						Index:   i + j,
						Error:   err,
						Message: fmt.Sprintf(ConstStreamFailedToEnsureIdForObjectAtIndexIntVal, i+j, err),
					})
					continue
				}

				err = tx.Create(ctx, secCtx, obj)
				if err != nil {
					result.FailureCount++
					result.Errors = append(result.Errors, BulkOperationError{
						Index:   i + j,
						Error:   err,
						Message: fmt.Sprintf(ConstStreamFailedToCreateObjectAtIndexIntVal, i+j, err),
					})
					continue
				}

				result.SuccessCount++
				objID, _ := obj[objects.FieldKeyID].(string)
				if objID != emptyValue {
					result.Results = append(result.Results, map[string]any{objects.FieldKeyID: objID, objects.FieldKeyKind: kind})
				}
			}

			if err := tx.Commit(ctx); err != nil {
				// Chunk commit failure
				chunkSuccesses := result.SuccessCount - (result.TotalCount - len(objList) + i)
				result.FailureCount += len(chunk) - chunkSuccesses
				result.SuccessCount -= chunkSuccesses
				// Also remove the falsely reported results
				if len(result.Results) >= chunkSuccesses {
					result.Results = result.Results[:len(result.Results)-chunkSuccesses]
				}
				for k, obj := range chunk {
					result.Errors = append(result.Errors, BulkOperationError{
						Index:   i + k,
						Error:   err,
						Message: fmt.Sprintf("chunk commit failed for object %v: %v", obj[objects.FieldKeyID], err),
					})
				}
				return
			}
			chunkNeedsRollback = false
		}()
	}

	return result, nil
}

// BulkUpdate updates multiple objects atomically using a transaction
func (f *FileObjectStorage) BulkUpdate(ctx context.Context, secCtx *pkgctx.SecurityContext, updates []BulkUpdateItem) (*BulkResult, error) {
	ctx = WithBulkCreateDeferFlush(ctx)
	defer func() {
		if f.projectRoot != emptyValue {
			q := GetListingIndexWriteQueueForProjectRoot(f.projectRoot)
			_ = q.FlushKindContext(context.Background(), objects.KindAuditEvent, IndexFlushAfterCreateTimeout)
			_ = q.FlushKindContext(context.Background(), objects.KindSchedulerJob, IndexFlushAfterCreateTimeout)
		}
	}()

	result := &BulkResult{
		TotalCount: len(updates),
		Results:    make([]map[string]any, 0),
		Errors:     make([]BulkOperationError, 0),
	}

	// Auto-detect cascade mode
	var originalCacheChecker func(string) (string, bool)
	if val := cacheChecker.Load(); val != nil {
		originalCacheChecker = val.(func(string) (string, bool))
	}
	defer func() {
		// Ensure original cacheChecker is always restored on exit
		if originalCacheChecker != nil {
			cacheChecker.Store(originalCacheChecker)
		} else {
			cacheChecker.Store((func(string) (string, bool))(nil))
		}
	}()

	if originalCacheChecker == nil && cacheOperationHandler != nil {
		newChecker := func(objectID string) (string, bool) { return "", false }
		cacheChecker.Store(newChecker)
	}

	// Process in chunks to cap peak memory usage (RECOMP-BULK-001)
	const updateChunkSize = 500
	for i := 0; i < len(updates); i += updateChunkSize {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}

		end := i + updateChunkSize
		if end > len(updates) {
			end = len(updates)
		}
		chunk := updates[i:end]

		// Transaction per chunk
		tx, err := f.BeginTransaction(ctx)
		if err != nil {
			return result, errfmt.Newf(ConstStreamFailedToBeginTransactionForChunkStartingAtInt, i).Wrap(err)
		}

		// Track if chunk needs rollback
		chunkNeedsRollback := true
		func() {
			defer func() {
				if chunkNeedsRollback {
					var _err_83260623 = tx.Rollback(ctx)
					if _err_83260623 != nil {
						logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83260623).Log()
					}
				}
			}()

			for j, update := range chunk {
				err := tx.Update(ctx, secCtx, update.ID, update.Updates)
				if err != nil {
					result.FailureCount++
					result.Errors = append(result.Errors, BulkOperationError{
						ID:      update.ID,
						Index:   i + j,
						Error:   err,
						Message: fmt.Sprintf(ConstStreamFailedToUpdateObjectStrVal, update.ID, err),
					})
					continue
				}
				result.SuccessCount++
				// Store only ID to avoid memory accumulation (full object not needed for output)
				result.Results = append(result.Results, map[string]any{objects.FieldKeyID: update.ID})
			}

			if err := tx.Commit(ctx); err != nil {
				// Chunk commit failure - mark chunk as failed
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("CHUNK COMMIT FAILURE", err).Log()
				result.FailureCount += len(chunk) - (result.SuccessCount - (result.TotalCount - len(updates) + i))
				return
			}
			chunkNeedsRollback = false
		}()
	}

	return result, nil
}

// BulkGet retrieves multiple objects by ID
func (f *FileObjectStorage) BulkGet(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string) (*BulkResult, error) {
	result := &BulkResult{
		TotalCount: len(ids),
		Results:    make([]map[string]any, 0),
		Errors:     make([]BulkOperationError, 0),
	}

	// Process each ID (no transaction needed for reads)
	for i, id := range ids {
		obj, err := f.Read(ctx, secCtx, id)
		if err != nil {
			result.FailureCount++
			result.Errors = append(result.Errors, BulkOperationError{
				ID:      id,
				Index:   i,
				Error:   err,
				Message: fmt.Sprintf(ConstStreamFailedToReadObjectStrVal, id, err),
			})
			// Continue processing other IDs
			continue
		}

		result.SuccessCount++
		result.Results = append(result.Results, obj)
	}

	return result, nil
}

// BulkDelete deletes multiple objects atomically using a transaction
func (f *FileObjectStorage) BulkDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, cascade bool) (*BulkResult, error) {
	// Require CLI authorization for deletions
	if !IsCLIOperation(ctx, secCtx) {
		return nil, errfmt.Errorf(ConstStreamDeleteOperationsMustBePerformedThroughCli)
	}

	ctx = WithBulkCreateDeferFlush(ctx)
	defer func() {
		if f.projectRoot != emptyValue {
			q := GetListingIndexWriteQueueForProjectRoot(f.projectRoot)
			_ = q.FlushKindContext(context.Background(), objects.KindAuditEvent, IndexFlushAfterCreateTimeout)
			_ = q.FlushKindContext(context.Background(), objects.KindSchedulerJob, IndexFlushAfterCreateTimeout)
		}
	}()

	result := &BulkResult{
		TotalCount: len(ids),
		Results:    make([]map[string]any, 0),
		Errors:     make([]BulkOperationError, 0),
	}

	// Process in chunks to cap peak memory usage (RECOMP-BULK-001)
	const deleteChunkSize = 500
	for i := 0; i < len(ids); i += deleteChunkSize {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}

		end := i + deleteChunkSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[i:end]

		// Use transaction for atomicity per chunk
		tx, err := f.BeginTransaction(ctx)
		if err != nil {
			return result, errfmt.Newf(ConstStreamFailedToBeginTransactionForDeleteChunkStartingAt, i).Wrap(err)
		}

		// Track if we need to rollback the chunk
		chunkNeedsRollback := true
		func() {
			defer func() {
				if chunkNeedsRollback {
					var _err_83263528 = tx.Rollback(ctx)
					if _err_83263528 != nil {
						logging.Fluent(

							// Track which IDs we're attempting to delete in this chunk
							logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83263528).Log()
					}
				}
			}()

			chunkIDsToDelete := make([]string, 0, len(chunk))
			chunkIDToIndex := make(map[string]int)

			for j, id := range chunk {
				err := tx.Delete(ctx, secCtx, id, cascade)
				if err != nil {
					result.FailureCount++
					result.Errors = append(result.Errors, BulkOperationError{
						ID:      id,
						Index:   i + j,
						Error:   err,
						Message: fmt.Sprintf(ConstStreamFailedToDeleteObjectStrVal, id, err),
					})
					continue
				}
				chunkIDsToDelete = append(chunkIDsToDelete, id)
				chunkIDToIndex[id] = i + j
			}

			// Commit chunk transaction
			if err := tx.Commit(ctx); err != nil {
				// Mark chunk as failed if commit fails
				result.FailureCount += len(chunkIDsToDelete)
				return
			}

			// Record successful deletions in this chunk
			for _, id := range chunkIDsToDelete {
				result.SuccessCount++
				result.Results = append(result.Results, map[string]any{objects.FieldKeyID: id})
			}
			chunkNeedsRollback = false
		}()
	}

	return result, nil
}
