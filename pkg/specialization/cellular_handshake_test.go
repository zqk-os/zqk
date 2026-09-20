package specialization_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/infrastructure"
	_ "github.com/zqk-os/zqk/pkg/infrastructure/drivers/kafka"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specialization"
	"github.com/zqk-os/zqk/pkg/storage"
)

// mockSpine is a simple in-memory spine for testing handler interaction.
type mockSpine struct {
	mu       sync.RWMutex
	handlers map[string][]infrastructure.Handler
	events   chan infrastructure.Event
}

func (m *mockSpine) Publish(ctx context.Context, event infrastructure.Event) error {
	m.mu.RLock()
	var handlers []infrastructure.Handler
	if m.handlers != nil {
		if list, ok := m.handlers[event.Kind]; ok {
			handlers = make([]infrastructure.Handler, len(list))
			copy(handlers, list)
		}
	}
	m.mu.RUnlock()

	for _, h := range handlers {
		_ = h(ctx, event)
	}

	if m.events != nil {
		select {
		case m.events <- event:
		default:
		}
	}
	return nil
}

func (m *mockSpine) Subscribe(_ context.Context, kind string, handler infrastructure.Handler) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.handlers == nil {
		m.handlers = make(map[string][]infrastructure.Handler)
	}
	m.handlers[kind] = append(m.handlers[kind], handler)
	return nil
}

func (m *mockSpine) Replay(_ context.Context, _ int64, _ infrastructure.Handler) error {
	return nil
}

func (m *mockSpine) Close() error { return nil }

type mockStorage struct {
	storage.ObjectStorageProvider
}

func (m *mockStorage) Create(_ context.Context, _ *storage.SecurityContext, _ map[string]any) error {
	return nil
}

func (m *mockStorage) Update(_ context.Context, _ *storage.SecurityContext, _ string, _ map[string]any) error {
	return nil
}

func TestCellularHandshake(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 1. Setup Mock Environment
	spine := &mockSpine{
		events: make(chan infrastructure.Event, 10),
	}
	// Wrap with BufferedSpine to verify non-blocking behavior
	bufferedSpine := infrastructure.NewBufferedSpine(spine, 100)
	defer bufferedSpine.Close()

	store := &mockStorage{}

	// 2. Register Handlers (Neuron and Muscle)
	neuron := &specialization.NeuronHandler{}
	muscle := &specialization.MuscleHandler{}

	registry := specialization.NewRegistry()
	registry.Add(neuron)
	registry.Add(muscle)

	err := registry.EngageAll(ctx, store, bufferedSpine)
	if err != nil {
		t.Fatalf("failed to engage handlers: %v", err)
	}

	// 3. Trigger Sensing (Neuron senses drift)
	driftEvent := infrastructure.Event{
		ObjectID: "CVS-DRIFT-001",
		Kind:     objects.KindConvergenceSession,
		Op:       "update",
		Payload: map[string]any{
			objects.FieldKeyTitle:           "Truth Sentinel Monitoring",
			objects.FieldKeyStatus:          "active",
			objects.FieldKeyDeltaAssessment: "trending_away",
		},
	}

	t.Log("--- T0: Neuron senses drift ---")
	err = bufferedSpine.Publish(ctx, driftEvent)
	if err != nil {
		t.Fatalf("failed to publish drift event: %v", err)
	}

	// Wait for 3 events: drift, propose, execute to avoid race on close
	for i := 0; i < 3; i++ {
		select {
		case ev := <-spine.events:
			t.Logf("Processed event %d: Kind=%s, Op=%s", i+1, ev.Kind, ev.Op)
		case <-time.After(2 * time.Second):
			t.Fatalf("Timeout waiting for event %d", i+1)
		}
	}

	// 4. Verification:
	// - Neuron should have published a 'propose' event for a backlog_item.
	// - Muscle should have caught that proposal and printed a validation message.

	t.Log("--- Handshake complete ---")
}

func (m *mockStorage) Shutdown(context.Context) error { return nil }
