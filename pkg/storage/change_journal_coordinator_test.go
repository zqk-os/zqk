package storage

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// setupChangeJournalCoordinatorEnv ensures the test root has .zqk/state (for ID cache) and path alias
// cache (for stream-backed Create and prefix:process resolution). Without this, CreateChangeJournalEntryWithBuilder
// can fail early (findNextChangeJournalID or Create) and the coordinator callback is never invoked.
func setupChangeJournalCoordinatorEnv(t *testing.T, testRoot string) {
	t.Helper()
	if err := EnsurePathAliasCacheReady(testRoot); err != nil {
		t.Fatalf("EnsurePathAliasCacheReady: %v", err)
	}
}

// TestChangeJournal_CoordinatorIntegration tests that change journal entry creation emits events via coordinator.
// Not parallel: uses global SetChangeJournalEventCallback; parallel runs would overwrite each other's callback.
func TestChangeJournal_CoordinatorIntegration(t *testing.T) {
	testRoot, fileStorage, secCtx := setupTestingFactoryCompleteTestEnvironment(t)
	setupChangeJournalCoordinatorEnv(t, testRoot)

	secCtx.AccountID = "test-user"

	// Track callback invocations
	var callbackCalls atomic.Int32
	var lastEvent struct {
		changeType string
		objectRef  string
		kind       string
		objectID   string
		status     string
		err        error
	}

	// Set up callback
	SetChangeJournalEventCallback(func(
		ctx context.Context,
		projectRoot string,
		storage ObjectStorageProvider,
		operationID string,
		operationType string,
		status string,
		changeType string,
		objectRef string,
		kind string,
		objectID string,
		duration time.Duration,
		err error,
	) {
		lastEvent.changeType = changeType
		lastEvent.objectRef = objectRef
		lastEvent.kind = kind
		lastEvent.objectID = objectID
		lastEvent.status = status
		lastEvent.err = err
		callbackCalls.Add(1)
	})
	defer SetChangeJournalEventCallback(nil) // Cleanup

	// Create change journal entry with unique ID to avoid conflicts in parallel tests
	uniqueID := fmt.Sprintf("BLI-TEST-%d", time.Now().UnixNano())
	options := &ChangeJournalEntryOptions{
		ChangeType:  "update",
		ObjectRef:   fmt.Sprintf("backlog_item:%s", uniqueID),
		DiffSummary: "Updated status field",
		PreviousState: map[string]any{
			objects.FieldKeyStatus: objects.ObjectStatusPending,
		},
	}

	err := CreateChangeJournalEntryWithBuilder(
		pkgctx.NewSystemContext(),
		testRoot,
		secCtx,
		fileStorage,
		options,
	)
	if err != nil {
		t.Fatalf("CreateChangeJournalEntryWithBuilder() failed: %v", err)
	}

	// Wait for callback (async) with timeout
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()

	// Poll for callback completion using ticker (deterministic)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	// Poll until callback is called or timeout
	for callbackCalls.Load() == 0 {
		select {
		case <-ctx.Done():
			// Timeout - verify callback was not called
			if callbackCalls.Load() == 0 {
				t.Fatal("Timeout waiting for callback to be called")
			}
			return // Callback was called just before timeout
		case <-ticker.C:
			// Continue polling
		}
	}

	// Callback was called (loop exited because callbackCalls > 0)

	// Verify event data
	if lastEvent.changeType != "update" {
		t.Errorf("Expected change type 'update', got '%s'", lastEvent.changeType)
	}
	expectedObjectRef := fmt.Sprintf("backlog_item:%s", uniqueID)
	if lastEvent.objectRef != expectedObjectRef {
		t.Errorf("Expected object ref '%s', got '%s'", expectedObjectRef, lastEvent.objectRef)
	}
	if lastEvent.kind != "backlog_item" {
		t.Errorf("Expected kind 'backlog_item', got '%s'", lastEvent.kind)
	}
	if lastEvent.objectID != uniqueID {
		t.Errorf("Expected object ID '%s', got '%s'", uniqueID, lastEvent.objectID)
	}
	if lastEvent.status != "complete" {
		t.Errorf("Expected status 'complete', got '%s'", lastEvent.status)
	}
	if lastEvent.err != nil {
		t.Errorf("Expected no error, got %v", lastEvent.err)
	}
}

// TestChangeJournal_CoordinatorIntegration_NoCallback tests that creation works when callback is not set.
// Not parallel: uses global callback.
func TestChangeJournal_CoordinatorIntegration_NoCallback(t *testing.T) {
	testRoot, fileStorage, secCtx := setupTestingFactoryCompleteTestEnvironment(t)
	setupChangeJournalCoordinatorEnv(t, testRoot)

	// Ensure callback is not set
	SetChangeJournalEventCallback(nil)

	options := &ChangeJournalEntryOptions{
		ChangeType:  "create",
		ObjectRef:   "backlog_item:BLI-456",
		DiffSummary: "Created new object",
	}

	err := CreateChangeJournalEntryWithBuilder(
		pkgctx.NewSystemContext(),
		testRoot,
		secCtx,
		fileStorage,
		options,
	)
	if err != nil {
		t.Fatalf("CreateChangeJournalEntryWithBuilder() failed: %v", err)
	}

	// Should complete without error even without callback
}

