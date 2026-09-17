package scheduler

import (
	"context"

	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/hostload"
)

// globalTestJobSlotKey is the reserved key under which the global test budget is held in the same
// MapConcurrencyLimiter that tracks per-package slots. Reusing that limiter means the waiter queue,
// max-wait behavior, and Snapshot reporting are the already-tested ones rather than a second
// implementation. The key is not a Go package path, so it cannot collide with a real one.
const globalTestJobSlotKey = "__global_test_jobs__"

// globalTestJobLimit is the host-derived ceiling on concurrently dispatched run_wrapper jobs.
// TRACK: TDE-CEF-HOST-CPU-BACKPRESSURE-001 — shrink when the host is already CPU-bound
// (AV, other tenants) instead of always using the static NumCPU/2 budget.
func globalTestJobLimit() int {
	n := concurrency.GetGlobalConcurrencyConfig().SchedulerMaxConcurrentTestJobs
	return hostload.Scale(n, hostload.CurrentLevel())
}

// acquireGlobalTestJobSlot blocks until a slot in the global test budget is free, the limiter's
// max-wait elapses, or ctx is done.
//
// Returns nil when no limiter is configured. That is deliberate: this gate exists to protect the
// host from oversubscription, and a scheduler constructed without a limiter (as several tests do)
// should keep dispatching rather than refuse every test job. The per-package gate takes the same
// position for the same reason.
func (s *Scheduler) acquireGlobalTestJobSlot(ctx context.Context) error {
	if s == nil || s.packageConcurrencyLimiter == nil {
		return nil
	}
	limit := globalTestJobLimit()
	if limit <= 0 {
		return nil
	}
	s.packageConcurrencyLimiter.MergeLimits(map[string]int{globalTestJobSlotKey: limit})
	return s.packageConcurrencyLimiter.Acquire(ctx, globalTestJobSlotKey)
}

// releaseGlobalTestJobSlot returns a slot to the global test budget.
func (s *Scheduler) releaseGlobalTestJobSlot() {
	if s == nil || s.packageConcurrencyLimiter == nil {
		return
	}
	if globalTestJobLimit() <= 0 {
		return
	}
	s.packageConcurrencyLimiter.Release(globalTestJobSlotKey)
}
