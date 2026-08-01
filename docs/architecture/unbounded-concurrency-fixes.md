# Unbounded Concurrency Fixes (Thread Explosion)

**Context**: Process samples after initial bounds (CAS list workers, hydration semaphore) still showed thread explosion with many threads in `readdir_r`/`open`. Those bounds limited *concurrency* but not *goroutine count*: we were still creating one goroutine per item (file path or job).

## Root cause

- **Semaphore-only bounds**: A semaphore limits how many goroutines run at once, but we were still spawning N goroutines (one per file or per job). With 10k files or 2k jobs, that's 10k or 2k goroutines and associated OS threads.
- **Worker-pool pattern**: We need a fixed number of worker goroutines that pull work from a channel, instead of spawning one goroutine per work item.

## Locations fixed (Feb 2026)

### 1. File-based List (`pkg/storage/object_storage_file_list_main.go`)

- **Before**: Loop over `filePaths` spawned one `NewGoroutine("file_storage_parse_file", ...)` per path, with a semaphore of 64. Goroutine count = len(filePaths).
- **After**: Single work channel; 64 workers (or fewer if file count is smaller) range over the channel. Goroutine count capped at 64 + 1 closer.

### 2. Count with filters (`pkg/storage/object_storage_file_list_query.go`)

- **Before**: Loop over `filePaths` spawned one `NewGoroutine("file_storage_count_file", ...)` per path, with a semaphore of 10. Goroutine count = len(filePaths).
- **After**: Work channel; 64 workers range over the channel. Goroutine count capped at 64 + 1 closer.

### 3. Job hydration (`pkg/scheduler/job_management.go`)

- **Before**: Loop over `rawJobs` spawned one `NewGoroutine("scheduler_hydrate_job", ...)` per job, with a semaphore of 16. Goroutine count = len(rawJobs).
- **After**: Work channel of `hydrationWorkItem`; 16 workers (or fewer if job count is smaller) range over the channel. Goroutine count capped at 16.

### 4. Event- and lifecycle-triggered jobs (`pkg/scheduler/lifecycle_coordination.go` + `scheduler.go`)

- **Before**: `TriggerJobByEvent` and `TriggerJobByLifecycle` spawned one `NewGoroutine("scheduler_job_executor", ...)` per matching job. Many jobs with broad filters (e.g. `*`) or frequent events led to unbounded goroutines (e.g. 4500+ threads).
- **After**: Bounded pool (64 workers, channel buffer 512). Trigger functions send `triggeredJobWork` to the channel; workers pull and call `executeJob`. Goroutine count for triggered execution capped at 64.

### 5. Concurrent List/Count cap (`pkg/storage/list_count_concurrency.go` + list/query)

- **Before**: Each file-based List used 64 workers and each Count (with filters or countFiles) used 64 or 1. With 64 scheduler jobs running, each doing List or Count, total = 64×64 = 4096+ list workers. Sample showed 5k+ threads in readdir/open.
- **After**: Global semaphore allows at most 8 concurrent file-heavy List or Count operations. Total list+count workers capped at 8×64 + 8×64 = 1024 (instead of unbounded per job).

### 6. CAS list path slot (same file, `object_storage_file_list_main.go`)

- **Before**: The CAS index list path did not acquire the global List/Count slot. N jobs listing CAS kinds created N×64 CAS read goroutines. Dump showed 889+ goroutines in syscall/readdir.
- **After**: CAS list path now acquires `AcquireListCountSlot(ctx)` before starting the 64 CAS read workers and defers `ReleaseListCountSlot()`. Concurrent CAS list operations are capped at 8.

## Previously bounded (unchanged)

- **CAS list**: Already used a worker pool (64 workers, work channel of IDs).
- **CAS index listing**: Bounded in the CAS list path; `EnsureCASIndexPopulatedFromScan` is single-threaded.

### 7. Goroutine ceiling (circuit breaker)

- **Solution**: `WaitUnderGoroutineCeiling(ctx, ceiling, pollInterval)` blocks until `NumGoroutine() < ceiling` or ctx is done. Used at triggered-job submission. Health monitor logs a warning when count > 1500. Coordinator event `goroutine_ceiling_block` emitted when we block.

### 8. Cron-triggered jobs

- **After**: Cron callback submits work to `triggeredJobCh` (same pool as event/lifecycle), after `WaitUnderGoroutineCeiling`. Concurrent job executions capped at 64 workers.

### 9. Job recovery (missed jobs)

- **After**: Recovery submits work to `triggeredJobCh` (same pool); no per-recovery goroutine.

## Using metrics to isolate cause

**Scheduler health metrics** include `goroutine_count` and `runtime_thread_count`. List with `zqk object list --kind scheduler_health_metric --limit 100`. Coordinator events: `goroutine_ceiling_block` (operation `goroutine_ceiling_block`).

### 10. CAS.Read bucket-search path (`pkg/storage/content_addressable_storage_crud.go`)

- **Before**: CAS.Read ran `os.ReadDir` in a new goroutine (with 5s timeout). Under load this produced 3k+ goroutines.
- **After**: Process-wide bounded pool (`getCASReadDirPool()`: 64 workers, queue 256). CAS.Read submits a ReadDir task to the pool and blocks on the result.

## Goroutine audit: paths not using the bounded pool (Feb 2026)

- **One unbounded per-call path** was fixed: CAS.Read bucket path (now uses pool).
- **~12 production code paths** use raw `go func`; all are one-off or naturally bounded (one goroutine per operation or per component).
- **Per-item spawns** that remain use goroutinelabels with budget + semaphore (e.g. async validator).

If new "one goroutine per X" code is added, prefer the bounded pool (or at least goroutinelabels + budget) for hot paths.

**Concurrency audit:** For a single list of all scheduler and storage worker counts, caps, and recommendations for low/mid-tier hardware, see **SCHEDULER_CONCURRENCY_AUDIT.md**.

## If thread growth continues

Search for: `for ... range ... {` followed by `goroutinelabels.NewGoroutine` or `go func` (one goroutine per iteration). Prefer: work channel + fixed number of workers that `for item := range workCh { ... }`. Add `WaitUnderGoroutineCeiling` at gates so we never exceed a global cap.