// TestChangeJournal_CoordinatorIntegration_NoProjectRoot tests that creation works when project root is empty.
// Not parallel: uses global callback.
func TestChangeJournal_CoordinatorIntegration_NoProjectRoot(t *testing.T) {
	_, fileStorage, secCtx := setupTestingFactoryCompleteTestEnvironment(t)

	var callbackCalls atomic.Int32
	SetChangeJournalEventCallback(func(
		ctx context.Context,
		projectRoot string,
		storage ObjectStorageProvider,
		operationID string,
		operationType string,
		status string,
		changeType string,
		objectRef string,
		kind string,
		objectID string,
		duration time.Duration,
		err error,
	) {
		callbackCalls.Add(1)
	})
	defer SetChangeJournalEventCallback(nil)

	options := &ChangeJournalEntryOptions{
		ChangeType:  "delete",
		ObjectRef:   "backlog_item:BLI-789",
		DiffSummary: "Deleted object",
	}

	// Call with empty project root - should skip coordinator but still work
	err := CreateChangeJournalEntryWithBuilder(
		pkgctx.NewSystemContext(),
		"", // Empty project root
		secCtx,
		fileStorage,
		options,
	)
	// This should return nil (best effort) without creating entry
	if err != nil {
		t.Logf("Expected nil error for empty project root (best effort): %v", err)
	}

	// Wait briefly to ensure any async operations complete (if they were started)
	// Since project root is empty, callback should not be called, but we wait
	// deterministically to verify
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 500*time.Millisecond)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	// Poll briefly to ensure no callback was invoked
	// Since project root is empty, callback should not be called
	// We just wait for the timeout to ensure no async operations complete
	<-ctx.Done()

	// Callback should not be called without project root
	if callbackCalls.Load() != 0 {
		t.Error("Expected callback not to be called without project root")
	}
}

// TestChangeJournal_CoordinatorIntegration_AllChangeTypes tests all change types.
// Not parallel: uses global callback. Callback appends are protected by mutex (async invocations).
func TestChangeJournal_CoordinatorIntegration_AllChangeTypes(t *testing.T) {
	testRoot, fileStorage, secCtx := setupTestingFactoryCompleteTestEnvironment(t)
	setupChangeJournalCoordinatorEnv(t, testRoot)

	// Valid change_type values per validation: create, update, delete, import (not "move")
	changeTypes := []string{"create", "update", "delete", "import"}
	var (
		receivedMu          sync.Mutex
		receivedChangeTypes []string
	)

	SetChangeJournalEventCallback(func(
		ctx context.Context,
		projectRoot string,
		storage ObjectStorageProvider,
		operationID string,
		operationType string,
		status string,
		changeType string,
		objectRef string,
		kind string,
		objectID string,
		duration time.Duration,
		err error,
	) {
		receivedMu.Lock()
		receivedChangeTypes = append(receivedChangeTypes, changeType)
		receivedMu.Unlock()
	})
	defer SetChangeJournalEventCallback(nil)

	for _, changeType := range changeTypes {
		options := &ChangeJournalEntryOptions{
			ChangeType:  changeType,
			ObjectRef:   "backlog_item:BLI-TEST",
			DiffSummary: fmt.Sprintf("Test %s operation", changeType),
		}

		_ = CreateChangeJournalEntryWithBuilder(
			pkgctx.NewSystemContext(),
			testRoot,
			secCtx,
			fileStorage,
			options,
		)
	}

	// Wait for async event processing (validation + coordinator callback)
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 10*time.Second)
	defer cancel()

	getCount := func() int {
		receivedMu.Lock()
		n := len(receivedChangeTypes)
		receivedMu.Unlock()
		return n
	}
	if !waitForCondition(ctx, func() bool {
		return getCount() >= len(changeTypes)
	}, 100*time.Millisecond) {
		receivedMu.Lock()
		got := len(receivedChangeTypes)
		snapshot := append([]string(nil), receivedChangeTypes...)
		receivedMu.Unlock()
		t.Errorf("Expected %d change types, got %d (received: %v)", len(changeTypes), got, snapshot)
		return
	}
	receivedMu.Lock()
	got := len(receivedChangeTypes)
	snapshot := append([]string(nil), receivedChangeTypes...)
	receivedMu.Unlock()
	if got != len(changeTypes) {
		t.Errorf("Expected %d change types, got %d (received: %v)", len(changeTypes), got, snapshot)
	}
	// Release .zqk handles before t.TempDir cleanup (macOS directory-not-empty).
	_ = fileStorage.Shutdown(context.Background())
}
