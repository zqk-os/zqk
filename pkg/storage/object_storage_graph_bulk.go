package storage

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// BulkCreate creates multiple objects atomically using a transaction
func (g *GraphObjectStorage) BulkCreate(ctx context.Context, secCtx *pkgctx.SecurityContext, objList []map[string]any) (*BulkResult, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	result := &BulkResult{
		TotalCount: len(objList),
		Results:    make([]map[string]any, 0),
		Errors:     make([]BulkOperationError, 0),
	}

	// Use transaction for atomicity
	tx, err := g.BeginTransaction(ctx)
	if err != nil {
		return nil, errfmt.Newf(ConstStreamFailedToBeginTransaction).Wrap(err)
	}

	// Track if we need to rollback
	needsRollback := true
	defer func() {
		if needsRollback {
			var _err_83521820 = tx.Rollback(ctx)
			if _err_83521820 !=

				// Process each object
				nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83521820).Log()
			}
		}
	}()

	for i, obj := range objList {
		err := tx.Create(ctx, secCtx, obj)
		if err != nil {
			result.FailureCount++
			result.Errors = append(result.Errors, BulkOperationError{
				Index:   i,
				Error:   err,
				Message: fmt.Sprintf(ConstStreamFailedToCreateObjectAtIndexIntVal, i, err),
			})
			// Continue processing other objects
			continue
		}

		result.SuccessCount++
		// Store only ID to avoid memory accumulation (full object not needed for output)
		objID, _ := obj[objects.FieldKeyID].(string)
		kind, _ := obj[objects.FieldKeyKind].(string)
		if objID != emptyValue {
			result.Results = append(result.Results, map[string]any{objects.FieldKeyID: objID, objects.FieldKeyKind: kind})
		}
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, errfmt.Newf(ConstStreamFailedToCommitBulkCreateTransaction).Wrap(err)
	}

	needsRollback = false
	return result, nil
}

// BulkUpdate updates multiple objects atomically using a transaction
func (g *GraphObjectStorage) BulkUpdate(ctx context.Context, secCtx *pkgctx.SecurityContext, updates []BulkUpdateItem) (*BulkResult, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	result := &BulkResult{
		TotalCount: len(updates),
		Results:    make([]map[string]any, 0),
		Errors:     make([]BulkOperationError, 0),
	}

	// Use transaction for atomicity
	tx, err := g.BeginTransaction(ctx)
	if err != nil {
		return nil, errfmt.Newf(ConstStreamFailedToBeginTransaction).Wrap(err)
	}

	// Track if we need to rollback
	//nolint:errcheck // Intentional error ignored
	needsRollback := true
	defer func() {
		if needsRollback {
			var _err_83523472 = tx.Rollback(ctx)
			if _err_83523472 !=

				// Process each update
				nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83523472).Log()
			}
		}
	}()

	for i, update := range updates {
		err := tx.Update(ctx, secCtx, update.ID, update.Updates)
		if err != nil {
			result.FailureCount++
			result.Errors = append(result.Errors, BulkOperationError{
				ID:      update.ID,
				Index:   i,
				Error:   err,
				Message: fmt.Sprintf(ConstStreamFailedToUpdateObjectStrVal, update.ID, err),
			})
			// Continue processing other updates
			continue
		}

		result.SuccessCount++
		// Store only ID to avoid memory accumulation (full object not needed for output)
		result.Results = append(result.Results, map[string]any{objects.FieldKeyID: update.ID})
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, errfmt.Newf(ConstStreamFailedToCommitBulkUpdateTransaction).Wrap(err)
	}

	needsRollback = false
	return result, nil
}

// BulkGet retrieves multiple objects by ID
func (g *GraphObjectStorage) BulkGet(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string) (*BulkResult, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	result := &BulkResult{
		TotalCount: len(ids),
		Results:    make([]map[string]any, 0),
		Errors:     make([]BulkOperationError, 0),
	}

	// Process each ID (no transaction needed for reads)
	for i, id := range ids {
		obj, err := g.Read(ctx, secCtx, id)
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
func (g *GraphObjectStorage) BulkDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, cascade bool) (*BulkResult, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	// Require CLI authorization for deletions
	if !IsCLIOperation(ctx, secCtx) {
		return nil, errfmt.Errorf(ConstStreamDeleteOperationsMustBePerformedThroughCli)
	}

	result := &BulkResult{
		TotalCount: len(ids),
		Results:    make([]map[string]any, 0),
		Errors:     make([]BulkOperationError, 0),
	}

	// Use transaction for atomicity
	tx, err := g.BeginTransaction(ctx)
	if err != nil {
		return nil, errfmt.Newf(ConstStreamFailedToBeginTransaction).Wrap(err)
	}

	// Track if we need to rollback
	needsRollback := true
	defer func() {
		if needsRollback {
			var _err_83525962 = tx.Rollback(ctx)
			if _err_83525962 !=

				// Process each delete
				nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83525962).Log()
			}
		}
	}()

	batchSize := 1000
	for i, id := range ids {
		err := tx.Delete(ctx, secCtx, id, cascade)
		if err != nil {
			result.FailureCount++
			result.Errors = append(result.Errors, BulkOperationError{
				ID:      id,
				Index:   i,
				Error:   err,
				Message: fmt.Sprintf(ConstStreamFailedToDeleteObjectStrVal, id, err),
			})
			// Continue processing other deletes
			continue
		}

		result.SuccessCount++
		// Store only ID to avoid memory accumulation (full object not needed for output)
		result.Results = append(result.Results, map[string]any{objects.FieldKeyID: id})

		if (i+1)%batchSize == 0 {
			if err := tx.Commit(ctx); err != nil {
				return nil, errfmt.Newf(ConstStreamFailedToCommitBulkDeleteTransaction).Wrap(err)
			}
			tx, err = g.BeginTransaction(ctx)
			if err != nil {
				return nil, errfmt.Newf(ConstStreamFailedToBeginTransaction).Wrap(err)
			}
		}
	}

	// Commit remaining transaction
	if len(ids)%batchSize != 0 {
		if err := tx.Commit(ctx); err != nil {
			return nil, errfmt.Newf(ConstStreamFailedToCommitBulkDeleteTransaction).Wrap(err)
		}
	}

	needsRollback = false
	return result, nil
}
