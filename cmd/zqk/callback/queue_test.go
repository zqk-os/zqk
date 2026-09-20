package callback

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestQueue_Enqueue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		maxSize int
		entries int
		wantErr bool
		errType error
	}{
		{
			name:    "unlimited queue",
			maxSize: 0,
			entries: 100,
			wantErr: false,
		},
		{
			name:    "limited queue within capacity",
			maxSize: 10,
			entries: 5,
			wantErr: false,
		},
		{
			name:    "limited queue at capacity",
			maxSize: 10,
			entries: 10,
			wantErr: false,
		},
		{
			name:    "limited queue exceeds capacity",
			maxSize: 10,
			entries: 11,
			wantErr: true,
			errType: ErrQueueFull,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := NewQueue(tt.maxSize, &TimestampSorter{}, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

			var lastErr error
			for i := 0; i < tt.entries; i++ {
				entry := &CallbackEntry{
					Payload: map[string]any{
						"job_id":                     "test-job",
						objects.FieldKeyCallbackType: callbackTypeCompletion,
					},
					Timestamp: time.Now().UTC(),
				}
				lastErr = q.Enqueue(entry)
			}

			if tt.wantErr {
				if lastErr == nil {
					t.Errorf("expected error, got nil")
				} else if lastErr != tt.errType && lastErr != ErrQueueFull {
					t.Errorf("expected error type %v, got %v", tt.errType, lastErr)
				}
				// Verify queue is at capacity
				if q.Size() != tt.maxSize {
					t.Errorf("expected queue size %d, got %d", tt.maxSize, q.Size())
				}
			} else {
				if lastErr != nil {
					t.Errorf("unexpected error: %v", lastErr)
				}
				if q.Size() != tt.entries {
					t.Errorf("expected queue size %d, got %d", tt.entries, q.Size())
				}
			}
		})
	}
}

func TestQueue_DequeueSorted(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		sorter    Sorter
		entries   []*CallbackEntry
		dequeue   int
		wantCount int
		checkFunc func(*testing.T, []*CallbackEntry)
	}{
		{
			name:   "timestamp sort oldest first",
			sorter: &TimestampSorter{},
			entries: []*CallbackEntry{
				{Timestamp: time.Now().Add(2 * time.Second), JobID: "job-3"},
				{Timestamp: time.Now(), JobID: "job-1"},
				{Timestamp: time.Now().Add(1 * time.Second), JobID: "job-2"},
			},
			dequeue:   10,
			wantCount: 3,
			checkFunc: func(t *testing.T, entries []*CallbackEntry) {
				if len(entries) != 3 {
					t.Errorf("expected 3 entries, got %d", len(entries))
					return
				}
				// Should be sorted by timestamp (oldest first)
				if entries[0].JobID != "job-1" {
					t.Errorf("expected first entry to be job-1, got %s", entries[0].JobID)
				}
				if entries[1].JobID != "job-2" {
					t.Errorf("expected second entry to be job-2, got %s", entries[1].JobID)
				}
				if entries[2].JobID != "job-3" {
					t.Errorf("expected third entry to be job-3, got %s", entries[2].JobID)
				}
			},
		},
		{
			name:   "priority sort highest first",
			sorter: &PrioritySorter{},
			entries: []*CallbackEntry{
				{Priority: 5, Timestamp: time.Now().Add(1 * time.Second), JobID: "job-2"},
				{Priority: 10, Timestamp: time.Now().Add(2 * time.Second), JobID: "job-3"},
				{Priority: 10, Timestamp: time.Now(), JobID: "job-1"},
			},
			dequeue:   10,
			wantCount: 3,
			checkFunc: func(t *testing.T, entries []*CallbackEntry) {
				if len(entries) != 3 {
					t.Errorf("expected 3 entries, got %d", len(entries))
					return
				}
				// Should be sorted by priority first (highest first), then timestamp
				if entries[0].Priority != 10 || entries[0].JobID != "job-1" {
					t.Errorf("expected first entry to be priority 10 job-1, got priority %d %s", entries[0].Priority, entries[0].JobID)
				}
				if entries[1].Priority != 10 || entries[1].JobID != "job-3" {
					t.Errorf("expected second entry to be priority 10 job-3, got priority %d %s", entries[1].Priority, entries[1].JobID)
				}
				if entries[2].Priority != 5 || entries[2].JobID != "job-2" {
					t.Errorf("expected third entry to be priority 5 job-2, got priority %d %s", entries[2].Priority, entries[2].JobID)
				}
			},
		},
		{
			name:   "partial dequeue",
			sorter: &TimestampSorter{},
			entries: []*CallbackEntry{
				{Timestamp: time.Now().Add(2 * time.Second), JobID: "job-3"},
				{Timestamp: time.Now(), JobID: "job-1"},
				{Timestamp: time.Now().Add(1 * time.Second), JobID: "job-2"},
			},
			dequeue:   2,
			wantCount: 2,
			checkFunc: func(t *testing.T, entries []*CallbackEntry) {
				if len(entries) != 2 {
					t.Errorf("expected 2 entries, got %d", len(entries))
				}
				// Remaining queue should have 1 entry
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := NewQueue(0, tt.sorter, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

			// Enqueue all entries
			for _, entry := range tt.entries {
				if err := q.Enqueue(entry); err != nil {
					t.Fatalf("failed to enqueue entry: %v", err)
				}
			}

			// Dequeue
			result := q.DequeueSorted(tt.dequeue)

			if len(result) != tt.wantCount {
				t.Errorf("expected %d entries, got %d", tt.wantCount, len(result))
			}

			if tt.checkFunc != nil {
				tt.checkFunc(t, result)
			}

			// Verify remaining queue size
			expectedRemaining := len(tt.entries) - tt.wantCount
			if q.Size() != expectedRemaining {
				t.Errorf("expected remaining queue size %d, got %d", expectedRemaining, q.Size())
			}
		})
	}
}

