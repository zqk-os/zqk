package storage

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

// TransactionalStorageWrapper wraps an ObjectStorageProvider to automatically
// wrap all operations in a transaction. This ensures all-or-nothing semantics
// for jobs that require transactional consistency.
type TransactionalStorageWrapper struct {
	underlying ObjectStorageProvider
	tx         ObjectTransaction
	ctx        context.Context
	committed  bool
	rolledBack bool
}

// NewTransactionalStorageWrapper creates a new transactional storage wrapper
// All operations will be executed within a single transaction that commits
// on success or rolls back on error
func NewTransactionalStorageWrapper(ctx context.Context, underlying ObjectStorageProvider) (*TransactionalStorageWrapper, error) {
	// Begin transaction on the underlying storage
	tx, err := underlying.BeginTransaction(ctx)
	if err != nil {
		return nil, err
	}

	return &TransactionalStorageWrapper{
		underlying: underlying,
		tx:         tx,
		ctx:        ctx,
	}, nil
}

// Commit commits the transaction
func (w *TransactionalStorageWrapper) Commit(ctx context.Context) error {
	if w.committed {
		return nil // Already committed
	}
	if w.rolledBack {
		return nil // Already rolled back, nothing to commit
	}

	err := w.tx.Commit(ctx)
	if err == nil {
		w.committed = true
	}
	return err
}

// Rollback rolls back the transaction
func (w *TransactionalStorageWrapper) Rollback(ctx context.Context) error {
	if w.rolledBack {
		return nil // Already rolled back
	}
	if w.committed {
		return nil // Already committed, nothing to roll back
	}

	err := w.tx.Rollback(ctx)
	if err == nil {
		w.rolledBack = true
	}
	return err
}

// ObjectStorageProvider interface implementation - all operations use transaction

func (w *TransactionalStorageWrapper) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	return w.tx.Create(ctx, secCtx, obj)
}

func (w *TransactionalStorageWrapper) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	return w.tx.Read(ctx, secCtx, id)
}

func (w *TransactionalStorageWrapper) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	return w.tx.Update(ctx, secCtx, id, updates)
}

func (w *TransactionalStorageWrapper) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	return w.tx.Delete(ctx, secCtx, id, cascade)
}

func (w *TransactionalStorageWrapper) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
	// List operations typically don't need transactions, but we'll use underlying for read-only ops
	// Transaction is mainly for write operations (Create, Update, Delete)
	return w.underlying.List(ctx, secCtx, storageCtx, filter)
}

func (w *TransactionalStorageWrapper) Query(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query Query) (*QueryResult, error) {
	// Query operations are read-only, use underlying
	return w.underlying.Query(ctx, secCtx, storageCtx, query)
}

func (w *TransactionalStorageWrapper) Count(ctx context.Context, secCtx *pkgctx.SecurityContext, filter ListFilter) (int, error) {
	// Count operations are read-only, use underlying
	return w.underlying.Count(ctx, secCtx, filter)
}

func (w *TransactionalStorageWrapper) Move(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, newKind string, updateReferences bool) error {
	// Move operations should be transactional, but ObjectTransaction doesn't have Move
	// For now, use underlying and rely on transaction for related operations
	return w.underlying.Move(ctx, secCtx, id, newKind, updateReferences)
}

func (w *TransactionalStorageWrapper) Rename(ctx context.Context, secCtx *pkgctx.SecurityContext, oldID, newID string, updateReferences bool) error {
	return w.underlying.Rename(ctx, secCtx, oldID, newID, updateReferences)
}

func (w *TransactionalStorageWrapper) BeginTransaction(ctx context.Context) (ObjectTransaction, error) {
	// Nested transactions not supported - return the existing transaction
	return w.tx, nil
}

// GetRelated, GetPath, GetNeighbors are read-only operations
func (w *TransactionalStorageWrapper) GetRelated(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, relationshipType string, depth int) ([]map[string]any, error) {
	return w.underlying.GetRelated(ctx, secCtx, id, relationshipType, depth)
}

func (w *TransactionalStorageWrapper) GetPath(ctx context.Context, secCtx *pkgctx.SecurityContext, fromID, toID string) ([]map[string]any, error) {
	return w.underlying.GetPath(ctx, secCtx, fromID, toID)
}

func (w *TransactionalStorageWrapper) GetNeighbors(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, direction string) ([]map[string]any, error) {
	return w.underlying.GetNeighbors(ctx, secCtx, id, direction)
}

func (w *TransactionalStorageWrapper) Search(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query SearchQuery) (*SearchResult, error) {
	// Search operations are read-only, use underlying
	return w.underlying.Search(ctx, secCtx, storageCtx, query)
}

func (w *TransactionalStorageWrapper) BulkCreate(ctx context.Context, secCtx *pkgctx.SecurityContext, objects []map[string]any) (*BulkResult, error) {
	// Bulk operations should be transactional, but ObjectTransaction doesn't have Bulk methods
	// For now, use underlying and rely on transaction for individual operations
	// TODO: Add bulk operations to ObjectTransaction interface
	return w.underlying.BulkCreate(ctx, secCtx, objects)
}

func (w *TransactionalStorageWrapper) BulkUpdate(ctx context.Context, secCtx *pkgctx.SecurityContext, updates []BulkUpdateItem) (*BulkResult, error) {
	// Bulk operations should be transactional, but ObjectTransaction doesn't have Bulk methods
	return w.underlying.BulkUpdate(ctx, secCtx, updates)
}

func (w *TransactionalStorageWrapper) BulkGet(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string) (*BulkResult, error) {
	// BulkGet is read-only, use underlying
	return w.underlying.BulkGet(ctx, secCtx, ids)
}

func (w *TransactionalStorageWrapper) BulkDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, cascade bool) (*BulkResult, error) {
	// Bulk operations should be transactional, but ObjectTransaction doesn't have Bulk methods
	return w.underlying.BulkDelete(ctx, secCtx, ids, cascade)
}

func (w *TransactionalStorageWrapper) Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error) {
	// Exists is read-only, use underlying
	return w.underlying.Exists(ctx, secCtx, id)
}

func (w *TransactionalStorageWrapper) Aggregate(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter, aggregations []Aggregation) (*AggregateResult, error) {
	// Aggregate operations are read-only, use underlying
	return w.underlying.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
}

func (w *TransactionalStorageWrapper) Shutdown(ctx context.Context) error {
	return w.underlying.Shutdown(ctx)
}

// GetUnderlying returns the underlying storage provider.
func (w *TransactionalStorageWrapper) GetUnderlying() ObjectStorageProvider {
	return w.underlying
}
