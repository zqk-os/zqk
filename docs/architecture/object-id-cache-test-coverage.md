# Object ID Cache: Test Coverage Evaluation

**Date:** 2025-02-06  
**Context:** After system-check optimizations (single storage warm, cache-driven discovery, warm-after-Load path).

## Summary

| Area | Coverage | Notes |
|------|----------|--------|
| **Cache loading** | Partial | BuildCache, SaveCache, LoadCache, tryLoadExistingCache exercised; EnsureObjectIDCacheReady only indirectly |
| **Validation (ref + cache)** | Good | Ref validation with cache hit/miss and stale cache covered |
| **Invalidation** | Partial | InvalidateKind used in streaming template tests; InvalidateObjectIDCache / UpdateObjectIDCache / InvalidateObjectIDCacheKind not directly tested |
| **Speed** | One test | TestWarmCASIndexesFromCache_Performance bounds build+warm; no test for EnsureObjectIDCacheReady timeout or discovery speed |
| **New surface** | Gaps | GetEntriesByKind, IsPopulatedForProject, discoverFromCache, storageForWarm path untested |

---

## 1. Cache loading

**What exists**

- **check_impl_cache_refresh_test.go**: BuildCache(projectRoot, true), SaveCache, LoadCache; cache populated and entries verified with Get(); process mtime and refresh behaviour; multiple cache instances loading from same file.
- **check_cache_warm_performance_test.go**: BuildCache(projectRoot, true) with 150 CAS entries; asserts duration &lt; 25s.
- **check_duplicate_id_test.go**, **check_bulk_path_test.go**, **auto_fix_scenario_test.go**, integrity tests: BuildCache on test roots.

**Gaps**

- **EnsureObjectIDCacheReady** is not called directly in tests (only via production code paths). No test that:
  - Passes `storageForWarm` and asserts the same storage is warmed (no cold refs).
  - Asserts behaviour when loader returns early (cache already loaded): warm still runs for provided storage.
- **tryLoadExistingCache** is only exercised indirectly via BuildCache (load path when file exists and is valid). No dedicated test for invalid/stale file, wrong project, or missing file.
- **Loader timeout**: no test that EnsureObjectIDCacheReady respects loader timeout or returns an error on timeout.

**Recommendation**

- Add a test that calls EnsureObjectIDCacheReady with a non-nil storageForWarm and (e.g. after a second Load for same project) verifies that storage’s CAS index was used (e.g. discovery or ref check sees objects).
- Optionally: test EnsureObjectIDCacheReady(..., nil) vs (..., storage) and assert warm is skipped inside BuildCache when storage is provided (to avoid regressing the single-warm behaviour).

---

## 2. Validation (ref + cache)

**What exists**

- **check_impl_cache_refresh_test.go**: TestObjectIDCache_ReferenceValidationWithStaleCache: ref validation fails when target is not in cache; after rebuild, ref validation passes. Uses checkReferencesWithCache(..., cache, nil).
- Various integration tests pass ObjectIDCache into check flows and rely on cache for ref resolution.

**Gaps**

- No test that explicitly verifies ref validation uses **storage.Exists** on cache miss (fallback path).
- Cache item strategy / diagnostics on cache miss are not asserted.

**Recommendation**

- Keep as is for now; ref validation + cache is well covered by the stale-cache test. Add a test for storage fallback only if we want to lock in that behaviour.

---

## 3. Invalidation

**What exists**

- **streaming_template_cache_test.go** / **streaming_template_cache_defensive_test.go**: ObjectIDCache.InvalidateKind("criteria") and concurrent Get + InvalidateKind. These use the cache type’s InvalidateKind, not the package-level InvalidateObjectIDCacheKind.
- **check_impl_cache_refresh_test.go**: Direct map delete to simulate stale cache (delete(cache.cache, "CRIT-9003")), not the public Invalidate API.

**Gaps**

