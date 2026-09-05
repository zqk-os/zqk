package storage

import (
	"context"
	"testing"
	"time"
)

// TestAuditBuffer_FlushCallbackAtomicPointer verifies the lock-free atomic.Pointer round-trip
// for AuditBufferFlushEventCallback: Set → get (non-nil + call-through) → clear → get (nil).
// Do NOT use t.Parallel(): this test mutates global state (globalAuditBufferFlushEventCallback).
func TestAuditBuffer_FlushCallbackAtomicPointer(t *testing.T) {
	// Capture and restore pre-test global state.
	initCb := getAuditBufferFlushEventCallback()
	defer SetAuditBufferFlushEventCallback(initCb)

	called := false
	dummyCb := func(
		ctx context.Context,
		projectRoot string,
		storage ObjectStorageProvider,
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
		called = true
	}

	// Set and verify non-nil retrieval.
	SetAuditBufferFlushEventCallback(dummyCb)
	gotCb := getAuditBufferFlushEventCallback()
	if gotCb == nil {
		t.Fatalf("expected non-nil callback after SetAuditBufferFlushEventCallback")
	}

	// Call-through: verify the retrieved callback is functionally identical.
	gotCb(context.Background(), "", nil, "", "", "", "", "", "", 0, "", 0, nil)
	if !called {
		t.Fatalf("expected callback to be called via retrieved pointer")
	}

	// Clear and verify nil.
	SetAuditBufferFlushEventCallback(nil)
	if getAuditBufferFlushEventCallback() != nil {
		t.Fatalf("expected nil callback after SetAuditBufferFlushEventCallback(nil)")
	}
}