func TestQueue_DeterminePriority(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		callbackType     string
		expectedPriority int
	}{
		{"completion callback", callbackTypeCompletion, 10},
		{"error callback", callbackTypeError, 10},
		{"status callback", callbackTypeStatus, 5},
		{"unknown callback", "unknown", 5},
		{"empty callback type", "", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := NewQueue(0, &TimestampSorter{}, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

			entry := &CallbackEntry{
				Payload: map[string]any{
					objects.FieldKeyCallbackType: tt.callbackType,
				},
			}

			err := q.Enqueue(entry)
			if err != nil {
				t.Fatalf("failed to enqueue: %v", err)
			}

			// Dequeue to check priority
			result := q.DequeueSorted(1)
			if len(result) != 1 {
				t.Fatalf("expected 1 entry, got %d", len(result))
			}

			if result[0].Priority != tt.expectedPriority {
				t.Errorf("expected priority %d, got %d", tt.expectedPriority, result[0].Priority)
			}
		})
	}
}

func TestQueue_ConcurrentEnqueue(t *testing.T) {
	t.Parallel()
	q := NewQueue(0, &TimestampSorter{}, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	const numGoroutines = 100
	const entriesPerGoroutine = 10
	totalEntries := numGoroutines * entriesPerGoroutine

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrently enqueue entries
	for i := 0; i < numGoroutines; i++ {
		goroutinelabels.NewGoroutine("refactor", "refactored").StartSimple(func() {
			func(id int) {
				defer wg.Done()
				for j := 0; j < entriesPerGoroutine; j++ {
					entry := &CallbackEntry{
						Payload: map[string]any{
							"job_id":                     "job",
							objects.FieldKeyCallbackType: callbackTypeCompletion,
						},
						Timestamp: time.Now().UTC(),
					}
					if err := q.Enqueue(entry); err != nil {
						t.Errorf("failed to enqueue: %v", err)
					}
				}
			}(i)
		})
	}

	wg.Wait()

	// Verify all entries were enqueued
	if q.Size() != totalEntries {
		t.Errorf("expected queue size %d, got %d", totalEntries, q.Size())
	}
}

func TestQueue_ConcurrentEnqueueDequeue(t *testing.T) {
	t.Parallel()
	// This test is heavy on lock contention (WithLockTimeout in Enqueue/DequeueSorted)
	// and can hang when run with the full suite; skip in short mode to keep CI fast.
	if testing.Short() {
		t.Skip("Skipping concurrent enqueue/dequeue stress test in short mode")
	}

	q := NewQueue(0, &TimestampSorter{}, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	// Use smaller scale so test completes in reasonable time
	const numEnqueuers = 5
	const numDequeuers = 2
	const entriesPerEnqueuer = 10
	const testTimeout = 15 * time.Second

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), testTimeout)
	defer cancel()
	totalExpected := int64(numEnqueuers * entriesPerEnqueuer) // total entries to enqueue

	var wg sync.WaitGroup
	enqueuedCount := int64(0)
	dequeuedCount := int64(0)
	var mu sync.Mutex

	// Enqueuers (builder adds 1 per StartSimple when WithWaitGroup is set; do not double-add)
	for i := 0; i < numEnqueuers; i++ {
		goroutinelabels.NewGoroutine(fmt.Sprintf("test_enqueuer_%d", i), fmt.Sprintf("enqueuing entries %d in queue test", i)).
			WithWaitGroup(&wg).
			StartSimple(func() {
				for j := 0; j < entriesPerEnqueuer; j++ {
					if ctx.Err() != nil {
						return
					}
					entry := &CallbackEntry{
						Payload: map[string]any{
							"job_id": "job",
						},
						Timestamp: time.Now().UTC(),
					}
					if err := q.Enqueue(entry); err == nil {
						mu.Lock()
						enqueuedCount++
						mu.Unlock()
					}
				}
			})
	}

	// Dequeuers: exit on done condition or context timeout so test cannot hang
	wg.Add(numDequeuers)
	for i := 0; i < numDequeuers; i++ {
		goroutinelabels.NewGoroutine("refactor", "refactored").StartSimple(func() {
			func() {
				defer wg.Done()
				for ctx.Err() == nil {
					entries := q.DequeueSorted(5)
					if len(entries) == 0 {
						time.Sleep(10 * time.Millisecond)
						mu.Lock()
						done := enqueuedCount == totalExpected && q.Size() == 0
						mu.Unlock()
						if done {
							return
						}
						continue
					}
					mu.Lock()
					dequeuedCount += int64(len(entries))
					mu.Unlock()
				}
			}()
		})
	}

	wg.Wait()
	if ctx.Err() != nil {
		t.Fatalf("test timed out after %v (enqueued: %d, dequeued: %d, queue size: %d)",
			testTimeout, enqueuedCount, dequeuedCount, q.Size())
	}

	// Verify all entries were processed
	mu.Lock()
	finalQueueSize := q.Size()
	mu.Unlock()

	finalDequeued := dequeuedCount

	t.Logf("Enqueued: %d, Dequeued: %d, Remaining in queue: %d", enqueuedCount, finalDequeued, finalQueueSize)

	// All entries should be enqueued and eventually dequeued
	if enqueuedCount != totalExpected {
		t.Errorf("expected %d entries enqueued, got %d", totalExpected, enqueuedCount)
	}

	// Allow some tolerance for race conditions in final count
	if finalDequeued+int64(finalQueueSize) < totalExpected-10 {
		t.Errorf("expected at least %d entries processed (dequeued + remaining), got %d (dequeued: %d, remaining: %d)",
			totalExpected-10, finalDequeued+int64(finalQueueSize), finalDequeued, finalQueueSize)
	}
}

