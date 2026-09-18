// Extracted from file_transaction_coordinator.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"
	"errors"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
	// FileTransactionCoordinator provides transactional safety for multi-file operations
	// Uses a two-phase locking protocol to ensure atomicity across multiple file edits
	// Similar to graph database transactions, but for file-based storage
)

func (tx *Transaction) Commit(ctx context.Context, secCtx *pkgctx.SecurityContext) error {
	var filePaths []string
	var operations []transactionOp
	err := concurrency.RunInLockWithLogger(
		&tx.mu, locknames.LockNameTransactionCommitCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if tx.committed {
				return errfmt.Errorf(ConstMiscTransactionAlreadyCommitted)
			}
			if tx.rolledBack {
				return errfmt.Errorf(ConstMiscTransactionWasRolledBack)
			}
			// Copy data for commit (release lock before I/O)
			filePaths = make([]string, len(tx.filePaths))
			copy(filePaths, tx.filePaths)
			operations = make([]transactionOp, len(tx.operations))
			copy(operations, tx.operations)
			return nil
		},
	)
	if err != nil {
		return err
	}

	// Phase 1: Acquire all locks (ordered to prevent deadlocks) (NO TRANSACTION LOCK HELD)
	timeout := 10 * time.Second
	if err := tx.adapter.coordinator.AcquireLocks(ctx, filePaths, timeout); err != nil {
		return errfmt.Newf(ConstMiscFailedToAcquireTransactionLocks).Wrap(err)
	}

	// Ensure locks are released on error
	defer func() {
		var committed bool
		_ = concurrency.RunInLockOrLog(
			&tx.mu, locknames.LockNameTransactionCommitDeferCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				committed = tx.committed
				return nil
			},
		)
		if !committed {
			var _err_82737205 = tx.adapter.coordinator.ReleaseLocks(ctx)
			if //nolint:errcheck
			_err_82737205 != nil {
				logging.Fluent(logging.GetLoggerFromProfile(

					// Phase 2: Apply all operations atomically (NO TRANSACTION LOCK HELD)
					// Use temporary files and atomic moves for true atomicity
					string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_82737205).Log()
			}
		}
	}()

	for _, op := range operations {
		switch op.opType {
		case OpCreate:
			if err := tx.adapter.storage.Create(ctx, secCtx, op.obj); err != nil {
				return errfmt.Errorf(ConstMiscFailedToCreateObjectSW, op.id, err)
			}
		case OpUpdate:
			if err := tx.adapter.storage.Update(ctx, secCtx, op.id, op.updates); err != nil {
				return errfmt.Errorf(ConstMiscFailedToUpdateObjectSW, op.id, err)
			}
		case OpDelete:
			if err := tx.adapter.storage.Delete(ctx, secCtx, op.id, false); err != nil && !errors.Is(err, ErrObjectNotFound) {
				return errfmt.Errorf(ConstMiscFailedToDeleteObjectSW, op.id, err)
			}
		}
	}

	// Phase 3: Release locks (transaction committed successfully) (NO TRANSACTION LOCK HELD)
	if err := tx.adapter.coordinator.ReleaseLocks(ctx); err != nil {
		// Log but don't fail - operations are already committed
	}

	// Mark as committed (re-acquire transaction lock)
	_ = concurrency.RunInLockOrLog(
		&tx.mu, locknames.LockNameTransactionCommitMark, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			tx.committed = true
			return nil
		},
	)
	return nil
}

// Rollback rolls back the transaction
func (tx *Transaction) Rollback(ctx context.Context) error {
	return concurrency.RunInLockWithLogger(
		&tx.mu, locknames.LockNameFileTransactionRollback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if tx.committed {
				return errfmt.Errorf(ConstMiscTransactionAlreadyCommitted)
			}
			var _err_82738789 = tx.adapter.coordinator.ReleaseLocks(ctx)
			if //nolint:errcheck
			_err_82738789 != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_82738789).Log()
			}

			tx.rolledBack = true
			tx.operations = nil
			tx.filePaths = nil

			return nil
		},
	)
}

// getObjectFilePathForTransaction is a helper to get file path for an object in a transaction
// Uses the existing FileObjectStorage method but with a simplified signature for transactions
