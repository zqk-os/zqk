package storage

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
	// FileTransactionCoordinator provides transactional safety for multi-file operations
	// Uses a two-phase locking protocol to ensure atomicity across multiple file edits
	// Similar to graph database transactions, but for file-based storage
)

type FileTransactionCoordinator struct {
	projectRoot      string
	lockStrategy     FileLockStrategy
	transactionLocks map[string]*transactionLock // Map of file path -> lock handle
	mu               sync.Mutex
}

// transactionLock represents a lock held for a transaction
type transactionLock struct {
	handle     LockHandle
	filePath   string
	acquiredAt time.Time
}

// NewFileTransactionCoordinator creates a new transaction coordinator
func NewFileTransactionCoordinator(projectRoot string, lockStrategy FileLockStrategy) *FileTransactionCoordinator {
	if lockStrategy == nil {
		lockStrategy = NewAutoCleanupStrategy()
	}
	return &FileTransactionCoordinator{
		projectRoot:      projectRoot,
		lockStrategy:     lockStrategy,
		transactionLocks: make(map[string]*transactionLock),
	}
}

// AcquireLocks acquires locks for all files in the transaction
// Uses ordered locking to prevent deadlocks (locks files in sorted path order)
func (c *FileTransactionCoordinator) AcquireLocks(ctx context.Context, filePaths []string, timeout time.Duration) error {
	// Sort file paths to ensure consistent lock ordering (prevents deadlocks)
	sortedPaths := make([]string, len(filePaths))
	copy(sortedPaths, filePaths)
	sort.Strings(sortedPaths)

	// Track which locks we've acquired (for rollback on failure)
	acquiredLocks := make([]string, 0, len(sortedPaths))

	// Acquire locks in order (release coordinator lock before I/O)
	for _, filePath := range sortedPaths {
		// Check if already locked (quick check with lock)
		var alreadyLocked bool
		_ = concurrency.RunInLockOrLog(
			&c.mu, locknames.LockNameFileTransactionCheckLock, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				_, alreadyLocked = c.transactionLocks[filePath]
				return nil
			},
		)
		if alreadyLocked {
			continue
		}

		// Create lock file path
		lockPath := filePath + ".txn.lock"

		// Acquire lock using strategy (NO COORDINATOR LOCK HELD - prevents deadlock)
		handle, err := c.lockStrategy.AcquireLock(lockPath, timeout)
		if err != nil {
			// Release all acquired locks on failure under coordinator lock (BLI-CEF-CON-001)
			_ = concurrency.RunInLockOrLog(
				&c.mu, locknames.LockNameFileTransactionReleaseLocks, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					c.releaseLocks(acquiredLocks)
					return nil
				},
			)
			return errfmt.Errorf(ConstMiscFailedToAcquireLockForSW, filePath, err)
		}

		// Store lock handle (re-acquire coordinator lock)
		_ = concurrency.RunInLockOrLog(
			&c.mu, locknames.LockNameFileTransactionStoreLock, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				c.transactionLocks[filePath] = &transactionLock{
					handle:     handle,
					filePath:   filePath,
					acquiredAt: time.Now(),
				}
				return nil
			},
		)
		acquiredLocks = append(acquiredLocks, filePath)
	}

	return nil
}

// ReleaseLocks releases all locks held by the transaction
func (c *FileTransactionCoordinator) ReleaseLocks(ctx context.Context) error {
	var err error
	_ = concurrency.RunInLockOrLog(
		&c.mu, locknames.LockNameFileTransactionReleaseLocks, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			err = c.releaseAllLocks()
			return nil
		},
	)
	return err
}

// releaseAllLocks releases all locks (internal, assumes mutex is held)
func (c *FileTransactionCoordinator) releaseAllLocks() error {
	var errors []error
	for filePath, txnLock := range c.transactionLocks {
		if err := txnLock.handle.Release(); err != nil {
			errors = append(errors, errfmt.Errorf(ConstMiscFailedToReleaseLockForSW, filePath, err))
		}
		delete(c.transactionLocks, filePath)
	}

	if len(errors) > 0 {
		return errfmt.Errorf(ConstMiscErrorsReleasingLocksV, errors)
	}

	return nil
}

