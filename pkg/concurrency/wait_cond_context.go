package concurrency

import (
	"context"
	"sync"
)

// WaitCondContext blocks the calling goroutine until ready returns true or ctx is cancelled.
// mu must be the mutex passed to sync.NewCond when cond was created. ready is always called with
// mu held and must return true when the wait should end.
//
// Cancellation uses [context.AfterFunc] to run cond.Broadcast while holding mu (required by
// sync.Cond), which wakes the waiter so it can observe ctx.Err() and return. The caller therefore
// waits in the same goroutine that holds the lock around cond.Wait, matching the transactional
// rule in README.md (do not run the predicate in another goroutine without the lock).
func WaitCondContext(ctx context.Context, cond *sync.Cond, mu *sync.Mutex, ready func() bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() {
		mu.Lock()
		cond.Broadcast()
		mu.Unlock()
	})
	defer stop()

	mu.Lock()
	defer mu.Unlock()
	for !ready() {
		if err := ctx.Err(); err != nil {
			return err
		}
		cond.Wait()
	}
	return nil
}
