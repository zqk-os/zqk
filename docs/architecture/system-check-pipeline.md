# System check pipeline architecture and specs

**Last Verified:** 2026-08-31


**Version:** 1.0.1  
**Status:** Active  
**Related:** Performance targets (`system-check-performance-targets.md`), CPU profile notes (`system-check-cpu-profile-notes.md`). **Target:** Cache-first, async population, quick CLI — see `system-check-cache-first-and-async.md`.

## Purpose

This document defines the **canonical pipeline** for `zqk system check`: phase order, I/O boundaries, reuse rules, and time budgets. Implementations must follow this architecture so performance targets (cold ≤ 20s, warm ≤ 1–2s) are achievable and the same bottlenecks do not reappear.

## Pipeline phases (canonical order)

The system check MUST execute in this order. No phase may start until its predecessors have completed (except where explicitly allowed to overlap with bounded concurrency within a phase).

| Phase | Name | Description | I/O / constraints |
|-------|------|-------------|--------------------|
| 1 | **Cache load/build** | Load object ID cache from disk or rebuild from process dir. | Load: read JSON cache file. Rebuild: one read per file (ID + kind via `ReadIDAndKindFromYAMLFile`); no double read in `createCacheEntry`. Reject cache if any entry missing `FilePath`. |
| 2 | **Warm CAS** | Pre-initialize kind mapper, then merge id→path per kind from object ID cache into CAS indexes, then flush. | **Pre-initialize** DynamicKindMapper before warm workers run (no `GetDirectoryFromKind` → `Initialize()` under lock in workers). Use only cache data: `EnsureCASIndexFromPaths(kind, idToPath)` per kind; no `EnsureCASIndexPopulatedFromScan`. Flush bounded: `FlushAllCASIndexesForProjectRootWithTimeout(projectRoot, 60s)`. Progress: "Merging CAS indexes for N kinds…", "CAS indexes warmed". |
| 3 | **Discovery** | Obtain set of kinds and list paths per kind for validation. | When cache is populated: kinds from **`ObjectIDCache.GetKinds()`** only; do not call `discoverObjectKinds(processDir)` (field registry LoadFields + GetAllKinds). When cache not populated: fall back to discovery. List paths via CAS index / directory list (no per-file read for discovery). |
| 4 | **Enqueue** | Queue discovered paths for validation. | Validation state cache (mtime + optional checksum) checked here; cache hit skips re-validation. |
| 5 | **Validation** | Workers validate enqueued objects. | Single `StorageProvider` for entire run; ref lookups O(1) via cache. Bounded concurrency (e.g. semaphore), goroutine labels, WaitGroup, clear done/progress callback. |

## Key implementation rules

### Kind mapper and warm phase

- **Pre-initialize** the kind mapper (in parallel where applicable, then wait) **before** starting warm workers. Workers must not trigger `DynamicKindMapper.Initialize()` under lock (spec load, YAML, dir scan), which serializes workers and dominates warm time.
- Warm phase uses **cache only**: id→path per kind from object ID cache → `EnsureCASIndexFromPaths` → flush. No directory scan or per-file reads in warm.

### Kinds source when cache is populated

- When the object set comes from the object ID cache, **kinds MUST come from `ObjectIDCache.GetKinds()`**.
- Do **not** use `discoverObjectKinds(processDir)` when cache is populated; that duplicates work (field registry LoadFields + GetAllKinds) and can diverge from the cached set.

### Post–cache-load diagnostic loop

- The loop that iterates over all object IDs and calls `HandleCacheLoad` for each is **optional**.
- Run this loop **only when `ZQK_CACHE_DIAGNOSTIC_OBJECTS` is set**; otherwise skip after cache is ready to avoid redundant work.

### Reuse and concurrency pattern

- Use a **single pattern** for parallel work: bounded concurrency (e.g. semaphore 8), goroutine labels, WaitGroup, and a clear done/progress callback.
- Single shared storage/validator/caches for the run; no per-object creation of loaders that can be shared (e.g. kind mapper, spec loader, lifecycle loader).

### Progress emission

- **Progress emission MUST NOT block the pipeline.** Delivering progress (e.g. "Merging CAS indexes…", "CAS indexes warmed") to the coordinator or CLI must be non-blocking (e.g. fire-and-forget or async delivery). Blocking on subscriber I/O (e.g. stderr) would stall phases and violate time budgets.

### Post–warm cache verification

- An **optional, bounded** cache verification may run after phase 2 (Warm CAS) completes. It may take a read lock on the object ID cache to confirm size/ready state for logging or diagnostics. It **MUST** be bounded (e.g. timeout or goroutine with select/time.After); if the lock cannot be acquired in time, the implementation MUST proceed to phase 3 (Discovery) and MUST NOT block the pipeline beyond the timeout. Phase 2 is considered complete when cache load/build and warm have finished; this verification is best-effort only.

## Time budgets (reference)

- **Cold:** ≤ 20 s for typical repos (~10k objects). Cache load/build + warm CAS (with pre-init) + discovery (kinds from cache when applicable) + validation.
- **Warm:** ≤ 1–2 s. Load caches + merge/flush CAS + discovery from index + cache lookups + stat; no rebuild, no discovery from disk.

## Traceability

Requirements and criteria that enforce this pipeline and these rules are maintained as process objects and are traceable via the zqk CLI:

- **Requirement:** REQ-200 — System check pipeline architecture (references this doc and performance targets).
- **Criteria:** CRIT-9501 (kind mapper pre-init), CRIT-9509502 (discovery uses GetKinds when cache populated), CRIT-9509503 (HandleCacheLoad loop only when ZQK_CACHE_DIAGNOSTIC_OBJECTS), CRIT-9504 (canonical phase order), CRIT-9509505 (bounded concurrency and shared caches).

List/link: `zqk object get REQ-200`, `zqk object list criteria --filter requirement_refs=REQ-200` (after linking criteria to requirement if desired).

## References

- `docs/architecture/system-check-performance-targets.md` — Product targets and how we get there.
- `docs/architecture/system-check-cpu-profile-notes.md` — Profiling and kind-mapper fix.
- `docs/architecture/system-check-architecture-alignment.md` — Alignment of implementation to this pipeline (gaps and recommendations).
- `docs/architecture/system-check-data-flow.md` — Data flow, completion signal (`checkDone`), wait points, lock sequencing, and why the process could block indefinitely (panic path, fix).
- `docs/process/testing/async-validation-performance-guide.md` — Async validation and metrics.
- Code: `cmd/zqk/system/check_cache.go` (warm, GetKinds), `async_check_helpers.go` (discovery, kinds from cache), `setup_async_validation_helpers.go` (post-cache diagnostic loop), `async_check.go` (discoverFromCache, flow), `cache_coordination.go` (progress emission).
