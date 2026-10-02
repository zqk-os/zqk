package concurrency

import (
	"context"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// WaitGroupManager manages WaitGroup lifecycle for concurrent operations.
// Provides centralized tracking, timeout observation, and cleanup for all WaitGroups.
type WaitGroupManager struct {
	wgs      map[string]*WaitGroupEntry
	mu       sync.RWMutex
	observer WaitGroupObserver // Optional observer for tracking
}

// WaitGroupEntry tracks a WaitGroup and its metadata.
type WaitGroupEntry struct {
	WG         *sync.WaitGroup
	CreatedAt  time.Time
	LastAccess time.Time
	Operation  string // Operation type (e.g., "batch", "audit_event", "worker")
}

// NewWaitGroupEntry creates a new WaitGroupEntry.
func NewWaitGroupEntry(wg *sync.WaitGroup, operation string) *WaitGroupEntry {
	now := time.Now()
	return &WaitGroupEntry{
		WG:         wg,
		CreatedAt:  now,
		LastAccess: now,
		Operation:  operation,
	}
}

// Touch updates the last access time.
func (e *WaitGroupEntry) Touch() {
	e.LastAccess = time.Now()
}

// WaitGroupObserver can be implemented to track WaitGroup lifecycle events.
type WaitGroupObserver interface {
	OnGroupCreated(id, operation string)
	OnGroupAdd(id string, delta int)
	OnGroupDone(id string)
	OnGroupWait(id string)
	OnGroupCompleted(id string, duration time.Duration)
}

// NewWaitGroupManager creates a new WaitGroupManager.
func NewWaitGroupManager() *WaitGroupManager {
	return &WaitGroupManager{
		wgs: make(map[string]*WaitGroupEntry),
	}
}

// SetObserver sets an observer for tracking WaitGroup lifecycle events.
func (m *WaitGroupManager) SetObserver(observer WaitGroupObserver) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observer = observer
}

// CreateGroup creates a new WaitGroup with the given ID and operation type.
// Returns the WaitGroup for direct use if needed.
func (m *WaitGroupManager) CreateGroup(id, operation string) *sync.WaitGroup {
	m.mu.Lock()
	var observer WaitGroupObserver
	if entry, exists := m.wgs[id]; exists {
		entry.Touch()
		m.mu.Unlock()
		return entry.WG
	}
	wg := &sync.WaitGroup{}
	entry := NewWaitGroupEntry(wg, operation)
	m.wgs[id] = entry
	observer = m.observer
	m.mu.Unlock()

	if observer != nil {
		observer.OnGroupCreated(id, operation)
	}

	return wg
}

// CreateGroupForGoroutine creates a WaitGroup (or reuses existing) for use with goroutinelabels.WithWaitGroup.
func (m *WaitGroupManager) CreateGroupForGoroutine(id, operation string) *sync.WaitGroup {
	return m.CreateGroup(id, operation)
}

// GetGroup returns an existing WaitGroup by ID, or nil if it doesn't exist.
func (m *WaitGroupManager) GetGroup(id string) *sync.WaitGroup {
	entry, _, exists := m.getEntryAndObserver(id)
	if !exists {
		return nil
	}
	entry.Touch()
	return entry.WG
}

func (m *WaitGroupManager) getEntryAndObserver(id string) (*WaitGroupEntry, WaitGroupObserver, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, exists := m.wgs[id]
	return entry, m.observer, exists
}

func (m *WaitGroupManager) recordCompleted(id string, observer WaitGroupObserver, start time.Time) {
	if observer != nil {
		observer.OnGroupCompleted(id, time.Since(start))
	}
}

func (m *WaitGroupManager) waitAsync(entry *WaitGroupEntry, label, desc string) <-chan struct{} {
	done := make(chan struct{})
	goroutinelabels.NewGoroutine(label, desc).
		StartSimple(func() {
			entry.WG.Wait()
			close(done)
		})
	return done
}

// Add increments the WaitGroup counter for the given ID.
func (m *WaitGroupManager) Add(id string, delta int) {
	entry, observer, exists := m.getEntryAndObserver(id)
	if !exists {
		return
	}
	entry.WG.Add(delta)
	if observer != nil {
		observer.OnGroupAdd(id, delta)
	}
}

// Done decrements the WaitGroup counter for the given ID.
func (m *WaitGroupManager) Done(id string) {
	entry, observer, exists := m.getEntryAndObserver(id)
	if !exists {
		return
	}
	entry.WG.Done()
	if observer != nil {
		observer.OnGroupDone(id)
	}
}

// Wait blocks until the WaitGroup counter is zero.
func (m *WaitGroupManager) Wait(id string) {
	entry, observer, exists := m.getEntryAndObserver(id)
	if !exists {
		return
	}
	start := time.Now()
	if observer != nil {
		observer.OnGroupWait(id)
	}
	entry.WG.Wait()
	m.recordCompleted(id, observer, start)
}

// WaitWithTimeout blocks until the WaitGroup counter is zero or timeout expires.
func (m *WaitGroupManager) WaitWithTimeout(id string, timeout time.Duration) bool {
	entry, observer, exists := m.getEntryAndObserver(id)
	if !exists {
		return true
	}

	start := time.Now()
	if observer != nil {
		observer.OnGroupWait(id)
	}

	done := m.waitAsync(entry, "waitgroup_wait_timeout", "waiting for group completion")
	select {
	case <-done:
		m.recordCompleted(id, observer, start)
		return true
	case <-time.After(timeout):
		return false
	}
}

// WaitWithContext blocks until the WaitGroup counter is zero or context is cancelled.
func (m *WaitGroupManager) WaitWithContext(ctx context.Context, id string) error {
	entry, observer, exists := m.getEntryAndObserver(id)
	if !exists {
		return nil
	}

	start := time.Now()
	if observer != nil {
		observer.OnGroupWait(id)
	}

	done := m.waitAsync(entry, "waitgroup_wait_context", "waiting for group completion with context")
	select {
	case <-done:
		m.recordCompleted(id, observer, start)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// DeleteGroup removes a WaitGroup from management.
func (m *WaitGroupManager) DeleteGroup(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.wgs, id)
}

// CleanupStaleGroups removes WaitGroups that haven't been accessed for the given maxAge.
func (m *WaitGroupManager) CleanupStaleGroups(maxAge time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	cleaned := 0

	for id, entry := range m.wgs {
		if now.Sub(entry.LastAccess) > maxAge {
			delete(m.wgs, id)
			cleaned++
		}
	}

	return cleaned
}

// GroupCount returns the number of active WaitGroups being managed.
func (m *WaitGroupManager) GroupCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.wgs)
}
