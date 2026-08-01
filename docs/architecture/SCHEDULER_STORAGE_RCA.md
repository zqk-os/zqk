# Scheduler and Storage: Root Cause Analysis and Performance Baselines

**Purpose:** Root cause analysis for the scheduler overload and storage/list issues addressed in 2026-02, plus lessons learned and baseline metrics to form foundational perf specs.

**Related:** [SCHEDULER_OVERLOAD_AND_TIMEOUT.md](./SCHEDULER_OVERLOAD_AND_TIMEOUT.md), [PERFORMANCE_BOTTLENECK_AUDIT.md](./PERFORMANCE_BOTTLENECK_AUDIT.md), [DIAGNOSTIC_REPORT_MINING.md](./DIAGNOSTIC_REPORT_MINING.md), [LESSONS_LEARNED.md](./LESSONS_LEARNED.md).

---

## 1. Root causes (summary)

| Cause | Mechanism | Evidence |
|-------|-----------|----------|
| **HashRegistry per call** | Every create/update/delete/move (and CAS verify) created a new `HashRegistry` and, on first `Save()`, started a `startSaveWorker` goroutine. No reuse per (kind, dir). | Goroutine dumps showed hundreds of goroutines in `HashRegistry.startSaveWorker` with many distinct registries (e.g. 061124: 1362 goroutines, ~1244 mentions of startSaveWorker/HashRegistry). |
| **List/count slot contention** | Only 8 concurrent List/Count operations; many jobs (audit aggregation, retention) blocked on `AcquireListCountSlot`. | 177+ goroutines in `AcquireListCountSlot`; jobs timing out after 30–45 min. |
| **Goroutine ceiling blocking submission** | Before submitting timer/lifecycle jobs, scheduler waited under 2000 goroutines. With 1k+ goroutines (or 1M in extreme case), count never dropped, so no new jobs were submitted. | Critical jobs never scheduled; only already-queued work ran. |
| **Cache prewarm tier-3 wait** | Tier-3 tasks used job `ctx` instead of 50s timeout context; one goroutine ran `wgTier3.Wait()` with no timeout, so it could block forever. | One goroutine stuck in `Wait()` for 98+ minutes. |
| **NewFileObjectStorage per update (change path)** | Change notification or similar paths created a new `FileObjectStorage` per update instead of using the global storage provider cache. | Extra load and cache duplication; fixed by using global cache. |

---

## 2. Fixes implemented

1. **HashRegistry cache** — `FileObjectStorage` caches `*HashRegistry` per `(kind, dir)` via `hashRegistryCache` (`ResourceCache[*HashRegistry]`). Same (kind, dir) reuses one registry and at most one save worker. **Audit path:** `WriteSystemObjectAndRegisterHash` uses `fileStorage.newHashRegistry(...)` when `fileStorage != nil`.
2. **List/count concurrency** — `listCountMaxConcurrent` increased from 8 to 16 in `pkg/storage/list_count_concurrency.go`.
3. **Goroutine ceiling bypass** — Critical job types (`audit_event_aggregation`, `retention_tolerance`) skip `WaitUnderGoroutineCeiling` so they are still submitted when over ceiling (they still run in the same pool and can timeout on their own).
4. **Cache prewarm tier-3** — Tier-3 tasks receive a 50s timeout context (`tier3Ctx`) so they exit and call `Done()`; the goroutine that runs `wgTier3.Wait()` no longer blocks indefinitely.
5. **Change notification / scheduler storage** — Use global storage provider cache instead of creating a new `NewFileObjectStorage` per update.
6. **List/Count/Search context and shutdown** — Context cancellation and closer/worker drain on context done; Exists/Count/Search check context; Search uses `sort.Slice` and bounded concurrency.

---

## 3. Lessons learned (for LESSONS_LEARNED and practices)

- **Cache heavy, per-identity resources:** Anything that creates a goroutine or expensive state per logical identity (e.g. per kind, per kind+dir) should be cached by that identity so growth is bounded. Prefer existing patterns: `ResourceCache[T]`, `GetOrCreate`, or a map keyed by identity with `sync.Once` per key.
- **Bounded concurrency:** List/count/search and similar fan-out must use a bounded pool or semaphore, not unbounded goroutines per call. See [unbounded-concurrency-fixes.md](./unbounded-concurrency-fixes.md).
- **Timeout context for all waiters:** Any goroutine that waits on a `WaitGroup` (or similar) for work started by the same component must run that work under a timeout context and pass that context to the workers so they exit and the waiter returns.
- **Critical path submission:** When the system is overloaded (e.g. over goroutine ceiling), critical health jobs (aggregation, retention) should still be submitted so they can run or timeout on their own; blocking submission entirely makes recovery harder.
- **Observe with dumps before and after:** Use diagnostic dumps (goroutines, threads, RSS) before and after restarts and code fixes so we have evidence (e.g. goroutine count drop from 1362 to 137 after HashRegistry cache) and can document baselines.

---

## 4. Performance baselines (from diagnostic dumps)

Metrics extracted with `scripts/diagnostics/diagnostic_metrics_report.sh` from `.zqk/diagnostics/scheduler_start/`. RSS in KB (ps aux column 6).

| Phase | Timestamp (example) | Goroutines | Threads (note) | RSS (KB) |
|-------|----------------------|------------|----------------|----------|
| Pre-restart (overload) | 060219 | ~948 | 10 | ~12,073,056 |
| Post-restart (jobs paused) | 060516 | ~92 | 10 | ~135,488 |
| Jobs on, before HashRegistry cache | 061124 | ~1362 | 10 | ~1,901,104 |
| Jobs on, after HashRegistry cache | 063021 | ~137 | 10 | ~1,960,720 |

**Note:** “OS thread count” in threads.txt is currently `runtime.NumCPU()`, not true OS thread count.

**Suggested foundational ranges (scheduler daemon, jobs enabled):**

- **Goroutines:** Target steady state on the order of low hundreds (e.g. &lt; 500) under normal load. Alert if sustained &gt; 1000 without a clear reason (e.g. one-off bulk run).
- **RSS:** Depends on object count and caches; use same workload and compare before/after changes. Post-fix ~1.9–2M KB (~2 GB) with jobs on is a reasonable reference for one environment.
- **Threads:** Use for trend only until capture reports real OS thread count.

These ranges should be refined with more runs and documented in a single “foundational perf specs” doc when we formalize SLOs.

---

## 5. Verification (report mining)

To reproduce and extend the comparison:

```bash
./scripts/diagnostics/diagnostic_metrics_report.sh scheduler_start
./scripts/diagnostics/diagnostic_metrics_report.sh scheduler_start --compare 061124 063021
```

See [DIAGNOSTIC_REPORT_MINING.md](./DIAGNOSTIC_REPORT_MINING.md) for full usage and interpretation.

---

## 6. References

- **Overload and timeout (detailed):** [SCHEDULER_OVERLOAD_AND_TIMEOUT.md](./SCHEDULER_OVERLOAD_AND_TIMEOUT.md)
- **Bottleneck audit:** [PERFORMANCE_BOTTLENECK_AUDIT.md](./PERFORMANCE_BOTTLENECK_AUDIT.md)
- **Lessons learned:** [LESSONS_LEARNED.md](./LESSONS_LEARNED.md)
- **Pre-change checklist:** [PRE_CHANGE_CHECKLIST.md](./PRE_CHANGE_CHECKLIST.md)
