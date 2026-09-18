package storage

import (
	"context"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
)

// ProxyStorage wraps an ObjectStorageProvider to intercept operations during snapshot
type ProxyStorage struct {
	underlying     ObjectStorageProvider
	queue          *SnapshotOperationQueue
	snapshotActive bool
	mu             sync.RWMutex
	logger         logging.Logger
}

// NewProxyStorage creates a new proxy storage wrapper
func NewProxyStorage(underlying ObjectStorageProvider, queue *SnapshotOperationQueue) *ProxyStorage {
	return &ProxyStorage{
		underlying: underlying,
		queue:      queue,
		logger:     logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// BeginSnapshot activates the proxy (operations will be queued)
func (p *ProxyStorage) BeginSnapshot() {
	_ = concurrency.RunInLockOrLog(
		&p.mu, locknames.LockNameSnapshotProxyBegin, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			p.snapshotActive = true
			StorageLog(p.logger).Info(LogEventStorageSnapshotProxyActivatedInfo).Log()
			return nil
		},
	)
}

// EndSnapshot deactivates the proxy (operations will execute normally)
func (p *ProxyStorage) EndSnapshot() {
	_ = concurrency.RunInLockOrLog(
		&p.mu, locknames.LockNameSnapshotProxyEnd, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			p.snapshotActive = false
			StorageLog(p.logger).Info(LogEventStorageSnapshotProxyDeactivatedInfo).Log()
			return nil
		},
	)
}

// IsSnapshotActive returns whether snapshot is currently active
func (p *ProxyStorage) IsSnapshotActive() bool {
	var active bool
	_ = concurrency.RunInRLockOrLog(
		&p.mu, locknames.LockNameSnapshotProxyIsActive, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			active = p.snapshotActive
			return nil
		},
	)
	return active
}

// Create creates a new object (proxied during snapshot)
func (p *ProxyStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	var snapshotActive bool
	_ = concurrency.RunInRLockOrLog(
		&p.mu, locknames.LockNameSnapshotProxyCreateCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			snapshotActive = p.snapshotActive
			return nil
		},
	)

	if snapshotActive {
		// Queue operation instead of executing
		objectID, _ := obj[objects.FieldKeyID].(string)
		kind, _ := obj[objects.FieldKeyKind].(string)

		op := &SnapshotOperation{
			Type:      SnapshotOpCreate,
			ObjectID:  objectID,
			Kind:      kind,
			Data:      obj,
			SecCtx:    secCtx,
			Timestamp: time.Now().UTC(),
		}

		if err := p.queue.Enqueue(op); err != nil {
			return errfmt.Newf(ConstMiscFailedToQueueCreateOperation).Wrap(err)
		}

		// Return success immediately (operation appears to succeed)
		return nil
	}

	// Normal execution
	return p.underlying.Create(ctx, secCtx, obj)
}

// Read retrieves an object (proxied if object has pending writes)
func (p *ProxyStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	var snapshotActive bool
	_ = concurrency.RunInRLockOrLog(
		&p.mu, locknames.LockNameSnapshotProxyReadCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			snapshotActive = p.snapshotActive
			return nil
		},
	)

	if snapshotActive {
		// Check if object has pending writes
		if p.queue.HasPendingWrite(id) {
			// Return data from most recent queued write for consistency
			data, err := p.queue.GetDataFromPendingWrite(id)
			if err == nil {
				StorageLog(p.logger).Debug(LogEventStorageSnapshotProxyReadPendingWriteDebug).
					ObjectID(id).
					Log()
				return data, nil
			}
			// If error, fall through to normal read
		}
	}

	// Normal execution (no pending writes or snapshot not active)
	return p.underlying.Read(ctx, secCtx, id)
}

// Update updates an existing object (proxied during snapshot)
func (p *ProxyStorage) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	var snapshotActive bool
	_ = concurrency.RunInRLockOrLog(
		&p.mu, locknames.LockNameSnapshotProxyUpdateCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			snapshotActive = p.snapshotActive
			return nil
		},
	)

	if snapshotActive {
		// Queue operation instead of executing
		op := &SnapshotOperation{
			Type:      SnapshotOpUpdate,
			ObjectID:  id,
			Data:      updates,
			SecCtx:    secCtx,
			Timestamp: time.Now().UTC(),
		}

		if err := p.queue.Enqueue(op); err != nil {
			return errfmt.Newf(ConstMiscFailedToQueueUpdateOperation).Wrap(err)
		}

		// Return success immediately
		return nil
	}

	// Normal execution
	return p.underlying.Update(ctx, secCtx, id, updates)
}

