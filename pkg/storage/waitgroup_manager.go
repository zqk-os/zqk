package storage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
)

// WaitGroupManager manages WaitGroup lifecycle for async operations
// Provides centralized tracking and observability for all WaitGroups
type WaitGroupManager struct {
	wgs      map[string]*concurrency.WaitGroupEntry
	mu       sync.RWMutex
	observer WaitGroupObserver // Optional observer for tracking
}

// waitGroupEntry tracks a WaitGroup and its metadata
type waitGroupEntry = concurrency.WaitGroupEntry

// WaitGroupObserver can be implemented to track WaitGroup lifecycle events
type WaitGroupObserver = concurrency.WaitGroupObserver

// NewWaitGroupManager creates a new WaitGroupManager
func NewWaitGroupManager() *WaitGroupManager {
	return &WaitGroupManager{
		wgs: make(map[string]*concurrency.WaitGroupEntry),
	}
}

// SetObserver sets an observer for tracking WaitGroup lifecycle events
func (m *WaitGroupManager) SetObserver(observer WaitGroupObserver) {
	_ = concurrency.RunInLockOrLog(&m.mu, locknames.LockNameWaitgroupManagerSetObserver, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		m.observer = observer
		return nil
	})
}

// CreateGroup creates a new WaitGroup with the given ID and operation type
// Returns the WaitGroup for direct use if needed
func (m *WaitGroupManager) CreateGroup(id, operation string) *sync.WaitGroup {
	var wg *sync.WaitGroup
	var observer WaitGroupObserver
	err := concurrency.RunInLockWithLogger(&m.mu, locknames.LockNameWaitgroupManagerCreateGroup, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if entry, exists := m.wgs[id]; exists {
			entry.Touch()
			wg = entry.WG
			return nil
		}
		wg = &sync.WaitGroup{}
		m.wgs[id] = concurrency.NewWaitGroupEntry(wg, operation)
		observer = m.observer
		return nil
	})
	if err != nil {
		// Lock timeout - log and return nil to indicate failure
		StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Warn(LogEventStorageWaitGroupCreateLockTimeoutWarn).
			String("group_id", id).
			String("operation", operation).
			WithError(err).
			Log()
		return nil
	}

	// Notify observer outside lock
	if observer != nil {
		observer.OnGroupCreated(id, operation)
	}

	return wg
}

// CreateGroupForGoroutine creates a WaitGroup (or reuses existing) for use with goroutinelabels.WithWaitGroup(wg).
// Returns the WaitGroup; the builder will call Add(1) when the goroutine starts and Done() when it exits.
// If CreateGroup returns nil (e.g. lock timeout), nil is returned; pass it to WithWaitGroup(nil)—the builder no-ops Add/Done.
// Use this when starting a single goroutine: wg := manager.CreateGroupForGoroutine(id, op); NewGoroutine(...).WithWaitGroup(wg).StartSimple(fn).
func (m *WaitGroupManager) CreateGroupForGoroutine(id, operation string) *sync.WaitGroup {
	return m.CreateGroup(id, operation)
}

// GetGroup returns an existing WaitGroup by ID, or nil if it doesn't exist
func (m *WaitGroupManager) GetGroup(id string) *sync.WaitGroup {
	var wg *sync.WaitGroup
	_ = concurrency.RunInRLockOrLog(&m.mu, locknames.LockNameWaitgroupManagerGetGroup, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		entry, exists := m.wgs[id]
		if !exists {
			return nil
		}
		entry.Touch()
		wg = entry.WG
		return nil
	})
	return wg
}

func (m *WaitGroupManager) getGroupEntry(lockName, id string) (*concurrency.WaitGroupEntry, WaitGroupObserver, bool) {
	var entry *concurrency.WaitGroupEntry
	var exists bool
	var observer WaitGroupObserver
	_ = concurrency.RunInRLockOrLog(&m.mu, lockName, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		entry, exists = m.wgs[id]
		observer = m.observer
		return nil
	})
	return entry, observer, exists
}

// Add increments the WaitGroup counter for the given ID
// Panics if the group doesn't exist (to catch bugs early)
func (m *WaitGroupManager) Add(id string, delta int) {
	entry, observer, exists := m.getGroupEntry(locknames.LockNameWaitgroupManagerAdd, id)
	if !exists {
		// Under extreme contention or ID collision, another caller may have already deleted this group.
		// Log and no-op instead of panic so the scheduler does not crash (observe-hypothesize-test-verify).
		StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Warn(LogEventStorageWaitGroupAddMissingSkipWarn).
			String("group_id", id).
			Log()
		return
	}

	entry.WG.Add(delta)
	entry.Touch()

	// Notify observer outside lock
	if observer != nil {
		observer.OnGroupAdd(id, delta)
	}
}

