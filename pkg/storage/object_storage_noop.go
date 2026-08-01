package storage

import (
	"context"
	"errors"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

// ErrNoopObjectStorage is returned by [NoopObjectStorage] methods other than Create.
var ErrNoopObjectStorage = errors.New("noop object storage: not implemented")

// NoopObjectStorage implements [ObjectStorageProvider] with Create succeeding and all other methods
// returning [ErrNoopObjectStorage]. Use in tests that only need a non-nil provider (e.g. constructing
// components that hold a reference but do not execute CRUD paths).
type NoopObjectStorage struct{}

// NewNoopObjectStorage returns an [ObjectStorageProvider] backed by [NoopObjectStorage].
func NewNoopObjectStorage() ObjectStorageProvider {
	return NoopObjectStorage{}
}

func (NoopObjectStorage) Create(context.Context, *pkgctx.SecurityContext, map[string]any) error {
	return nil
}

func (NoopObjectStorage) Read(context.Context, *pkgctx.SecurityContext, string) (map[string]any, error) {
	return nil, ErrNoopObjectStorage
}

func (NoopObjectStorage) Update(context.Context, *pkgctx.SecurityContext, string, map[string]any) error {
	return ErrNoopObjectStorage
}

func (NoopObjectStorage) Delete(context.Context, *pkgctx.SecurityContext, string, bool) error {
	return ErrNoopObjectStorage
}

//nolint:gocritic // Interface requires value semantics for ListFilter
func (NoopObjectStorage) List(context.Context, *pkgctx.SecurityContext, *pkgctx.StorageContext, ListFilter) (*QueryResult, error) {
	return nil, ErrNoopObjectStorage
}

func (NoopObjectStorage) Query(context.Context, *pkgctx.SecurityContext, *pkgctx.StorageContext, Query) (*QueryResult, error) {
	return nil, ErrNoopObjectStorage
}

//nolint:gocritic // Interface requires value semantics for SearchQuery
func (NoopObjectStorage) Search(context.Context, *pkgctx.SecurityContext, *pkgctx.StorageContext, SearchQuery) (*SearchResult, error) {
	return nil, ErrNoopObjectStorage
}

func (NoopObjectStorage) BeginTransaction(context.Context) (ObjectTransaction, error) {
	return nil, ErrNoopObjectStorage
}

func (NoopObjectStorage) BulkCreate(context.Context, *pkgctx.SecurityContext, []map[string]any) (*BulkResult, error) {
	return nil, ErrNoopObjectStorage
}

func (NoopObjectStorage) BulkUpdate(context.Context, *pkgctx.SecurityContext, []BulkUpdateItem) (*BulkResult, error) {
	return nil, ErrNoopObjectStorage
}

func (NoopObjectStorage) BulkGet(context.Context, *pkgctx.SecurityContext, []string) (*BulkResult, error) {
	return nil, ErrNoopObjectStorage
}

func (NoopObjectStorage) BulkDelete(context.Context, *pkgctx.SecurityContext, []string, bool) (*BulkResult, error) {
	return nil, ErrNoopObjectStorage
}

func (NoopObjectStorage) Exists(context.Context, *pkgctx.SecurityContext, string) (bool, error) {
	return false, ErrNoopObjectStorage
}

//nolint:gocritic // Interface requires value semantics for ListFilter
func (NoopObjectStorage) Count(context.Context, *pkgctx.SecurityContext, ListFilter) (int, error) {
	return 0, ErrNoopObjectStorage
}

//nolint:gocritic // Interface requires value semantics for ListFilter
func (NoopObjectStorage) Aggregate(context.Context, *pkgctx.SecurityContext, *pkgctx.StorageContext, ListFilter, []Aggregation) (*AggregateResult, error) {
	return nil, ErrNoopObjectStorage
}

func (NoopObjectStorage) GetRelated(context.Context, *pkgctx.SecurityContext, string, string, int) ([]map[string]any, error) {
	return nil, ErrNoopObjectStorage
}

func (NoopObjectStorage) GetPath(context.Context, *pkgctx.SecurityContext, string, string) ([]map[string]any, error) {
	return nil, ErrNoopObjectStorage
}

func (NoopObjectStorage) GetNeighbors(context.Context, *pkgctx.SecurityContext, string, string) ([]map[string]any, error) {
	return nil, ErrNoopObjectStorage
}

func (NoopObjectStorage) Move(context.Context, *pkgctx.SecurityContext, string, string, bool) error {
	return ErrNoopObjectStorage
}

func (NoopObjectStorage) Rename(context.Context, *pkgctx.SecurityContext, string, string, bool) error {
	return ErrNoopObjectStorage
}

var _ ObjectStorageProvider = NoopObjectStorage{}

func (n NoopObjectStorage) Shutdown(ctx context.Context) error { return nil }
