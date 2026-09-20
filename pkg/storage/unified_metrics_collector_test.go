package storage

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// mockMetricsEventEmitter is a test implementation of MetricsEventEmitter
type mockMetricsEventEmitter struct {
	emittedEvents []metricCollectedEvent
	mu            sync.Mutex
}

type metricCollectedEvent struct {
	MetricType  string
	MetricID    string
	WindowStart time.Time
	WindowEnd   time.Time
	Error       error
}

func (m *mockMetricsEventEmitter) EmitMetricCollected(ctx context.Context, metricType, metricID string, windowStart, windowEnd time.Time, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emittedEvents = append(m.emittedEvents, metricCollectedEvent{
		MetricType:  metricType,
		MetricID:    metricID,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		Error:       err,
	})
}

func (m *mockMetricsEventEmitter) getEmittedEvents() []metricCollectedEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]metricCollectedEvent, len(m.emittedEvents))
	copy(result, m.emittedEvents)
	return result
}

// mockMetricsCollector is a test implementation of MetricsCollector
type mockMetricsCollector struct {
	collectCount         int
	collectAndResetCount int
	shouldError          bool
	metricID             string
}

func (m *mockMetricsCollector) CollectMetrics(ctx context.Context, secCtx *pkgctx.SecurityContext, windowStart, windowEnd time.Time) (string, error) {
	m.collectCount++
	if m.shouldError {
		return "", fmt.Errorf("mock error")
	}
	return m.metricID, nil
}

func (m *mockMetricsCollector) CollectAndReset(ctx context.Context, secCtx *pkgctx.SecurityContext, windowStart, windowEnd time.Time) (string, error) {
	m.collectAndResetCount++
	if m.shouldError {
		return "", fmt.Errorf("mock error")
	}
	return m.metricID, nil
}

