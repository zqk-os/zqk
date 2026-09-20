package storage

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/validation"
)

// RoutingObjectStorage delegates all operations to the appropriate provider returned by the factory.
// It implements the ObjectStorageProvider interface.
type RoutingObjectStorage struct {
	factory     *StorageFactory
	idValidator *validation.IDValidator
}

// NewRoutingObjectStorage creates a new RoutingObjectStorage
func NewRoutingObjectStorage(factory *StorageFactory) *RoutingObjectStorage {
	return &RoutingObjectStorage{
		factory:     factory,
		idValidator: validation.GetIDValidator(),
	}
}

// GetStorageFactory returns the underlying storage factory
func (r *RoutingObjectStorage) GetStorageFactory() *StorageFactory {
	return r.factory
}

// GetPool returns the connection pool if the underlying default storage provider supports it.
func (r *RoutingObjectStorage) GetPool() provider.ConnectionPool {
	if p, ok := r.factory.defaultStorage.(interface {
		GetPool() provider.ConnectionPool
	}); ok {
		return p.GetPool()
	}
	return nil
}

// Create creates a new object
func (r *RoutingObjectStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	return r.factory.GetStorageForObject(obj).Create(ctx, secCtx, obj)
}

// Read retrieves an object by ID
func (r *RoutingObjectStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	kind := r.idValidator.InferKindFromID(id)
	return r.factory.GetStorageForKind(kind).Read(ctx, secCtx, id)
}

// Update updates an existing object
func (r *RoutingObjectStorage) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	// First try to get kind from updates map
	kind, _ := updates[objects.FieldKeyKind].(string)
	if kind == "" {
		// If not in updates, infer from ID
		kind = r.idValidator.InferKindFromID(id)
	}

	if kind != "" {
		return r.factory.GetStorageForKind(kind).Update(ctx, secCtx, id, updates)
	}

	// Fallback to default storage for object
	return r.factory.GetStorageForObject(updates).Update(ctx, secCtx, id, updates)
}

// Delete deletes an object
func (r *RoutingObjectStorage) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	kind := r.idValidator.InferKindFromID(id)
	return r.factory.GetStorageForKind(kind).Delete(ctx, secCtx, id, cascade)
}

// List lists objects with filtering, sorting, pagination, and grouping
func (r *RoutingObjectStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
	return r.factory.GetStorageForKind(filter.Kind).List(ctx, secCtx, storageCtx, filter)
}

// Query executes a custom query
func (r *RoutingObjectStorage) Query(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query Query) (*QueryResult, error) {
	return r.factory.GetStorageForKind(query.Kind).Query(ctx, secCtx, storageCtx, query)
}

// Search performs full-text search across objects
func (r *RoutingObjectStorage) Search(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query SearchQuery) (*SearchResult, error) {
	// Search can be cross-kind, so we use the default storage
	return r.factory.GetStorage().Search(ctx, secCtx, storageCtx, query)
}

// BeginTransaction starts a transaction for atomic multi-object operations
func (r *RoutingObjectStorage) BeginTransaction(ctx context.Context) (ObjectTransaction, error) {
	// Transaction context is not kind-specific, so we use the default storage
	return r.factory.GetStorage().BeginTransaction(ctx)
}

// BulkCreate creates multiple objects atomically
func (r *RoutingObjectStorage) BulkCreate(ctx context.Context, secCtx *pkgctx.SecurityContext, objects []map[string]any) (*BulkResult, error) {
	if len(objects) == 0 {
		return &BulkResult{}, nil
	}
	return r.factory.GetStorageForObject(objects[0]).BulkCreate(ctx, secCtx, objects)
}

// BulkUpdate updates multiple objects atomically
func (r *RoutingObjectStorage) BulkUpdate(ctx context.Context, secCtx *pkgctx.SecurityContext, updates []BulkUpdateItem) (*BulkResult, error) {
	if len(updates) == 0 {
		return &BulkResult{}, nil
	}
	return r.factory.GetStorageForObject(updates[0].Updates).BulkUpdate(ctx, secCtx, updates)
}

// BulkGet retrieves multiple objects by ID
func (r *RoutingObjectStorage) BulkGet(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string) (*BulkResult, error) {
	if len(ids) == 0 {
		return &BulkResult{}, nil
	}
	kind := r.idValidator.InferKindFromID(ids[0])
	return r.factory.GetStorageForKind(kind).BulkGet(ctx, secCtx, ids)
}

// BulkDelete deletes multiple objects atomically
func (r *RoutingObjectStorage) BulkDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, cascade bool) (*BulkResult, error) {
	if len(ids) == 0 {
		return &BulkResult{}, nil
	}
	kind := r.idValidator.InferKindFromID(ids[0])
	return r.factory.GetStorageForKind(kind).BulkDelete(ctx, secCtx, ids, cascade)
}

// Exists checks if an object exists by ID
func (r *RoutingObjectStorage) Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error) {
	kind := r.idValidator.InferKindFromID(id)
	return r.factory.GetStorageForKind(kind).Exists(ctx, secCtx, id)
}

// Count counts objects matching the filter
func (r *RoutingObjectStorage) Count(ctx context.Context, secCtx *pkgctx.SecurityContext, filter ListFilter) (int, error) {
	return r.factory.GetStorageForKind(filter.Kind).Count(ctx, secCtx, filter)
}

// Aggregate performs aggregations on objects matching the filter
func (r *RoutingObjectStorage) Aggregate(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter, aggregations []Aggregation) (*AggregateResult, error) {
	return r.factory.GetStorageForKind(filter.Kind).Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
}

// GetRelated finds objects related to the given object via reference fields
func (r *RoutingObjectStorage) GetRelated(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, relationshipType string, depth int) ([]map[string]any, error) {
	kind := r.idValidator.InferKindFromID(id)
	return r.factory.GetStorageForKind(kind).GetRelated(ctx, secCtx, id, relationshipType, depth)
}

// GetPath finds a path between two objects via reference relationships
func (r *RoutingObjectStorage) GetPath(ctx context.Context, secCtx *pkgctx.SecurityContext, fromID, toID string) ([]map[string]any, error) {
	// Route based on fromID
	kind := r.idValidator.InferKindFromID(fromID)
	return r.factory.GetStorageForKind(kind).GetPath(ctx, secCtx, fromID, toID)
}

// GetNeighbors finds immediate neighbors of an object
func (r *RoutingObjectStorage) GetNeighbors(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, direction string) ([]map[string]any, error) {
	kind := r.idValidator.InferKindFromID(id)
	return r.factory.GetStorageForKind(kind).GetNeighbors(ctx, secCtx, id, direction)
}

// Move moves an object to a different directory/kind
func (r *RoutingObjectStorage) Move(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, newKind string, updateReferences bool) error {
	kind := r.idValidator.InferKindFromID(id)
	return r.factory.GetStorageForKind(kind).Move(ctx, secCtx, id, newKind, updateReferences)
}

// Rename changes an object's ID
func (r *RoutingObjectStorage) Rename(ctx context.Context, secCtx *pkgctx.SecurityContext, oldID, newID string, updateReferences bool) error {
	kind := r.idValidator.InferKindFromID(oldID)
	return r.factory.GetStorageForKind(kind).Rename(ctx, secCtx, oldID, newID, updateReferences)
}

// Ensure RoutingObjectStorage implements ObjectStorageProvider
var _ ObjectStorageProvider = (*RoutingObjectStorage)(nil)

func (r *RoutingObjectStorage) Shutdown(ctx context.Context) error {
	return r.factory.Shutdown(ctx)
}
