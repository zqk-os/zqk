# Index-First, Low-CPU Scan Strategy

**Last Verified:** 2026-08-31


**Goal:** Reduce CPU load by at least 50% while improving perceived performance by using **change-aware, index-first** behavior. Full directory scans should be **rare**: only on initial startup or when reconciling a detected catastrophic disparity. Background scanning should be **lightweight and throttled** (fewer threads, optional yielding) so the rest of the system stays fluid.

**Context:** Sampling showed high CPU from (1) massive readdir usage across many threads, (2) scheduler audit aggregation triggering CAS recovery (repeated index loads + stat storms), (3) JSON encode/decode for CAS index. This design addresses those without introducing user-visible bottlenecks.

---

## 1. Principles

| Principle | Meaning |
|-----------|--------|
| **Index-first** | CAS index, object ID cache, and reverse reference index are the source of truth for list/count/get when populated. Trust them; do not scan to “verify” on every operation. |
| **Change-aware** | Data changes are known from the **write path** (Create/Update/Delete → index updates, WAL). Use that to keep indices current. Full scan is not the primary way to “discover” state. |
| **Scan rarely** | Full directory scan only when: (a) **initial startup** (cold index or first use), (b) **explicit refresh** (e.g. `--refresh-cache`), (c) **catastrophic disparity** (e.g. repeated failures or integrity check). |
| **Throttled background** | When a scan must run in the background, use a **single reconciliation worker** (or very low concurrency), with optional sleep/yield between units of work so CPU is available for foreground work. |
| **Fewer threads** | Reduce parallelism for cache warm, cache build, and any discovery/scan path so background work does not starve the rest of the system. |

---

## 2. When to Full-Scan (Only These Cases)

- **Cold start:** Index empty on first List/Count/Get for that kind → one sync or async scan to populate (see §4 for async shape).
- **Explicit refresh:** User or job runs “refresh cache” / “reconcile” → allowed to run a full scan.
- **Catastrophic disparity:** Defined as (e.g.) aggregation or integrity job failing due to missing files, or a dedicated “disparity detector” that samples and finds severe index-vs-disk mismatch. Then trigger **one** reconciliation run (throttled), not continuous recovery on every job.

**Not:** Periodic “just in case” full recovery or full scan on every audit aggregation run.

---

## 3. CAS Recovery and Audit Aggregation

**Current behavior:** Every audit aggregation job runs `runCASRecoveryForKind(ctx, projectRoot, "audit_event")`, which loads the CAS index and stats every index entry’s file. That causes repeated JSON loads and many `os.Stat` calls and contributes heavily to CPU.

**Recommendation:**

- **Do not run full CAS recovery on every audit aggregation.**
- **Option A (recommended):** Run CAS recovery only when there is **evidence of need**:
  - Pre-aggregation health check fails (e.g. missing file when building metric), or
  - A separate, **less frequent** job (e.g. integrity_check or a dedicated “cas_recovery” job) runs recovery for `audit_event` (and optionally other kinds) on a schedule (e.g. daily or on-demand).
- **Option B:** If recovery must stay in the aggregation path, run it only when the **previous** aggregation run failed with a “missing file” or “index stale” style error (one retry with recovery, then back to normal).
- **Option C:** Reuse a **shared** CAS instance (or at least a shared index load) for the process so recovery does not repeatedly call `NewContentAddressableStorage` and reload the same index JSON; use the same cache as the rest of the storage layer when available.

Implementing **Option A** plus **Option C** (shared index/cache for recovery when it does run) gives the largest CPU win and keeps aggregation fast.

---

## 4. Single Background Reconciliation Worker

**Current behavior:** When list/count detect index-vs-disk count mismatch, they call `triggerBackgroundCASIndexRefresh(kind)`, which starts a **new goroutine** per project+kind that runs `EnsureCASIndexPopulatedFromScan`. Multiple kinds can be refreshing in parallel, each doing full ReadDir + YAML reads, which adds up to many threads and high readdir CPU.

**Recommendation:**

- Introduce a **single global reconciliation worker** (or one per project root, if multi-root is required):
  - One goroutine (or a pool of 1–2 workers) that pulls **reconciliation requests** (projectRoot + kind) from a queue.
  - Processes **one** project+kind at a time.
  - Between kinds (or between batches within a kind), **sleep or yield** (e.g. 50–200 ms) so the rest of the system gets CPU.
- **triggerBackgroundCASIndexRefresh(kind)** enqueues a request (projectRoot, kind) to this queue instead of starting a new goroutine. Deduplication remains (e.g. one pending request per project+kind).
- Same pattern can be used for any other “background full scan” (e.g. object ID cache rebuild from disk when disparity is detected).

**Effect:** Many concurrent scans become one (or two) sequential, throttled scan(s). Slower in wall-clock for the background task, but no user-facing bottleneck and large reduction in CPU and thread count.

---

## 5. Reduce Concurrency for Cache Warm and Cache Build

**Current behavior:**

