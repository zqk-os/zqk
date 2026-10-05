package scheduler

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestPrioritizeCachePrewarmTriggers(t *testing.T) {
	t.Parallel()
	in := []JobTriggerRequest{
		{JobID: "SCH-run-bundle-1"},
		{JobID: DefaultCachePrewarmJobID},
		{JobID: "SCH-run-bundle-2"},
	}
	out := prioritizeCachePrewarmTriggers(in)
	if len(out) != 3 || out[0].JobID != DefaultCachePrewarmJobID {
		t.Fatalf("want SCH-cache-prewarm first, got %v", out)
	}
	if isSilentStaleTriggerDrop(DefaultCachePrewarmJobID) {
		t.Fatal("SCH-cache-prewarm must not use silent stale drop")
	}
}

func TestJobIDLooksLikeCASInstanceSchedulerJobID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		id     string
		expect bool
	}{
		{"SCH-1775021610902509000-965bf042", true},
		{"SCH-001", false},
		{"SCH-1775021610902509000", false},
		{"SCH-1775021610902509000-965bf0", false},
		{"SCH-1775021610902509000-965bf04g", false},
		{"SCH-123456789-965bf042", false},
	}
	for _, tc := range cases {
		got := jobIDLooksLikeCASInstanceSchedulerJobID(tc.id)
		if got != tc.expect {
			t.Errorf("jobIDLooksLikeCASInstanceSchedulerJobID(%q) = %v, want %v", tc.id, got, tc.expect)
		}
	}
	if !batchNeedsCASReconcileBeforeTriggerReload([]JobTriggerRequest{{JobID: "SCH-1775021610902509000-965bf042"}}) {
		t.Error("batchNeedsCASReconcileBeforeTriggerReload should be true for CAS-instance scheduler_job id")
	}
	if batchNeedsCASReconcileBeforeTriggerReload([]JobTriggerRequest{{JobID: "SCH-001"}}) {
		t.Error("batchNeedsCASReconcileBeforeTriggerReload should be false for short SCH-001 alone")
	}
}

func TestJobTriggerQueue_HasPendingTriggerWithOrigin(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	queue := NewJobTriggerQueue(tmpDir)
	has, err := queue.HasPendingTriggerWithOrigin("SCH-a", TriggerOriginDataCellEnvelopeTick)
	if err != nil || has {
		t.Fatalf("empty queue: err=%v has=%v", err, has)
	}
	if err := queue.EnqueueTriggerRequestWithOrigin("SCH-a", TriggerOriginDataCellEnvelopeTick); err != nil {
		t.Fatal(err)
	}
	has, err = queue.HasPendingTriggerWithOrigin("SCH-a", TriggerOriginDataCellEnvelopeTick)
	if err != nil || !has {
		t.Fatalf("want pending: err=%v has=%v", err, has)
	}
	has, err = queue.HasPendingTriggerWithOrigin("SCH-a", "other-origin")
	if err != nil || has {
		t.Fatalf("wrong origin should not match: err=%v has=%v", err, has)
	}
}

func TestJobTriggerQueue_EnqueueDequeue_SingleProcess(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	queue := NewJobTriggerQueue(tmpDir)

	// Enqueue a request
	if err := queue.EnqueueTriggerRequest("SCH-001"); err != nil {
		t.Fatalf("Failed to enqueue request: %v", err)
	}

	// Dequeue requests
	requests, err := queue.DequeueTriggerRequests(-1)
	if err != nil {
		t.Fatalf("Failed to dequeue requests: %v", err)
	}

	if len(requests) != 1 {
		t.Fatalf("Expected 1 request, got %d", len(requests))
	}

	if requests[0].JobID != "SCH-001" {
		t.Errorf("Expected job ID SCH-001, got %s", requests[0].JobID)
	}

	// Queue should be empty now
	requests, err = queue.DequeueTriggerRequests(-1)
	if err != nil {
		t.Fatalf("Failed to dequeue requests: %v", err)
	}

	if len(requests) != 0 {
		t.Errorf("Expected 0 requests after dequeue, got %d", len(requests))
	}
}

