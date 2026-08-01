package storage

import (
	"context"
	"encoding/base64"
	"path/filepath"

	"github.com/lanceman/zqk/pkg/when"

	"sync"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/locknames"
	"gopkg.in/yaml.v3"
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
func (tx *FileObjectTransaction) DeleteByIDAndKind(ctx context.Context, id, kind string) error {
	if tx.committed || tx.rolledBack {
		return errfmt.Errorf(ConstStreamTransactionAlreadyCommittedOrRolledBack)
	}
	if id == emptyValue || kind == emptyValue {
		return errfmt.Errorf(ConstStreamIdAndKindRequiredForDeletebyidandkind)
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
func (tx *FileObjectTransaction) commitStageWriteBehindBatchWAL(ctx context.Context, secCtx *pkgctx.SecurityContext, f *FileObjectStorage, allDeletes bool) (done bool, err error) {
	if f.wal == nil || f.writeBuf == nil || len(tx.ops) == 0 || allDeletes {
		return false, nil
	}

	recs, prepErr := f.prepareBatchWALRecords(ctx, secCtx, tx.ops)
	if prepErr != nil || len(recs) != len(tx.ops) {
		return false, nil
	}

	var appendErr error
	_ = concurrency.RunInLockOrLog(&f.walMu, locknames.LockNameCommitBatchWal, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if err := f.wal.AppendBatch(recs); err != nil {
			appendErr = err
			return nil
		}
		appendErr = f.wal.Sync()
		return nil
	})
	if appendErr != nil {
		return false, nil
	}

	// Enqueue outside walMu so buffer back-pressure (e.g. maxReplayBacklog) doesn't hold the lock.
	for _, rec := range recs {
		var data []byte
		if rec.DataB64 != emptyValue {
			var decodeErr error
			data, decodeErr = base64.StdEncoding.DecodeString(rec.DataB64)
			if decodeErr != nil {
				appendErr = errfmt.Errorf(ConstStreamDecodeRecordDataForStrStrErr, rec.Kind, rec.ID, decodeErr)
				break
			}
		}
		f.writeBuf.Enqueue(rec.Op, rec.Kind, rec.ID, rec.Seq, data)
	}
	if appendErr != nil {
		return false, nil
	}

	f.writeBehindWorker.Notify()
	for _, op := range tx.ops {
		RecordObjectStateChange(ctx, op.opType, op.id)
		obj := op.obj
		if op.opType == OpUpdate {
			var _err_83883913 error
			obj, _err_83883913 = f.Read(ctx, secCtx, op.id)
			if _err_83883913 != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83883913).Log()
			}
			if obj != nil {
				for k, v := range op.updates {
					obj[k] = v
				}
			}
		}
		if obj == nil && op.opType == OpDelete {
			obj = map[string]any{objects.FieldKeyID: op.id, objects.FieldKeyKind: op.kind}
		}
		if obj != nil {
			executeChangeNotification(ctx, op.opType, op.kind, op.id, obj)
		}
	}
	tx.committed = true
	return true, nil
}

