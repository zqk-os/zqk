package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// WorkerState represents the runtime health state of a registered worker.
type WorkerState string

const (
	WorkerStateHealthy   WorkerState = "healthy"
	WorkerStateStalled   WorkerState = "stalled"
	WorkerStateAborted   WorkerState = "aborted"
	WorkerStateCompleted WorkerState = "completed"
)

// WorkerMeta holds registration configuration for a worker.
type WorkerMeta struct {
	Kind           string
	Interval       time.Duration
	StaleThreshold time.Duration
	CancelFunc     context.CancelFunc
	Metadata       map[string]any
}

// WorkerStatus records snapshot telemetry for an actively tracked worker.
type WorkerStatus struct {
	ID           string
	Meta         WorkerMeta
	State        WorkerState
	RegisteredAt time.Time
	LastPingAt   time.Time
	MissedCount  int
}

// HeartbeatTracker provides thread-safe, low-latency tracking of active worker goroutines.
type HeartbeatTracker struct {
	mu      sync.RWMutex
	workers map[string]*WorkerStatus
}

var (
	globalHeartbeatTracker     *HeartbeatTracker
	globalHeartbeatTrackerOnce sync.Once
)

// GetGlobalHeartbeatTracker returns the kernel-wide singleton HeartbeatTracker.
func GetGlobalHeartbeatTracker() *HeartbeatTracker {
	globalHeartbeatTrackerOnce.Do(func() {
		globalHeartbeatTracker = NewHeartbeatTracker()
	})
	return globalHeartbeatTracker
}

// NewHeartbeatTracker creates an initialized HeartbeatTracker.
func NewHeartbeatTracker() *HeartbeatTracker {
	return &HeartbeatTracker{
		workers: make(map[string]*WorkerStatus),
	}
}

// Register adds a new worker to the tracker. Returns an error if the worker is already registered.
func (t *HeartbeatTracker) Register(workerID string, meta WorkerMeta) error {
	if workerID == "" {
		return fmt.Errorf("workerID cannot be empty")
	}
	if meta.StaleThreshold <= 0 {
		meta.StaleThreshold = 30 * time.Second
	}
	if meta.Interval <= 0 {
		meta.Interval = 5 * time.Second
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if _, exists := t.workers[workerID]; exists {
		return fmt.Errorf("worker %s is already registered", workerID)
	}

	now := time.Now()
	t.workers[workerID] = &WorkerStatus{
		ID:           workerID,
		Meta:         meta,
		State:        WorkerStateHealthy,
		RegisteredAt: now,
		LastPingAt:   now,
	}
	return nil
}

// Ping updates the LastPingAt timestamp of a registered worker.
func (t *HeartbeatTracker) Ping(workerID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	ws, exists := t.workers[workerID]
	if !exists {
		return fmt.Errorf("worker %s not registered", workerID)
	}

	ws.LastPingAt = time.Now()
	if ws.State == WorkerStateStalled {
		ws.State = WorkerStateHealthy
	}
	return nil
}

// Unregister removes a worker from tracking (e.g. upon normal exit).
func (t *HeartbeatTracker) Unregister(workerID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.workers, workerID)
}

// GetWorker returns a snapshot copy of a worker's status.
func (t *HeartbeatTracker) GetWorker(workerID string) (WorkerStatus, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	ws, exists := t.workers[workerID]
	if !exists {
		return WorkerStatus{}, false
	}
	return *ws, true
}

// GetStaleWorkers returns all workers that have not pinged within their StaleThreshold relative to the given reference time.
func (t *HeartbeatTracker) GetStaleWorkers(now time.Time) []WorkerStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var stale []WorkerStatus
	for _, ws := range t.workers {
		if ws.State == WorkerStateCompleted || ws.State == WorkerStateAborted {
			continue
		}
		if now.Sub(ws.LastPingAt) > ws.Meta.StaleThreshold {
			stale = append(stale, *ws)
		}
	}
	return stale
}

// MarkStalled updates a worker's state to WorkerStateStalled.
func (t *HeartbeatTracker) MarkStalled(workerID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if ws, exists := t.workers[workerID]; exists {
		ws.State = WorkerStateStalled
		ws.MissedCount++
	}
}

// ListWorkers returns a snapshot list of all tracked workers.
func (t *HeartbeatTracker) ListWorkers() []WorkerStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()

	list := make([]WorkerStatus, 0, len(t.workers))
	for _, ws := range t.workers {
		list = append(list, *ws)
	}
	return list
}

// Reset clears all registered workers (primarily for testing).
func (t *HeartbeatTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.workers = make(map[string]*WorkerStatus)
}
