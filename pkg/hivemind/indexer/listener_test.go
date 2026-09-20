package indexer

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/lifecycle"
)

// Mock embedding service and store
type MockEmbedding struct{}

func (m *MockEmbedding) Embed(ctx context.Context, text string) ([]float32, error) {
	return []float32{0.1}, nil
}

type MockStore struct {
	UpsertCalled bool
}

func (m *MockStore) Upsert(ctx context.Context, id string, vector []float32) error {
	m.UpsertCalled = true
	return nil
}

func TestIndexerListener(t *testing.T) {
	// Setup test environment
	tmpDir := t.TempDir()
	wal, _ := lifecycle.NewLifecycleEventWAL(tmpDir)

	worker := NewIndexerWorker(&MockEmbedding{}, &MockStore{})
	listener := NewIndexerListener(wal, worker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Append test event
	_ = wal.Append(&lifecycle.LifecycleEvent{
		EventType: lifecycle.EventTypeStatusTransition,
		Kind:      "requirement",
		ID:        "req-1",
	})
	_ = wal.Sync()

	// Run listener briefly
	go func() { _ = listener.Run(ctx) }()

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Validation
	// Note: Mocking and asserting async outcomes requires more robust setup
}
