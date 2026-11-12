package community

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestProviderEventMesh_StaticFloorInvariant(t *testing.T) {
	evt := &ProviderEvent{
		ID:        "evt-001",
		Topic:     "provider.telemetry.metrics",
		Source:    "test-source",
		Payload:   map[string]interface{}{"metric": "cpu", "value": 42},
		Timestamp: time.Now().UTC(),
	}

	if err := evt.Validate(); err != nil {
		t.Fatalf("expected valid event, got: %v", err)
	}

	// Topic cannot contain wildcards for publishing
	badEvt := &ProviderEvent{
		ID:    "evt-002",
		Topic: "provider.*",
	}
	if err := badEvt.Validate(); !errors.Is(err, ErrInvalidTopic) {
		t.Errorf("expected ErrInvalidTopic for wildcard publication, got: %v", err)
	}

	// Missing ID
	missingID := &ProviderEvent{
		Topic: "provider.test",
	}
	if err := missingID.Validate(); !errors.Is(err, ErrInvalidEvent) {
		t.Errorf("expected ErrInvalidEvent for missing ID, got: %v", err)
	}
}

func TestProviderEventMesh_OperationalDynamicProof(t *testing.T) {
	mesh := NewProviderEventMesh()
	defer mesh.Close()

	ctx := context.Background()
	var exactCount int32
	var wildCount int32

	// Subscribe exact topic
	unsub1, err := mesh.Subscribe("provider.storage.write", func(ctx context.Context, e ProviderEvent) error {
		atomic.AddInt32(&exactCount, 1)
		return nil
	})
	if err != nil {
		t.Fatalf("failed to subscribe exact: %v", err)
	}

	// Subscribe wildcard pattern
	_, err = mesh.Subscribe("provider.storage.*", func(ctx context.Context, e ProviderEvent) error {
		atomic.AddInt32(&wildCount, 1)
		return nil
	})
	if err != nil {
		t.Fatalf("failed to subscribe wildcard: %v", err)
	}

	if mesh.SubscriberCount() != 2 {
		t.Fatalf("expected 2 subscribers, got %d", mesh.SubscriberCount())
	}

	// Publish matching both
	evtWrite := ProviderEvent{
		ID:    "evt-100",
		Topic: "provider.storage.write",
	}
	if err := mesh.Publish(ctx, evtWrite); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	if atomic.LoadInt32(&exactCount) != 1 {
		t.Errorf("expected exactCount=1, got %d", atomic.LoadInt32(&exactCount))
	}
	if atomic.LoadInt32(&wildCount) != 1 {
		t.Errorf("expected wildCount=1, got %d", atomic.LoadInt32(&wildCount))
	}

	// Publish matching only wildcard
	evtRead := ProviderEvent{
		ID:    "evt-101",
		Topic: "provider.storage.read",
	}
	if err := mesh.Publish(ctx, evtRead); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	if atomic.LoadInt32(&exactCount) != 1 {
		t.Errorf("expected exactCount still 1, got %d", atomic.LoadInt32(&exactCount))
	}
	if atomic.LoadInt32(&wildCount) != 2 {
		t.Errorf("expected wildCount=2, got %d", atomic.LoadInt32(&wildCount))
	}

	// Unsubscribe exact
	unsub1()
	if mesh.SubscriberCount() != 1 {
		t.Errorf("expected 1 subscriber after unsub, got %d", mesh.SubscriberCount())
	}
}

func TestProviderEventMesh_NegativeBoundaryAndErrorRecovery(t *testing.T) {
	mesh := NewProviderEventMesh()
	defer mesh.Close()

	ctx := context.Background()

	// Nil handler
	if _, err := mesh.Subscribe("test.topic", nil); !errors.Is(err, ErrSubscriberFailed) {
		t.Errorf("expected ErrSubscriberFailed for nil handler, got: %v", err)
	}

	// Invalid topic pattern
	if _, err := mesh.Subscribe("invalid space topic", func(ctx context.Context, e ProviderEvent) error { return nil }); !errors.Is(err, ErrInvalidTopic) {
		t.Errorf("expected ErrInvalidTopic, got: %v", err)
	}

	// Subscriber panic isolation
	var healthyReceived int32
	_, err := mesh.Subscribe("provider.alert", func(ctx context.Context, e ProviderEvent) error {
		panic("simulated subscriber catastrophic failure")
	})
	if err != nil {
		t.Fatalf("subscribe panic handler failed: %v", err)
	}

	_, err = mesh.Subscribe("provider.alert", func(ctx context.Context, e ProviderEvent) error {
		atomic.AddInt32(&healthyReceived, 1)
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe healthy handler failed: %v", err)
	}

	// Publishing should NOT crash and healthy subscriber should still receive event
	evt := ProviderEvent{
		ID:    "evt-alert",
		Topic: "provider.alert",
	}
	if err := mesh.Publish(ctx, evt); err != nil {
		t.Fatalf("publish failed despite panic isolation: %v", err)
	}

	if atomic.LoadInt32(&healthyReceived) != 1 {
		t.Errorf("expected healthy subscriber to receive event, got %d", atomic.LoadInt32(&healthyReceived))
	}

	// Close mesh and verify rejected actions
	mesh.Close()
	if err := mesh.Publish(ctx, evt); !errors.Is(err, ErrMeshClosed) {
		t.Errorf("expected ErrMeshClosed on publish, got %v", err)
	}
	if _, err := mesh.Subscribe("test", func(ctx context.Context, e ProviderEvent) error { return nil }); !errors.Is(err, ErrMeshClosed) {
		t.Errorf("expected ErrMeshClosed on subscribe, got %v", err)
	}
}

func TestProviderEventMesh_DocumentationVerification(t *testing.T) {
	adrPath := filepath.Join("..", "..", "docs", "architecture", "ADR_AUTONOMOUS_PROVIDER_EVENT_MESH_AND_DYNAMIC_DISPATCH_BUS_V036.md")
	data, err := fileutil.ReadFile(adrPath)
	if err != nil {
		t.Fatalf("expected ADR-0041 to exist at %s, got: %v", adrPath, err)
	}
	content := string(data)
	if len(content) == 0 {
		t.Fatalf("ADR-0041 content cannot be empty")
	}
	if len(content) > 8000 {
		t.Errorf("ADR-0041 exceeds POL-DOC-001 character limit (8000), size: %d", len(content))
	}
}
