package graph

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// StateLocker defines a locking mechanism for graph nodes or files to prevent concurrent access by Swarm Workers.
type StateLocker interface {
	// Lock attempts to acquire a lock for a given resource ID.
	// The lock is held until the returned release function is called or the lockTTL expires.
	// If the lock cannot be acquired within waitTimeout, it returns an error.
	Lock(ctx context.Context, resourceID string, lockTTL time.Duration, waitTimeout time.Duration) (release func() error, err error)
}

// ErrLockTimeout is returned when waiting for a lock exceeds the timeout.
var ErrLockTimeout = fmt.Errorf("timeout waiting for lock")

// memoryStateLocker provides an in-memory implementation of StateLocker.
type memoryStateLocker struct {
	mu    sync.Mutex
	locks map[string]lockState
}

type lockState struct {
	expiresAt time.Time
	ch        chan struct{}
}

// NewMemoryStateLocker creates a new in-memory StateLocker.
func NewMemoryStateLocker() StateLocker {
	return &memoryStateLocker{
		locks: make(map[string]lockState),
	}
}

func (l *memoryStateLocker) Lock(ctx context.Context, resourceID string, lockTTL time.Duration, waitTimeout time.Duration) (func() error, error) {
	waitDeadline := time.Now().Add(waitTimeout)
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
			l.locks[resourceID] = lockState{
				expiresAt: time.Now().Add(lockTTL),
				ch:        ch,
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
			}, nil
		}
		waitCh := state.ch
		l.mu.Unlock()

		select {
		case <-waitCh:
			// Lock was released or expired, try again
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Until(waitDeadline)):
			return nil, ErrLockTimeout
		}
	}
}
