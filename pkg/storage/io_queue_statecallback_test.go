package storage

import (
	"context"
	"testing"
)

// TestIOQueue_StateChangeCallbackAtomicPointer verifies the lock-free atomic.Pointer round-trip
// for IOQueueStateChangeEventCallback: Set → get (non-nil + call-through) → clear → get (nil).
// Do NOT use t.Parallel(): this test mutates global state (globalIOQueueStateChangeEventCallback).
func TestIOQueue_StateChangeCallbackAtomicPointer(t *testing.T) {
	// Capture and restore pre-test global state.
	initCb := getIOQueueStateChangeEventCallback()
	defer SetIOQueueStateChangeEventCallback(initCb)

	called := false
	dummyCb := func(
		ctx context.Context,
		projectRoot string,
		storageProvider ObjectStorageProvider,
		changeType string,
	) {
		called = true
	}

	// Set and verify non-nil retrieval.
	SetIOQueueStateChangeEventCallback(dummyCb)
	gotCb := getIOQueueStateChangeEventCallback()
	if gotCb == nil {
		t.Fatalf("expected non-nil callback after SetIOQueueStateChangeEventCallback")
	}

	// Call-through: verify the retrieved callback is functionally identical.
	gotCb(context.Background(), "", nil, "")
	if !called {
		t.Fatalf("expected callback to be called via retrieved pointer")
	}

	// Clear and verify nil.
	SetIOQueueStateChangeEventCallback(nil)
	if getIOQueueStateChangeEventCallback() != nil {
		t.Fatalf("expected nil callback after SetIOQueueStateChangeEventCallback(nil)")
	}
}
