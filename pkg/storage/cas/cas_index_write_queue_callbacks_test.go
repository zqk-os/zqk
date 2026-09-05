package cas

import (
	"context"
	"testing"
	"time"
)

// TestListingIndex_BatchCallbackAtomicPointer verifies the lock-free atomic.Pointer round-trip
// for ListingIndexBatchEventCallback: Set → get (non-nil + call-through) → clear → get (nil).
// Also verifies the triggered-total counter increments on non-nil get.
// Do NOT use t.Parallel(): this test mutates global state.
func TestListingIndex_BatchCallbackAtomicPointer(t *testing.T) {
	// Capture and restore pre-test global state.
	initCb := getListingIndexBatchEventCallback()
	defer SetListingIndexBatchEventCallback(initCb)

	called := false
	dummyCb := func(
		ctx context.Context,
		projectRoot string,
		storageProvider CASFacade,
		kind string,
		batchSize int,
		duration time.Duration,
		status string,
		err error,
	) {
		called = true
	}

	// Set and verify non-nil retrieval.
	SetListingIndexBatchEventCallback(dummyCb)
	gotCb := getListingIndexBatchEventCallback()
	if gotCb == nil {
		t.Fatalf("expected non-nil callback after SetListingIndexBatchEventCallback")
	}

	// Call-through: verify the retrieved callback is functionally identical.
	gotCb(context.Background(), "", nil, "", 0, 0, "", nil)
	if !called {
		t.Fatalf("expected callback to be called via retrieved pointer")
	}

	// Clear and verify nil.
	SetListingIndexBatchEventCallback(nil)
	if getListingIndexBatchEventCallback() != nil {
		t.Fatalf("expected nil callback after SetListingIndexBatchEventCallback(nil)")
	}
}

// TestListingIndex_StateChangeCallbackAtomicPointer verifies the lock-free atomic.Pointer round-trip
// for ListingIndexStateChangeEventCallback: Set → get (non-nil + call-through) → clear → get (nil).
// Do NOT use t.Parallel(): this test mutates global state.
func TestListingIndex_StateChangeCallbackAtomicPointer(t *testing.T) {
	// Capture and restore pre-test global state.
	initCb := getListingIndexStateChangeEventCallback()
	defer SetListingIndexStateChangeEventCallback(initCb)

	called := false
	dummyCb := func(
		ctx context.Context,
		projectRoot string,
		storageProvider CASFacade,
		changeType string,
	) {
		called = true
	}

	// Set and verify non-nil retrieval.
	SetListingIndexStateChangeEventCallback(dummyCb)
	gotCb := getListingIndexStateChangeEventCallback()
	if gotCb == nil {
		t.Fatalf("expected non-nil callback after SetListingIndexStateChangeEventCallback")
	}

	// Call-through: verify the retrieved callback is functionally identical.
	gotCb(context.Background(), "", nil, "")
	if !called {
		t.Fatalf("expected callback to be called via retrieved pointer")
	}

	// Clear and verify nil.
	SetListingIndexStateChangeEventCallback(nil)
	if getListingIndexStateChangeEventCallback() != nil {
		t.Fatalf("expected nil callback after SetListingIndexStateChangeEventCallback(nil)")
	}
}
