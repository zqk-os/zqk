# System check performance targets

**Last Verified:** 2026-08-31


**See also:** `docs/architecture/CLI_PERFORMANCE_AND_CONSISTENCY.md` for the general CLI contract (1 s max response in most cases, async with progress when long-running) and caching rules. System check targets below must align with that.

**Product targets (non-negotiable):**

- **Cold run (no validation cache, or first run after changes):** full `zqk system check` completes in **≤ 20 seconds** for typical repos (~10k objects).
- **Warm run (validation cache hit, object ID cache + CAS index already warm):** **≤ 1–2 seconds**.

The CLI timeout for system check is a **safety net only**. The goal is to meet these targets, not to allow long timeouts.

## Pipeline phases (canonical order)

The system check pipeline runs in this strict order. See `docs/architecture/system-check-pipeline.md` for full architecture and specs.

1. **Cache load/build** — Object ID cache: load from disk or rebuild (one read per file; no double read).
2. **Warm CAS** — Pre-initialize kind mapper (see below), then merge id→path per kind from cache into CAS indexes, then flush (bounded 60s).
3. **Discovery** — List paths per kind; when using cache, kinds MUST come from `ObjectIDCache.GetKinds()` (not from field-registry discovery).
4. **Enqueue** — Queue paths for validation (validation state cache checked here).
5. **Validation** — Workers validate; ref lookups O(1) via cache; single storage for entire run.

**Consistency rule:** When the object set comes from the object ID cache, kinds must be obtained from the cache (e.g. `GetKinds()`), not from `discoverObjectKinds(processDir)` (field registry LoadFields + GetAllKinds), to avoid redundant work and inconsistent behavior.

## How we get there

### Warm path (1–2 s)

- **Object ID cache:** Load from disk (JSON); no rebuild.
- **CAS indexes:** Already warm (kind mapper pre-initialized; merge + flush only); no per-entry path resolution.
- **Discovery:** Kinds from **`ObjectIDCache.GetKinds()`**; `ListPathsForDiscovery` per kind using the CAS index (no field-registry discovery when cache is populated).
- **Validation:** For each discovered path, `Enqueue` checks the **validation state cache** (mtime + optional checksum). If the file is unchanged since `LastValidated`, we treat it as a cache hit and skip re-validation. With all hits, validation work is effectively zero; we only do discovery + cache lookups + stat.

So warm = load caches + list paths (from index) + 10k cache lookups + stat; all in-memory and index-based I/O. Target 1–2 s is achievable if discovery and cache lookups are lean.

**Note:** With `--auto-fix` we clear the validation cache so that run is cold. Warm target applies to a repeat run **without** `--auto-fix` when no files (or only a few) changed.

### Cold path (≤ 20 s)

- **Object ID cache:** Load from disk if valid (and all entries have `FilePath`); otherwise rebuild. Rebuild does one read per file in the scanner (ID + kind in one read via `objects.ReadIDAndKindFromYAMLFile`); no double read in `createCacheEntry`.
- **CAS warm:** **Kind mapper must be pre-initialized** (in parallel, then wait) before warm workers run. Workers must not call `GetDirectoryFromKind` → `DynamicKindMapper.Initialize()` under lock (that serializes workers: spec load, YAML, dir scan). Then: build id→path per kind from object ID cache, call `EnsureCASIndexFromPaths(kind, idToPath)` per kind, then `FlushAllCASIndexesForProjectRootWithTimeout(projectRoot, 60s)`.
- **Discovery:** When cache is used, get kinds from **`ObjectIDCache.GetKinds()`**; only fall back to `discoverObjectKinds(processDir)` when cache is not populated.
- **Post–cache-load diagnostic loop:** The loop that calls `HandleCacheLoad` for each object ID runs **only when `ZQK_CACHE_DIAGNOSTIC_OBJECTS` is set**; otherwise skip to avoid redundant work when cache is populated.
- **Validation:** Each object read once, parse, validate, ref checks. Ref lookups O(1) via cache; single storage for entire run.

Optimizations in place: kind mapper pre-init before warm workers; kinds-from-cache in discovery; optional diagnostic loop; single read per file in cache build; warm from cache only (no `EnsureCASIndexPopulatedFromScan` in check path); validation cache for warm path; single storage for ref validation; bounded 60s CAS flush. **Reuse pattern:** bounded concurrency (e.g. semaphore 8), goroutine labels, WaitGroup, and a clear done/progress callback for parallel phases. **Cache pre-warm:** Scheduler job `cache_prewarm` (Tier 3) builds object ID cache in the background so interactive `zqk system check` often hits warm path.

## Verification

- **E2E test:** `TestSystemCheckAutoFix_E2E` (in `cmd/zqk/root_object_flow_integration_test.go`) runs a single-object cold `system check backlog_item --auto-fix` and asserts completion within the same 20 s ceiling. It records and logs duration so regressions are caught in CI.
