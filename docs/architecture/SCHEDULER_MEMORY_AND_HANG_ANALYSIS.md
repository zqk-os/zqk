# Scheduler daemon: memory bloat and hang analysis

**Last Verified:** 2026-08-31


**Status**: Critical – immediate action required  
**Based on**: macOS `sample` outputs for pid 64202 (memory bloat) and pid 68013 (hang)  
**Date**: 2026-02-24

## 1. Summary

| Sample        | PID    | Parent  | Footprint | Observation |
|---------------|--------|---------|-----------|-------------|
| memory-bloat  | 64202  | launchd | **7.7 GB** | Scheduler daemon; massive RSS. |
| other-hang    | 68013  | zsh     | 103 MB    | CLI (or child); all threads waiting. |

**Root causes** (see below): unbounded in-memory growth in the **scheduler daemon** (high-volume event cache, ID index, JSON marshal of large structures), plus **too many scheduler_job objects** causing slow reload and blocking. The “hang” is consistent with processes blocked on I/O or locks (e.g. waiting for list/reload or storage).

---

## 2. Memory bloat (7.7 GB) – pid 64202 (daemon)

**Process**: `zqk` under **launchd** → scheduler daemon.

**Call stacks point to**:

1. **`encoding/json`** – `stateInString`, `appendIndent`, `checkValid`, `unquoteBytes`, `Marshal`, `Unmarshal`, `mapEncoder.encode`, `decodeState.object`.  
   Indicates **large JSON encode/decode** (e.g. full index or cache serialization).

2. **`pkg/storage.(*HighVolumeEventCache).rebuildTimeIndex`** and **`rebuildTimeIndex.func1`**  
   - Cache holds **id → entry** and a **byTime** slice (full copy of entries, sorted).  
   - **No cap on entry count**; with millions of `audit_event` IDs this is gigabytes.  
   - `rebuildTimeIndex()` allocates `byTime = make([]*, 0, len(c.cache))` and sorts – high allocation and CPU when cache is huge.

3. **`pkg/storage.(*indexQueue).startWorker`** → **`processBatch`** → **`saveMappingsNoLock`** → **`encoding/json.MarshalIndent`**  
   - ID index is saved as one big JSON (all id→path mappings).  
   - With millions of mappings, **MarshalIndent** allocates huge buffers and can fragment memory.

4. **`runtime.mapassign_faststr`**, **`mallocgc`**, **`memmove`**  
   - Heavy map/slice growth and copying.

**Conclusion**: Daemon memory is dominated by:

- **HighVolumeEventCache**: unbounded `cache` map + full `byTime` slice.
- **ID index**: full in-memory mappings plus MarshalIndent of the whole index.
- Large **scheduler_job** set (if present) exacerbates load (list + hydrate all jobs every 30s).

---

## 3. Hang (103 MB) – pid 68013 (CLI under zsh)

**Process**: `zqk` under **zsh** → likely a CLI invocation (list/count/scheduler command).

**Thread state** (all ~2459 samples per thread):

- **Main thread**: `pthread_cond_wait` (blocked).
- **Others**: `pthread_cond_wait`, `pthread_cond_timedwait`, `kevent`, `read`, `usleep`.

**Interpretation**: Process is **idle/waiting**, not busy-looping. Typical causes:

- CLI **list** or **count** that hits storage while the **daemon** is holding locks or saturating disk/CPU (e.g. during reload or index save).
- CLI waiting on a **scheduler** RPC or shared storage that never completes because the daemon is stuck or overloaded.
- **Lock contention** (e.g. on the same storage/cache the daemon is using).

So the hang is treated as a **symptom of daemon overload and unbounded data**, not a separate bug in the CLI.

---

## 4. Recommended resolution (priority order)

### Immediate (operational)

1. **Stop the daemon**  
   `zqk scheduler stop`

2. **Reduce scheduler_job count** (see `docs/architecture/PRE_COMMIT_BACKGROUND_RESULTS.md` § “Scheduler not firing / too many scheduler jobs”):  
   - Bulk-delete **done one_time** jobs (list by `execution_mode=one_time` + `enabled=false` or `status=disabled`, then `zqk object bulk delete --file <ids.yaml>`).  
   - Ensures the next daemon start does not load tens of thousands of jobs and that **scheduler_job_retention** can run when scheduled.