// Delete deletes an object (proxied during snapshot)
func (p *ProxyStorage) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	var snapshotActive bool
	_ = concurrency.RunInRLockOrLog(
		&p.mu, locknames.LockNameSnapshotProxyDeleteCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			snapshotActive = p.snapshotActive
			return nil
		},
	)

	if snapshotActive {
		// Queue operation instead of executing
		op := &SnapshotOperation{
			Type:      SnapshotOpDelete,
			ObjectID:  id,
			Cascade:   cascade,
			SecCtx:    secCtx,
			Timestamp: time.Now().UTC(),
		}

		if err := p.queue.Enqueue(op); err != nil {
			return errfmt.Newf(ConstMiscFailedToQueueDeleteOperation).Wrap(err)
		}

		// Return success immediately
		return nil
	}

	// Normal execution
	return p.underlying.Delete(ctx, secCtx, id, cascade)
}

// List lists objects (not proxied - always executes normally)
//
//nolint:gocritic // Interface requires value semantics for ListFilter
func (p *ProxyStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
	// List operations are not proxied - always execute normally
	return p.underlying.List(ctx, secCtx, storageCtx, filter)
}

// Query executes a custom query (not proxied - always executes normally)
func (p *ProxyStorage) Query(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query Query) (*QueryResult, error) {
	// Query operations are not proxied - always execute normally
	return p.underlying.Query(ctx, secCtx, storageCtx, query)
}

// Search performs full-text search (not proxied - always executes normally)
//
//nolint:gocritic // Interface requires value semantics for SearchQuery
func (p *ProxyStorage) Search(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query SearchQuery) (*SearchResult, error) {
	// Search operations are not proxied - always execute normally
	return p.underlying.Search(ctx, secCtx, storageCtx, query)
}

// BeginTransaction starts a transaction (not proxied - always executes normally)
func (p *ProxyStorage) BeginTransaction(ctx context.Context) (ObjectTransaction, error) {
	// Transactions are not proxied - always execute normally
	return p.underlying.BeginTransaction(ctx)
}

// BulkCreate creates multiple objects (proxied during snapshot)
func (p *ProxyStorage) BulkCreate(ctx context.Context, secCtx *pkgctx.SecurityContext, objs []map[string]any) (*BulkResult, error) {
	var snapshotActive bool
	_ = concurrency.RunInRLockOrLog(
		&p.mu, locknames.LockNameSnapshotProxyBulkCreateCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			snapshotActive = p.snapshotActive
			return nil
		},
	)

	if snapshotActive {
		// Queue each object creation individually
		for _, obj := range objs {
			objectID, _ := obj[objects.FieldKeyID].(string)
			kind, _ := obj[objects.FieldKeyKind].(string)

			op := &SnapshotOperation{
				Type:      SnapshotOpCreate,
				ObjectID:  objectID,
				Kind:      kind,
				Data:      obj,
				SecCtx:    secCtx,
				Timestamp: time.Now().UTC(),
			}

			if err := p.queue.Enqueue(op); err != nil {
				return nil, errfmt.Newf(ConstMiscFailedToQueueBulkCreateOperation).Wrap(err)
			}
		}

		// Return success for all operations
		return &BulkResult{
			SuccessCount: len(objs),
			FailureCount: 0,
		}, nil
	}

	// Normal execution
	return p.underlying.BulkCreate(ctx, secCtx, objs)
}

// BulkUpdate updates multiple objects (proxied during snapshot)
func (p *ProxyStorage) BulkUpdate(ctx context.Context, secCtx *pkgctx.SecurityContext, updates []BulkUpdateItem) (*BulkResult, error) {
	var snapshotActive bool
	_ = concurrency.RunInRLockOrLog(
		&p.mu, locknames.LockNameSnapshotProxyBulkUpdateCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			snapshotActive = p.snapshotActive
			return nil
		},
	)

	if snapshotActive {
		// Queue each update individually
		for _, update := range updates {
			op := &SnapshotOperation{
				Type:      SnapshotOpUpdate,
				ObjectID:  update.ID,
				Data:      update.Updates,
				SecCtx:    secCtx,
				Timestamp: time.Now().UTC(),
			}

			if err := p.queue.Enqueue(op); err != nil {
				return nil, errfmt.Newf(ConstMiscFailedToQueueBulkUpdateOperation).Wrap(err)
			}
		}

		// Return success for all operations
		return &BulkResult{
			SuccessCount: len(updates),
			FailureCount: 0,
		}, nil
	}

	// Normal execution
	return p.underlying.BulkUpdate(ctx, secCtx, updates)
}

// BulkGet retrieves multiple objects (not proxied - always executes normally)
func (p *ProxyStorage) BulkGet(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string) (*BulkResult, error) {
	// BulkGet is not proxied - always execute normally
	return p.underlying.BulkGet(ctx, secCtx, ids)
}

