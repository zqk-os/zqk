package scheduler

import (
	"context"

	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/hostload"
)

// globalTestJobSlotKey is the reserved key under which the global test budget is held.
// The key is not a Go package path, so it cannot collide with a real one.
//
// Global slots live on a dedicated limiter whose max-wait is the dispatch resource
// budget (default 2h), not the per-package limiter (default 2s). Sharing one limiter
// dropped run_wrapper jobs under ordinary maintenance load.
// TRACK: TDE-1789763617048880000-b8016f74 / BLI-1789866677572207000-df80dba2
const globalTestJobSlotKey = "__global_test_jobs__"

// globalTestJobLimit is the host-derived ceiling on concurrently dispatched run_wrapper jobs.
// TRACK: TDE-CEF-HOST-CPU-BACKPRESSURE-001 — shrink when the host is already CPU-bound
// (AV, other tenants) instead of always using the static NumCPU/2 budget.
func globalTestJobLimit() int {
	n := concurrency.GetGlobalConcurrencyConfig().SchedulerMaxConcurrentTestJobs
	return hostload.Scale(n, hostload.CurrentLevel())
}

// acquireGlobalTestJobSlot blocks until a slot in the global test budget is free, the global
// limiter's max-wait (dispatch resource budget, default 2h) elapses, or ctx is done.
//
// Returns nil when no global limiter is configured. That is deliberate: this gate exists to
// protect the host from oversubscription, and a scheduler constructed without a limiter (as
// several tests do) should keep dispatching rather than refuse every test job. The per-package
// gate takes the same position for the same reason.
func (s *Scheduler) acquireGlobalTestJobSlot(ctx context.Context) error {
	if s == nil || s.globalTestConcurrencyLimiter == nil {
		return nil
	}
	limit := globalTestJobLimit()
	if limit <= 0 {
		return nil
	}
	s.globalTestConcurrencyLimiter.MergeLimits(map[string]int{globalTestJobSlotKey: limit})
	return s.globalTestConcurrencyLimiter.Acquire(ctx, globalTestJobSlotKey)
}

// releaseGlobalTestJobSlot returns a slot to the global test budget.
func (s *Scheduler) releaseGlobalTestJobSlot() {
	if s == nil || s.globalTestConcurrencyLimiter == nil {
		return
	}
	if globalTestJobLimit() <= 0 {
		return
	}
	s.globalTestConcurrencyLimiter.Release(globalTestJobSlotKey)
}