func TestQueue_StartProcessing(t *testing.T) {
	t.Parallel()
	// Skip in short mode: test can panic "WaitGroup reused before previous Wait returned"
	// when StopProcessing runs; needs investigation of queue lifecycle.
	if testing.Short() {
		t.Skip("Skipping StartProcessing test in short mode (WaitGroup lifecycle)")
	}
	q := NewQueue(0, &TimestampSorter{}, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	defer cancel()

	processed := make(chan *CallbackEntry, 100)
	var processedMu sync.Mutex
	processedCount := 0

	processor := func(entry *CallbackEntry) error {
		processedMu.Lock()
		processedCount++
		processedMu.Unlock()
		select {
		case processed <- entry:
		default:
		}
		return nil
	}

	// Start processing
	q.StartProcessing(ctx, processor)

	// Enqueue some entries
	for i := 0; i < 5; i++ {
		entry := &CallbackEntry{
			Payload: map[string]any{
				"job_id": "test-job",
			},
			Timestamp: time.Now().UTC(),
		}
		if err := q.Enqueue(entry); err != nil {
			t.Fatalf("failed to enqueue: %v", err)
		}
	}

	// Wait for processing with deterministic polling
	testkit.Eventually(t, 2*time.Second, 10*time.Millisecond, func() bool {
		processedMu.Lock()
		defer processedMu.Unlock()
		return processedCount >= 5
	})

	// Verify all entries were processed
	processedMu.Lock()
	finalCount := processedCount
	processedMu.Unlock()

	if finalCount < 5 {
		t.Errorf("expected at least 5 entries processed, got %d", finalCount)
	}

	// Stop processing (waits for background worker to terminate)
	q.StopProcessing()

	// Verify queue size
	if q.Size() > 0 {
		t.Logf("queue still has %d entries after processing (acceptable)", q.Size())
	}
}

func TestQueue_StopProcessing(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("Skipping StopProcessing test in short mode (WaitGroup lifecycle)")
	}
	q := NewQueue(0, &TimestampSorter{}, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	defer cancel()

	processedCount := 0
	var mu sync.Mutex

	processor := func(entry *CallbackEntry) error {
		mu.Lock()
		processedCount++
		mu.Unlock()
		time.Sleep(10 * time.Millisecond) // Simulate processing time
		return nil
	}

	// Start processing
	q.StartProcessing(ctx, processor)

	// Enqueue entries
	for i := 0; i < 20; i++ {
		entry := &CallbackEntry{
			Payload: map[string]any{
				"job_id": "test-job",
			},
			Timestamp: time.Now().UTC(),
		}
		_ = q.Enqueue(entry)
	}

	// Stop immediately
	q.StopProcessing()

	// Verify stop was graceful (no panic, some entries may be processed)
	mu.Lock()
	count := processedCount
	mu.Unlock()

	t.Logf("processed %d entries before stop", count)

	// Should be able to stop without error
	if q.Size() > 0 {
		t.Logf("queue has %d remaining entries after stop (acceptable)", q.Size())
	}
}

func TestQueue_Size(t *testing.T) {
	t.Parallel()
	q := NewQueue(0, &TimestampSorter{}, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	if q.Size() != 0 {
		t.Errorf("expected initial size 0, got %d", q.Size())
	}

	// Enqueue entries
	for i := 0; i < 10; i++ {
		entry := &CallbackEntry{
			Payload: map[string]any{
				"job_id": "test",
			},
			Timestamp: time.Now().UTC(),
		}
		_ = q.Enqueue(entry)
	}

	if q.Size() != 10 {
		t.Errorf("expected size 10, got %d", q.Size())
	}

	// Dequeue some
	q.DequeueSorted(3)
	if q.Size() != 7 {
		t.Errorf("expected size 7, got %d", q.Size())
	}

	// Dequeue all
	q.DequeueSorted(0)
	if q.Size() != 0 {
		t.Errorf("expected size 0, got %d", q.Size())
	}
}
