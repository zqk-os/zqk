package evolution_test

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/evolution"
	"github.com/lanceman/zqk/pkg/infrastructure"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

type mockSpine struct {
	infrastructure.SpinalSpine
	published []infrastructure.Event
}

func (m *mockSpine) Publish(_ context.Context, event infrastructure.Event) error {
	m.published = append(m.published, event)
	return nil
}

type mockStorage struct {
	storage.ObjectStorageProvider
	objects map[string]map[string]any
}

func (m *mockStorage) Read(_ context.Context, _ *storage.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.objects[id]; ok {
		return obj, nil
	}
	return nil, storage.ErrObjectNotFound
}

type mockSigner struct{}

func (s *mockSigner) Sign(data []byte) (string, error) {
	return "mock-signature", nil
}

func (s *mockSigner) PublicKey() string {
	return "mock-public-key"
}

func TestFissionController_Trigger(t *testing.T) {
	ctx := context.Background()
	spine := &mockSpine{}
	store := &mockStorage{
		objects: map[string]map[string]any{
			"system_vitality": {
				objects.FieldKeyProjectConfidenceScore: 90,
			},
		},
	}
	signer := &mockSigner{}

	controller := evolution.NewFissionController(store, spine, signer)

	// Manually trigger fission
	if err := controller.Trigger(ctx); err != nil {
		t.Fatalf("Trigger failed: %v", err)
	}

	// Verify event publication
	if len(spine.published) != 1 {
		t.Errorf("expected 1 published event, got %d", len(spine.published))
	}

	event := spine.published[0]
	if event.Kind != "fission_event" || event.Op != "initiated" {
		t.Errorf("unexpected event kind/op: %s/%s", event.Kind, event.Op)
	}

	if event.Payload[objects.FieldKeyReason] != "autonomous_saturation_fission" {
		t.Errorf("expected reason 'autonomous_saturation_fission', got %v", event.Payload[objects.FieldKeyReason])
	}
}

func (m *mockStorage) Shutdown(context.Context) error { return nil }
