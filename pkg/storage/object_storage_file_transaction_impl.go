package storage

import (
	"context"

	"sync"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	// BeginTransaction starts a transaction for atomic multi-object operations
)

func (f *FileObjectStorage) BeginTransaction(ctx context.Context) (ObjectTransaction, error) {
	return &FileObjectTransaction{
		storage: f,
		ctx:     ctx,
		ops:     make([]fileTransactionOp, 0),
	}, nil
}

// BeginEnhancedTransaction starts a transaction with enhanced safety guarantees
// Uses FileTransactionCoordinator with two-phase locking for better atomicity
// This provides graph-database-like transactional guarantees for multi-file operations
// NOTE: This returns a different transaction type than BeginTransaction
// Use this when you need stronger atomicity guarantees across multiple files
func (f *FileObjectStorage) BeginEnhancedTransaction(ctx context.Context) (*Transaction, error) {
	adapter := NewFileSystemAdapter(f.projectRoot, f)
	return adapter.BeginTransaction(ctx), nil
}

// FileObjectTransaction implements ObjectTransaction for file storage
type FileObjectTransaction struct {
	storage    *FileObjectStorage
	ctx        context.Context
	mu         sync.Mutex
	ops        []fileTransactionOp
	committed  bool
	rolledBack bool
}

type fileTransactionOp struct {
	opType  string // OpCreate, OpUpdate, or OpDelete
	id      string
	kind    string
	obj     map[string]any
	updates map[string]any
}

func allTransactionOpsAreDeletes(ops []fileTransactionOp) bool {
	for _, op := range ops {
		if op.opType != OpDelete {
			return false
		}
	}
	return len(ops) > 0
}

// Create creates a new object within the transaction
func (tx *FileObjectTransaction) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	if tx.committed || tx.rolledBack {
		return errfmt.Errorf(ConstStreamTransactionAlreadyCommittedOrRolledBack)
	}

	kind, _ := obj[objects.FieldKeyKind].(string)
	id, _ := obj[objects.FieldKeyID].(string)
	if kind == emptyValue || id == emptyValue {
		return errfmt.Errorf(ConstStreamObjectMustHaveKindAndId)
	}

	tx.mu.Lock()
	tx.ops = append(tx.ops, fileTransactionOp{
		opType: OpCreate,
		id:     id,
		kind:   kind,
		obj:    obj,
	})
	tx.mu.Unlock()
	return nil
}

// Read reads an object within the transaction (reads from current state, not transaction)
func (tx *FileObjectTransaction) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	// For file backend, reads are always from current state
	// We could implement read-your-writes by checking tx.ops, but for simplicity, read from disk
	return tx.storage.Read(ctx, secCtx, id)
}

// Update updates an object within the transaction
func (tx *FileObjectTransaction) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	if tx.committed || tx.rolledBack {
		return errfmt.Errorf(ConstStreamTransactionAlreadyCommittedOrRolledBack)
	}

	// Read current object to get kind
	obj, err := tx.storage.Read(ctx, secCtx, id)
	if err != nil {
		return err
	}
	kind, _ := obj[objects.FieldKeyKind].(string)

	tx.mu.Lock()
	tx.ops = append(tx.ops, fileTransactionOp{
		opType:  OpUpdate,
		id:      id,
		kind:    kind,
		updates: updates,
	})
	tx.mu.Unlock()
	return nil
}

// Delete deletes an object within the transaction
func (tx *FileObjectTransaction) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	if tx.committed || tx.rolledBack {
		return errfmt.Errorf(ConstStreamTransactionAlreadyCommittedOrRolledBack)
	}

	// Read current object to get kind
	obj, err := tx.storage.Read(ctx, secCtx, id)
	if err != nil {
		return err
	}
	kind, _ := obj[objects.FieldKeyKind].(string)

	if err := denyCoreKernelHardDelete(ctx, secCtx, kind, id); err != nil {
		return err
	}

	tx.mu.Lock()
	tx.ops = append(tx.ops, fileTransactionOp{
		opType: OpDelete,
		id:     id,
		kind:   kind,
	})
	tx.mu.Unlock()
	return nil
}

// DeleteByIDAndKind appends a delete op without reading the object.
// Use for bulk delete when (id, kind) are already known to avoid N reads.
// Critical kinds are refused here — callers must use Delete → kernel.cas_object_erase.
// TRACK: BLI-REDACTED
func (tx *FileObjectTransaction) DeleteByIDAndKind(ctx context.Context, id, kind string) error {
	if tx.committed || tx.rolledBack {
		return errfmt.Errorf(ConstStreamTransactionAlreadyCommittedOrRolledBack)
	}
	if id == emptyValue || kind == emptyValue {
		return errfmt.Errorf(ConstStreamIdAndKindRequiredForDeletebyidandkind)
	}
	if err := denyCoreKernelHardDelete(ctx, pkgctx.GetSecurityContext(ctx), kind, id); err != nil {
		return err
	}
	tx.mu.Lock()
	tx.ops = append(tx.ops, fileTransactionOp{
		opType: OpDelete,
		id:     id,
		kind:   kind,
	})
	tx.mu.Unlock()
	return nil
}

// Commit commits the transaction by applying all operations.
// When write-behind is enabled and multiple ops are present, appends one WAL batch and enqueues to the buffer instead of N separate appends.
//
// Pipeline (read top to bottom):
//  1. commitStageWriteBehindBatchWAL — optional fast path: one WAL batch + write-behind enqueue + notifications; returns early if successful.
//  2. commitStageApplyAllDeletesBatch — all-delete transactions: batched stream / CAS / legacy deletes by kind.
//  3. commitStageApplyPerOpMixed — create/update/delete applied one op at a time (may rollback on create/update failure).
//  4. commitStageFinalize — mark committed + log non-fatal delete errors from batch paths.
func (tx *FileObjectTransaction) Commit(ctx context.Context) error {
	if tx.committed {
		return errfmt.Errorf(ConstStreamTransactionAlreadyCommitted)
	}
	if tx.rolledBack {
		return errfmt.Errorf(ConstStreamTransactionWasRolledBack)
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	f := tx.storage

	// When all ops are deletes, use batch delete paths so stream/CAS work completes before Commit returns.
	allDeletes := allTransactionOpsAreDeletes(tx.ops)

	if done, err := tx.commitStageWriteBehindBatchWAL(ctx, secCtx, f, allDeletes); done {
		return err
	}

	var deleteErrors []error
	if allDeletes {
		deleteErrors = tx.commitStageApplyAllDeletesBatch(ctx, secCtx, f)
	} else {
		var err error
		deleteErrors, err = tx.commitStageApplyPerOpMixed(ctx, secCtx)
		if err != nil {
			return err
		}
	}

	tx.commitStageFinalize(deleteErrors)
	return nil
}

// commitStageWriteBehindBatchWAL runs the write-behind batch WAL path when eligible.
// Returns done=true if the transaction is fully committed by this stage (caller returns immediately).
