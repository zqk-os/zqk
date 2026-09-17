package scheduler

import (
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

func TestHeartbeatTracker_RegisterAndPing(t *testing.T) {
	tracker := NewHeartbeatTracker()

	workerID := "worker-alpha"
	meta := WorkerMeta{
		Kind:           "daemon",
		Interval:       100 * time.Millisecond,
		StaleThreshold: 250 * time.Millisecond,
	}

	if err := tracker.Register(workerID, meta); err != nil {
		t.Fatalf("unexpected register error: %v", err)
	}

	// Double register should fail
	if err := tracker.Register(workerID, meta); err == nil {
		t.Fatalf("expected error on duplicate register, got nil")
	}

	// Ping
	if err := tracker.Ping(workerID); err != nil {
		t.Fatalf("unexpected ping error: %v", err)
	}

	// Status check
	status, found := tracker.GetWorker(workerID)
	if !found {
		t.Fatalf("expected worker %s to be found", workerID)
	}
	if status.State != WorkerStateHealthy {
		t.Fatalf("expected healthy state, got %s", status.State)
	}

	// Unregister
	tracker.Unregister(workerID)
	if _, found := tracker.GetWorker(workerID); found {
		t.Fatalf("expected worker to be unregistered")
	}
}

func TestHeartbeatTracker_StaleDetection(t *testing.T) {
	tracker := NewHeartbeatTracker()

	workerID := "worker-stale"
	meta := WorkerMeta{
		Kind:           "subagent",
		Interval:       50 * time.Millisecond,
		StaleThreshold: 100 * time.Millisecond,
	}

	if err := tracker.Register(workerID, meta); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// Immediately shouldn't be stale
	stale := tracker.GetStaleWorkers(time.Now())
	if len(stale) != 0 {
		t.Fatalf("expected 0 stale workers, got %d", len(stale))
	}

	// After threshold, should be identified as stale
	futureTime := time.Now().Add(150 * time.Millisecond)
	stale = tracker.GetStaleWorkers(futureTime)
	if len(stale) != 1 || stale[0].ID != workerID {
		t.Fatalf("expected worker %s to be stale, got %+v", workerID, stale)
	}
}

func TestHeartbeatTracker_ConcurrentPings(t *testing.T) {
	tracker := NewHeartbeatTracker()

	workerCount := 20
	pingsPerWorker := 50

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		workerID := "worker-" + time.Duration(i).String()
		meta := WorkerMeta{
			Kind:           "task",
			Interval:       10 * time.Millisecond,
			StaleThreshold: 500 * time.Millisecond,
		}
		if err := tracker.Register(workerID, meta); err != nil {
			t.Fatalf("register error: %v", err)
		}

		wg.Add(1)
		id := workerID
		goroutinelabels.NewGoroutine("test.heartbeat_tracker", "concurrent ping worker").StartSimple(func() {
			defer wg.Done()
			for j := 0; j < pingsPerWorker; j++ {
				_ = tracker.Ping(id)
				time.Sleep(1 * time.Millisecond)
			}
		})
	}

	wg.Wait()

	workers := tracker.ListWorkers()
	if len(workers) != workerCount {
		t.Fatalf("expected %d workers, got %d", workerCount, len(workers))
	}
}