func TestJobTriggerQueue_EnqueueMultiple(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	queue := NewJobTriggerQueue(tmpDir)

	// Enqueue multiple requests
	jobIDs := []string{"SCH-001", "SCH-002", "SCH-003"}
	for _, jobID := range jobIDs {
		if err := queue.EnqueueTriggerRequest(jobID); err != nil {
			t.Fatalf("Failed to enqueue request %s: %v", jobID, err)
		}
	}

	// Dequeue all requests
	requests, err := queue.DequeueTriggerRequests(-1)
	if err != nil {
		t.Fatalf("Failed to dequeue requests: %v", err)
	}

	if len(requests) != len(jobIDs) {
		t.Fatalf("Expected %d requests, got %d", len(jobIDs), len(requests))
	}

	// Verify all job IDs are present
	requestedIDs := make(map[string]bool)
	for _, req := range requests {
		requestedIDs[req.JobID] = true
	}

	for _, jobID := range jobIDs {
		if !requestedIDs[jobID] {
			t.Errorf("Job ID %s not found in dequeued requests", jobID)
		}
	}
}

func TestJobTriggerQueue_EnqueueTriggerRequestsBatch(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	queue := NewJobTriggerQueue(tmpDir)

	jobIDs := []string{"SCH-A", "SCH-B", "SCH-C"}
	if err := queue.EnqueueTriggerRequests(jobIDs, ""); err != nil {
		t.Fatalf("EnqueueTriggerRequests: %v", err)
	}

	requests, err := queue.DequeueTriggerRequests(-1)
	if err != nil {
		t.Fatalf("DequeueTriggerRequests: %v", err)
	}
	if len(requests) != len(jobIDs) {
		t.Fatalf("expected %d requests, got %d", len(jobIDs), len(requests))
	}
	for i, req := range requests {
		if req.JobID != jobIDs[i] {
			t.Errorf("request[%d].JobID = %q, want %q", i, req.JobID, jobIDs[i])
		}
		if req.TriggerOrigin != emptyValue {
			t.Errorf("request[%d].TriggerOrigin = %q, want empty", i, req.TriggerOrigin)
		}
	}
}

func TestJobTriggerQueue_ConcurrentEnqueue(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	queue := NewJobTriggerQueue(tmpDir)

	const numGoroutines = 10
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	// Multiple goroutines enqueueing simultaneously
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("scheduler_test", "concurrent enqueue").StartSimple(func() {
			func(id int) {
				defer wg.Done()
				jobID := fmt.Sprintf("SCH-%03d", id)
				if err := queue.EnqueueTriggerRequest(jobID); err != nil {
					errors <- fmt.Errorf("Goroutine %d: Failed to enqueue: %w", id, err)
				}
			}(i)
		})
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Error(err)
	}

	// Dequeue all requests
	requests, err := queue.DequeueTriggerRequests(-1)
	if err != nil {
		t.Fatalf("Failed to dequeue requests: %v", err)
	}

	if len(requests) != numGoroutines {
		t.Errorf("Expected %d requests, got %d", numGoroutines, len(requests))
	}
}

func TestJobTriggerQueue_ConcurrentEnqueueDequeue(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	queue := NewJobTriggerQueue(tmpDir)

	const numEnqueuers = 5
	const numDequeuers = 2
	var wg sync.WaitGroup

	// Enqueuers
	for i := 0; i < numEnqueuers; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("scheduler_test", "concurrent enqueue").StartSimple(func() {
			func(id int) {
				defer wg.Done()
				jobID := fmt.Sprintf("SCH-ENQ-%03d", id)
				if err := queue.EnqueueTriggerRequest(jobID); err != nil {
					t.Errorf("Enqueuer %d: Failed to enqueue: %v", id, err)
				}
			}(i)
		})
	}

	// Dequeuers (non-blocking, so they may get empty results)
	for i := 0; i < numDequeuers; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("scheduler_test", "concurrent dequeue").StartSimple(func() {
			func(id int) {
				defer wg.Done()
				// Try to dequeue (may get empty if lock is held by enqueuer)
				requests, err := queue.DequeueTriggerRequests(-1)
				if err != nil {
					t.Errorf("Dequeuer %d: Failed to dequeue: %v", id, err)
				}
				_ = requests // May be empty, that's OK
			}(i)
		})
	}

	wg.Wait()

	// Final dequeue to get any remaining requests
	requests, err := queue.DequeueTriggerRequests(-1)
	if err != nil {
		t.Fatalf("Failed to final dequeue: %v", err)
	}

	// Should have at least some requests (may be less than numEnqueuers if dequeuers got some)
	if len(requests) > numEnqueuers {
		t.Errorf("Got more requests than enqueued: %d > %d", len(requests), numEnqueuers)
	}
}

