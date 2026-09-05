# Scheduler overload and timeout

**Last Verified:** 2026-08-31


**Status:** Analysis complete; mitigations implemented (ceiling bypass, timer-maintenance priority pool, tier-3 timeout, list/count 16)  
**Based on:** Goroutine dumps (012002, 010539, 005604), `sample` output, scheduler event logs, `issues.json`, `dispatch_pressure.jsonl`  
**Date:** 2026-02-25 (updated 2026-04-05)

**See also:** [EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md](./EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md) — *Scheduler automation discipline (efficiency / DRY)*: prefer parameterized maintenance jobs and shared scripts over many persisted **`scheduler_job`** copies; keep **`scheduler_maintenance_config.yaml`** aligned with jobs you rely on. **Convergence operator scripts / env tables:** [scripts/README.md — Convergence promotion and overseer helpers](../scripts/README.md#convergence-promotion-and-overseer-helpers).

## 1. Summary

Under sustained load the scheduler daemon can reach a state where:

- **Memory** drops from peak (e.g. 21G) and stabilizes (e.g. ~4.7G) while **goroutine count** stays very high (~1M).
- **Jobs that require List/count** (e.g. `audit_event_aggregation`, `retention_tolerance`) **time out** every run; event logs show `timeout` after 30–45 minutes.
- **Sample** shows almost all CPU time in `__psynch_cvwait` (waiting); little useful work completes.
- **Cache prewarm** can complete on schedule (e.g. SCH-007 every ~10 min), but one prewarm run may leave a goroutine **stuck in `WaitGroup.Wait`** for tier-3 indefinitely.

This document explains the root causes and sketches exact code changes for two fixes: **goroutine ceiling behavior** and **cache prewarm tier-3 wait**.

---

## 2. Root causes

### 2.1 List/count slot contention (primary bottleneck)

- **Location:** `pkg/storage/list_count_concurrency.go`
- **Mechanism:** Only **8** concurrent List/Count operations are allowed (`listCountMaxConcurrent = 8`). A single semaphore (`listCountSem`) is shared by all callers.
- **Observed:** In dumps, **177+ goroutines** had `AcquireListCountSlot` in their stack. Audit aggregation (in `preExecutionHealthCheck`) and retention tolerance (in `mergeStrategyTolerance`) both call `List` and block waiting for a slot. With 8 slots and 177 waiters, wait time can exceed the job’s `max_runtime_seconds`, so the job times out without making progress.
- **Evidence:** Goroutine 475 (audit aggregation) and 508 (retention tolerance) were in `select` in `AcquireListCountSlot` → `FileObjectStorage.List` → handler code.

**Mitigation (implemented):** `listCountMaxConcurrent` increased from 8 to 16 in `pkg/storage/list_count_concurrency.go` to reduce contention; progress message updated to match.

### 2.2 Goroutine ceiling blocks new work

- **Location:** `pkg/concurrency/goroutine_ceiling.go` (`DefaultGoroutineCeiling = 2000`); call sites in `pkg/scheduler/job_management.go`, `lifecycle_coordination.go`.
- **Mechanism:** Before submitting a timer or recovery job, the scheduler calls `WaitUnderGoroutineCeiling(ctx, DefaultGoroutineCeiling, 200*time.Millisecond)`. If `NumGoroutine() >= 2000`, the caller blocks until the count drops or `ctx` is cancelled.
- **Dispatch budget:** The context used for that wait (and for blocking pool submit) is bounded by **`ZQK_SCHEDULER_DISPATCH_RESOURCE_WAIT_MAX`** (default **2h**), not by `max_runtime_seconds`. Execution time is still capped separately once the job actually runs. When the dispatch budget expires, **that attempt is abandoned** (cron: skip this fire; recovery: skip this cycle); the **next** tick or trigger tries again—analogous to “if we can’t get to you in two hours, try again later,” not a 24h silent block.
- **Trigger queue “callback”:** For jobs triggered via `.zqk/scheduler/triggers/`, **pool full** returns `ErrPoolFull` and the watcher **re-enqueues** remaining IDs (`trigger_queue_pool_full`) so work is not lost while capacity catches up.
- **Observed (historical):** With ~1M goroutines, the count never drops below 2000, so without bypass for critical job types new timer submissions could stall until the dispatch context deadline.
- **Result:** Critical jobs (retention, audit aggregation) that were submitted earlier may still be blocked on list/count slots (above) and time out; mitigations include ceiling bypass and priority dispatch (§4) and fixing list/count contention (§2.1).
- **Timer maintenance starvation (testing load):** When many `run_wrapper` jobs (`category: testing`) fill the main triggered pool or drive `NumGoroutine()` over the ceiling, **timer** jobs with **`category: maintenance`** could previously skip dispatch (`dispatch_pressure.jsonl`: `source: "cron"`, `reason: dispatch_resource_wait_deadline_exceeded`). Those jobs now use **`priorityDispatchJob`** (§4): they bypass the ceiling at submit and run on a **separate priority worker pool** so scheduled housekeeping is not starved by scan-tests traffic.

### 2.3 Cache prewarm tier-3 wait cannot time out

- **Location:** `pkg/scheduler/handlers_cache_prewarm.go` (Tier 3 and wait logic).
- **Mechanism:** Tier 3 starts three tasks (object ID cache, validation state cache, reverse reference index) with a shared `sync.WaitGroup`. A **separate goroutine** is started that only runs `wgTier3.Wait()` and then closes `doneTier3`. The main prewarm flow does `select { case <-doneTier3; case <-tier3Ctx.Done() }` with a 50s timeout. The tier-3 **tasks** are started with the job’s `ctx`, not `tier3Ctx`, so they are not cancelled when the 50s timeout fires.
- **Bug:** If a tier-3 task (e.g. object ID cache build) never completes (e.g. blocked on List or I/O), it never calls `Done()`. The goroutine that called `wgTier3.Wait()` then blocks **forever**. The main flow’s 50s select fires and the job continues with a warning, but the “tier3_wait” goroutine remains stuck. Dumps showed one goroutine in `CachePrewarmHandler.Execute.func10` at `wgTier3.Wait()` for 98+ minutes.
- **Fix:** Ensure tier-3 tasks are limited by the same 50s timeout so they exit and call `Done()`; then `Wait()` returns and no goroutine is left blocked indefinitely.

---

## 3. Recommendations (overview)

1. **Short term:** Restart the scheduler to reset goroutine count and clear the submission backlog.
2. **List/count slots:** Increase concurrency or give scheduler jobs dedicated slots (see §2.1).
3. **Goroutine ceiling:** Allow critical jobs to bypass or use a separate path so they are still submitted when over ceiling (§4).
4. **Prewarm tier-3:** Make tier-3 tasks respect the 50s timeout so the wait goroutine never blocks forever (§5).
5. **Long term:** Cap goroutine growth (HashRegistry, List workers, etc.) so the process stays below the ceiling.

---

## 4. Code change (2): Goroutine ceiling behavior and priority dispatch

**Goal:** When the process is over the goroutine ceiling, health-critical jobs still get **submitted** so they can at least run and time out on their own `max_runtime_seconds`, instead of never being submitted. **Timer maintenance** jobs must not be starved by testing `run_wrapper` load on the main pool.

### 4.1 `criticalJobTypesForSubmission` and `priorityDispatchJob`

- **File:** `pkg/scheduler/job_management.go`
- **`criticalJobTypesForSubmission`:** `audit_event_aggregation`, `cache_prewarm`, `retention_tolerance` — bypass the goroutine ceiling at submit (cron, `recoverMissedJob`) and are routed to the **priority** triggered pool when started.
- **`priorityDispatchJob(job)`:** `true` if the job type is in `criticalJobTypesForSubmission`, **or** if `trigger_type == "timer"` and `category == "maintenance"` (see `CategoryMaintenance` in `pkg/scheduler/constants.go`). Timer maintenance includes e.g. `convergence_session_tick` and maintenance `run_wrapper` jobs that are not in the small critical **type** set.

**Cron path:** Inside `scheduleTimerJob`’s `AddFunc` callback, `WaitUnderGoroutineCeiling` is skipped when `priorityDispatchJob(job)` is true, then work is submitted via `submitTriggeredJob`.

**`recoverMissedJob`:** Uses the same `priorityDispatchJob` rule before `WaitUnderGoroutineCeiling` (file `lifecycle_coordination.go`).

**Event / lifecycle trigger paths:** Still use **`criticalJobTypesForSubmission` only** for ceiling bypass (not the full `priorityDispatchJob`), so ad-hoc event-driven work does not automatically bypass the ceiling just because `category` is maintenance.

### 4.2 Priority triggered pool

- **File:** `pkg/scheduler/scheduler.go` (daemon `Start`)
- Separate pool (`triggeredPriorityPool`) with its own worker count and queue depth so timer maintenance and critical types do not queue behind the default `SCH-run-*` / testing workload on the main triggered pool. Sizes are defined next to pool creation (currently **8** workers, **96** queue slots — adjust there if maintenance parallelism needs tuning).

**Status:** Implemented: `submitTriggeredJob` selects the priority pool when `priorityDispatchJob(work.job)` and the pool exists.

**Testing:** `TestPriorityDispatchJob_TimerMaintenance` and `TestCriticalJobTypesForSubmission_IncludesMaintenanceTypes` in `pkg/scheduler/scheduler_test.go`. With ceiling at 2000 and goroutine count above 2000, a `priorityDispatchJob` job should still be submitted; a timer **testing**-category job should still respect the ceiling unless its job type is in the critical map.

---

## 5. Code change (3): Cache prewarm tier-3 wait

**Goal:** Tier-3 tasks are cancelled after 50s so they always call `Done()` and the goroutine that runs `wgTier3.Wait()` never blocks forever.

**Current flow:** Tier-3 timeout and context are created **after** the three tier-3 goroutines are started; those goroutines receive the job’s `ctx`, not the 50s timeout context.

**Fix:** Create `tier3Ctx` (with 50s timeout) **before** starting the tier-3 tasks, and pass **`tier3Ctx`** into `StartWithContext` for all three tier-3 tasks. Then when 50s elapses, the tier-3 tasks see `tier3Ctx.Done()`, return, and the WaitGroup’s `Done()` is called (by the goroutinelabels wrapper), so `wgTier3.Wait()` returns and the “tier3_wait” goroutine exits.

- **File:** `pkg/scheduler/handlers_cache_prewarm.go`
- **Section:** Tier 3 block (approximately lines 176–246).

**Steps:**

1. **Move** the tier-3 timeout and context creation to **before** the three `NewGoroutine(...).WithWaitGroup(&wgTier3).StartWithContext(...)` calls. Keep `defer tier3Cancel()`.
2. **Replace** the context passed to each tier-3 `StartWithContext` from `ctx` to `tier3Ctx` (so the closure receives the 50s-limited context).
3. **Leave** the rest unchanged: the helper goroutine that runs `wgTier3.Wait()` and closes `doneTier3`, and the main flow’s `select { case <-doneTier3; case <-tier3Ctx.Done() }`.

**Concrete edit sketch:**

- After the Tier 2 block and before `var wgTier3 sync.WaitGroup`, insert the timeout calculation and create `tier3Ctx`:

```go
// Tier 3: Composite caches (parallel, after Tier 2 completes)
tier3Timeout := 50 * time.Second
if deadline, ok := ctx.Deadline(); ok {
	remaining := time.Until(deadline)
	if remaining < tier3Timeout {
		tier3Timeout = remaining - 5*time.Second
		if tier3Timeout < 0 {
			tier3Timeout = 1 * time.Second
		}
	}
}
tier3Ctx, tier3Cancel := context.WithTimeout(ctx, tier3Timeout)
defer tier3Cancel()

var wgTier3 sync.WaitGroup
```

- Change each of the three tier-3 `StartWithContext(ctx, ...)` to `StartWithContext(tier3Ctx, ...)`.
- **Remove** the duplicate `tier3Timeout` / `tier3Ctx` block that currently appears after the three starts (lines 215–226) and keep only the `doneTier3` channel, the helper goroutine that calls `wgTier3.Wait()` and closes `doneTier3`, and the `select`.

**Result:** When tier3Ctx expires after 50s, the tier-3 tasks are cancelled, they return, the WaitGroup is completed, `Wait()` returns, and no goroutine remains stuck in `Wait()`.

**Status:** Implemented and DRY. **All tiers** use a single pattern in `handlers_cache_prewarm.go`: `tierTimeoutFromJob(ctx, defaultTimeout, buffer)` for job-derived timeouts; `runSequentialTier` for Tier 1 (single task); `runParallelTier` for Tier 2 and Tier 3 (and any future Tier 4+). Each parallel tier creates its context *before* starting tasks and passes it to every task so the wait goroutine never blocks forever. Adding Tier 4 is a single `runParallelTier(ctx, job.ID, 4, "Tier 4", defaultTimeout, buffer, []tierTask{...})` call.

---

## 6. Immediate-load batching (optional startup throttling)

On daemon **startup** (and full reload), **immediate** `scheduler_job` rows are scheduled in **pass 2** of `loadAndScheduleJobs`, after timers/cron/bootstrap. Submitting hundreds of immediates at once can fill the main triggered pool queue before workers drain.

**Optional admission control:** set **`ZQK_SCHEDULER_IMMEDIATE_LOAD_BATCH_SIZE`** to a positive integer (capped in code). When set, immediate jobs are scheduled in chunks of that size; each chunk runs under `jobsMu`, then the lock is released before the next chunk. **`ZQK_SCHEDULER_IMMEDIATE_LOAD_BATCH_PAUSE`** is a **`time.ParseDuration`** string (e.g. `200ms`, `1s`) for an optional pause **between chunks only** (never while holding `jobsMu`). **`0`** / unset batch size restores the legacy behavior (one lock for all immediates). Cancelling load context aborts during an inter-chunk pause (see **`TestLoadAndScheduleJobs_chunkedImmediateRespectsCancelDuringPause`** in `pkg/scheduler`).

Log event when chunking applies: `scheduler_job_mgmt_immediate_load_chunking` (`batch_size`, `batch_pause`, `immediate_jobs`).

**Traceability:** requirement **`[REDACTED-ID]`** (criteria **`[REDACTED-ID]`**, **`[REDACTED-ID]`**, **`[REDACTED-ID]`**; test case **`[REDACTED-ID]`**). Inspect with `zqk object get <id>`.

---

## 7. Conflict manager: testing vs maintenance `run_wrapper`

**Symptom (observed):** `dispatch_pressure.jsonl` fills with `dispatch_resource_wait_deadline_exceeded` for maintenance `run_wrapper` jobs while `SCH-run-*` test bundles are active.

**Root cause (fixed):** `ConflictManager.CanRun` treated *any* running `run_wrapper` as blocking *all* non-concurrent `run_wrapper` jobs. Scan-tests marks testing bundles with `ConcurrentAllowed=true`; those jobs still registered as running with `JobType=run_wrapper`, so **non-concurrent maintenance `run_wrapper` could never pass `CanRun` while any test bundle was executing**—even though concurrent runners are supposed to be independent. Maintenance work was starved at the conflict gate before pool/ceiling limits mattered.

**Fix (evolved):** For a non-concurrent candidate job, consider **other non-concurrent** jobs of the same `JobType` as conflicts **except** for `run_wrapper`: there, only the **same `scheduler_job` id** may not overlap (duplicate cron/recovery). Different maintenance scripts (`run_wrapper`, distinct ids) may run concurrently. Concurrent runners (testing bundles, cache prewarm, etc.) are skipped in the same-type scan where it still applies.

**Code:** `pkg/scheduler/conflict_manager.go`; tests: `TestConflictManager_MaintenanceRunWrapperNotBlockedByTestingBundles`, `TestConflictManager_CanRun_RunWrapperDifferentIDsAllowed`.

**Operational tuning (still applies):** Goroutine ceiling, triggered pool size (`ZQK_SCHEDULER_TRIGGERED_POOL_SIZE`), and `scan-tests --max-parallel` reduce queue depth; the conflict fix removes an incorrect serialization layer. If maintenance still hits `dispatch_pressure` with `source: "cron"` while testing is active, see **§4** (`priorityDispatchJob` + priority pool).

---

## 8. References

- **Goroutine ceiling:** `pkg/concurrency/goroutine_ceiling.go`, `DefaultGoroutineCeiling`, `WaitUnderGoroutineCeiling`.
- **Submission paths:** `pkg/scheduler/job_management.go` (`priorityDispatchJob`, cron), `pkg/scheduler/scheduler.go` (`submitTriggeredJob`, priority pool), `pkg/scheduler/lifecycle_coordination.go` (lifecycle, event, recovery).
- **List/count slots:** `pkg/storage/list_count_concurrency.go`, `AcquireListCountSlot`, `listCountMaxConcurrent`.
- **Cache prewarm:** `pkg/scheduler/handlers_cache_prewarm.go` (Tier 3 and wait logic).
- **Conflict manager:** `pkg/scheduler/conflict_manager.go`; testing concurrency: `isConcurrentAllowed` in `lifecycle_coordination.go`.
- **Immediate-load batching:** `pkg/zqkenv/vars.go` (`SchedulerImmediateLoadBatchSize`, `SchedulerImmediateLoadBatchPause`), `pkg/scheduler/job_management.go` (pass 2), §6 above.
- **Related:** [SCHEDULER_MEMORY_AND_HANG_ANALYSIS.md](./SCHEDULER_MEMORY_AND_HANG_ANALYSIS.md), [unbounded-concurrency-fixes.md](./unbounded-concurrency-fixes.md).
