package scheduler

import (
	"os"
	"testing"
	"time"

	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_JobTriggerQueue_DeepCoverage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-trigger-queue-deep-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Setenv("ZQK_TEST_ROOT", tmpDir)
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(t.Context())
		_ = os.RemoveAll(tmpDir)
	}()

	qIface := NewJobTriggerQueue(tmpDir)
	q := qIface.(*JobTriggerQueue)

	// 1. Debounce and reconciliation logic
	q.SetCASDebounceInterval(100 * time.Millisecond)
	if !q.shouldReconcileCASForBatch(true, true) {
		t.Errorf("expected true when hasMissingJob is true")
	}
	if q.shouldReconcileCASForBatch(false, false) {
		t.Errorf("expected false when needsCAS is false")
	}
	if !q.shouldReconcileCASForBatch(false, true) {
		t.Errorf("expected true on initial call (lastCASReconcileAt is zero)")
	}
	q.markCASReconciled()
	// Immediately after reconcile, debounce should prevent rerun
	if q.shouldReconcileCASForBatch(false, true) {
		t.Errorf("expected false immediately after markCASReconciled")
	}
	// After waiting > debounce interval
	time.Sleep(120 * time.Millisecond)
	if !q.shouldReconcileCASForBatch(false, true) {
		t.Errorf("expected true after debounce interval passed")
	}

	// 2. Initial empty queue checks
	peekEmpty, err := q.PeekTriggerRequests()
	if err != nil || len(peekEmpty) != 0 {
		t.Errorf("expected empty peek, got %v err %v", peekEmpty, err)
	}

	// 3. EnqueueLifecycleTrigger
	if err := q.EnqueueLifecycleTrigger("backlog_item", "exploring", "planned", map[string]any{"id": "BLI-1"}); err != nil {
		t.Fatalf("EnqueueLifecycleTrigger failed: %v", err)
	}

	// 4. EnqueueTriggerRequest
	if err := q.EnqueueTriggerRequest("SCH-test-job-1"); err != nil {
		t.Fatalf("EnqueueTriggerRequest failed: %v", err)
	}

	// Enqueue duplicate (should skip)
	if err := q.EnqueueTriggerRequest("SCH-test-job-1"); err != nil {
		t.Errorf("EnqueueTriggerRequest duplicate failed: %v", err)
	}

	// Check HasPendingTriggerWithOrigin
	hasPending, err := q.HasPendingTriggerWithOrigin("SCH-test-job-1", "")
	if err != nil || !hasPending {
		t.Errorf("expected hasPending true, got %v err %v", hasPending, err)
	}
	hasPendingNo, err := q.HasPendingTriggerWithOrigin("SCH-nonexistent", "")
	if err != nil || hasPendingNo {
		t.Errorf("expected hasPending false for nonexistent, got %v err %v", hasPendingNo, err)
	}

	// 5. EnqueueTriggerRequests batch
	batchJobs := []string{"SCH-batch-1", "SCH-batch-2"}
	if err := q.EnqueueTriggerRequests(batchJobs, "pre_commit"); err != nil {
		t.Fatalf("EnqueueTriggerRequests failed: %v", err)
	}
	// Empty batch should be no-op
	if err := q.EnqueueTriggerRequests(nil, ""); err != nil {
		t.Errorf("EnqueueTriggerRequests nil failed: %v", err)
	}

	// 6. EnqueueTriggerRequestStructs
	structs := []JobTriggerRequest{
		{
			JobID:         "SCH-struct-1",
			RequestedAt:   time.Now(),
			ReloadRetries: 1,
		},
	}
	if err := q.EnqueueTriggerRequestStructs(structs); err != nil {
		t.Fatalf("EnqueueTriggerRequestStructs failed: %v", err)
	}
	if err := q.EnqueueTriggerRequestStructs(nil); err != nil {
		t.Errorf("EnqueueTriggerRequestStructs nil failed: %v", err)
	}

	// 7. Peek and verify total count
	peek, err := q.PeekTriggerRequests()
	if err != nil {
		t.Fatalf("PeekTriggerRequests failed: %v", err)
	}
	// Expected: 1 lifecycle + 1 single + 2 batch + 1 struct = 5 items
	if len(peek) != 5 {
		t.Errorf("expected 5 items in queue, got %d", len(peek))
	}

	// 8. DequeueTriggerRequests with limit
	dequeuedPartial, err := q.DequeueTriggerRequests(2)
	if err != nil {
		t.Fatalf("DequeueTriggerRequests(2) failed: %v", err)
	}
	if len(dequeuedPartial) != 2 {
		t.Errorf("expected 2 dequeued items, got %d", len(dequeuedPartial))
	}

	dequeuedRemaining, err := q.DequeueTriggerRequests(10)
	if err != nil {
		t.Fatalf("DequeueTriggerRequests(10) failed: %v", err)
	}
	if len(dequeuedRemaining) != 3 {
		t.Errorf("expected 3 remaining dequeued items, got %d", len(dequeuedRemaining))
	}

	// Queue should now be empty
	peekAfter, _ := q.PeekTriggerRequests()
	if len(peekAfter) != 0 {
		t.Errorf("expected 0 items in peek after dequeue, got %d", len(peekAfter))
	}
}
