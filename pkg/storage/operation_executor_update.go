// Extracted from operation_executor_operations.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

func (e *OperationExecutor) executeUpdateWithCache(ctx context.Context, op *Operation) error {
	if e.queue.notifier != nil {
		logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressCreate, fmt.Sprintf(opMsgUpdatingFmt, op.ObjectKind, op.ObjectID)))
	}

	current, err := e.readObjectForOp(ctx, op)
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
	ctx, cancel := context.WithTimeout(ctx, e.ioTimeout)
	defer cancel()

	if e.queue.notifier != nil {
		logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressWrite, fmt.Sprintf(opMsgApplyingUpdatesFmt, op.ObjectKind, op.ObjectID)))
	}

	hashManager, operationID := e.registerDeferredOperation(op, "update")

	if err := e.storage.Update(ctx, op.SecCtx, op.ObjectID, op.Updates); err != nil {
		logging.LogSwallowedError(hashManager.CompleteOperation(op.ObjectID, operationID))
		return errfmt.Newf(ConstMiscFailedToUpdateObject).Wrap(err)
	}

	e.completeDeferredOperation(hashManager, op.ObjectID, operationID)

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

	_, err := e.readObjectForOp(ctx, op)
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
	ctx, cancel := context.WithTimeout(ctx, e.ioTimeout)
	defer cancel()

	cascade := op.Metadata[opMetadataCascade] == opMetadataBoolTrue
	if e.queue.notifier != nil {
		logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressWrite, fmt.Sprintf(opMsgDeletingCascadeFmt, op.ObjectKind, op.ObjectID, cascade)))
	}

	hashManager, operationID := e.registerDeferredOperation(op, "delete")

	if err := e.storage.Delete(ctx, op.SecCtx, op.ObjectID, cascade); err != nil {
		logging.LogSwallowedError(hashManager.CompleteOperation(op.ObjectID, operationID))
		return errfmt.Newf(ConstMiscFailedToDeleteObject).Wrap(err)
	}

	e.completeDeferredOperation(hashManager, op.ObjectID, operationID)

	// Invalidate cache in background
	e.cacheManager.InvalidateAsync(ctx, []string{op.ObjectID}, fmt.Sprintf("Deleted %s", op.ObjectID))

	if e.queue.notifier != nil {
		logging.LogSwallowedError(e.queue.notifier.NotifyProgress(op, opProgressApplied, fmt.Sprintf(opMsgDeletedFmt, op.ObjectKind, op.ObjectID)))
	}

	return nil
}

// executeDelete executes a delete operation (legacy - kept for compatibility)
