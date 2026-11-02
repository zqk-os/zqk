package agentclaim

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCheckinWriteQueue(t *testing.T) {
	// Create a temporary project root
	tmpDir := t.TempDir()

	queue := &CheckinWriteQueue{
		items:      make(map[string]checkinWriteRequest),
		flushing:   make(map[string]checkinWriteRequest),
		shutdownCh: make(chan chan struct{}),
		doneCh:     make(chan struct{}),
	}

	// Start the worker manually using labeled goroutine
	goroutinelabels.NewGoroutine("test_checkin_write_queue", "test checkin queue worker").StartSimple(queue.worker)
	defer queue.Stop()

	timer1 := &CheckinTimer{
		TaskID:    "ATK-123",
		ClaimedBy: "AGENT-1",
		ExpiresAt: "2026-01-01T00:00:00Z",
	}

	// Enqueue an item
	queue.Enqueue(tmpDir, timer1)

	// Verify it was written using assert.Eventually with a 2-3s window to account for async channel dispatch under runner load
	path := CheckinTimerPath(tmpDir, "ATK-123")
	assert.Eventually(t, func() bool {
		return fileutil.Exists(path)
	}, 3*time.Second, 20*time.Millisecond, "Expected timer to be written to %s, but it was not", path)
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

func TestCheckinWriteQueue_Tombstone(t *testing.T) {
	tmpDir := t.TempDir()

	queue := &CheckinWriteQueue{
		items:      make(map[string]checkinWriteRequest),
		flushing:   make(map[string]checkinWriteRequest),
		tombstones: make(map[string]struct{}),
		shutdownCh: make(chan chan struct{}),
		doneCh:     make(chan struct{}),
	}
	goroutinelabels.NewGoroutine("test_checkin_tombstone", "test checkin queue worker").StartSimple(queue.worker)
	defer queue.Stop()

	timer := &CheckinTimer{
		TaskID:    "ATK-789",
		ClaimedBy: "AGENT-1",
		ExpiresAt: "2026-01-01T00:00:00Z",
	}

	queue.Enqueue(tmpDir, timer)
	path := CheckinTimerPath(tmpDir, "ATK-789")
	queue.Tombstone(path)

	if !queue.IsTombstoned(path) {
		t.Errorf("Expected path %s to be tombstoned", path)
	}

	if val, ok := queue.Get(path); ok || val != nil {
		t.Errorf("Expected Get on tombstoned path to return nil, false, got %v", val)
	}

	queue.FlushWait()

	if fileutil.Exists(path) {
		t.Errorf("Expected timer NOT to be written to %s after tombstone, but it exists", path)
	}
}