// commitStageApplyAllDeletesBatch applies delete-only transactions using batched stream/CAS paths per kind.
func (tx *FileObjectTransaction) commitStageApplyAllDeletesBatch(ctx context.Context, secCtx *pkgctx.SecurityContext, f *FileObjectStorage) (deleteErrors []error) {
	byKind := make(map[string][]fileTransactionOp)
	for _, op := range tx.ops {
		byKind[op.kind] = append(byKind[op.kind], op)
	}
	for kind, ops := range byKind {
		when.When(func() bool { return StreamStorageEnabledForKind(kind) }).Then(func() {
			ids := make([]string, 0, len(ops))
			for _, op := range ops {
				ids = append(ids, op.id)
				f.removeStreamLocation(op.id, kind)
				var _err_83884841 = RemoveStreamBackedCurrentState(f.projectRoot, kind, op.id)
				if //nolint:errcheck // best-effort overlay cleanup
				_err_83884841 != nil {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83884841).Log()
				}
			}
			if err := BatchAddStreamDeletedIDs(f.projectRoot, kind, ids); err != nil {
				deleteErrors = append(deleteErrors, errfmt.Errorf(ConstStreamBatchStreamDeletedStrErr, kind, err))
			}
			for _, op := range ops {
				updateHighVolumeEventCacheOnDelete(op.id)
				updateReverseReferenceIndexOnDelete(op.id)
				RecordObjectStateChange(ctx, OpDelete, op.id)
				executeChangeNotification(ctx, OpDelete, kind, op.id, nil)
			}
		}).OrElseWhen(func() bool { return f.usesContentAddressableStorage(kind) }).Then(func() {
			cas, casErr := f.getContentAddressableStorage(kind)
			if casErr == nil && cas != nil {
				ids := make([]string, 0, len(ops))
				for _, op := range ops {
					ids = append(ids, op.id)
				}
				if batchErr := cas.BatchDelete(ids); batchErr != nil {
					deleteErrors = append(deleteErrors, errfmt.Errorf(ConstStreamBatchCasDeleteStrErr, kind, batchErr))
				} else {
					var kindDir string
					if dirName := objects.GetDirectoryFromKind(kind); dirName != emptyValue {
						kindDir = filepath.Join(f.processDir, dirName)
					}
					for _, op := range ops {
						// Mirror synchronous CAS Delete: orphan blobs + runtime_delta overlay
						// (BatchDelete only removes the primary hash file + index entry).
						if kindDir != emptyValue {
							removeOrphanCASFilesForObjectID(op.id, kindDir)
						}
						var _err_83886061 = RemoveRuntimeDeltaCurrentState(f.projectRoot, kind, op.id)
						if //nolint:errcheck
						_err_83886061 != nil {
							logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83886061).Log()
						}
						updateHighVolumeEventCacheOnDelete(op.id)
						updateReverseReferenceIndexOnDelete(op.id)
						RecordObjectStateChange(ctx, OpDelete, op.id)
						executeChangeNotification(ctx, OpDelete, kind, op.id, nil)
					}
				}
			} else {
				for _, op := range ops {
					if err := tx.storage.Delete(ctx, secCtx, op.id, false); err != nil && err != ErrObjectNotFound {
						deleteErrors = append(deleteErrors, errfmt.Errorf(ConstStreamFailedToDeleteObjectStrErr, op.id, err))
					}
				}
			}
		}).OrElse(func() {
			batchCtx := WithSuppressHashRegistryUpdate(ctx)
			for _, op := range ops {
				if err := tx.storage.Delete(batchCtx, secCtx, op.id, false); err != nil && err != ErrObjectNotFound {
					deleteErrors = append(deleteErrors, errfmt.Errorf(ConstStreamFailedToDeleteObjectStrErr, op.id, err))
				}
			}

			if len(ops) > 0 {
				kindDir := tx.storage.GetKindDir(kind)
				if kindDir != emptyValue {
					hashRegistry := tx.storage.newHashRegistry(ctx, kind, kindDir)
					if err := hashRegistry.Load(); err == nil {
						config := GetStorageConfig()
						for _, op := range ops {
							hashRegistry.DeleteHash(op.id + config.YAMLExtension)
							hashRegistry.DeleteHash(op.id + config.YAMLAltExtension)
						}
						_ = tx.storage.saveHashRegistry(hashRegistry)
					}
				}
			}
		}).Run()
	}
	tx.committed = true
	return deleteErrors
}

// commitStageApplyPerOpMixed applies create/update/delete in order; create/update failures roll back and return a fatal error.
func (tx *FileObjectTransaction) commitStageApplyPerOpMixed(ctx context.Context, secCtx *pkgctx.SecurityContext) (deleteErrors []error, err error) {
	for _, op := range tx.ops {
		switch op.opType {
		case OpCreate:
			if err := tx.storage.Create(ctx, secCtx, op.obj); err != nil {
				var _err_83887600 = tx.Rollback(ctx)
				if _err_83887600 != nil {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83887600).Log()
				}
				return nil, errfmt.Errorf(ConstStreamFailedToCreateObjectStrErr, op.id, err)
			}
		case OpUpdate:
			if err := tx.storage.Update(ctx, secCtx, op.id, op.updates); err != nil {
				var _err_83887851 = tx.Rollback(ctx)
				if _err_83887851 != nil {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83887851).Log()
				}
				return nil, errfmt.Errorf(ConstStreamFailedToUpdateObjectStrErr, op.id, err)
			}
		case OpDelete:
			if err := tx.storage.Delete(ctx, secCtx, op.id, false); err != nil {
				if err != ErrObjectNotFound {
					deleteErrors = append(deleteErrors, errfmt.Errorf(ConstStreamFailedToDeleteObjectStrErr, op.id, err))
				}
			}
		}
	}
	if !tx.committed {
		tx.committed = true
	}
	return deleteErrors, nil
}

