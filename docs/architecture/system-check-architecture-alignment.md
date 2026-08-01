# System Check: Architecture vs Implementation Alignment

**Purpose:** Validate that `zqk system check` implementation follows the design in `system-check-pipeline.md` and `system-check-performance-targets.md`. This doc lists what is aligned and where logic diverges or is unspecified.

**Source of truth:** `docs/architecture/system-check-pipeline.md`, `docs/architecture/system-check-performance-targets.md`.

---

## Pipeline Phase Order (Design)

| Phase | Name          | Description                          |
|-------|---------------|--------------------------------------|
| 1     | Cache load/build | Load or rebuild object ID cache   |
| 2     | Warm CAS      | Pre-init kind mapper, merge from cache, flush (bounded 60s) |
| 3     | Discovery     | Kinds from `ObjectIDCache.GetKinds()` when cache populated; list paths per kind |
| 4     | Enqueue       | Queue paths for validation (state cache checked) |
| 5     | Validation    | Workers validate; single storage; ref lookups O(1) |

Rule: *No phase may start until its predecessors have completed* (except bounded concurrency within a phase).

---

## Aligned

1. **Phase order**  
   Implementation runs: (1) cache load/build and (2) warm CAS inside `EnsureObjectIDCacheReady` (via `buildObjectIDCacheIfNeeded` → `setupAsyncValidationFunction`). Then (3) discovery and (4) enqueue in `discoverAndEnqueueObjects`, then (5) validation via async validator. Discovery runs only after `setupAsyncValidationFunction` returns, so phases 1 and 2 are complete before phase 3 starts.

2. **Kinds from cache when populated**  
   - `determineCheckTarget` (async_check_helpers.go): uses `ObjectIDCache.IsPopulatedForProject` and `GetKinds()`; falls back to `discoverObjectKindsWithTimeout` only when `len(allKinds) == 0`.  
   - `discoverAndEnqueueObjects`: when `useCacheDiscovery` (cache populated), uses `objectIDCache.GetKinds()`; discovers from field registry only when kinds list is empty.  
   Matches: *“When the object set comes from the object ID cache, kinds MUST come from ObjectIDCache.GetKinds()”*.

3. **Post–cache-load diagnostic loop**  
   The loop that iterates object IDs and calls `HandleCacheLoad` runs only when `ZQK_CACHE_DIAGNOSTIC_OBJECTS` is set (setup_async_validation_helpers.go). Matches pipeline rule.

4. **Kind mapper pre-init before warm workers**  
   In `warmCASIndexesFromCache` (check_cache.go): `GetGlobalKindMapper().EnsureReady(ctx)` and per-kind pre-init (preInitWg) run before the warm worker loop that calls `EnsureCASIndexFromPaths`. Matches: *“Pre-initialize the kind mapper … before starting warm workers”*.

5. **Warm from cache only**  
   Warm phase uses `cache.GetAll()`, builds id→path per kind, then `EnsureCASIndexFromPaths(kind, idToPath)` only. No `EnsureCASIndexPopulatedFromScan` in the check path. Matches design.

6. **Single storage for run**  
   `checkCtx.StorageProvider` is set once (from setup loaders), passed as `storageForWarm` into `buildObjectIDCacheIfNeeded` and used for warm; `asyncCtx.StorageProvider` is the same instance for discovery and ref validation. Matches: *“Single StorageProvider for entire run”*.

7. **Bounded CAS flush**  
   `FlushAllCASIndexesForProjectRootWithTimeout(projectRoot, 60*time.Second)` used in `warmCASIndexesFromCache`. Matches 60s bound.

8. **Progress messages for warm**  
   “Merging CAS indexes for N kinds…”, “CAS indexes warmed” (and “Warming CAS indexes…”) are emitted from `warmCASIndexesFromCache` via `notifyCacheProgress`. Design’s required progress is present.

9. **Reuse and concurrency**  
   Bounded concurrency (semaphore), goroutine labels (`goroutinelabels.NewGoroutine`), WaitGroup, and clear done/progress callback are used in warm and discovery. Single shared caches/storage. Matches pipeline “Reuse and concurrency pattern”.

---

## Gaps / Not Explicitly in Design

1. **Progress emission must not block pipeline**  
   Design specifies *what* progress to show (“Merging CAS indexes…”, “CAS indexes warmed”) but not *how*. Implementation emits cache progress **asynchronously** (goroutine in `emitObjectIDCacheProgressViaCoordinator`) so the pipeline never blocks on coordinator or subscriber (e.g. stderr). This avoids the pipeline hanging on I/O and supports the time budgets.  
   **Recommendation:** In the pipeline or performance doc, add one line: *“Progress emission must not block the pipeline (e.g. emit via fire-and-forget or async delivery).”*

2. **Post-warm cache verification (bounded)**  
   After `EnsureObjectIDCacheReady` returns, implementation runs an optional “verify cache ready” step: it takes a read lock on the object ID cache (in a labeled goroutine) with a **5s timeout** to read size/nil and log warnings or run the diagnostic loop. If the lock is not acquired in time, it logs and continues with `cacheSize = 0`. The pipeline does not mention this step. It does not violate phase order (phase 2 is considered complete when `EnsureObjectIDCacheReady` returns; the verify is best-effort).  
   **Recommendation:** Optional. In the pipeline doc, add: *“An optional, bounded cache verification (e.g. read-lock with timeout) may run after warm; it must not block phase 3 beyond the timeout.”*

3. **Validator started before discovery**  
   Implementation calls `startAsyncValidator` before `discoverAndEnqueueObjects`. Workers are idle until discovery enqueues work. Design says Phase 4 Enqueue then Phase 5 Validation; starting workers early is an implementation detail to have them ready. No conflict with “no phase may start until predecessors complete” because validation *work* starts only when items are enqueued. Aligned with intent.

---

## Not Aligned / To Fix

- **None.** No current logic was found that contradicts the pipeline or performance docs. The items above are either aligned or are implementation details that the design does not yet spell out (and for which recommendations are given).

---

## References

- `docs/architecture/system-check-pipeline.md` — canonical pipeline and rules  
- `docs/architecture/system-check-performance-targets.md` — cold ≤20s, warm ≤1–2s  
- `docs/architecture/README.md` — lock and context analysis (separate from pipeline phase order)  
- Code: `cmd/zqk/system/async_check.go`, `async_check_helpers.go`, `setup_async_validation_helpers.go`, `check_cache.go`, `cache_coordination.go`
