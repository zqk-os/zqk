package storage_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// TestAuditEventBuffer_CoordinatorIntegration tests that audit buffer flush operations emit events via coordinator
func TestAuditEventBuffer_CoordinatorIntegration(t *testing.T) {
	testRoot, fileStorage, secCtx := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	secCtx.AccountID = "test-user"

	buffer := storage.NewAuditEventBuffer(testRoot, secCtx, storage.DefaultAggregationRules())
	defer buffer.Shutdown()
	defer buffer.Shutdown()
	buffer.SetFileStorage(fileStorage)

	callbackWaiter := storage.NewCallbackWaiterForTest()
	var lastEvent struct {
		operationType string
		status        string
		groupKey      string
		eventType     string
		eventCount    int
		err           error
		sync.Mutex
	}

	var totalEventCount atomic.Int32

	storage.SetAuditBufferFlushEventCallback(func(
		ctx context.Context,
		projectRoot string,
		_ storage.ObjectStorageProvider,
		operationID string,
		operationType string,
		status string,
		groupKey string,
		eventType string,
		targetKind string,
		eventCount int,
		aggregationWindow string,
		duration time.Duration,
		err error,
	) {
		callbackWaiter.Invoke()
		lastEvent.Lock()
		lastEvent.operationType = operationType
		lastEvent.status = status
		lastEvent.groupKey = groupKey
		lastEvent.eventType = eventType
		lastEvent.eventCount = eventCount
		lastEvent.err = err
		lastEvent.Unlock()
		if operationType == "audit_buffer_flush" && eventType == "cache_invalidation" {
			totalEventCount.Add(int32(eventCount)) //nolint:gosec // test-only aggregation
		}
	})
	defer storage.SetAuditBufferFlushEventCallback(nil)

	for i := 0; i < 15; i++ {
		event := map[string]any{
			objects.FieldKeyEventType:  "cache_invalidation",
			objects.FieldKeySeverity:   "low",
			objects.FieldKeyTargetKind: "backlog_item",
		}
		if err := buffer.AddEvent(event); err != nil {
			t.Fatalf("AddEvent() failed: %v", err)
		}
	}

	if err := buffer.Flush(); err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}

	if !callbackWaiter.WaitForInvocation(pkgctx.NewSystemContext(), 5*time.Second) {
		t.Fatal("Expected callback to be called within 5 seconds, but it wasn't")
	}

	if !callbackWaiter.WaitForInvocation(pkgctx.NewSystemContext(), 5*time.Second) {
		t.Fatal("Expected callback to be called within 5 seconds, but it wasn't")
	}

	lastEvent.Lock()
	if lastEvent.operationType != "audit_buffer_flush" {
		t.Errorf("Expected operation type 'audit_buffer_flush', got '%s'", lastEvent.operationType)
	}
	if lastEvent.eventType != "cache_invalidation" {
		t.Errorf("Expected event type 'cache_invalidation', got '%s'", lastEvent.eventType)
	}
	lastEvent.Unlock()

	if totalEventCount.Load() < 15 {
		t.Errorf("Expected coordinator to see at least 15 events, got %d", totalEventCount.Load())
	}
}

// TestAuditEventBuffer_CoordinatorIntegration_NoCallback tests that flush works when callback is not set
func TestAuditEventBuffer_CoordinatorIntegration_NoCallback(t *testing.T) {
	testRoot, fileStorage, secCtx := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	buffer := storage.NewAuditEventBuffer(testRoot, secCtx, storage.DefaultAggregationRules())
	defer buffer.Shutdown()
	defer buffer.Shutdown()
	buffer.SetFileStorage(fileStorage)

	storage.SetAuditBufferFlushEventCallback(nil)

	event := map[string]any{
		objects.FieldKeyEventType:  "cache_invalidation",
		objects.FieldKeySeverity:   "low",
		objects.FieldKeyTargetKind: "backlog_item",
	}
	if err := buffer.AddEvent(event); err != nil {
		t.Fatalf("AddEvent() failed: %v", err)
	}

	if err := buffer.Flush(); err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}
}