func TestUnifiedMetricsCollector_CollectAllMetrics(t *testing.T) {
	// Setup
	storage := &mockObjectStorage{}
	eventEmitter := &mockMetricsEventEmitter{}

	casCollector := &mockMetricsCollector{metricID: "CASM-123"}
	fileLockCollector := &mockMetricsCollector{metricID: "FLM-456"}
	auditCollector := &mockMetricsCollector{metricID: "AUDIT-789"}

	collector := &UnifiedMetricsCollector{
		storage:           storage,
		eventEmitter:      eventEmitter,
		casCollector:      casCollector,
		fileLockCollector: fileLockCollector,
		auditCollector:    auditCollector,
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	windowStart := time.Now().Add(-1 * time.Hour)
	windowEnd := time.Now()

	// Test
	metricIDs, err := collector.CollectAllMetrics(ctx, secCtx, windowStart, windowEnd)

	// Verify
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(metricIDs) != 3 {
		t.Fatalf("Expected 3 metric IDs, got %d", len(metricIDs))
	}

	expectedIDs := map[string]bool{
		"CASM-123":  true,
		"FLM-456":   true,
		"AUDIT-789": true,
	}
	for _, id := range metricIDs {
		if !expectedIDs[id] {
			t.Errorf("Unexpected metric ID: %s", id)
		}
	}

	// Verify collectors were called
	if casCollector.collectAndResetCount != 1 {
		t.Errorf("Expected CAS collector to be called once, got %d", casCollector.collectAndResetCount)
	}
	if fileLockCollector.collectAndResetCount != 1 {
		t.Errorf("Expected FileLock collector to be called once, got %d", fileLockCollector.collectAndResetCount)
	}
	if auditCollector.collectAndResetCount != 1 {
		t.Errorf("Expected Audit collector to be called once, got %d", auditCollector.collectAndResetCount)
	}

	// Verify events were emitted
	events := eventEmitter.getEmittedEvents()
	if len(events) != 3 {
		t.Fatalf("Expected 3 emitted events, got %d", len(events))
	}

	eventTypes := map[string]bool{}
	for _, event := range events {
		eventTypes[event.MetricType] = true
		if event.Error != nil {
			t.Errorf("Expected no error in event for %s, got: %v", event.MetricType, event.Error)
		}
	}

	expectedTypes := []string{"cas_metric", "file_lock_metric", "audit_metric"}
	for _, expectedType := range expectedTypes {
		if !eventTypes[expectedType] {
			t.Errorf("Expected event type %s not found", expectedType)
		}
	}
}

func TestUnifiedMetricsCollector_CollectAllMetrics_WithErrors(t *testing.T) {
	// Setup
	storage := &mockObjectStorage{}
	eventEmitter := &mockMetricsEventEmitter{}

	casCollector := &mockMetricsCollector{metricID: "CASM-123", shouldError: true}
	fileLockCollector := &mockMetricsCollector{metricID: "FLM-456"}
	auditCollector := &mockMetricsCollector{metricID: "AUDIT-789"}

	collector := &UnifiedMetricsCollector{
		storage:           storage,
		eventEmitter:      eventEmitter,
		casCollector:      casCollector,
		fileLockCollector: fileLockCollector,
		auditCollector:    auditCollector,
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	windowStart := time.Now().Add(-1 * time.Hour)
	windowEnd := time.Now()

	// Test
	metricIDs, err := collector.CollectAllMetrics(ctx, secCtx, windowStart, windowEnd)

	// Verify - should still collect other metrics even if one fails
	if err == nil {
		t.Error("Expected error when some collectors fail")
	}

	if len(metricIDs) != 2 {
		t.Errorf("Expected 2 successful metric IDs, got %d", len(metricIDs))
	}

	// Verify error event was emitted
	events := eventEmitter.getEmittedEvents()
	errorEvents := 0
	for _, event := range events {
		if event.Error != nil {
			errorEvents++
			if event.MetricType != "cas_metric" {
				t.Errorf("Expected error event for cas_metric, got %s", event.MetricType)
			}
		}
	}
	if errorEvents != 1 {
		t.Errorf("Expected 1 error event, got %d", errorEvents)
	}
}

func TestUnifiedMetricsCollector_CollectMetricsByType(t *testing.T) {
	// Setup
	storage := &mockObjectStorage{}
	eventEmitter := &mockMetricsEventEmitter{}

	casCollector := &mockMetricsCollector{metricID: "CASM-123"}
	fileLockCollector := &mockMetricsCollector{metricID: "FLM-456"}
	auditCollector := &mockMetricsCollector{metricID: "AUDIT-789"}

	collector := &UnifiedMetricsCollector{
		storage:           storage,
		eventEmitter:      eventEmitter,
		casCollector:      casCollector,
		fileLockCollector: fileLockCollector,
		auditCollector:    auditCollector,
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	windowStart := time.Now().Add(-1 * time.Hour)
	windowEnd := time.Now()

	// Test CAS metrics
	metricID, err := collector.CollectMetricsByType(ctx, secCtx, "cas_metric", windowStart, windowEnd)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if metricID != "CASM-123" {
		t.Errorf("Expected CASM-123, got %s", metricID)
	}
	if casCollector.collectAndResetCount != 1 {
		t.Errorf("Expected CAS collector to be called once, got %d", casCollector.collectAndResetCount)
	}

	// Test file lock metrics
	metricID, err = collector.CollectMetricsByType(ctx, secCtx, "file_lock", windowStart, windowEnd)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if metricID != "FLM-456" {
		t.Errorf("Expected FLM-456, got %s", metricID)
	}

	// Test audit metrics
	metricID, err = collector.CollectMetricsByType(ctx, secCtx, "audit", windowStart, windowEnd)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if metricID != "AUDIT-789" {
		t.Errorf("Expected AUDIT-789, got %s", metricID)
	}

	// Test unknown type
	_, err = collector.CollectMetricsByType(ctx, secCtx, "unknown", windowStart, windowEnd)
	if err == nil {
		t.Error("Expected error for unknown metric type")
	}
}

func TestUnifiedMetricsCollector_AsyncCollection(t *testing.T) {
	// Setup
	casCollector := &mockMetricsCollector{metricID: "CASM-123"}
	asyncCollector := NewAsyncMetricsCollector(casCollector, 10)

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	windowStart := time.Now().Add(-1 * time.Hour)
	windowEnd := time.Now()

	// Test async collection
	done := make(chan bool)
	var collectedID string
	var collectErr error

	err := asyncCollector.CollectMetricsAsync(ctx, secCtx, windowStart, windowEnd, func(metricID string, err error) {
		collectedID = metricID
		collectErr = err
		done <- true
	})
	if err != nil {
		t.Fatalf("Expected no error queuing async collection, got: %v", err)
	}

	// Wait for async collection to complete
	select {
	case <-done:
		if collectErr != nil {
			t.Fatalf("Expected no error in async collection, got: %v", collectErr)
		}
		if collectedID != "CASM-123" {
			t.Errorf("Expected CASM-123, got %s", collectedID)
		}
		if casCollector.collectAndResetCount != 1 {
			t.Errorf("Expected collector to be called once, got %d", casCollector.collectAndResetCount)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Async collection timeout")
	}

	// Cleanup
	asyncCollector.Stop()
}

func TestUnifiedMetricsCollector_Stop(t *testing.T) {
	// Setup
	storage := &mockObjectStorage{}
	eventEmitter := &mockMetricsEventEmitter{}

	casCollector := &mockMetricsCollector{metricID: "CASM-123"}
	fileLockCollector := &mockMetricsCollector{metricID: "FLM-456"}

	asyncCAS := NewAsyncMetricsCollector(casCollector, 10)
	asyncFileLock := NewAsyncMetricsCollector(fileLockCollector, 10)

	collector := &UnifiedMetricsCollector{
		storage:                storage,
		eventEmitter:           eventEmitter,
		casCollector:           casCollector,
		fileLockCollector:      fileLockCollector,
		casAsyncCollector:      asyncCAS,
		fileLockAsyncCollector: asyncFileLock,
	}

	// Stop should not panic
	collector.Stop()

	// Verify async collectors are stopped (should silently skip when disabled)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	windowStart := time.Now().Add(-1 * time.Hour)
	windowEnd := time.Now()

	// After stop, async collectors should be disabled and silently skip
	err := asyncCAS.CollectMetricsAsync(ctx, secCtx, windowStart, windowEnd, nil)
	if err != nil {
		t.Fatalf("Expected no error (should silently skip when disabled), got: %v", err)
	}

	// Verify collectors are actually stopped (not called)
	if casCollector.collectAndResetCount != 0 {
		t.Errorf("Expected collector not to be called after stop, but was called %d times", casCollector.collectAndResetCount)
	}
}

// Note: TestMetricsCollectorAdapter_EmitsViaCoordinator is in pkg/coordination package
// to avoid import cycles

// Note: mockObjectStorage is defined in file_lock_metrics_async_test.go
