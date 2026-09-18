package coordination

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// mockEventCoordinator is a test implementation of EventCoordinator
type mockEventCoordinator struct {
	emittedEvents []*EventContext
	mu            sync.Mutex
}

func (m *mockEventCoordinator) Emit(ctx context.Context, eventCtx *EventContext) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emittedEvents = append(m.emittedEvents, eventCtx)
	return nil
}

func (m *mockEventCoordinator) Subscribe(subscriber OperationalEventSubscriber) string {
	return "mock-subscriber-id"
}

func (m *mockEventCoordinator) Unsubscribe(subscriberID string) {
	// No-op for testing
}

func (m *mockEventCoordinator) getEmittedEvents() []*EventContext {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*EventContext, len(m.emittedEvents))
	copy(result, m.emittedEvents)
	return result
}

func TestMetricsCollectorAdapter_EmitsViaCoordinator(t *testing.T) {
	t.Parallel()
	// Setup mock coordinator
	mockCoordinator := &mockEventCoordinator{
		emittedEvents: []*EventContext{},
	}

	adapter := NewMetricsCollectorAdapter(mockCoordinator)

	ctx := pkgctx.NewSystemContext()
	windowStart := time.Now().Add(-1 * time.Hour)
	windowEnd := time.Now()

	// Test successful emission
	adapter.EmitMetricCollected(ctx, "cas_metric", "CASM-123", windowStart, windowEnd, nil)

	// Verify event was emitted
	events := mockCoordinator.getEmittedEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 emitted event, got %d", len(events))
	}

	event := events[0]
	if event.OperationType != "metric_collection" {
		t.Errorf("Expected operation type 'metric_collection', got %s", event.OperationType)
	}
	if event.Status != "success" {
		t.Errorf("Expected status 'success', got %s", event.Status)
	}
	if !event.EmitMetrics {
		t.Error("Expected EmitMetrics to be true")
	}
	if event.EventData == nil || event.EventData.MetricsData == nil {
		t.Error("Expected metrics data to be set")
	}

	// Test error emission
	adapter.EmitMetricCollected(ctx, "file_lock_metric", "", windowStart, windowEnd, fmt.Errorf("test error"))

	events = mockCoordinator.getEmittedEvents()
	if len(events) != 2 {
		t.Fatalf("Expected 2 emitted events, got %d", len(events))
	}

	errorEvent := events[1]
	if errorEvent.Status != "error" {
		t.Errorf("Expected status 'error', got %s", errorEvent.Status)
	}
	if errorEvent.Error == nil {
		t.Error("Expected error to be set")
	}
}