func TestJobTriggerQueue_EmptyQueue(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	queue := NewJobTriggerQueue(tmpDir)

	// Dequeue from empty queue
	requests, err := queue.DequeueTriggerRequests(-1)
	if err != nil {
		t.Fatalf("Failed to dequeue from empty queue: %v", err)
	}

	if len(requests) != 0 {
		t.Errorf("Expected 0 requests from empty queue, got %d", len(requests))
	}
}

func TestJobTriggerQueue_EmptyQueue_DoesNotModifyDisk(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	queue := NewJobTriggerQueue(tmpDir)

	// 1. Enqueue and then dequeue all items so that queue.json exists on disk with "[]".
	if err := queue.EnqueueTriggerRequest("SCH-001"); err != nil {
		t.Fatalf("Failed to enqueue: %v", err)
	}
	dequeued, err := queue.DequeueTriggerRequests(10)
	if err != nil || len(dequeued) != 1 {
		t.Fatalf("Failed initial dequeue: err=%v len=%d", err, len(dequeued))
	}

	queueFile := filepath.Join(tmpDir, paths.ProjectDataDir, TriggerQueueDir, TriggerQueueFile)
	fiBefore, err := fileutil.Stat(queueFile)
	if err != nil {
		t.Fatalf("Expected queue file to exist: %v", err)
	}
	modTimeBefore := fiBefore.ModTime()

	// Small pause to ensure filesystem mtime would advance if modified
	time.Sleep(20 * time.Millisecond)

	// 2. Subsequent DequeueTriggerRequests on empty queue should NOT modify file or mtime
	for i := 0; i < 5; i++ {
		reqs, err := queue.DequeueTriggerRequests(5)
		if err != nil {
			t.Fatalf("Dequeue failed: %v", err)
		}
		if len(reqs) != 0 {
			t.Fatalf("Expected 0 requests, got %d", len(reqs))
		}
	}

	fiAfter, err := fileutil.Stat(queueFile)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if !fiAfter.ModTime().Equal(modTimeBefore) {
		t.Errorf("Queue file was rewritten on empty dequeue! Before=%v, After=%v", modTimeBefore, fiAfter.ModTime())
	}
}

func TestJobTriggerQueue_PeekTriggerRequests(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	queue := NewJobTriggerQueue(tmpDir)

	// Peek when queue file does not exist yet: returns empty, no error
	peeked, err := queue.PeekTriggerRequests()
	if err != nil {
		t.Fatalf("Peek on missing queue file should return no error, got: %v", err)
	}
	if len(peeked) != 0 {
		t.Errorf("Expected 0 from peek (no file yet), got %d", len(peeked))
	}

	// Enqueue two requests (creates queue file)
	if err := queue.EnqueueTriggerRequest("SCH-001"); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	if err := queue.EnqueueTriggerRequest("SCH-002"); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Peek sees both (does not remove)
	peeked, err = queue.PeekTriggerRequests()
	if err != nil {
		t.Fatalf("Peek after enqueue failed: %v", err)
	}
	if len(peeked) != 2 {
		t.Errorf("Expected 2 from peek, got %d", len(peeked))
	}

	// Dequeue removes all
	requests, err := queue.DequeueTriggerRequests(-1)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if len(requests) != 2 {
		t.Errorf("Expected 2 from dequeue, got %d", len(requests))
	}

	// Peek now sees empty (file exists but is empty)
	peeked, err = queue.PeekTriggerRequests()
	if err != nil {
		t.Fatalf("Peek after dequeue failed: %v", err)
	}
	if len(peeked) != 0 {
		t.Errorf("Expected 0 from peek after dequeue, got %d", len(peeked))
	}
}