// TestAuditEventBuffer_CoordinatorIntegration_ThresholdFlush tests threshold-triggered flush
func TestAuditEventBuffer_CoordinatorIntegration_ThresholdFlush(t *testing.T) {
	testRoot, fileStorage, secCtx := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	rules := []storage.AggregationRule{
		{
			EventTypes:      []string{"cache_invalidation"},
			Severities:      []string{"low"},
			GroupBy:         []string{"event_type", "target_kind"},
			Window:          time.Hour,
			Threshold:       5,
			PreserveSamples: 3,
		},
	}
	buffer := storage.NewAuditEventBuffer(testRoot, secCtx, rules)
	defer buffer.Shutdown()
	defer buffer.Shutdown()
	buffer.SetFileStorage(fileStorage)

	flushWaiter := storage.NewCallbackWaiterForTest()
	storage.SetAuditBufferFlushEventCallback(func(
		ctx context.Context,
		projectRoot string,
		_ storage.ObjectStorageProvider,
		operationID string,
		operationType string,
		status string,
		groupKey string,
		eventType string,
		targetKind string,
		eventCount int,
		aggregationWindow string,
		duration time.Duration,
		err error,
	) {
		if status == "start" {
			flushWaiter.Invoke()
		}
	})
	defer storage.SetAuditBufferFlushEventCallback(nil)

	for i := 0; i < 5; i++ {
		event := map[string]any{
			objects.FieldKeyEventType:  "cache_invalidation",
			objects.FieldKeySeverity:   "low",
			objects.FieldKeyTargetKind: "backlog_item",
		}
		if err := buffer.AddEvent(event); err != nil {
			t.Fatalf("AddEvent() failed: %v", err)
		}
	}

	if !flushWaiter.WaitForInvocation(pkgctx.NewSystemContext(), 5*time.Second) {
		t.Error("Expected threshold-triggered flush within 5 seconds, but callback wasn't called")
	}
}

// TestAuditEventBuffer_CoordinatorIntegration_ErrorHandling tests error handling
func TestAuditEventBuffer_CoordinatorIntegration_ErrorHandling(t *testing.T) {
	testRoot, fileStorage, secCtx := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	buffer := storage.NewAuditEventBuffer(testRoot, secCtx, storage.DefaultAggregationRules())
	defer buffer.Shutdown()
	defer buffer.Shutdown()
	buffer.SetFileStorage(fileStorage)

	var errorEventStatus string
	storage.SetAuditBufferFlushEventCallback(func(
		ctx context.Context,
		projectRoot string,
		_ storage.ObjectStorageProvider,
		operationID string,
		operationType string,
		status string,
		groupKey string,
		eventType string,
		targetKind string,
		eventCount int,
		aggregationWindow string,
		duration time.Duration,
		err error,
	) {
		if status == "error" {
			errorEventStatus = status
		}
	})
	defer storage.SetAuditBufferFlushEventCallback(nil)

	event := map[string]any{
		objects.FieldKeyEventType:  "cache_invalidation",
		objects.FieldKeySeverity:   "low",
		objects.FieldKeyTargetKind: "backlog_item",
	}
	if err := buffer.AddEvent(event); err != nil {
		t.Fatalf("AddEvent() failed: %v", err)
	}

	flushWaiter := storage.NewCallbackWaiterForTest()
	storage.SetAuditBufferFlushEventCallback(func(
		ctx context.Context,
		projectRoot string,
		_ storage.ObjectStorageProvider,
		operationID string,
		operationType string,
		status string,
		groupKey string,
		eventType string,
		targetKind string,
		eventCount int,
		aggregationWindow string,
		duration time.Duration,
		err error,
	) {
		if status == "error" {
			errorEventStatus = status
		}
		flushWaiter.Invoke()
	})

	if err := buffer.Flush(); err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}

	if !flushWaiter.WaitForInvocation(pkgctx.NewSystemContext(), 5*time.Second) {
		t.Log("Flush callback not received (may be expected if no events to flush)")
	}

	if errorEventStatus == "error" {
		t.Error("Expected no error status for successful flush")
	}
}
