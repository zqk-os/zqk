package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

const (
	opProgressCreate   = 25
	opProgressWrite    = 50
	opProgressApplied  = 75
	opProgressComplete = 100

	opMsgStartingFmt          = "Starting %s operation on %s"
	opErrUnknownOperationFmt  = "unknown operation type: %s"
	opMsgCreatingFmt          = "Creating %s %s"
	opMsgWritingFmt           = "Writing %s %s"
	opMsgCreatedFmt           = "Created %s %s"
	opMsgUpdatingFmt          = "Updating %s %s"
	opMsgApplyingUpdatesFmt   = "Applying updates to %s %s"
	opMsgUpdatedFmt           = "Updated %s %s"
	opMsgDeletingFmt          = "Deleting %s %s"
	opMsgDeletingCascadeFmt   = "Deleting %s %s (cascade=%v)"
	opMsgDeletedFmt           = "Deleted %s %s"
	opMsgAlreadyDeletedFmt    = "Object %s already deleted (skipped)"
	opErrObjectExistsFmt      = "object already exists: %s"
	opErrReadObjectFmt        = "failed to read object: %w"
	opMetadataExpectedUpdated = "expected_updated_at"
	opMetadataCascade         = "cascade"
	opMetadataBoolTrue        = "true"
)

// executeOperationWithTimeout executes an operation with timeout and retry
func (e *OperationExecutor) executeOperationWithTimeout(op *Operation) error {
	// Create context with timeout for I/O operations
	ctx, cancel := context.WithTimeout(op.Context, e.ioTimeout)
	defer cancel()

	// Execute with retry
	err := e.executeWithRetry(ctx, op, func() error {
		return e.executeOperationOnce(ctx, op)
	})
	if err != nil {
		e.operationsFailedTotal.Add(1)
		return err
	}
	e.operationsExecutedTotal.Add(1)
	return nil
}

// executeWithRetry executes a function with retry logic using shared ExecuteSimpleRetry
// Adds progress notifications for operation queue integration
// Uses isRetryableErrorEnhanced which includes version conflict checking
func (e *OperationExecutor) executeWithRetry(ctx context.Context, op *Operation, fn func() error) error {
	// Track attempt count for progress notifications
	attemptCount := 0

	// Wrap the operation to add progress notifications
	wrappedFn := func() error {
		attemptCount++
		// Notify retry if this is not the first attempt
		if attemptCount > 1 && e.queue.notifier != nil {
			var err_swallow_64 = e.queue.notifier.NotifyProgress(op,
				int(float64(attemptCount-1)/float64(e.retryConfig.MaxAttempts)*100),
				fmt.Sprintf("Retry attempt %d/%d...", attemptCount, e.retryConfig.MaxAttempts))
			if //nolint:errcheck // Notification errors are non-critical
			err_swallow_64 != nil {
				logging.LogSwallowedError(err_swallow_64)
			}
		}
		return fn()
	}

	// Use shared ExecuteSimpleRetry with enhanced error checker (includes version conflicts)
	return ExecuteSimpleRetry(ctx, e.retryConfig, wrappedFn, isRetryableErrorEnhanced)
}

// executeOperationOnce executes a single operation attempt (with timeout)
func (e *OperationExecutor) executeOperationOnce(ctx context.Context, op *Operation) error {
	var err_swallow_65 = e.queue.UpdateStatus(op, StatusRunning)
	if //nolint:errcheck // Status update errors are non-critical
	err_swallow_65 != nil {
		logging.LogSwallowedError(err_swallow_65)

		// Notify progress
	}

	if e.queue.notifier != nil {
		logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, 0, fmt.Sprintf(opMsgStartingFmt, op.Type, op.ObjectID)))
	}

	var err error
	switch op.Type {
	case OperationCreate:
		err = e.executeCreateWithCache(ctx, op)
	case OperationUpdate:
		err = e.executeUpdateWithCache(ctx, op)
	case OperationDelete:
		err = e.executeDeleteWithCache(ctx, op)
	case OperationCascadeNullify:
		err = e.executeCascadeNullify(op)
	case OperationCascadeSetNull:
		err = e.executeCascadeSetNull(op)
	default:
		err = errfmt.Errorf(opErrUnknownOperationFmt, op.Type)
	}

	return err
}

