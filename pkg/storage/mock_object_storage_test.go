package storage

import (
	"context"
	"fmt"
	"sync"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// mockObjectStorage is a minimal ObjectStorageProvider for internal package tests
// (e.g. unified metrics, file-lock metrics) and for storage_test via NewMockObjectStorageForMetricsTests.
type mockObjectStorage struct {
	objects map[string]map[string]any
	mu      sync.Mutex
}

func (m *mockObjectStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.objects == nil {
		m.objects = make(map[string]map[string]any)
	}

	id, ok := obj[objects.FieldKeyID].(string)
	if !ok {
		id = "FLM-001"
		obj[objects.FieldKeyID] = id
	}

	m.objects[id] = obj
	return nil
}

func (m *mockObjectStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorage) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	return fmt.Errorf("not implemented")
}

func (m *mockObjectStorage) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	return fmt.Errorf("not implemented")
}

//nolint:gocritic // Interface requires value semantics for ListFilter
func (m *mockObjectStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorage) Query(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query Query) (*QueryResult, error) {
	return nil, fmt.Errorf("not implemented")
}

//nolint:gocritic // Interface requires value semantics for SearchQuery
func (m *mockObjectStorage) Search(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query SearchQuery) (*SearchResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorage) BeginTransaction(ctx context.Context) (ObjectTransaction, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorage) BulkCreate(ctx context.Context, secCtx *pkgctx.SecurityContext, objs []map[string]any) (*BulkResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorage) BulkUpdate(ctx context.Context, secCtx *pkgctx.SecurityContext, updates []BulkUpdateItem) (*BulkResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorage) BulkGet(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string) (*BulkResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorage) BulkDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, cascade bool) (*BulkResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorage) Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error) {
	return false, fmt.Errorf("not implemented")
}

//nolint:gocritic // Interface requires value semantics for ListFilter
func (m *mockObjectStorage) Count(ctx context.Context, secCtx *pkgctx.SecurityContext, filter ListFilter) (int, error) {
	return 0, fmt.Errorf("not implemented")
}

//nolint:gocritic // Interface requires value semantics for ListFilter
func (m *mockObjectStorage) Aggregate(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter, aggregations []Aggregation) (*AggregateResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorage) GetRelated(ctx context.Context, secCtx *pkgctx.SecurityContext, id, relationshipType string, depth int) ([]map[string]any, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorage) GetPath(ctx context.Context, secCtx *pkgctx.SecurityContext, fromID, toID string) ([]map[string]any, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorage) GetNeighbors(ctx context.Context, secCtx *pkgctx.SecurityContext, id, direction string) ([]map[string]any, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorage) Move(ctx context.Context, secCtx *pkgctx.SecurityContext, id, newKind string, updateReferences bool) error {
	return fmt.Errorf("not implemented")
}

func (m *mockObjectStorage) Rename(ctx context.Context, secCtx *pkgctx.SecurityContext, oldID, newID string, updateReferences bool) error {
	return fmt.Errorf("not implemented")
}
