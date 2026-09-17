package specialization

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/infrastructure"
	"github.com/lanceman/zqk/pkg/storage"
)

type mockSpine struct {
	published []infrastructure.Event
	handlers  map[string]infrastructure.Handler
}

func (m *mockSpine) Publish(ctx context.Context, event infrastructure.Event) error {
	m.published = append(m.published, event)
	return nil
}

func (m *mockSpine) Subscribe(ctx context.Context, kind string, handler infrastructure.Handler) error {
	if m.handlers == nil {
		m.handlers = make(map[string]infrastructure.Handler)
	}
	m.handlers[kind] = handler
	return nil
}

func (m *mockSpine) Replay(ctx context.Context, appliedSeq int64, handler infrastructure.Handler) error {
	return nil
}

func (m *mockSpine) Close() error { return nil }

type mockHandler struct {
	spine infrastructure.SpinalSpine
}

func (h *mockHandler) Initialize(ctx context.Context, store storage.ObjectStorageProvider, spine infrastructure.SpinalSpine) error {
	h.spine = spine
	return nil
}

func (h *mockHandler) Start(ctx context.Context) error { return nil }
func (h *mockHandler) Stop() error                     { return nil }

type subscribingHandler struct {
	mockHandler
	receivedEvents []infrastructure.Event
}

func (h *subscribingHandler) Initialize(ctx context.Context, store storage.ObjectStorageProvider, spine infrastructure.SpinalSpine) error {
	_ = h.mockHandler.Initialize(ctx, store, spine)
	return h.spine.Subscribe(ctx, "test-kind", h.handleEvent)
}

func (h *subscribingHandler) handleEvent(ctx context.Context, event infrastructure.Event) error {
	h.receivedEvents = append(h.receivedEvents, event)
	return nil
}

func TestShieldedHandler_Isolation(t *testing.T) {
	ctx := context.Background()
	innerSpine := &mockSpine{}

	h := &mockHandler{}
	shielded := NewShieldedHandler(h)

	// Initialize with main spine
	if err := shielded.Initialize(ctx, nil, innerSpine); err != nil {
		t.Fatalf("failed to initialize shielded handler: %v", err)
	}

	// Publish an event from the handler
	testEvent := infrastructure.Event{
		ObjectID: "test-1",
		Kind:     "test-kind",
		Op:       "create",
	}

	if err := h.spine.Publish(ctx, testEvent); err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	// Verify it did NOT hit the inner spine (Rubber Room isolation)
	if len(innerSpine.published) > 0 {
		t.Errorf("expected 0 events in main spine, got %d", len(innerSpine.published))
	}

	// Verify it DID hit the shadow spine
	shadowEvents := shielded.GetShadowEvents()
	if len(shadowEvents) != 1 {
		t.Errorf("expected 1 event in shadow spine, got %d", len(shadowEvents))
	} else if shadowEvents[0].ObjectID != "test-1" {
		t.Errorf("expected ObjectID 'test-1', got '%s'", shadowEvents[0].ObjectID)
	}
}

func TestShieldedHandler_SubscriptionPassThrough(t *testing.T) {
	ctx := context.Background()
	mainSpine := &mockSpine{}

	h := &subscribingHandler{}
	shielded := NewShieldedHandler(h)

	// Initialize
	if err := shielded.Initialize(ctx, nil, mainSpine); err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}

	// Verify handler is registered in main spine
	if _, ok := mainSpine.handlers["test-kind"]; !ok {
		t.Fatalf("expected handler to be registered in main spine for 'test-kind'")
	}

	// Manually trigger the handler through the main spine
	testEvent := infrastructure.Event{ObjectID: "evt-1", Kind: "test-kind"}
	handler := mainSpine.handlers["test-kind"]
	if err := handler(ctx, testEvent); err != nil {
		t.Fatalf("handler failed: %v", err)
	}

	// Verify handler received the event
	if len(h.receivedEvents) != 1 {
		t.Errorf("expected 1 received event, got %d", len(h.receivedEvents))
	} else if h.receivedEvents[0].ObjectID != "evt-1" {
		t.Errorf("expected ObjectID 'evt-1', got '%s'", h.receivedEvents[0].ObjectID)
	}
}
