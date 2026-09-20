package sync

import (
	"context"
	"errors"
	"testing"

	"github.com/zqk-os/zqk/pkg/hivemind"
)

type mockMemoryStore struct {
	missingIDs []string
	missingErr error
	linkedIDs  map[string]string
	linkErr    error
}

func (m *mockMemoryStore) RetrieveSemantically(ctx context.Context, query string, k int) ([]hivemind.MemoryResult, error) {
	return nil, nil
}

func (m *mockMemoryStore) RetrieveGraphContext(ctx context.Context, id string, depth int) (*hivemind.GraphSubgraph, error) {
	return nil, nil
}

func (m *mockMemoryStore) QueryHybrid(ctx context.Context, query string, limit int, constraints hivemind.HybridConstraints) ([]hivemind.MemoryResult, error) {
	return nil, nil
}

func (m *mockMemoryStore) FindObjectsMissingVectors(ctx context.Context, limit int) ([]string, error) {
	return m.missingIDs, m.missingErr
}

func (m *mockMemoryStore) LinkVectorID(ctx context.Context, objectID string, vectorID string) error {
	if m.linkErr != nil {
		return m.linkErr
	}
	if m.linkedIDs == nil {
		m.linkedIDs = make(map[string]string)
	}
	m.linkedIDs[objectID] = vectorID
	return nil
}

type mockIndexerWorker struct {
	indexErr error
	indexed  map[string]string
}

func (w *mockIndexerWorker) Index(ctx context.Context, id string, text string) error {
	if w.indexErr != nil {
		return w.indexErr
	}
	if w.indexed == nil {
		w.indexed = make(map[string]string)
	}
	w.indexed[id] = text
	return nil
}

type mockObjectLoader struct {
	loadErr error
}

func (l *mockObjectLoader) LoadContent(ctx context.Context, id string) (string, error) {
	if l.loadErr != nil {
		return "", l.loadErr
	}
	return "content for " + id, nil
}

func TestHiveMindSynchronizer_SyncNow(t *testing.T) {
	store := &mockMemoryStore{
		missingIDs: []string{"OBJ-1", "OBJ-2"},
	}
	worker := &mockIndexerWorker{}
	loader := &mockObjectLoader{}

	sync := NewHiveMindSynchronizer(store, worker, loader)

	err := sync.SyncNow(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(worker.indexed) != 2 {
		t.Errorf("expected 2 objects indexed, got %d", len(worker.indexed))
	}

	if len(store.linkedIDs) != 2 {
		t.Errorf("expected 2 objects linked, got %d", len(store.linkedIDs))
	}

	if store.linkedIDs["OBJ-1"] == "" || store.linkedIDs["OBJ-2"] == "" {
		t.Errorf("expected valid vector IDs linked")
	}
}

func TestHiveMindSynchronizer_SyncNow_FindError(t *testing.T) {
	store := &mockMemoryStore{
		missingErr: errors.New("db error"),
	}
	worker := &mockIndexerWorker{}
	loader := &mockObjectLoader{}

	sync := NewHiveMindSynchronizer(store, worker, loader)

	err := sync.SyncNow(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
