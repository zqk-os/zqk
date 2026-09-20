package storage

import (
	"context"
	"testing"
	"time"
)

// TestHashRegistry_CallbackAtomicPointer verifies the lock-free atomic.Pointer round-trip
// for HashRegistryEventCallback: Set → get (non-nil + call-through) → clear → get (nil).
// Do NOT use t.Parallel(): this test mutates global state (globalHashRegistryEventCallback).
func TestHashRegistry_CallbackAtomicPointer(t *testing.T) {
	// Capture and restore pre-test global state.
	initCb := getHashRegistryEventCallback()
	defer SetHashRegistryEventCallback(initCb)

	called := false
	dummyCb := func(
		ctx context.Context,
		projectRoot string,
		storage ObjectStorageProvider,
		kind string,
		batchSize int,
		hashCount int,
		duration time.Duration,
		status string,
		err error,
	) {
		called = true
	}

	// Set and verify non-nil retrieval.
	SetHashRegistryEventCallback(dummyCb)
	gotCb := getHashRegistryEventCallback()
	if gotCb == nil {
		t.Fatalf("expected non-nil callback after SetHashRegistryEventCallback")
	}

	// Call-through: verify the retrieved callback is functionally identical.
	gotCb(context.Background(), "", nil, "", 0, 0, 0, "", nil)
	if !called {
		t.Fatalf("expected callback to be called via retrieved pointer")
	}

	// Clear and verify nil.
	SetHashRegistryEventCallback(nil)
	if getHashRegistryEventCallback() != nil {
		t.Fatalf("expected nil callback after SetHashRegistryEventCallback(nil)")
	}
}
