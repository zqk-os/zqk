// Extracted from object_write_behind_worker.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
)

func (w *ObjectWriteBehindWorker) apply(ctx context.Context, op *PendingOp, secCtx *pkgctx.SecurityContext) error {
	switch op.Op {
	case "create":
		return w.storage.applyCreateFromBuffer(ctx, op.ID, op.Kind, op.Data, secCtx)
	case "update":
		return w.storage.applyUpdateFromBuffer(ctx, op.ID, op.Kind, op.Data, secCtx)
	case "delete":
		return w.storage.applyDeleteFromBuffer(ctx, op.ID, op.Kind, secCtx)
	default:
		return nil
	}
}

func (w *ObjectWriteBehindWorker) drain(ctx context.Context, secCtx *pkgctx.SecurityContext, logger logging.Logger) {
	deadline := time.Now().Add(shutdownDrainDeadline)
	for w.buf.Len() > 0 && time.Now().Before(deadline) {
		op := w.buf.Peek()
		if op == nil {
			break
		}
		if err := w.apply(ctx, op, secCtx); err != nil {
			if isRetryableWriteBehindApplyDrop(err) {
				w.buf.RemoveFront(op)
				logging.LogSwallowedError(WriteAppliedSeq(w.projectRoot, op.Seq))
				continue
			}
			StorageLog(logger).Error(LogEventStorageWriteBehindShutdownDrainApplyFailed, err).
				String("op", op.Op).
				Kind(op.Kind).
				ObjectID(op.ID).
				Log()
			break
		}
		w.buf.RemoveFront(op)
		logging.LogSwallowedError(WriteAppliedSeq(w.projectRoot, op.Seq))
	}
	if w.buf.Len() > 0 {
		StorageLog(logger).Warn(LogEventStorageWriteBehindShutdownDrainTimeout).
			Int("remaining", w.buf.Len()).
			ProjectRoot(w.projectRoot).
			Log()
	}
}

// Stop signals the worker to stop and drain, then blocks until done.

func isRetryableWriteBehindApplyDrop(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), ConstStreamSaveQueueIsFull)
}

// InitiateShutdown signals the queue to stop accepting new operations