// releaseLocks releases specific locks (internal, assumes mutex is held)
func (c *FileTransactionCoordinator) releaseLocks(filePaths []string) {
	for _, filePath := range filePaths {
		if txnLock, exists := c.transactionLocks[filePath]; exists {
			var _err_82728409 = txnLock.handle.Release()
			if //nolint:errcheck // Best effort cleanup
			_err_82728409 != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_82728409).Log()
			}
			delete(c.transactionLocks, filePath)
		}
	}
}

// IsLocked returns whether a file is currently locked by this transaction
func (c *FileTransactionCoordinator) IsLocked(filePath string) bool {
	var isLocked bool
	_ = concurrency.RunInLockOrLog(
		&c.mu, locknames.LockNameFileTransactionIsLocked, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			txnLock, exists := c.transactionLocks[filePath]
			isLocked = exists && txnLock.handle.IsLocked()
			return nil
		},
	)
	return isLocked
}

// GetLockedFiles returns all files currently locked by this transaction
func (c *FileTransactionCoordinator) GetLockedFiles() []string {
	var files []string
	_ = concurrency.RunInLockOrLog(
		&c.mu, locknames.LockNameFileTransactionGetLockedFiles, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			files = make([]string, 0, len(c.transactionLocks))
			for filePath := range c.transactionLocks {
				files = append(files, filePath)
			}
			return nil
		},
	)
	return files
}

// FileSystemAdapter provides a micro-database interface for file-based storage
// Ensures transactional safety across multiple file operations
type FileSystemAdapter struct {
	coordinator *FileTransactionCoordinator
	storage     *FileObjectStorage
}

// NewFileSystemAdapter creates a new file system adapter with transaction support
func NewFileSystemAdapter(projectRoot string, storage *FileObjectStorage) *FileSystemAdapter {
	lockStrategy := NewAutoCleanupStrategy()
	coordinator := NewFileTransactionCoordinator(projectRoot, lockStrategy)
	return &FileSystemAdapter{
		coordinator: coordinator,
		storage:     storage,
	}
}

// Transaction represents a transactional context for multi-file operations
type Transaction struct {
	adapter    *FileSystemAdapter
	ctx        context.Context
	filePaths  []string // Files that will be modified
	operations []transactionOp
	committed  bool
	rolledBack bool
	mu         sync.Mutex
}

type transactionOp struct {
	opType  string // OpCreate, OpUpdate, or OpDelete
	id      string
	kind    string
	obj     map[string]any
	updates map[string]any
}

// BeginTransaction starts a new transaction
func (a *FileSystemAdapter) BeginTransaction(ctx context.Context) *Transaction {
	return &Transaction{
		adapter:    a,
		ctx:        ctx,
		filePaths:  make([]string, 0),
		operations: make([]transactionOp, 0),
	}
}

// AddFile adds a file to the transaction's lock set
func (tx *Transaction) AddFile(filePath string) {
	_ = concurrency.RunInLockOrLog(
		&tx.mu, locknames.LockNameTransactionAddFile, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Check if already added
			for _, existing := range tx.filePaths {
				if existing == filePath {
					return nil
				}
			}
			tx.filePaths = append(tx.filePaths, filePath)
			return nil
		},
	)
}

// Create adds a create operation to the transaction
func (tx *Transaction) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	var err error
	_ = concurrency.RunInLockOrLog(
		&tx.mu, locknames.LockNameTransactionCreate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if tx.committed || tx.rolledBack {
				err = errfmt.Errorf(ConstMiscTransactionAlreadyCommittedOrRolledBack)
				return err
			}

			kind, _ := obj[objects.FieldKeyKind].(string)
			id, _ := obj[objects.FieldKeyID].(string)
			if kind == emptyValue || id == emptyValue {
				err = errfmt.Errorf(ConstMiscObjectMustHaveKindAndId)
				return err
			}

			// Determine file path for this object
			filePath, pathErr := tx.adapter.getObjectFilePathForTransaction(kind, id)
			if pathErr != nil {
				err = errfmt.Newf(ConstMiscFailedToDetermineFilePath).Wrap(pathErr)
				return err
			}
			// AddFile will acquire its own lock
			tx.filePaths = append(tx.filePaths, filePath)

			tx.operations = append(tx.operations, transactionOp{
				opType: OpCreate,
				id:     id,
				kind:   kind,
				obj:    obj,
			})
			return nil
		},
	)
	return err
}

