package community

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrDeadlockDetected is returned when acquiring a lock would create a cyclic wait dependency.
	ErrDeadlockDetected = errors.New("cyclic lock dependency detected; deadlock prevented")
	// ErrLockNotHeld is returned when an operation is attempted by a non-holder.
	ErrLockNotHeld = errors.New("lock is not currently held by caller")
	// ErrLockTimeout is returned when lock acquisition times out.
	ErrLockTimeout = errors.New("timed out waiting to acquire lock")
)

// ResourceLock represents an acquired resource lease.
type ResourceLock struct {
	ResourceID string
	HolderID   string
	ExpiresAt  time.Time
}

// ProviderLockManager coordinates distributed resource locking with deadlock cycle detection.
type ProviderLockManager struct {
	mu           sync.Mutex
	locks        map[string]*ResourceLock   // resourceID -> lock
	waitGraph    map[string]map[string]bool // waiterID -> map of heldIDs being waited on
	defaultLease time.Duration
}

// NewProviderLockManager creates an initialized deadlock-safe lock manager.
func NewProviderLockManager(defaultLease time.Duration) *ProviderLockManager {
	if defaultLease <= 0 {
		defaultLease = 5 * time.Second
	}
	return &ProviderLockManager{
		locks:        make(map[string]*ResourceLock),
		waitGraph:    make(map[string]map[string]bool),
		defaultLease: defaultLease,
	}
}

// hasCycle checks if adding an edge from waiter to holder creates a cycle in waitGraph.
func (lm *ProviderLockManager) hasCycle(waiter, targetHolder string) bool {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var dfs func(curr string) bool
	dfs = func(curr string) bool {
		visited[curr] = true
		recStack[curr] = true

		for next := range lm.waitGraph[curr] {
			if !visited[next] {
				if dfs(next) {
					return true
				}
			} else if recStack[next] {
				return true
			}
		}

		recStack[curr] = false
		return false
	}

	// Tentatively simulate adding the edge waiter -> targetHolder
	if lm.waitGraph[waiter] == nil {
		lm.waitGraph[waiter] = make(map[string]bool)
	}
	lm.waitGraph[waiter][targetHolder] = true
	defer delete(lm.waitGraph[waiter], targetHolder)

	return dfs(waiter)
}

// AcquireLock attempts to acquire a lock for providerID on resourceID.
func (lm *ProviderLockManager) AcquireLock(ctx context.Context, providerID, resourceID string) (*ResourceLock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	lm.mu.Lock()
	defer lm.mu.Unlock()

	now := time.Now()

	// Check if resource is currently locked and not expired
	lock, exists := lm.locks[resourceID]
	if exists && now.Before(lock.ExpiresAt) {
		if lock.HolderID == providerID {
			// Re-entrant acquisition extends lease
			lock.ExpiresAt = now.Add(lm.defaultLease)
			return lock, nil
		}

		// Would waiting create a cycle?
		if lm.hasCycle(providerID, lock.HolderID) {
			return nil, fmt.Errorf("%w: %s waiting on %s for resource %s", ErrDeadlockDetected, providerID, lock.HolderID, resourceID)
		}

		// Not a deadlock, but currently busy
		return nil, ErrLockTimeout
	}

	// Grant lock
	newLock := &ResourceLock{
		ResourceID: resourceID,
		HolderID:   providerID,
		ExpiresAt:  now.Add(lm.defaultLease),
	}
	lm.locks[resourceID] = newLock

	// Clear any wait edges for this provider on this resource
	delete(lm.waitGraph, providerID)

	return newLock, nil
}

// RenewLease extends the expiration time of an existing lock lease.
func (lm *ProviderLockManager) RenewLease(providerID, resourceID string, extension time.Duration) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lock, exists := lm.locks[resourceID]
	if !exists || lock.HolderID != providerID {
		return ErrLockNotHeld
	}

	lock.ExpiresAt = time.Now().Add(extension)
	return nil
}

// ReleaseLock frees the resource lock.
func (lm *ProviderLockManager) ReleaseLock(providerID, resourceID string) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lock, exists := lm.locks[resourceID]
	if !exists || lock.HolderID != providerID {
		return ErrLockNotHeld
	}

	delete(lm.locks, resourceID)
	delete(lm.waitGraph, providerID)
	return nil
}