3. **Ensure scheduler_job_retention exists and is scheduled** (e.g. daily).  
   So job count and audit volume stay bounded.

4. **Restart the daemon**  
   `zqk scheduler start`  
   After cleanup, memory and responsiveness should improve.

5. **If high-volume event cache is huge on disk** (e.g. `high-volume-events-cache.json` hundreds of MB):  
   - Consider removing or renaming it once the daemon is stopped so the next run rebuilds from a smaller set (e.g. after retention/aggregation has run).  
   - Only if you accept a one-time full cache rebuild and have run retention/aggregation so event volume is under control.

### Short-term (code / config)

6. **Cap HighVolumeEventCache size**  
   - **Option A**: Max entries (e.g. 500k or 1M); evict oldest by `CreatedAt` when at cap.  
   - **Option B**: Time-window (e.g. keep only last N days); drop entries outside the window on load and on rebuild.  
   - **Option C**: Both: cap by count and by age.  
   Prevents unbounded growth of `cache` and `byTime` in memory.

7. **ID index / saveMappingsNoLock**  
   - Avoid marshalling the **entire** mapping in one go if the index is very large (e.g. stream or chunked write, or append-only segments with a compact merge).  
   - Reduces peak allocation and lock hold time during save.

8. **Scheduler job load**  
   - Already documented: keep job count low via **scheduler_job_retention** and bulk delete when needed.  
   - If product requirements allow, consider a **hard limit** in the job loader (e.g. load at most 10k jobs and log a warning) to avoid accidental “load all” of millions of jobs; only if acceptable for your deployment.

### Verification

- After cleanup and restart, **sample the daemon again** (e.g. after 10–30 min):  
  - Physical footprint should be **well below 1 GB** (ideally hundreds of MB).  
- **Scheduler activity**: timer jobs (e.g. retention, pre-commit) should fire on schedule.  
- **CLI** list/count/scheduler commands should complete in a few seconds.

---

## 5. Single delete (or Read) hangs: index queue holding idx.mu during load

**Symptom**: A single `zqk object delete <id>` (or any command that does a Read, e.g. list/count) hangs; sample shows main thread in `pthread_cond_wait` and index queue worker in `processBatch` → `loadLocked` → `encoding/json.Unmarshal`.

**Cause**: The CAS index queue worker held `idx.mu` (write lock) for the entire duration of `loadLocked()` (ReadFile + JSON Unmarshal of the full index). Any Read path that needs `GetHash` (RLock on `idx.mu`) blocks until the worker releases the lock. With a large index, loadLocked can take seconds, so single deletes and reads appear hung.

**Fix (applied)**:

1. **idx.mu**: `processBatch` no longer calls `loadLocked()` while holding `idx.mu`. It only preserves in-memory state, applies the batch, and copies out for save. The critical section is now a brief merge/copy (microseconds), so Read/GetHash (RLock) are not blocked.

2. **CAS index file lock**: The index queue used to acquire the index **file lock** at the start of `processBatch` and hold it for merge + ValidateMappings + save. Single delete’s `RemoveMapping` blocks on that same file lock, so the CLI hung until the queue released it. The queue now acquires the file lock **only around the save** (just before `saveMappingsLocked`, released after). Merge and ValidateMappings run without the file lock, so RemoveMapping can acquire it and complete while the queue is validating.

---

## 6. Log events: "Operation in progress: scheduler_scan-tests" and "scheduler_start"

**What you see** (e.g. in `.zqk/logs/log-events-human.log` or `scheduler-events.json`):

- Repeated lines like:  
  `Operation in progress: scheduler_scan-tests` or  
  `Operation in progress: scheduler_start`  
  with `new_status=in_progress old_status=in_progress`.

**What they are**: These are **heartbeat** messages from the async CLI progress pattern. Any long-running command that uses `BindAsyncProgress` (e.g. `zqk scheduler scan-tests`, `zqk scheduler start`) emits a progress line every ~5 seconds so the process does not appear to hang without feedback.

**Why they matter**:

