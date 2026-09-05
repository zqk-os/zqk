package concurrency

import (
	"sync"
)

// RunInLockOrLog executes the provided function within the lock.
// If acquiring the lock returns an error, it logs the error and returns it.
// If the function returns an error, it returns it.
func RunInLockOrLog(mu sync.Locker, lockName string, logger LockLogger, fn func() error) error {
	return RunInLockWithLogger(mu, lockName, logger, fn)
}

// RunInRLockOrLog executes the provided function within the read lock.
// If acquiring the lock returns an error, it logs the error and returns it.
// If the function returns an error, it returns it.
func RunInRLockOrLog(mu *sync.RWMutex, lockName string, logger LockLogger, fn func() error) error {
	return RunInRLockWithLogger(mu, lockName, logger, fn)
}
