package storage

import (
	"context"
	"testing"
	"time"
)

// TestOrphanCleanup_CallbackAtomicPointer verifies the lock-free atomic.Pointer round-trip
// for OrphanCleanupEventCallback: Set → get (non-nil + call-through) → clear → get (nil).
// Do NOT use t.Parallel(): this test mutates global state (globalOrphanCleanupEventCallback).
func TestOrphanCleanup_CallbackAtomicPointer(t *testing.T) {
	// Capture and restore pre-test global state.
	initCb := getOrphanCleanupEventCallback()
	defer SetOrphanCleanupEventCallback(initCb)

	called := false
	dummyCb := func(
		ctx context.Context,
		projectRoot string,
		storage ObjectStorageProvider,
		operationID string,
		operationType string,
		status string,
		batchSize int,
		successCount int,
		failureCount int,
		duration time.Duration,
		failedFiles []string,
	) {
		called = true
	}

	// Set and verify non-nil retrieval.
	SetOrphanCleanupEventCallback(dummyCb)
	gotCb := getOrphanCleanupEventCallback()
	if gotCb == nil {
		t.Fatalf("expected non-nil callback after SetOrphanCleanupEventCallback")
	}

	// Call-through: verify the retrieved callback is functionally identical.
	gotCb(context.Background(), "", nil, "", "", "", 0, 0, 0, 0, nil)
	if !called {
		t.Fatalf("expected callback to be called via retrieved pointer")
	}

	// Clear and verify nil.
	SetOrphanCleanupEventCallback(nil)
	if getOrphanCleanupEventCallback() != nil {
		t.Fatalf("expected nil callback after SetOrphanCleanupEventCallback(nil)")
	}
}