1. **`scheduler_scan-tests`**  
   - Indicates a **`zqk scheduler scan-tests`** CLI run (e.g. `--package ./pkg/...`, `--all`, or `--load-bundles ...`) is (or was) in progress.  
   - That command **creates** `scheduler_job` objects (test bundle jobs, e.g. `SCH-run-bundle-*`). Running it with `--all` or broad packages can create **many** jobs in one go.  
   - So repeated or broad scan-tests runs are a **direct cause** of high scheduler_job count. High job count then leads to slow or stuck `LoadJobs()` and scheduler start (see below).

2. **`scheduler_start`**  
   - Indicates **`zqk scheduler start`** is running. The actual work is: set goroutine budget, create triggered pool, then **loadAndScheduleJobs** (LoadJobs with **no limit** → list all scheduler_job objects, hydrate, schedule).  
   - If there are 1500+ jobs, that load is slow and the process can exhaust the goroutine budget (e.g. `pool_creation_declined` for `cas_readdir` with `reserved=512`). Then the daemon never reaches "cron started" and **no scheduler jobs run**.  
   - So long runs of `scheduler_start` heartbeats usually mean: start is **stuck or very slow** in job load, and the large job set is the cause or a major contributor.

**Causal chain**:  
`scan-tests` (or similar) creates many jobs → job count grows → `scheduler start` does unbounded LoadJobs → slow / budget exhausted / daemon never "ready" → no jobs run; meanwhile other CLI (e.g. single delete) can also block on storage/locks.

**What to do**:  
Reduce job count (bulk delete done one_time / test bundle jobs as in PRE_COMMIT_BACKGROUND_RESULTS.md), ensure **scheduler_job_retention** is scheduled, and avoid running `scan-tests --all` (or broad packages) repeatedly without cleaning up old test bundle jobs.

---

## 7. Bulk delete appears hung (sample analysis)

**Symptom**: `zqk object bulk delete --file ids.yaml` (e.g. hundreds of scheduler_job IDs) appears to hang; samples show main thread and many threads in `pthread_cond_wait`, and some threads in **readdir** / **getdirentries64** (e.g. `AsyncCacheValidationStrategy.scannerLoop` → `performScan` → `scanDirectory`).

**What’s actually happening** (no deadlock):

1. **Delete path**: CAS `Delete()` calls **EnqueueRemove** and blocks on `<-done` until that remove is processed. So every bulk-delete worker blocks on its completion channel.
2. **Queue**: One **index queue worker per kind** processes requests in batches (e.g. 100). When a batch is full or the batch timeout (e.g. 200 ms) fires, it runs **processBatch**: merge into in-memory index, **ValidateMappings**, acquire file lock, **save** (write full index to temp file, sync, rename), then signal all `done` channels for that batch.
3. **Main goroutine**: Bulk delete main loop is blocked on `for res := range results`, i.e. waiting for workers to send delete results. Workers can’t send until their `Delete()` returns, which is after their `<-done` is signaled. So progress is gated by batch processing.
4. **Readdir in sample**: The threads in **readdir** are the **AsyncCacheValidationStrategy** background **scanner** (`scannerLoop` → `performScan` → `scanDirectory`), not the queue worker. The queue worker’s **ValidateMappings** for async strategy uses the **cache** (no readdir); only if cache is empty does it fall back to **validateSync** (os.Stat per mapping, no full directory scan).

**Why it feels “hung”**: With 1500 deletes and batch size 100, there are 15 batches. Each batch does one full index save (marshal + write + sync). For a large index that can be hundreds of ms per batch, so total time is several seconds with **no progress output** during the wait. The main thread and workers are all correctly blocked on channels; the only “active” work is the single queue worker processing batches and the async scanner doing readdir (adding I/O load).

**Optional improvements** (defer until object counts and scheduler are under control):

- **Progress / heartbeat**: Emit a progress message (e.g. “Deleting objects… N completed”) or heartbeat every 1–2 s during bulk delete so the CLI doesn’t appear stuck. *(Do after object counts and scheduler are under control.)*
- **Batch size**: For remove-only workloads, consider a larger batch (e.g. 200–500) to reduce the number of saves (fewer, bigger batches).
- **Scanner**: Consider pausing or throttling the async validation scanner during heavy bulk delete to reduce readdir contention (optional; scanner is not the primary bottleneck).