// BulkDelete deletes multiple objects (proxied during snapshot)
func (p *ProxyStorage) BulkDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, cascade bool) (*BulkResult, error) {
	var snapshotActive bool
	_ = concurrency.RunInRLockOrLog(
		&p.mu, locknames.LockNameSnapshotProxyBulkDeleteCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			snapshotActive = p.snapshotActive
			return nil
		},
	)

	if snapshotActive {
		// Queue each delete individually
		for _, id := range ids {
			op := &SnapshotOperation{
				Type:      SnapshotOpDelete,
				ObjectID:  id,
				Cascade:   cascade,
				SecCtx:    secCtx,
				Timestamp: time.Now().UTC(),
			}

			if err := p.queue.Enqueue(op); err != nil {
				return nil, errfmt.Newf(ConstMiscFailedToQueueBulkDeleteOperation).Wrap(err)
			}
		}

		// Return success for all operations
		return &BulkResult{
			SuccessCount: len(ids),
			FailureCount: 0,
		}, nil
	}

	// Normal execution
	return p.underlying.BulkDelete(ctx, secCtx, ids, cascade)
}

// GetRelated finds related objects (not proxied - always executes normally)
func (p *ProxyStorage) GetRelated(ctx context.Context, secCtx *pkgctx.SecurityContext, id, relationshipType string, depth int) ([]map[string]any, error) {
	// GetRelated is not proxied - always execute normally
	return p.underlying.GetRelated(ctx, secCtx, id, relationshipType, depth)
}

// GetPath finds a path between objects (not proxied - always executes normally)
func (p *ProxyStorage) GetPath(ctx context.Context, secCtx *pkgctx.SecurityContext, fromID, toID string) ([]map[string]any, error) {
	// GetPath is not proxied - always execute normally
	return p.underlying.GetPath(ctx, secCtx, fromID, toID)
}

// GetNeighbors finds immediate neighbors (not proxied - always executes normally)
func (p *ProxyStorage) GetNeighbors(ctx context.Context, secCtx *pkgctx.SecurityContext, id, direction string) ([]map[string]any, error) {
	// GetNeighbors is not proxied - always execute normally
	return p.underlying.GetNeighbors(ctx, secCtx, id, direction)
}

// Move moves an object (proxied during snapshot)
func (p *ProxyStorage) Move(ctx context.Context, secCtx *pkgctx.SecurityContext, id, newKind string, updateReferences bool) error {
	var snapshotActive bool
	_ = concurrency.RunInRLockOrLog(
		&p.mu, locknames.LockNameSnapshotProxyMoveCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			snapshotActive = p.snapshotActive
			return nil
		},
	)

	if snapshotActive {
		// Move is treated as update + create, so we'll queue it
		// For now, treat as update operation
		op := &SnapshotOperation{
			Type:      SnapshotOpUpdate,
			ObjectID:  id,
			Data:      map[string]any{objects.FieldKeyKind: newKind},
			SecCtx:    secCtx,
			Timestamp: time.Now().UTC(),
		}

		if err := p.queue.Enqueue(op); err != nil {
			return errfmt.Newf(ConstMiscFailedToQueueMoveOperation).Wrap(err)
		}

		return nil
	}

	// Normal execution
	return p.underlying.Move(ctx, secCtx, id, newKind, updateReferences)
}

// Rename changes an object's ID (proxied during snapshot; otherwise delegates to underlying).
func (p *ProxyStorage) Rename(ctx context.Context, secCtx *pkgctx.SecurityContext, oldID, newID string, updateReferences bool) error {
	var snapshotActive bool
	_ = concurrency.RunInRLockOrLog(
		&p.mu, locknames.LockNameSnapshotProxyRenameCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			snapshotActive = p.snapshotActive
			return nil
		},
	)
	if snapshotActive {
		op := &SnapshotOperation{
			Type:      SnapshotOpUpdate,
			ObjectID:  oldID,
			Data:      map[string]any{objects.FieldKeyID: newID},
			SecCtx:    secCtx,
			Timestamp: time.Now().UTC(),
		}
		if err := p.queue.Enqueue(op); err != nil {
			return errfmt.Newf(ConstMiscFailedToQueueRenameOperation).Wrap(err)
		}
		return nil
	}
	return p.underlying.Rename(ctx, secCtx, oldID, newID, updateReferences)
}

// ReplayQueuedOperations replays all queued operations to the underlying storage
func (p *ProxyStorage) ReplayQueuedOperations(ctx context.Context) error {
	return p.queue.Replay(ctx, p.underlying)
}

// GetQueueSize returns the current queue size
func (p *ProxyStorage) GetQueueSize() int {
	return p.queue.Size()
}