func TestJobTriggerQueue_RequestMetadata(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	queue := NewJobTriggerQueue(tmpDir)

	// Enqueue a request
	jobID := "SCH-001"
	if err := queue.EnqueueTriggerRequest(jobID); err != nil {
		t.Fatalf("Failed to enqueue request: %v", err)
	}

	// Dequeue and verify metadata
	requests, err := queue.DequeueTriggerRequests(-1)
	if err != nil {
		t.Fatalf("Failed to dequeue requests: %v", err)
	}

	if len(requests) != 1 {
		t.Fatalf("Expected 1 request, got %d", len(requests))
	}

	req := requests[0]
	if req.JobID != jobID {
		t.Errorf("Expected job ID %s, got %s", jobID, req.JobID)
	}

	if req.RequestedBy == emptyValue {
		t.Error("RequestedBy should not be empty")
	}

	if req.RequestedAt.IsZero() {
		t.Error("RequestedAt should not be zero")
	}

	// RequestedAt should be recent (within last minute)
	if time.Since(req.RequestedAt) > 1*time.Minute {
		t.Errorf("RequestedAt should be recent, got %v", req.RequestedAt)
	}
}

func TestJobTriggerQueue_Persistence(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	queue1 := NewJobTriggerQueue(tmpDir)

	// Enqueue a request
	if err := queue1.EnqueueTriggerRequest("SCH-001"); err != nil {
		t.Fatalf("Failed to enqueue request: %v", err)
	}

	// Create a new queue instance (simulating different process)
	queue2 := NewJobTriggerQueue(tmpDir)

	// Dequeue from second queue instance
	requests, err := queue2.DequeueTriggerRequests(-1)
	if err != nil {
		t.Fatalf("Failed to dequeue requests: %v", err)
	}

	if len(requests) != 1 {
		t.Fatalf("Expected 1 request, got %d", len(requests))
	}

	if requests[0].JobID != "SCH-001" {
		t.Errorf("Expected job ID SCH-001, got %s", requests[0].JobID)
	}
}

func TestJobTriggerQueue_QueueFileFormat(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	queue := NewJobTriggerQueue(tmpDir)

	// Enqueue a request
	if err := queue.EnqueueTriggerRequest("SCH-001"); err != nil {
		t.Fatalf("Failed to enqueue request: %v", err)
	}

	// Read queue file directly
	queueFile := filepath.Join(tmpDir, paths.ProjectDataDir, TriggerQueueDir, TriggerQueueFile)
	data, err := fileutil.ReadFile(queueFile)
	if err != nil {
		t.Fatalf("Failed to read queue file: %v", err)
	}

	// Verify it's valid JSON
	var requests []JobTriggerRequest
	if err := json.Unmarshal(data, &requests); err != nil {
		t.Fatalf("Queue file is not valid JSON: %v", err)
	}

	if len(requests) != 1 {
		t.Fatalf("Expected 1 request in file, got %d", len(requests))
	}
}

// Note: WatchTriggerQueue tests are integration tests that require a real Scheduler instance.
// These are tested in scheduler integration tests. Unit tests focus on queue operations.

