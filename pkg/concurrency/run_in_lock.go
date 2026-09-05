package concurrency

import (
	"sync"
	"time"
)

// RunInLock runs fn in the same goroutine that holds the lock (transactional critical section).
// The lock is acquired, fn() is executed, then the lock is released. No other goroutine runs fn(),
// so shared state modified inside fn is never accessed without the lock.
//
// Use this pattern whenever you need to read or modify shared state under a mutex. It avoids
// the pitfall of lock helpers that run the callback in a separate goroutine (which would access
// shared state without holding the lock and can cause data races).
//
// Lock ordering: to avoid deadlocks, establish a global order when multiple locks are needed
// (e.g. always acquire lock A before lock B) and document it. Prefer holding one lock at a time
// and copying data out before releasing when possible.
//
// Example:
//
//	err := concurrency.RunInLock(&buf.mu, func() error {
//	    buf.items[key] = value
//	    return nil
//	})
func RunInLock(mu sync.Locker, fn func() error) error {
	mu.Lock()
	defer mu.Unlock()
	return fn()
}

// RunInRLock is the read-lock variant: runs fn in the same goroutine that holds the RLock.
// Use for read-only critical sections on RWMutex.
func RunInRLock(mu *sync.RWMutex, fn func() error) error {
	mu.RLock()
	defer mu.RUnlock()
	return fn()
}

// runInLockHoldThreshold is the duration above which we log when using RunInLockWithLogger.
const runInLockHoldThreshold = 100 * time.Millisecond

// RunInLockWithLogger is like RunInLock but logs when the lock is held longer than runInLockHoldThreshold.
// Use when you want observability for long-held locks without changing the same-goroutine guarantee.
func RunInLockWithLogger(mu sync.Locker, operation string, logger LockLogger, fn func() error) error {
	mu.Lock()
	holdStart := time.Now()
	defer func() {
		holdTime := time.Since(holdStart)
		if holdTime > runInLockHoldThreshold && logger != nil {
			logger.Debug("Lock held for extended period",
				LockField{Key: "operation", Value: operation},
				LockField{Key: "hold_time", Value: holdTime.String()})
		}
		mu.Unlock()
	}()
	return fn()
}

// RunInRLockWithLogger is like RunInRLock but logs when the lock is held longer than runInLockHoldThreshold.
func RunInRLockWithLogger(mu *sync.RWMutex, operation string, logger LockLogger, fn func() error) error {
	mu.RLock()
	holdStart := time.Now()
	defer func() {
		holdTime := time.Since(holdStart)
		if holdTime > runInLockHoldThreshold && logger != nil {
			logger.Debug("RLock held for extended period",
				LockField{Key: "operation", Value: operation},
				LockField{Key: "hold_time", Value: holdTime.String()})
		}
		mu.RUnlock()
	}()
	return fn()
}