// executeCreateWithCache executes create with cache awareness
func (e *OperationExecutor) executeCreateWithCache(ctx context.Context, op *Operation) error {
	if e.queue.notifier != nil {
		logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressCreate, fmt.Sprintf(opMsgCreatingFmt, op.ObjectKind, op.ObjectID)))
	}

	// Check for conflicts (with timeout)
	ctx, cancel := context.WithTimeout(ctx, e.ioTimeout)
	defer cancel()

	_, err := e.storage.Read(ctx, op.SecCtx, op.ObjectID)
	if err == nil || !errors.Is(err, ErrObjectNotFound) {
		// Object exists or other error
		if err == nil {
			return errfmt.Errorf(opErrObjectExistsFmt, op.ObjectID)
		}
		return errfmt.Newf(ConstMiscFailedToCheckIfObjectExists).Wrap(err)
	}

	// Create object (with timeout)
	ctx, cancel = context.WithTimeout(ctx, e.ioTimeout)
	defer cancel()

	if e.queue.notifier != nil {
		logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressWrite, fmt.Sprintf(opMsgWritingFmt, op.ObjectKind, op.ObjectID)))
	}

	// Register operation with deferred hash manager
	hashManager := GetDeferredHashManager(e.storage)
	operationID := fmt.Sprintf("create-%s-%d", op.ObjectID, time.Now().UnixNano())
	// Get file path from storage (if file storage)
	var filePath string
	if fileStorage, ok := e.storage.(*FileObjectStorage); ok {
		var err error
		filePath, err = fileStorage.getObjectFilePath(op.ObjectID, op.ObjectKind)
		if err != nil {
			// Log warning but continue - file path will be inferred later
			StorageLog(e.logger.Logger()).Warn(LogEventStorageOperationExecutorDeferredHashFilePathWarn).
				ObjectID(op.ObjectID).
				WithError(err).
				Log()
		}
	}
	hashManager.RegisterOperation(op.ObjectID, op.ObjectKind, filePath, operationID)

	if err := e.storage.Create(ctx, op.SecCtx, op.Data); err != nil {
		logging.LogSwallowedError(hashManager.CompleteOperation(op.ObjectID, operationID))
		return errfmt.Newf(ConstMiscFailedToCreateObject).Wrap(err)
	}

	// Mark operation as complete (will trigger hash update if no other pending ops)
	if err := hashManager.CompleteOperation(op.ObjectID, operationID); err != nil {
		// Log warning but don't fail - hash update is best effort
		StorageLog(e.logger.Logger()).Warn(LogEventStorageOperationExecutorDeferredHashCompleteWarn).
			ObjectID(op.ObjectID).
			WithError(err).
			Log()
	}

	// Invalidate cache in background
	e.cacheManager.InvalidateAsync(ctx, []string{op.ObjectID}, fmt.Sprintf("Created %s", op.ObjectID))

	if e.queue.notifier != nil {
		logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressApplied, fmt.Sprintf(opMsgCreatedFmt, op.ObjectKind, op.ObjectID)))
	}

	return nil
}

// executeCreate executes a create operation (legacy - kept for compatibility)
func (e *OperationExecutor) executeCreate(op *Operation) error {
	if e.queue.notifier != nil {
		logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressCreate, fmt.Sprintf(opMsgCreatingFmt, op.ObjectKind, op.ObjectID)))
	}

	// Check for conflicts (object already exists)
	_, err := e.storage.Read(op.Context, op.SecCtx, op.ObjectID)
	if err == nil || !errors.Is(err, ErrObjectNotFound) {
		// Object exists - conflict detected
		conflict := &Conflict{
			Type:     "exists",
			ObjectID: op.ObjectID,
			Message:  fmt.Sprintf(ConstMiscObjectSAlreadyExists, op.ObjectID),
		}

		if e.conflictResolver != nil {
			strategy, resolveErr := e.conflictResolver.ResolveConflict(op.Context, op, conflict)
			if resolveErr != nil {
				return resolveErr
			}

			switch strategy {
			case StrategySkip:
				// Silently skip (idempotent)
				return nil
			case StrategyReject:
				return errfmt.Errorf(opErrObjectExistsFmt, op.ObjectID)
			case StrategyMerge:
				// Treat as update
				op.Type = OperationUpdate
				op.Updates = op.Data
				return e.executeUpdate(op)
			default:
				return errfmt.Errorf(opErrObjectExistsFmt, op.ObjectID)
			}
		}

		return errfmt.Errorf(opErrObjectExistsFmt, op.ObjectID)
	}

	// At this point, err == ErrObjectNotFound (object doesn't exist, proceed with create)
	// Create object
	if e.queue.notifier != nil {
		logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressWrite, fmt.Sprintf(opMsgWritingFmt, op.ObjectKind, op.ObjectID)))
	}

	if err := e.storage.Create(op.Context, op.SecCtx, op.Data); err != nil {
		return errfmt.Newf(ConstMiscFailedToCreateObject).Wrap(err)
	}

	if e.queue.notifier != nil {
		logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressApplied, fmt.Sprintf(opMsgCreatedFmt, op.ObjectKind, op.ObjectID)))
	}

	return nil
}