func TestJobIDLooksLikeCrossProcessTimestampTestRunner(t *testing.T) {
	t.Parallel()
	cases := []struct {
		id   string
		want bool
	}{
		{"SCH-1774181824", true},
		{"SCH-1234567890", true},
		{"SCH-1788056998365283000", false},
		{"SCH-001", false},
		{"SCH-123", false},
		{"SCH-run-test-1774181824", false},
		{"SCH-run-pkg-storage-1", false},
		{"OTHER-1774181824", false},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			got := jobIDLooksLikeCrossProcessTimestampTestRunner(tc.id)
			if got != tc.want {
				t.Errorf("jobIDLooksLikeCrossProcessTimestampTestRunner(%q) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}

func TestBatchNeedsCASReconcileBeforeTriggerReload(t *testing.T) {
	t.Parallel()
	run := func(name string, reqs []JobTriggerRequest, want bool) {
		t.Helper()
		got := batchNeedsCASReconcileBeforeTriggerReload(reqs)
		if got != want {
			t.Errorf("%s: got %v want %v", name, got, want)
		}
	}
	run("empty", nil, false)
	run("short_id", []JobTriggerRequest{{JobID: "SCH-001"}}, false)
	run("legacy_timestamp", []JobTriggerRequest{{JobID: "SCH-1774181824"}}, true)
	run("cli_submit_nanos", []JobTriggerRequest{{JobID: "SCH-1788056998365283000"}}, true)
	run("cli_submit_origin", []JobTriggerRequest{{JobID: "SCH-other", TriggerOrigin: TriggerOriginCLISubmit}}, true)
	run("run_prefix", []JobTriggerRequest{{JobID: "SCH-run-manual-1"}}, true)
	run("mixed", []JobTriggerRequest{{JobID: "SCH-001"}, {JobID: "SCH-1774181824"}}, true)
}

func TestJobTriggerQueue_ConcurrentEnqueueAndDequeue(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	queue := NewJobTriggerQueue(tmpDir)

	const producerCount = 4
	const itemsPerProducer = 10
	var wg sync.WaitGroup

	// Start concurrent producers
	for p := 0; p < producerCount; p++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("test.job_trigger_producer", "concurrent test producer").StartSimple(func() {
			defer wg.Done()
			for i := 0; i < itemsPerProducer; i++ {
				jobID := fmt.Sprintf("SCH-run-bundle-%d-%d", p, i)
				if err := queue.EnqueueTriggerRequest(jobID); err != nil {
					t.Errorf("EnqueueTriggerRequest failed: %v", err)
				}
				time.Sleep(2 * time.Millisecond)
			}
		})
	}

	// Concurrent consumer draining queue
	var dequeuedMu sync.Mutex
	dequeued := make(map[string]bool)
	stopConsumer := make(chan struct{})
	var consumerWg sync.WaitGroup

	consumerWg.Add(1)
	goroutinelabels.NewGoroutine("test.job_trigger_consumer", "concurrent test consumer").StartSimple(func() {
		defer consumerWg.Done()
		for {
			select {
			case <-stopConsumer:
				// Drain remainder
				reqs, _ := queue.DequeueTriggerRequests(100)
				dequeuedMu.Lock()
				for _, r := range reqs {
					dequeued[r.JobID] = true
				}
				dequeuedMu.Unlock()
				return
			default:
				reqs, err := queue.DequeueTriggerRequests(5)
				if err == nil && len(reqs) > 0 {
					dequeuedMu.Lock()
					for _, r := range reqs {
						dequeued[r.JobID] = true
					}
					dequeuedMu.Unlock()
				}
				time.Sleep(5 * time.Millisecond)
			}
		}
	})

	wg.Wait()
	close(stopConsumer)
	consumerWg.Wait()

	// Verify all items were dequeued
	dequeuedMu.Lock()
	totalDequeued := len(dequeued)
	dequeuedMu.Unlock()

	expectedTotal := producerCount * itemsPerProducer
	if totalDequeued != expectedTotal {
		t.Errorf("Concurrent enqueue/dequeue lost triggers: got %d, want %d", totalDequeued, expectedTotal)
	}

	// Verify peek reports empty queue after draining
	remaining, err := queue.PeekTriggerRequests()
	if err != nil {
		t.Fatalf("PeekTriggerRequests failed: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("expected empty queue, got %d remaining", len(remaining))
	}
}

func TestJobTriggerQueue_ShouldReconcileCASForBatch(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	queue := NewJobTriggerQueue(tmpDir).(*JobTriggerQueue)

	// When needsCAS is false, should never reconcile
	if queue.shouldReconcileCASForBatch(false, false) {
		t.Error("expected false when needsCAS is false and hasMissingJob is false")
	}
	if queue.shouldReconcileCASForBatch(true, false) {
		t.Error("expected false when needsCAS is false even if hasMissingJob is true")
	}

	// First time (lastCASReconcileAt is zero) with needsCAS=true: should reconcile
	if !queue.shouldReconcileCASForBatch(false, true) {
		t.Error("expected true on initial check when needsCAS is true")
	}

	// Mark reconciled
	queue.markCASReconciled()

	// Immediately after: within debounce interval and no missing job -> should be false
	if queue.shouldReconcileCASForBatch(false, true) {
		t.Error("expected false immediately after reconcile when no jobs missing")
	}

	// Cache miss always reconciles (CLI one-shots retry faster than any 2s floor).
	if !queue.shouldReconcileCASForBatch(true, true) {
		t.Error("expected true immediately after reconcile when a job is missing from cache")
	}

	// Simulate elapsed > 2s but < casDebounceInterval (15s)
	queue.mu.Lock()
	queue.lastCASReconcileAt = time.Now().Add(-3 * time.Second)
	queue.mu.Unlock()

	// No missing jobs -> still within 15s debounce, should be false
	if queue.shouldReconcileCASForBatch(false, true) {
		t.Error("expected false at 3s elapsed when casDebounceInterval is 15s")
	}
	// Missing job -> >= 2s elapsed, should be true!
	if !queue.shouldReconcileCASForBatch(true, true) {
		t.Error("expected true at 3s elapsed when job is missing")
	}

	// Simulate elapsed > casDebounceInterval (15s)
	queue.mu.Lock()
	queue.lastCASReconcileAt = time.Now().Add(-20 * time.Second)
	queue.mu.Unlock()

	// Now even with no missing jobs, debounce has elapsed -> should be true
	if !queue.shouldReconcileCASForBatch(false, true) {
		t.Error("expected true when debounce interval has elapsed")
	}

	// Test SetCASDebounceInterval
	queue.SetCASDebounceInterval(50 * time.Second)
	if queue.shouldReconcileCASForBatch(false, true) {
		t.Error("expected false after increasing debounce interval to 50s")
	}
}

func TestJobTriggerQueue_DrainAndProcessTriggerBatch_DebouncesCachedJobs(t *testing.T) {
	t.Parallel()
	testRoot := t.TempDir()
	jobID := "SCH-run-bundle-test-debounce"
	sched := &Scheduler{
		projectRoot: testRoot,
		jobs: map[string]*ScheduledJob{
			jobID: {
				ID:                jobID,
				JobType:           jobTypeRunWrapper,
				ConcurrentAllowed: true,
				Enabled:           true,
			},
		},
	}

	queue := NewJobTriggerQueue(testRoot).(*JobTriggerQueue)
	queue.SetCASDebounceInterval(30 * time.Second)
	// Mark as recently reconciled
	queue.markCASReconciled()
	initialReconcileAt := queue.lastCASReconcileAt

	// Enqueue trigger request for cached job
	if err := queue.EnqueueTriggerRequest(jobID); err != nil {
		t.Fatalf("EnqueueTriggerRequest: %v", err)
	}

	ctx := pkgctx.NewSystemContext()
	queue.drainAndProcessTriggerBatch(ctx, sched)

	// Verify lastCASReconcileAt did NOT change (reconcile was debounced)
	queue.mu.Lock()
	currentReconcileAt := queue.lastCASReconcileAt
	queue.mu.Unlock()
	if !currentReconcileAt.Equal(initialReconcileAt) {
		t.Errorf("expected CAS reconcile to be debounced, but lastCASReconcileAt changed from %v to %v", initialReconcileAt, currentReconcileAt)
	}
}

func TestJobTriggerQueue_GetTriggerQueueMutex(t *testing.T) {
	t.Parallel()

	pathA := "/tmp/test-lock-path-a.lock"
	pathB := "/tmp/test-lock-path-b.lock"

	muA1 := getTriggerQueueMutex(pathA)
	muA2 := getTriggerQueueMutex(pathA)
	muB := getTriggerQueueMutex(pathB)

	if muA1 == nil || muA2 == nil || muB == nil {
		t.Fatal("expected non-nil mutexes from getTriggerQueueMutex")
	}

	if muA1 != muA2 {
		t.Errorf("expected identical mutex instances for same path, got %p != %p", muA1, muA2)
	}

	if muA1 == muB {
		t.Errorf("expected distinct mutex instances for different paths, got identical %p", muA1)
	}

	// Concurrent retrieval safety
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("scheduler_test", "trigger queue mutex retrieval").StartSimple(func() {
			defer wg.Done()
			m := getTriggerQueueMutex(pathA)
			if m != muA1 {
				t.Errorf("concurrent getTriggerQueueMutex returned inconsistent mutex pointer")
			}
		})
	}
	wg.Wait()
}
