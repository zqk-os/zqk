package graph

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// StateLocker defines a locking mechanism for graph nodes or files to prevent concurrent access by Swarm Workers.
type StateLocker interface {
	// Lock attempts to acquire a lock for a given resource ID.
	// The lock is held until the returned release function is called or the lockTTL expires.
	// If the lock cannot be acquired within waitTimeout, it returns an error.
	Lock(ctx context.Context, resourceID string, lockTTL time.Duration, waitTimeout time.Duration) (release func() error, err error)
}

// FencedStateLocker extends StateLocker to support monotonic fencing tokens and lease validity checking.
type FencedStateLocker interface {
	StateLocker
	// LockWithFence acquires a lock and issues a monotonically increasing fencing token.
	LockWithFence(ctx context.Context, resourceID string, lockTTL time.Duration, waitTimeout time.Duration) (release func() error, fenceToken int64, err error)
	// ValidateFence reports whether the issued fencing token is still the valid, unexpired active lease for the resource.
	ValidateFence(resourceID string, fenceToken int64) bool
}

// ErrLockTimeout is returned when waiting for a lock exceeds the timeout.
var ErrLockTimeout = fmt.Errorf("timeout waiting for lock")

// memoryStateLocker provides an in-memory implementation of StateLocker.
type memoryStateLocker struct {
	mu           sync.Mutex
	locks        map[string]lockState
	tokenCounter atomic.Int64
}

type lockState struct {
	expiresAt  time.Time
	ch         chan struct{}
	fenceToken int64
}

// NewMemoryStateLocker creates a new in-memory StateLocker.
func NewMemoryStateLocker() StateLocker {
	return &memoryStateLocker{
		locks: make(map[string]lockState),
	}
}

func (l *memoryStateLocker) Lock(ctx context.Context, resourceID string, lockTTL time.Duration, waitTimeout time.Duration) (func() error, error) {
	rel, _, err := l.LockWithFence(ctx, resourceID, lockTTL, waitTimeout)
	return rel, err
}

func (l *memoryStateLocker) LockWithFence(ctx context.Context, resourceID string, lockTTL time.Duration, waitTimeout time.Duration) (func() error, int64, error) {
	waitDeadline := time.Now().Add(waitTimeout)
	timer := time.NewTimer(waitTimeout)
	defer timer.Stop()

	for {
		l.mu.Lock()
		state, exists := l.locks[resourceID]

		// Handle orphan/expired locks gracefully
		if exists && time.Now().After(state.expiresAt) {
			close(state.ch)
			delete(l.locks, resourceID)
			exists = false
		}

		if !exists {
			ch := make(chan struct{})
			token := l.tokenCounter.Add(1)
			l.locks[resourceID] = lockState{
				expiresAt:  time.Now().Add(lockTTL),
				ch:         ch,
				fenceToken: token,
			}
			l.mu.Unlock()
			return func() error {
				l.mu.Lock()
				defer l.mu.Unlock()
				if st, ok := l.locks[resourceID]; ok && st.ch == ch {
					delete(l.locks, resourceID)
					close(ch)
				}
				return nil
			}, token, nil
		}
		waitCh := state.ch
		l.mu.Unlock()

		remaining := time.Until(waitDeadline)
		if remaining <= 0 {
			return nil, 0, ErrLockTimeout
		}

		select {
		case <-waitCh:
			// Lock was released or expired, try again
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case <-timer.C:
			return nil, 0, ErrLockTimeout
		}
	}
}

func (l *memoryStateLocker) ValidateFence(resourceID string, fenceToken int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	st, exists := l.locks[resourceID]
	if !exists {
		return false
	}
	if time.Now().After(st.expiresAt) {
		return false
	}
	return st.fenceToken == fenceToken
}
