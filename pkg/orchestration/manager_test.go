package orchestration

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/hivemind"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/scheduler"
	"github.com/lanceman/zqk/pkg/storage"
)

// Mock objects for testing the orchestrator
type mockNegotiator struct{}

func (m *mockNegotiator) Propose(ctx context.Context, p scheduler.Proposal) (*scheduler.NegotiationResult, error) {
	return &scheduler.NegotiationResult{Accepted: true, Reason: "ok"}, nil
}

type mockMemoryStore struct{}

func (m *mockMemoryStore) RetrieveSemantically(ctx context.Context, q string, k int) ([]hivemind.MemoryResult, error) {
	return []hivemind.MemoryResult{{ID: "1", Score: 0.9}}, nil
}
func (m *mockMemoryStore) RetrieveGraphContext(ctx context.Context, id string, d int) (*hivemind.GraphSubgraph, error) {
	return &hivemind.GraphSubgraph{Nodes: map[string]any{"1": "node1"}}, nil
}
func (m *mockMemoryStore) QueryHybrid(ctx context.Context, q string, limit int, c hivemind.HybridConstraints) ([]hivemind.MemoryResult, error) {
	return []hivemind.MemoryResult{{ID: "1", Score: 0.9}}, nil
}
func (m *mockMemoryStore) FindObjectsMissingVectors(ctx context.Context, limit int) ([]string, error) {
	return []string{}, nil
}

func (m *mockMemoryStore) LinkVectorID(ctx context.Context, objectID string, vectorID string) error {
	return nil
}

type mockStorage struct {
	createdObjects map[string]map[string]any
}

func (m *mockStorage) Create(ctx context.Context, secCtx *storage.SecurityContext, obj map[string]any) error {
	m.createdObjects[obj[objects.FieldKeyID].(string)] = obj
	return nil
}

func (m *mockStorage) GenerateID(ctx context.Context, kind string) (string, error) {
	return "MOCK-123", nil
}
func (m *mockStorage) Read(ctx context.Context, secCtx *storage.SecurityContext, id string) (map[string]any, error) {
	return nil, nil
}
func (m *mockStorage) Update(ctx context.Context, secCtx *storage.SecurityContext, id string, updates map[string]any) error {
	return nil
}
func (m *mockStorage) Delete(ctx context.Context, secCtx *storage.SecurityContext, id string, cascade bool) error {
	return nil
}
func (m *mockStorage) List(ctx context.Context, secCtx *storage.SecurityContext, storageCtx *storage.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	return nil, nil
}
func (m *mockStorage) Query(ctx context.Context, secCtx *storage.SecurityContext, storageCtx *storage.StorageContext, query storage.Query) (*storage.QueryResult, error) {
	return nil, nil
}
func (m *mockStorage) Search(ctx context.Context, secCtx *storage.SecurityContext, storageCtx *storage.StorageContext, query storage.SearchQuery) (*storage.SearchResult, error) {
	return nil, nil
}
func (m *mockStorage) BeginTransaction(ctx context.Context) (storage.ObjectTransaction, error) {
	return nil, nil
}
func (m *mockStorage) BulkCreate(ctx context.Context, secCtx *storage.SecurityContext, objects []map[string]any) (*storage.BulkResult, error) {
	return nil, nil
}
func (m *mockStorage) BulkUpdate(ctx context.Context, secCtx *storage.SecurityContext, updates []storage.BulkUpdateItem) (*storage.BulkResult, error) {
	return nil, nil
}
func (m *mockStorage) BulkGet(ctx context.Context, secCtx *storage.SecurityContext, ids []string) (*storage.BulkResult, error) {
	return nil, nil
}
func (m *mockStorage) BulkDelete(ctx context.Context, secCtx *storage.SecurityContext, ids []string, cascade bool) (*storage.BulkResult, error) {
	return nil, nil
}
func (m *mockStorage) Exists(ctx context.Context, secCtx *storage.SecurityContext, id string) (bool, error) {
	return false, nil
}
func (m *mockStorage) Count(ctx context.Context, secCtx *storage.SecurityContext, filter storage.ListFilter) (int, error) {
	return 0, nil
}
func (m *mockStorage) Aggregate(ctx context.Context, secCtx *storage.SecurityContext, storageCtx *storage.StorageContext, filter storage.ListFilter, aggregations []storage.Aggregation) (*storage.AggregateResult, error) {
	return nil, nil
}
func (m *mockStorage) GetRelated(ctx context.Context, secCtx *storage.SecurityContext, id string, relType string, depth int) ([]map[string]any, error) {
	return nil, nil
}
func (m *mockStorage) GetPath(ctx context.Context, secCtx *storage.SecurityContext, fromID, toID string) ([]map[string]any, error) {
	return nil, nil
}
func (m *mockStorage) GetNeighbors(ctx context.Context, secCtx *storage.SecurityContext, id string, direction string) ([]map[string]any, error) {
	return nil, nil
}
func (m *mockStorage) Move(ctx context.Context, secCtx *storage.SecurityContext, id string, newKind string, updateRefs bool) error {
	return nil
}
func (m *mockStorage) Rename(ctx context.Context, secCtx *storage.SecurityContext, oldID, newID string, updateRefs bool) error {
	return nil
}

func TestManager_ProcessIntent(t *testing.T) {
	mockStore := &mockStorage{createdObjects: make(map[string]map[string]any)}
	manager := NewManager(&mockNegotiator{}, &mockMemoryStore{}, mockStore)

	t.Run("Successful synthesis and commitment", func(t *testing.T) {
		intent := RawIntent{
			Signature: "test-feature",
			Payload: map[string]any{
				"test_case": "TC-101",
			},
		}
		ref, err := manager.ProcessIntent(context.Background(), intent)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if ref.ID != "CAP-TEST-FEATURE" {
			t.Errorf("expected ID CAP-TEST-FEATURE, got %s", ref.ID)
		}

		if _, ok := mockStore.createdObjects["CAP-TEST-FEATURE"]; !ok {
			t.Errorf("object was not committed to storage")
		}
	})

	t.Run("Synthesis fails without test case", func(t *testing.T) {
		intent := RawIntent{
			Signature: "test-feature-no-tc",
		}
		_, err := manager.ProcessIntent(context.Background(), intent)
		if err == nil {
			t.Fatalf("expected error due to missing test_case, got nil")
		}
	})
}

func (m *mockStorage) Shutdown(context.Context) error { return nil }
