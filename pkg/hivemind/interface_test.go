package hivemind

import (
	"context"
	"testing"
)

// mockMemoryStore implements MemoryStore for testing purposes.
type mockMemoryStore struct{}

func (m *mockMemoryStore) RetrieveSemantically(ctx context.Context, query string, k int) ([]MemoryResult, error) {
	return []MemoryResult{{ID: "test-id", Score: 0.9}}, nil
}

func (m *mockMemoryStore) RetrieveGraphContext(ctx context.Context, id string, depth int) (*GraphSubgraph, error) {
	return &GraphSubgraph{Nodes: map[string]any{"test-id": "data"}}, nil
}

func (m *mockMemoryStore) QueryHybrid(ctx context.Context, query string, limit int, constraints HybridConstraints) ([]MemoryResult, error) {
	return []MemoryResult{{ID: "test-id", Score: 0.9}}, nil
}

func (m *mockMemoryStore) FindObjectsMissingVectors(ctx context.Context, batchSize int) ([]string, error) {
	return []string{}, nil
}

func (m *mockMemoryStore) LinkVectorID(ctx context.Context, objectID string, vectorID string) error {
	return nil
}

func TestMemoryStoreInterface(t *testing.T) {
	var _ MemoryStore = &mockMemoryStore{}
}
