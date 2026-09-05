package concurrency

import (
	"context"
	"runtime"
	"time"
)

// DefaultGoroutineCeiling is the default max goroutine count before we block new work.
// When NumGoroutine() >= this, WaitUnderGoroutineCeiling blocks until count drops or ctx is done.
// Use at "gate" points (e.g. job submission, list slot acquire) so we don't add work when already overloaded.
const DefaultGoroutineCeiling = 2000

// GoroutineCountWarningThreshold is the count at which to log a warning (e.g. in health monitor).
// Should be below DefaultGoroutineCeiling so operators see "things slowing down" before new work is blocked.
const GoroutineCountWarningThreshold = 1500

// HeapAllocWarningThresholdBytes is the heap allocation size at which to log a memory bloat warning (512 MB).
const HeapAllocWarningThresholdBytes = 512 * 1024 * 1024

// WaitUnderGoroutineCeiling blocks until runtime.NumGoroutine() < ceiling or ctx is cancelled.
// pollInterval is how often we re-check (e.g. 200ms). Use before submitting work that would add goroutines
// (e.g. triggered jobs, or before starting a list operation) so the process doesn't grow unbounded.
// If ceiling <= 0, returns immediately (ceiling disabled).
func WaitUnderGoroutineCeiling(ctx context.Context, ceiling int, pollInterval time.Duration) error {
	if ceiling <= 0 {
		return nil
	}
	for runtime.NumGoroutine() >= ceiling {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
			// Re-check
		}
	}
	return nil
}