- **warmCASIndexesFromCache:** `warmWorkerCap()` = NumCPU*2, capped 4–16. So up to 16 workers for pre-init and 16 for CAS merge.
- **buildCacheInParallel:** numWorkers = min(24, max(4, NumCPU*2)), so up to 24 workers building the object ID cache.

**Recommendation:**

- **Lower caps** so background work uses fewer cores and leaves CPU for foreground:
  - **warmWorkerCap():** Cap at **2 or 4** (e.g. `min(4, max(2, runtime.NumCPU()/2))` or a fixed 2). Pre-init and CAS merge then use at most 2–4 workers.
  - **buildCacheInParallel:** Cap at **2 or 4** workers (e.g. `min(4, max(2, len(jobs)))` or fixed 2). Cache build takes longer but does not dominate CPU.
- Optionally make these configurable (env or config) so heavy servers can increase if needed.

**Effect:** Fewer threads doing readdir and JSON work; cache warm and cache build become “gentle” background tasks.

---

## 6. Disparity Detection Without Triggering a Scan on Every List

**Current behavior:** On every List (and Count), when the index is non-empty we call `getCachedDiskCountOrScan(filter.Kind)`. If mtimes match we use cached count; otherwise we run `countHashNamedFilesInKindDir` (ReadDir over the kind dir). If count ≠ len(casIDs), we trigger a full background CAS index refresh.

**Recommendation (keep semantics, reduce scan frequency):**

- **Keep** mtime-based cache for disk count so we do not re-count every time when nothing changed.
- **Optionally relax** when we trigger a full refresh:
  - Only trigger refresh if the **count mismatch** is above a threshold (e.g. > 5% or > N objects) to avoid refreshing on tiny races.
  - Or trigger at most once per kind per hour (or per run of a reconciliation job) unless there is an explicit refresh.
- **When** a refresh is triggered, it goes to the **single reconciliation queue** (§4), not a new goroutine per kind.

---

## 7. Summary of Concrete Changes

| Area | Change |
|------|--------|
| **Audit aggregation** | Remove unconditional `runCASRecoveryForKind`; run recovery only on evidence of need (e.g. health check failure) or from a separate, less-frequent job. |
| **CAS recovery** | When recovery does run, reuse shared CAS/index where possible to avoid repeated index JSON load. |
| **Background refresh** | Replace per-kind goroutine with a single reconciliation worker (queue + one worker, optional sleep between kinds). |
| **Cache warm** | Reduce warmWorkerCap to 2–4. |
| **Cache build** | Reduce buildCacheInParallel workers to 2–4. |
| **CAS readdir pool** | Reduce casReadDirPoolWorkers from 64 to 16 so CAS bucket-search ReadDir uses fewer threads; sampling showed __getdirentries64 as a major CPU hotspot. |
| **Disparity** | Optionally throttle how often we trigger full refresh (e.g. threshold or rate limit); when triggered, use reconciliation queue. |

---

## 8. Expected Outcomes

- **CPU:** Large reduction (target ≥50%) by (1) not running CAS recovery on every aggregation, (2) fewer threads scanning, (3) one throttled reconciliation worker instead of many parallel refreshes.
- **Responsiveness:** Foreground operations (list, count, create, aggregation) get more CPU; background scanning is intentionally slower and lighter.
- **Correctness:** Indices remain the source of truth; full scan only on cold start, explicit refresh, or detected catastrophic disparity. Change-aware updates (create/update/delete) keep indices in sync under normal operation.

---

## 9. Implementation Order

Suggested order so each step is shippable and measurable:

1. **Reduce worker caps** (§5): Change `warmWorkerCap()` and `buildCacheInParallel` to use 2–4 workers. Low risk; immediate CPU reduction.
2. **Gate CAS recovery in audit aggregation** (§3): Run `runCASRecoveryForKind` only when pre-execution health check fails (or remove it and rely on a separate integrity/cas_recovery job). Reduces repeated index load + stat storms.
3. **Single reconciliation worker** (§4): Add a reconciliation queue and one worker; have `triggerBackgroundCASIndexRefresh` enqueue instead of starting a goroutine. Dedupe by project+kind. Add optional sleep between kinds.
4. **Reuse shared CAS in recovery** (§3 Option C): When `RecoverCASKind` runs, accept an optional `FileObjectStorage` (or CAS cache) so it can reuse an already-loaded index instead of creating a new `NewContentAddressableStorage` and loading from disk.
5. **Optional:** Relax disparity trigger (§6) and/or add a dedicated low-frequency CAS recovery job for when recovery is still needed.

---

## 10. References

- **CAS list/get consistency:** `docs/architecture/CAS_LIST_GET_CONSISTENCY.md`
- **Reverse reference index:** `docs/architecture/REVERSE_REFERENCE_INDEX_DESIGN.md`
- **Cache management:** `docs/architecture/CACHE_MANAGEMENT_STRATEGY.md`
- **CLI performance:** `docs/architecture/CLI_PERFORMANCE_AND_CONSISTENCY.md`
- **Sample analysis:** User CPU sample showed readdir, audit aggregation → CAS recovery, and encoding/json as hot spots; this design addresses those directly.
