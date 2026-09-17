package agentclaim

import (
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestCheckinWriteQueue(t *testing.T) {
	// Create a temporary project root
	tmpDir := t.TempDir()

	queue := &CheckinWriteQueue{
		items:      make(map[string]checkinWriteRequest),
		shutdownCh: make(chan chan struct{}),
		doneCh:     make(chan struct{}),
	}

	// Start the worker manually
	go queue.worker()

	timer1 := &CheckinTimer{
		TaskID:    "ATK-123",
		ClaimedBy: "AGENT-1",
		ExpiresAt: "2026-01-01T00:00:00Z",
	}

	// Enqueue an item
	queue.Enqueue(tmpDir, timer1)

	// Wait for the next tick (50ms + buffer)
	time.Sleep(100 * time.Millisecond)

	// Verify it was written
	path := CheckinTimerPath(tmpDir, "ATK-123")
	if !fileutil.Exists(path) {
		t.Errorf("Expected timer to be written to %s, but it was not", path)
	}

	// Stop the queue
	queue.Stop()
}

func TestCheckinWriteQueue_Reducer(t *testing.T) {
	// Test that multiple enqueues of the same task are reduced
	tmpDir := t.TempDir()

	queue := &CheckinWriteQueue{
		items:      make(map[string]checkinWriteRequest),
		shutdownCh: make(chan chan struct{}),
		doneCh:     make(chan struct{}),
	}

	timer1 := &CheckinTimer{
		TaskID:    "ATK-456",
		ClaimedBy: "AGENT-1",
		ExpiresAt: "2026-01-01T00:00:00Z",
	}
	timer2 := &CheckinTimer{
		TaskID:    "ATK-456",
		ClaimedBy: "AGENT-1",
		ExpiresAt: "2026-01-02T00:00:00Z", // Different expiry
	}

	// Enqueue before worker starts
	queue.Enqueue(tmpDir, timer1)
	queue.Enqueue(tmpDir, timer2)

	queue.mu.Lock()
	count := len(queue.items)
	queue.mu.Unlock()

	if count != 1 {
		t.Errorf("Expected 1 item in queue due to reducer, got %d", count)
	}
}
