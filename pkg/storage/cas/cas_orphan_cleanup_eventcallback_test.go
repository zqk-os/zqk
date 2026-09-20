package cas_test

import (
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"context"
	"testing"
	"time"
)

// TestOrphanCleanup_CallbackAtomicPointer verifies the lock-free atomic.Pointer round-trip
// for OrphanCleanupEventCallback: Set → get (non-nil + call-through) → clear → get (nil).
// Do NOT use t.Parallel(): this test mutates global state (globalOrphanCleanupEventCallback).
func TestOrphanCleanup_CallbackAtomicPointer(t *testing.T) {
	// Capture and restore pre-test global state.
	initCb := caspkg.GetOrphanCleanupEventCallbackForTest()
	defer caspkg.SetOrphanCleanupEventCallback(initCb)

	called := false
	dummyCb := func(
		ctx context.Context,
		projectRoot string,
		storage caspkg.CASFacade,
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
	caspkg.SetOrphanCleanupEventCallback(dummyCb)
	gotCb := caspkg.GetOrphanCleanupEventCallbackForTest()
	if gotCb == nil {
		t.Fatalf("expected non-nil callback after caspkg.SetOrphanCleanupEventCallback")
	}

	// Call-through: verify the retrieved callback is functionally identical.
	gotCb(context.Background(), "", nil, "", "", "", 0, 0, 0, 0, nil)
	if !called {
		t.Fatalf("expected callback to be called via retrieved pointer")
	}

	// Clear and verify nil.
	caspkg.SetOrphanCleanupEventCallback(nil)
	if caspkg.GetOrphanCleanupEventCallbackForTest() != nil {
		t.Fatalf("expected nil callback after caspkg.SetOrphanCleanupEventCallback(nil)")
	}
}