---

## 8. Memory sample 2026-03-04 (16.5 GB – WAL replay hot path)

**Sample**: `big-mem-samp.txt` (pid 46361, launchd → scheduler daemon)  
**Physical footprint**: 16.5 GB (peak 17.2 GB)

**Observed hot path** (Sort by top of stack):

- **encoding/json**: `stateInString` (657), `checkValid` (385), `unquoteBytes` (324) – JSON decode during WAL parse.
- **gopkg.in/yaml.v3**: `yaml_emitter_analyze_scalar` (655), `write` (208), `write_plain_scalar` (178) – YAML marshal in compact→record path.
- **Call chain**: `ObjectWriteBehindWorker.run` → `processBacklogIfNeeded` → `ReplayWALChunk` → `parseWALLine` → `compactV2ToWALRecord` → `decodePayloadCompact` (JSON unmarshal + `yaml.Marshal`).

**Conclusion**: When the daemon is replaying a large WAL backlog, CPU and allocation are dominated by **WAL replay**: every v2 compact line is parsed (JSON), then `decodePayloadCompact` decodes compact payload to full object and marshals to YAML for the write buffer. With a large backlog (e.g. hundreds of thousands of records), this produces heavy allocation churn and can drive RSS high.

**Mitigations** (in addition to §2–4):

- **HighVolumeEventCache**: Fixed 2026-03-04 – `Set`/`Invalidate` no longer call `rebuildTimeIndex()` on every op; use incremental insert/remove in `byTime` to avoid 500k-entry rebuild per create/delete.
- **WAL replay**: `maxReplayBacklog` and replay batch size in `processBacklogIfNeeded` were reduced to lower peak buffer size and replay allocation spikes (see `object_write_behind_worker.go`).
- **Operational**: Keep WAL compacted (run compaction when buffer is empty), reduce create rate if backlog stays high, and ensure retention/aggregation keep event volume bounded.

**Spindump: unkillable zqk (zombie + turnstile)**  
If a spindump shows zqk as **(suspended) (zombie)** and the only thread is `(suspended, blocked by turnstile waiting for Cursor [858])`, the process has already exited but one thread is stuck in the kernel waiting on Cursor. Zombies do not respond to SIGKILL or Activity Monitor “Force Quit”; the parent (launchd [1]) cannot reap until that thread leaves the kernel. **Workaround:** Quit Cursor so the turnstile is released; then launchd can reap the zombies. If they persist, reboot. See § “Unexpected exit / zombie” in scheduler docs if we add it.

**Why didn’t the daemon compact the WAL itself?** Compaction in the write-behind worker is **opportunistic only**: it runs when (1) the buffer is empty, (2) the WAL file is ≥ 1 MB, and (3) at least 30 s have passed since the last compact. With a large WAL backlog, `processBacklogIfNeeded` refills the buffer as soon as it drains (replay 4k records when buffer is empty), so the buffer is rarely empty for long. The daemon never gets a sustained empty-buffer window in which to compact. So the self-maintaining system does not currently “order up” WAL compaction—it only does it when the buffer happens to be empty. To guarantee compaction, run **`zqk system compact-wal` (PRUNED)** on a schedule (e.g. daily) or after reducing create load so the backlog can drain; see OBJECT_COUNT_MANAGEMENT.md for adding a scheduled compact-wal job.

---

## 9. References

- `docs/architecture/PRE_COMMIT_BACKGROUND_RESULTS.md` – § “Scheduler not firing / too many scheduler jobs”, § “Refreshing test bundle jobs”
- `docs/process/scheduler/SCHEDULER_JOB_POLICY_AND_LIFECYCLE.md` – scheduler_job_retention, job store hygiene
- `pkg/storage/high_volume_event_cache.go` – `rebuildTimeIndex`, cache/byTime
- `pkg/storage/content_addressable_storage_index.go` – `saveMappingsNoLock`
- `pkg/scheduler/job_loader.go` – `LoadJobs` (no limit)
- `docs/architecture/SCHEDULER_MEMORY_AND_HANG_ANALYSIS.md` – §8 (WAL replay sample 2026-03-04)