// commitStageFinalize marks the transaction committed (if not already) and logs non-fatal delete errors from batch delete paths.
func (tx *FileObjectTransaction) commitStageFinalize(deleteErrors []error) {
	if len(deleteErrors) > 0 {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		for _, err := range deleteErrors {
			StorageLog(logger).Warn(LogEventStorageObjectTransactionDeleteFailedDuringCommit).WithError(err).Log()
		}
	}
	// Ensure high-volume event cache is saved to disk so subsequent CLI/daemon processes see updated counts (e.g. after retention).
	if tx.storage != nil {
		SaveHighVolumeEventCache(tx.storage.projectRoot)
	}
	tx.committed = true
}

// prepareBatchWALRecords builds WAL records for transaction ops for write-behind batch append.
// Returns (recs, nil) only when all ops can be prepared; otherwise returns (nil, err) and Commit falls back to per-op.
func (f *FileObjectStorage) prepareBatchWALRecords(ctx context.Context, secCtx *pkgctx.SecurityContext, ops []fileTransactionOp) ([]*WALRecord, error) {
	recs := make([]*WALRecord, 0, len(ops))
	for _, op := range ops {
		var rec WALRecord
		switch op.opType {
		case OpCreate:
			filePath, err := f.prepareObjectPath(op.id, op.kind)
			if err != nil {
				return nil, errfmt.Errorf(ConstStreamPreparePathStrErr, op.id, err)
			}
			if err := f.checkObjectExists(op.id, op.kind, filePath); err != nil {
				return nil, errfmt.Errorf(ConstStreamCheckExistsStrErr, op.id, err)
			}
			if err := f.validateObjectBeforeCreation(ctx, op.obj, op.kind, secCtx); err != nil {
				return nil, errfmt.Errorf(ConstStreamValidateStrErr, op.id, err)
			}
			data, err := f.marshalObjectForCreation(op.obj, filePath)
			if err != nil {
				return nil, errfmt.Errorf(ConstStreamMarshalStrErr, op.id, err)
			}
			rec = WALRecord{Op: "create", Kind: op.kind, ID: op.id, DataB64: base64.StdEncoding.EncodeToString(data)}
		case OpUpdate:
			// Skip ID-change updates; Commit will fall back to per-op for the whole transaction
			if newID := objects.GetString(op.updates, objects.FieldKeyID); newID != "" && newID != op.id {
				return nil, errfmt.Errorf(ConstStreamUpdateStrChangesIdUsePerOpPath, op.id)
			}
			existing, err := f.Read(ctx, secCtx, op.id)
			if err != nil {
				return nil, errfmt.Errorf("read %s: %w", op.id, err)
			}
			for k, v := range op.updates {
				if mergeMapPatchIntoExisting(existing, k, v) {
					continue
				}
				existing[k] = v
			}
			data, err := yaml.Marshal(existing)
			if err != nil {
				return nil, errfmt.Errorf(ConstStreamMarshalUpdateStrErr, op.id, err)
			}
			rec = WALRecord{Op: "update", Kind: op.kind, ID: op.id, DataB64: base64.StdEncoding.EncodeToString(data)}
		case OpDelete:
			rec = WALRecord{Op: "delete", Kind: op.kind, ID: op.id}
		default:
			return nil, errfmt.Errorf(ConstStreamUnknownOpTypeQuote, op.opType)
		}
		recs = append(recs, &rec)
	}
	return recs, nil
}

// Rollback rolls back the transaction (no-op for file backend since we haven't applied changes yet)
func (tx *FileObjectTransaction) Rollback(ctx context.Context) error {
	if tx.committed {
		return errfmt.Errorf(ConstStreamTransactionAlreadyCommitted)
	}
	tx.rolledBack = true
	tx.ops = nil // Clear operations
	return nil
}
