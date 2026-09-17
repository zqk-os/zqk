package storage_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// TestAuditEventBuffer_CoordinatorIntegration_EmptyBuffer tests flush with empty buffer
func TestAuditEventBuffer_CoordinatorIntegration_EmptyBuffer(t *testing.T) {
	testRoot, fileStorage, secCtx := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	buffer := storage.NewAuditEventBuffer(testRoot, secCtx, storage.DefaultAggregationRules())
	defer buffer.Shutdown()
	defer buffer.Shutdown()
	buffer.SetFileStorage(fileStorage)

	var callbackCalls atomic.Int32
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
		callbackCalls.Add(1)
	})
	defer storage.SetAuditBufferFlushEventCallback(nil)

	if err := buffer.Flush(); err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if callbackCalls.Load() != 0 {
		t.Error("Expected no callbacks for empty buffer flush")
	}
}

// TestAuditEventBuffer_CoordinatorIntegration_MultipleGroups tests flush with multiple groups
func TestAuditEventBuffer_CoordinatorIntegration_MultipleGroups(t *testing.T) {
	testRoot, fileStorage, secCtx := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	buffer := storage.NewAuditEventBuffer(testRoot, secCtx, storage.DefaultAggregationRules())
	defer buffer.Shutdown()
	defer buffer.Shutdown()
	buffer.SetFileStorage(fileStorage)

	var flushCount atomic.Int32
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
			flushCount.Add(1)
		}
	})
	defer storage.SetAuditBufferFlushEventCallback(nil)

	for i := 0; i < 5; i++ {
		event1 := map[string]any{
			objects.FieldKeyEventType:  "cache_invalidation",
			objects.FieldKeySeverity:   "low",
			objects.FieldKeyTargetKind: "backlog_item",
		}
		_ = buffer.AddEvent(event1)

		event2 := map[string]any{
			objects.FieldKeyEventType:  "cache_update",
			objects.FieldKeySeverity:   "low",
			objects.FieldKeyTargetKind: "backlog_item",
		}
		_ = buffer.AddEvent(event2)
	}

	if err := buffer.Flush(); err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}

	if !storage.WaitForConditionWithTimeoutForTest(
		pkgctx.NewSystemContext(),
		func() bool { return flushCount.Load() > 0 },
		5*time.Second,
		10*time.Millisecond,
	) {
		t.Errorf("Expected flush events for multiple groups within 5 seconds; observed flushCount=%d", flushCount.Load())
	}
}

// TestAuditEventBuffer_CoordinatorIntegration_NilStorage tests with nil storage
func TestAuditEventBuffer_CoordinatorIntegration_NilStorage(t *testing.T) {
	testRoot, _, secCtx := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	buffer := storage.NewAuditEventBuffer(testRoot, secCtx, storage.DefaultAggregationRules())
	defer buffer.Shutdown()
	defer buffer.Shutdown()

	var callbackCalls atomic.Int32
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
		callbackCalls.Add(1)
	})
	defer storage.SetAuditBufferFlushEventCallback(nil)

	event := map[string]any{
		objects.FieldKeyEventType:  "cache_invalidation",
		objects.FieldKeySeverity:   "low",
		objects.FieldKeyTargetKind: "backlog_item",
	}
	_ = buffer.AddEvent(event)

	if err := buffer.Flush(); err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if callbackCalls.Load() != 0 {
		t.Error("Expected callback not to be called without storage")
	}
}