// executeUpdateWithCache executes update with cache awareness
func (e *OperationExecutor) executeUpdate(op *Operation) error {
	if e.queue.notifier != nil {
		logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressCreate, fmt.Sprintf(opMsgUpdatingFmt, op.ObjectKind, op.ObjectID)))
	}

	// Read current object
	current, err := e.storage.Read(op.Context, op.SecCtx, op.ObjectID)
	if err != nil {
		return errfmt.Errorf(opErrReadObjectFmt, err)
	}

	// Check for version conflicts
	if expectedUpdatedAtStr, ok := op.Metadata[opMetadataExpectedUpdated]; ok && expectedUpdatedAtStr != emptyValue {
		currentUpdatedAt, _ := current[objects.FieldKeyUpdatedAt].(string)
		if currentUpdatedAt != expectedUpdatedAtStr {
			// Version conflict detected
			conflict := &Conflict{
				Type:     "version",
				ObjectID: op.ObjectID,
				Message:  fmt.Sprintf(ConstMiscObjectSWasModifiedSinceRead, op.ObjectID),
				Details: map[string]any{
					"expected": expectedUpdatedAtStr,
					"current":  currentUpdatedAt,
				},
			}

			if e.conflictResolver != nil {
				strategy, resolveErr := e.conflictResolver.ResolveConflict(op.Context, op, conflict)
				if resolveErr != nil {
					return resolveErr
				}

				switch strategy {
				case StrategyRetry:
					// Re-read and re-apply update
					if op.Metadata == nil {
						op.Metadata = make(map[string]string)
					}
					op.Metadata[opMetadataExpectedUpdated] = currentUpdatedAt
					// Merge current state with updates
					for k, v := range current {
						if _, exists := op.Updates[k]; !exists {
							op.Updates[k] = v
						}
					}
					return e.executeUpdate(op)
				case StrategyReject:
					return ErrVersionConflict
				default:
					return ErrVersionConflict
				}
			}

			return ErrVersionConflict
		}
	}

	// Apply updates
	if e.queue.notifier != nil {
		logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressWrite, fmt.Sprintf(opMsgApplyingUpdatesFmt, op.ObjectKind, op.ObjectID)))
	}

	if err := e.storage.Update(op.Context, op.SecCtx, op.ObjectID, op.Updates); err != nil {
		return errfmt.Newf(ConstMiscFailedToUpdateObject).Wrap(err)
	}

	if e.queue.notifier != nil {
		logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressApplied, fmt.Sprintf(opMsgUpdatedFmt, op.ObjectKind, op.ObjectID)))
	}

	return nil
}

// executeDeleteWithCache executes delete with cache awareness
func (e *OperationExecutor) executeDelete(op *Operation) error {
	if e.queue.notifier != nil {
		logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressCreate, fmt.Sprintf(opMsgDeletingFmt, op.ObjectKind, op.ObjectID)))
	}

	// Check if object exists
	_, err := e.storage.Read(op.Context, op.SecCtx, op.ObjectID)
	if err != nil && errors.Is(err, ErrObjectNotFound) {
		// Object already deleted - idempotent
		if e.queue.notifier != nil {
			logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressComplete, fmt.Sprintf(opMsgAlreadyDeletedFmt, op.ObjectID)))
		}
		return nil
	}
	if err != nil {
		return errfmt.Errorf(opErrReadObjectFmt, err)
	}

	// Delete object
	cascade := op.Metadata[opMetadataCascade] == opMetadataBoolTrue
	if e.queue.notifier != nil {
		logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressWrite, fmt.Sprintf(opMsgDeletingCascadeFmt, op.ObjectKind, op.ObjectID, cascade)))
	}

	if err := e.storage.Delete(op.Context, op.SecCtx, op.ObjectID, cascade); err != nil {
		return errfmt.Newf(ConstMiscFailedToDeleteObject).Wrap(err)
	}

	if e.queue.notifier != nil {
		logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressApplied, fmt.Sprintf(opMsgDeletedFmt, op.ObjectKind, op.ObjectID)))
	}

	return nil
}

// executeCascadeNullify removes a reference from a list field
func (e *OperationExecutor) executeCascadeNullify(_ *Operation) error {
	// This will be implemented when cascade update logic is added
	// For now, this is a placeholder
	return nil
}

// executeCascadeSetNull sets a single reference field to null
func (e *OperationExecutor) executeCascadeSetNull(_ *Operation) error {
	// This will be implemented when cascade update logic is added
	// For now, this is a placeholder
	return nil
}

// GetConsistencyStatus returns current cache consistency status
func (e *OperationExecutor) GetConsistencyStatus() *ConsistencyStatus {
	return e.cacheManager.GetConsistencyStatus()
}
