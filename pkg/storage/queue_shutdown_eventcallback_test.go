package storage

import (
	"context"
	"testing"
	"time"
)

// TestQueueShutdown_CallbackAtomicPointer verifies the lock-free atomic.Pointer round-trip
// for QueueShutdownEventCallback: Set → get (non-nil + call-through) → clear → get (nil).
// Do NOT use t.Parallel(): this test mutates global state (globalQueueShutdownEventCallback).
func TestQueueShutdown_CallbackAtomicPointer(t *testing.T) {
	// Capture and restore pre-test global state.
	initCb := getQueueShutdownEventCallback()
	defer SetQueueShutdownEventCallback(initCb)

	called := false
	dummyCb := func(
		ctx context.Context,
		queueName string,
		eventType string,
		pendingCount int64,
		isCritical bool,
		duration time.Duration,
		err error,
	) {
		called = true
	}

	// Set and verify non-nil retrieval.
	SetQueueShutdownEventCallback(dummyCb)
	gotCb := getQueueShutdownEventCallback()
	if gotCb == nil {
		t.Fatalf("expected non-nil callback after SetQueueShutdownEventCallback")
	}

	// Call-through: verify the retrieved callback is functionally identical.
	gotCb(context.Background(), "test-queue", "initiate", 0, false, 0, nil)
	if !called {
		t.Fatalf("expected callback to be called via retrieved pointer")
	}

	// Clear and verify nil.
	SetQueueShutdownEventCallback(nil)
	if getQueueShutdownEventCallback() != nil {
		t.Fatalf("expected nil callback after SetQueueShutdownEventCallback(nil)")
	}
}
