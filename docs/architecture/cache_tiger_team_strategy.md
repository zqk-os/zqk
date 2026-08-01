# Concurrency Guardrail Addendum: High-Speed Object Caching (PRI-17807959)

## Overview
This addendum details the impact of introducing an asynchronous Semantic Cache in `pkg/storage` on the ZQK Scheduler (`pkg/scheduler/`). Because the scheduler operates on a highly concurrent but strictly bounded architecture, introducing asynchronous I/O requires explicit guardrails.

## 1. Goroutine Pool Impact
- **Current State:** The scheduler heavily regulates its concurrent operations using a budget-based goroutine tracking system (`goroutinelabels` package) to prevent exhaustion. The `PackageConcurrencyLimiter` and various worker pools apply strict bounds on concurrent execution. 
- **Impact of Async Cache:** Spawning unmanaged background goroutines within `pkg/storage` for cache pre-warming, writes, or invalidations could bypass the scheduler's ceiling. This risks triggering `dispatchWaitStageGoroutineCeiling` pressure events, dropping scheduler jobs, and causing resource starvation.
- **Guardrails:** 
  - The Semantic Cache MUST integrate with `goroutinelabels` (e.g., `goroutinelabels.NewGoroutine`) for any background work to ensure it counts against the global scheduler budget.
  - Implement a bounded worker pool within the cache layer itself to avoid uncontrolled spikes in concurrent cache read/write operations during high-volume convergence evaluations.

## 2. Context \u0026 Timeouts Impact
- **Current State:** Scheduler jobs and convergence phases execute under strict timeouts using `context.WithTimeout` (e.g., `runWithTimeout`, `KeyTimeoutSeconds`). Convergence drift-control relies on tight deadlines.
- **Impact of Async Cache:** If async cache checks or saves do not propagate context correctly, they may stall beyond the scheduler's deadline. This leads to orphaned goroutines, memory leaks, and "zombie" cache operations that hold locks after the job has already been aborted or timed out.
- **Guardrails:**
  - All asynchronous cache operations MUST accept and respect the caller's `context.Context`.
  - Cache operations must abort immediately upon context cancellation.
  - Any decoupled background cache writes must use a bounded, context-aware queue so that scheduler shutdown sequences can drain or safely cancel pending cache writes without hanging the scheduler process.

## Conclusion
The Semantic Cache in `pkg/storage` cannot operate blindly in the background. It must act as a well-behaved citizen of the scheduler's concurrency ecosystem by respecting `goroutinelabels` budgets and strictly adhering to `context.Context` cancellation policies.