// Update adds an update operation to the transaction
func (tx *Transaction) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	if err := tx.checkActive(locknames.LockNameTransactionUpdate); err != nil {
		return err
	}

	kind, filePath, err := tx.resolveTargetKindAndPath(ctx, secCtx, id, ConstMiscFailedToReadObjectForUpdate)
	if err != nil {
		return err
	}

	// Re-acquire lock to add file and operation
	return concurrency.RunInLockOrLog(
		&tx.mu, locknames.LockNameTransactionUpdateAdd, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if tx.committed || tx.rolledBack {
				return errfmt.Errorf(ConstMiscTransactionAlreadyCommittedOrRolledBack)
			}
			tx.filePaths = append(tx.filePaths, filePath)
			tx.operations = append(tx.operations, transactionOp{
				opType:  OpUpdate,
				id:      id,
				kind:    kind,
				updates: updates,
			})
			return nil
		},
	)
}

// Delete adds a delete operation to the transaction
func (tx *Transaction) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) error {
	if err := tx.checkActive(locknames.LockNameTransactionDeleteCheck); err != nil {
		return err
	}

	kind, filePath, err := tx.resolveTargetKindAndPath(ctx, secCtx, id, ConstMiscFailedToReadObjectForDelete)
	if err != nil {
		return err
	}

	// Re-acquire lock to add file and operation
	return concurrency.RunInLockOrLog(
		&tx.mu, locknames.LockNameTransactionDeleteAdd, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if tx.committed || tx.rolledBack {
				return errfmt.Errorf(ConstMiscTransactionAlreadyCommittedOrRolledBack)
			}
			tx.filePaths = append(tx.filePaths, filePath)
			tx.operations = append(tx.operations, transactionOp{
				opType: OpDelete,
				id:     id,
				kind:   kind,
			})
			return nil
		},
	)
}

func (tx *Transaction) checkActive(lockName string) error {
	var err error
	_ = concurrency.RunInLockOrLog(
		&tx.mu, lockName, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if tx.committed || tx.rolledBack {
				err = errfmt.Errorf(ConstMiscTransactionAlreadyCommittedOrRolledBack)
				return err
			}
			return nil
		},
	)
	return err
}

func (tx *Transaction) resolveTargetKindAndPath(ctx context.Context, secCtx *pkgctx.SecurityContext, id, readErrMsg string) (string, string, error) {
	obj, readErr := tx.adapter.storage.Read(ctx, secCtx, id)
	if readErr != nil {
		return "", "", errfmt.Newf(readErrMsg).Wrap(readErr)
	}

	kind, _ := obj[objects.FieldKeyKind].(string)
	filePath, pathErr := tx.adapter.getObjectFilePathForTransaction(kind, id)
	if pathErr != nil {
		return "", "", errfmt.Newf(ConstMiscFailedToDetermineFilePath).Wrap(pathErr)
	}
	return kind, filePath, nil
}

// Commit commits the transaction atomically
// Uses two-phase commit: acquire locks, apply operations, release locks
func (a *FileSystemAdapter) getObjectFilePathForTransaction(kind, id string) (string, error) {
	// Use the storage's existing method - signature is getObjectFilePath(id, kind)
	// This handles CAS, bucketing, and all path resolution logic
	filePath, err := a.storage.getObjectFilePath(id, kind)
	if err != nil {
		return "", errfmt.Errorf("failed to determine file path for %s/%s: %w", kind, id, err)
	}
	return filePath, nil
}