- **InvalidateObjectIDCache(id)** (single-entry invalidation, audit event): no test.
- **UpdateObjectIDCache(id, kind, filePath)** (create/update path, audit event): no test.
- **InvalidateObjectIDCacheKind(kind)** (bulk invalidation, audit event): no test.
- **BulkInvalidateObjectIDCache(ids, projectRoot)**: no direct test (only via check flow that uses cacheInvalidationHandler).

**Recommendation**

- Add narrow unit-style tests: after BuildCache, call InvalidateObjectIDCache("ID"), then Get("ID") and assert miss; call UpdateObjectIDCache and assert Get returns updated entry; call InvalidateObjectIDCacheKind("kind") and assert all entries for that kind are gone. Optionally assert audit events if they are observable in tests.

---

## 4. Speed

**What exists**

- **TestWarmCASIndexesFromCache_Performance**: BuildCache(projectRoot, true) with 150 CAS entries; elapsed &lt; 25s. Protects against O(entries × scan) warm regression.

**Gaps**

- No test that **discovery** (discoverFromCache or discoverObjectsParallel) completes within a time bound for a given cache size or kind count.
- No test that **EnsureObjectIDCacheReady** (with or without storageForWarm) completes within a bound when cache is already loaded (fast path).
- No benchmark or test that compares “discovery from cache” vs “discovery from storage” for the same project.

**Recommendation**

- Consider a test: build cache, then run discoverFromCache (or the async check path that uses it) and assert completion within e.g. 5s for a few hundred entries. This would catch regressions that make cache-driven discovery slow.
- Optional: benchmark discoverFromCache vs discoverObjectsParallel on a fixed test tree and document expected ratio.

---

## 5. New surface (GetEntriesByKind, IsPopulatedForProject, discoverFromCache, storageForWarm)

**What exists**

- None of these are directly tested.

**Gaps**

- **GetEntriesByKind(kind)**: no test that it returns the correct subset of entries and only entries with that kind.
- **IsPopulatedForProject(projectRoot)**: no test that it returns true only when metadata.ProjectRoot matches and entry count &gt; 0.
- **discoverFromCache**: no test that it streams the same set as the cache for given kinds/targetIDs, or that filtering by targetIDs works.
- **storageForWarm path**: no test that warm uses the provided storage and that discovery/ref validation then see warmed data.

**Recommendation**

- **GetEntriesByKind**: add a short test: BuildCache with mixed kinds, call GetEntriesByKind("criteria"), assert count and that every entry has Kind == "criteria".
- **IsPopulatedForProject**: test that it returns false for empty cache, false for wrong project root (if testable without flakiness), true after BuildCache for that project.
- **discoverFromCache**: test with a small cache (e.g. 2 kinds, a few entries); consume stream and collectFinalResults(); assert total count and that every scannedFile has Path set and matches cache. Add a case with targetIDs and assert only those IDs appear.
- **storageForWarm**: covered under “Cache loading” above (EnsureObjectIDCacheReady with storageForWarm).

---

## 6. Adequacy verdict

- **Cache loading:** Adequate for BuildCache/LoadCache/SaveCache and refresh behaviour; **weak** for EnsureObjectIDCacheReady and the storageForWarm / warm-after-Load behaviour.
- **Validation:** Adequate for ref validation with cache and stale-cache scenario.
- **Invalidation:** **Weak** for public invalidation APIs (InvalidateObjectIDCache, UpdateObjectIDCache, InvalidateObjectIDCacheKind); InvalidateKind is exercised via streaming template tests.
- **Speed:** One solid warm performance test; **no** tests for discovery or EnsureObjectIDCacheReady timing.
- **New surface:** **Gaps** for GetEntriesByKind, IsPopulatedForProject, discoverFromCache, and the storageForWarm path.

**Overall:** Coverage is adequate to catch many regressions (build/load/save, ref validation, warm performance) but is **not adequate** for the new optimisations (single storage warm, cache-driven discovery) and for invalidation APIs. Adding the recommended tests would bring coverage to adequate for cache loading, validation, invalidation, speed, and the new code paths.
