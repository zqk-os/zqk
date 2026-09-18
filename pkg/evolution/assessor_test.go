package evolution

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/infrastructure"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type mockStorage struct {
	storage.ObjectStorageProvider
	objects map[string]map[string]any
}

func (m *mockStorage) Create(_ context.Context, _ *storage.SecurityContext, obj map[string]any) error {
	if m.objects == nil {
		m.objects = make(map[string]map[string]any)
	}
	id := obj[objects.FieldKeyID].(string)
	m.objects[id] = obj
	return nil
}

func (m *mockStorage) Read(_ context.Context, _ *storage.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.objects[id]; ok {
		return obj, nil
	}
	return nil, storage.ErrObjectNotFound
}

func (m *mockStorage) Update(_ context.Context, _ *storage.SecurityContext, id string, updates map[string]any) error {
	if obj, ok := m.objects[id]; ok {
		for k, v := range updates {
			obj[k] = v
		}
		return nil
	}
	return storage.ErrObjectNotFound
}

func TestFitnessAssessor_Compare(t *testing.T) {
	ctx := context.Background()
	store := &mockStorage{}
	assessor := NewFitnessAssessor(store)

	// 1. Shadow component finds something mainline misses
	shadowEvents := []infrastructure.Event{
		{ObjectID: "CVS-1", Kind: objects.KindConvergenceSession, Op: "drift_detected"},
	}
	realEvents := []infrastructure.Event{} // Mainline missed it

	score, err := assessor.Compare(ctx, "eccentric-neuron", shadowEvents, realEvents)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}

	if score <= 0.5 {
		t.Errorf("expected score > 0.5 for differentiated brilliance, got %f", score)
	}

	// Verify report creation
	reportID := "MAT-eccentric-neuron"
	if _, ok := store.objects[reportID]; !ok {
		t.Errorf("maturation report %s was not created", reportID)
	}
}

func (m *mockStorage) Shutdown(context.Context) error { return nil }
