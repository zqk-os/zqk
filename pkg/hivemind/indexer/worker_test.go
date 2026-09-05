package indexer

import (
	"context"
	"testing"
)

// MockEmbeddingService simulates an embedding provider.
type MockEmbeddingService struct{}

func (m *MockEmbeddingService) Embed(ctx context.Context, text string) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3}, nil
}

// MockMemoryStore simulates the vector database store.
type MockMemoryStore struct {
	Vectors map[string][]float32
}

func (m *MockMemoryStore) Upsert(ctx context.Context, id string, vector []float32) error {
	m.Vectors[id] = vector
	return nil
}

func TestIndexerWorker(t *testing.T) {
	t.Run("should index object text successfully", func(t *testing.T) {
		ctx := context.Background()
		embeddingService := &MockEmbeddingService{}
		memoryStore := &MockMemoryStore{Vectors: make(map[string][]float32)}

		worker := NewIndexerWorker(embeddingService, memoryStore)

		err := worker.Index(ctx, "obj-123", "some text content")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if len(memoryStore.Vectors) != 1 {
			t.Fatalf("expected 1 vector, got %d", len(memoryStore.Vectors))
		}
	})
}
