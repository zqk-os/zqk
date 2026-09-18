// Extracted from operation_executor_operations.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

func (e *OperationExecutor) executeUpdateWithCache(ctx context.Context, op *Operation) error {
	if e.queue.notifier != nil {
		logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressCreate, fmt.Sprintf(opMsgUpdatingFmt, op.ObjectKind, op.ObjectID)))
	}

	// Read current object (with timeout)
	ctx, cancel := context.WithTimeout(ctx, e.ioTimeout)
	defer cancel()

	current, err := e.storage.Read(ctx, op.SecCtx, op.ObjectID)
	if err != nil {
		return errfmt.Errorf(opErrReadObjectFmt, err)
	}

	// Check for version conflicts
	if expectedUpdatedAtVal, ok := op.Metadata[opMetadataExpectedUpdated]; ok && expectedUpdatedAtVal != emptyValue {
		expectedUpdatedAtStr := expectedUpdatedAtVal
		currentUpdatedAt, _ := current[objects.FieldKeyUpdatedAt].(string)
		if currentUpdatedAt != expectedUpdatedAtStr {
			// Version conflict - handled by retry logic
			return ErrVersionConflict
		}
	}

	// Apply updates (with timeout)
	ctx, cancel = context.WithTimeout(ctx, e.ioTimeout)
	defer cancel()

	if e.queue.notifier != nil {
		logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressWrite, fmt.Sprintf(opMsgApplyingUpdatesFmt, op.ObjectKind, op.ObjectID)))
	}

	// Register operation with deferred hash manager
	hashManager := GetDeferredHashManager(e.storage)
	operationID := fmt.Sprintf("update-%s-%d", op.ObjectID, time.Now().UnixNano())
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

	if err := e.storage.Update(ctx, op.SecCtx, op.ObjectID, op.Updates); err != nil {
		logging.LogSwallowedError(hashManager.CompleteOperation(op.ObjectID, operationID))
		return errfmt.Newf(ConstMiscFailedToUpdateObject).Wrap(err)
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
	e.cacheManager.InvalidateAsync(ctx, []string{op.ObjectID}, fmt.Sprintf("Updated %s", op.ObjectID))

	if e.queue.notifier != nil {
		logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressApplied, fmt.Sprintf(opMsgUpdatedFmt, op.ObjectKind, op.ObjectID)))
	}

	return nil
}

// executeUpdate executes an update operation (legacy - kept for compatibility)

func (e *OperationExecutor) executeDeleteWithCache(ctx context.Context, op *Operation) error {
	if e.queue.notifier != nil {
		logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressCreate, fmt.Sprintf(opMsgDeletingFmt, op.ObjectKind, op.ObjectID)))
	}

	// Check if object exists (with timeout)
	ctx, cancel := context.WithTimeout(ctx, e.ioTimeout)
	defer cancel()

	_, err := e.storage.Read(ctx, op.SecCtx, op.ObjectID)
	if err != nil && errors.Is(err, ErrObjectNotFound) {
		// Already deleted - idempotent
		if e.queue.notifier != nil {
			logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressComplete, fmt.Sprintf(opMsgAlreadyDeletedFmt, op.ObjectID)))
		}
		return nil
	}
	if err != nil {
		return errfmt.Errorf(opErrReadObjectFmt, err)
	}

	// Delete object (with timeout)
	ctx, cancel = context.WithTimeout(ctx, e.ioTimeout)
	defer cancel()

	cascade := op.Metadata[opMetadataCascade] == opMetadataBoolTrue
	if e.queue.notifier != nil {
		logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressWrite, fmt.Sprintf(opMsgDeletingCascadeFmt, op.ObjectKind, op.ObjectID, cascade)))
	}

	// Register operation with deferred hash manager
	hashManager := GetDeferredHashManager(e.storage)
	operationID := fmt.Sprintf("delete-%s-%d", op.ObjectID, time.Now().UnixNano())
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

	if err := e.storage.Delete(ctx, op.SecCtx, op.ObjectID, cascade); err != nil {
		logging.LogSwallowedError(hashManager.CompleteOperation(op.ObjectID, operationID))
		return errfmt.Newf(ConstMiscFailedToDeleteObject).Wrap(err)
	}

	// Mark operation as complete (will trigger hash update if no other pending ops)
	// Note: For deletes, hash is removed from registry, not updated
	if err := hashManager.CompleteOperation(op.ObjectID, operationID); err != nil {
		// Log warning but don't fail - hash update is best effort
		StorageLog(e.logger.Logger()).Warn(LogEventStorageOperationExecutorDeferredHashCompleteDeleteWarn).
			ObjectID(op.ObjectID).
			WithError(err).
			Log()
	}

	// Invalidate cache in background
	e.cacheManager.InvalidateAsync(ctx, []string{op.ObjectID}, fmt.Sprintf("Deleted %s", op.ObjectID))

	if e.queue.notifier != nil {
		logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressApplied, fmt.Sprintf(opMsgDeletedFmt, op.ObjectKind, op.ObjectID)))
	}

	return nil
}

// executeDelete executes a delete operation (legacy - kept for compatibility)
