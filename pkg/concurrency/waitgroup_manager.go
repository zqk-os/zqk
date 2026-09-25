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
	wgs      map[string]*waitGroupEntry
	mu       sync.RWMutex
	observer WaitGroupObserver // Optional observer for tracking
}

// waitGroupEntry tracks a WaitGroup and its metadata.
type waitGroupEntry struct {
	wg         *sync.WaitGroup
	createdAt  time.Time
	lastAccess time.Time
	operation  string // Operation type (e.g., "batch", "audit_event", "worker")
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
		wgs: make(map[string]*waitGroupEntry),
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
		entry.lastAccess = time.Now()
		m.mu.Unlock()
		return entry.wg
	}
	wg := &sync.WaitGroup{}
	entry := &waitGroupEntry{
		wg:         wg,
		createdAt:  time.Now(),
		lastAccess: time.Now(),
		operation:  operation,
	}
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
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, exists := m.wgs[id]
	if !exists {
		return nil
	}
	entry.lastAccess = time.Now()
	return entry.wg
}

// Add increments the WaitGroup counter for the given ID.
func (m *WaitGroupManager) Add(id string, delta int) {
	m.mu.RLock()
	entry, exists := m.wgs[id]
	observer := m.observer
	m.mu.RUnlock()

	if !exists {
		return
	}

	entry.wg.Add(delta)
	if observer != nil {
		observer.OnGroupAdd(id, delta)
	}
}

// Done decrements the WaitGroup counter for the given ID.
func (m *WaitGroupManager) Done(id string) {
	m.mu.RLock()
	entry, exists := m.wgs[id]
	observer := m.observer
	m.mu.RUnlock()

	if !exists {
		return
	}

	entry.wg.Done()
	if observer != nil {
		observer.OnGroupDone(id)
	}
}

// Wait blocks until the WaitGroup counter is zero.
func (m *WaitGroupManager) Wait(id string) {
	m.mu.RLock()
	entry, exists := m.wgs[id]
	observer := m.observer
	m.mu.RUnlock()

	if !exists {
		return
	}

	start := time.Now()
	if observer != nil {
		observer.OnGroupWait(id)
	}

	entry.wg.Wait()

	duration := time.Since(start)
	if observer != nil {
		observer.OnGroupCompleted(id, duration)
	}
}

// WaitWithTimeout blocks until the WaitGroup counter is zero or timeout expires.
func (m *WaitGroupManager) WaitWithTimeout(id string, timeout time.Duration) bool {
	m.mu.RLock()
	entry, exists := m.wgs[id]
	observer := m.observer
	m.mu.RUnlock()

	if !exists {
		return true
	}

	start := time.Now()
	if observer != nil {
		observer.OnGroupWait(id)
	}

	done := make(chan struct{})
	goroutinelabels.NewGoroutine("waitgroup_wait_timeout", "waiting for group completion").
		StartSimple(func() {
			entry.wg.Wait()
			close(done)
		})

	select {
	case <-done:
		duration := time.Since(start)
		if observer != nil {
			observer.OnGroupCompleted(id, duration)
		}
		return true
	case <-time.After(timeout):
		return false
	}
}

// WaitWithContext blocks until the WaitGroup counter is zero or context is cancelled.
func (m *WaitGroupManager) WaitWithContext(ctx context.Context, id string) error {
	m.mu.RLock()
	entry, exists := m.wgs[id]
	observer := m.observer
	m.mu.RUnlock()

	if !exists {
		return nil
	}

	start := time.Now()
	if observer != nil {
		observer.OnGroupWait(id)
	}

	done := make(chan struct{})
	goroutinelabels.NewGoroutine("waitgroup_wait_context", "waiting for group completion with context").
		StartSimple(func() {
			entry.wg.Wait()
			close(done)
		})

	select {
	case <-done:
		duration := time.Since(start)
		if observer != nil {
			observer.OnGroupCompleted(id, duration)
		}
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
		if now.Sub(entry.lastAccess) > maxAge {
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