// Done decrements the WaitGroup counter for the given ID.
// If the group does not exist (e.g. already deleted by another caller under contention), logs and no-ops instead of panicking.
func (m *WaitGroupManager) Done(id string) {
	entry, observer, exists := m.getGroupEntry(locknames.LockNameWaitgroupManagerDone, id)
	if !exists {
		// Group may have been deleted already (e.g. list_cas/list_parse race under contention). Log and no-op to avoid panic.
		StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Warn(LogEventStorageWaitGroupDoneMissingSkipWarn).
			String("group_id", id).
			Log()
		return
	}

	entry.WG.Done()
	entry.Touch()

	// Notify observer outside lock
	if observer != nil {
		observer.OnGroupDone(id)
	}
}

// Wait waits for the WaitGroup with the given ID to complete
// Returns immediately if the group doesn't exist
func (m *WaitGroupManager) Wait(id string) {
	entry, observer, exists := m.getGroupEntry(locknames.LockNameWaitgroupManagerWait, id)
	if !exists {
		return // Group doesn't exist, nothing to wait for
	}

	// Notify observer outside lock
	if observer != nil {
		observer.OnGroupWait(id)
	}

	// Wait with deterministic timeout
	startTime := time.Now()
	waitDone := make(chan struct{})
	// Use context with timeout for deterministic behavior
	waitCtx, waitCancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Minute)
	defer waitCancel()
	wgBud := goroutinelabels.DefaultBudget()
	wgWaitBuilder := goroutinelabels.NewGoroutine(fmt.Sprintf(ConstMiscWaitgroupWaitS, id), fmt.Sprintf(ConstMiscWaitingForWaitGroupS, id)).
		WithCleanup(func() {
			close(waitDone)
		})
	if wgBud != nil {
		wgWaitBuilder = wgWaitBuilder.WithBudget(wgBud)
	}
	wgWaitBuilder.StartWithContext(waitCtx, func(ctx context.Context) error {
		// WaitGroup.Wait() can't be cancelled, but we check context in the select below
		entry.WG.Wait()
		return nil
	})

	// Default timeout: 5 minutes (configurable per operation type if needed)
	// Timeout is handled via waitCtx.Done() above
	select {
	case <-waitDone:
		// WaitGroup completed normally
		duration := time.Since(startTime)
		if observer != nil {
			observer.OnGroupCompleted(id, duration)
		}
	case <-waitCtx.Done():
		// Timeout - log warning but don't fail (observer may want to track this)
		duration := time.Since(startTime)
		if observer != nil {
			observer.OnGroupCompleted(id, duration)
		}
		// Note: We still notify observer of completion (with timeout duration)
		// The observer can decide how to handle timeouts
	}
}

// DeleteGroup removes a WaitGroup from the manager
// Should only be called after Wait() has completed
func (m *WaitGroupManager) DeleteGroup(id string) {
	_ = concurrency.RunInLockOrLog(&m.mu, locknames.LockNameWaitgroupManagerDeleteGroup, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		delete(m.wgs, id)
		return nil
	})
}

// GetGroupInfo returns metadata about a WaitGroup
func (m *WaitGroupManager) GetGroupInfo(id string) (operation string, createdAt, lastAccess time.Time, exists bool) {
	var op string
	var created, accessed time.Time
	var found bool
	err := concurrency.RunInRLockWithLogger(&m.mu, locknames.LockNameWaitgroupManagerGetInfo, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		entry, found := m.wgs[id]
		if !found {
			return nil
		}
		op = entry.Operation
		created = entry.CreatedAt
		accessed = entry.LastAccess
		return nil
	})
	if err != nil {
		// Lock timeout - return false for exists
		StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Warn(LogEventStorageWaitGroupGetInfoLockTimeoutWarn).
			String("group_id", id).
			WithError(err).
			Log()
		return "", time.Time{}, time.Time{}, false
	}
	if !found {
		return "", time.Time{}, time.Time{}, false
	}
	return op, created, accessed, true
}

// ListGroups returns all WaitGroup IDs currently tracked
func (m *WaitGroupManager) ListGroups() []string {
	var ids []string
	_ = concurrency.RunInRLockOrLog(&m.mu, locknames.LockNameWaitgroupManagerListGroups, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		ids = make([]string, 0, len(m.wgs))
		for id := range m.wgs {
			ids = append(ids, id)
		}
		return nil
	})
	return ids
}

// Count returns the number of WaitGroups currently tracked
func (m *WaitGroupManager) Count() int {
	var count int
	_ = concurrency.RunInRLockOrLog(&m.mu, locknames.LockNameWaitgroupManagerCount, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		count = len(m.wgs)
		return nil
	})
	return count
}

// EnableLoggingObserver enables logging observer for this WaitGroupManager
// This is a convenience method that creates and sets a LoggingWaitGroupObserver
// If ctx is nil, uses system context
func (m *WaitGroupManager) EnableLoggingObserver(ctx context.Context) {
	observer := NewLoggingWaitGroupObserver(ctx)
	m.SetObserver(observer)
}
