# Performance Investigation and Recommendations

**Last Verified:** 2026-08-31


**Purpose:** Consolidated findings and prioritized recommendations from architecture docs, profiles, and scheduler/audit backlog. Use for planning and review.

**References:** OBJECT_OPERATIONS_PERFORMANCE.md, CLI_PERFORMANCE_AND_CONSISTENCY.md, PERFORMANCE_BOTTLENECK_AUDIT.md, SCHEDULER_OVERLOAD_AND_TIMEOUT.md, AGGREGATE_AUDIT_CPU_PROFILES.md, PRE_CHANGE_CHECKLIST.md.

---

## 1. Current state (already fixed or mitigated)

| Area | Status |
|------|--------|
| **HashRegistry per (kind, dir)** | Cached in `FileObjectStorage.hashRegistryCache`; one worker per kind/dir (PERFORMANCE_BOTTLENECK_AUDIT §1). |
| **CAS per kind** | Cached in `f.casCache` (PERFORMANCE_BOTTLENECK_AUDIT §2). |
| **Storage provider** | Global cache `GetGlobalStorageProviderCache().GetOrCreate` used by scheduler and audit (PERFORMANCE_BOTTLENECK_AUDIT §3). |
| **Spec loading** | SpecLoader has per-ontology and path cache (sync.Map); lock-free reads on cache hit (spec_loader.go). Ensure no hot path calls `ClearCache()`. |
| **List/count slots** | Increased to 16; bounded semaphore (SCHEDULER_OVERLOAD_AND_TIMEOUT §2.1). |
| **Goroutine ceiling** | Critical job types (e.g. audit_event_aggregation, retention_tolerance) can bypass so jobs still submit when over ceiling (SCHEDULER_OVERLOAD §4). |
| **Cache prewarm tier-3** | Tier-3 tasks respect timeout so wait goroutine does not block forever (SCHEDULER_OVERLOAD §5). |
| **Object count stall** | Count no longer triggers full disk scan per kind; returns from CAS index + legacy ID scan (CLI_PERFORMANCE_AND_CONSISTENCY §5). |
| **Bulk create flush** | `WithBulkCreateDeferFlush` used for scan-tests job generation; single FlushKind after batch (object_storage_file_create.go, audit_events.go). |
| **Audit_event max_count** | High-volume cache path added so retention deletes by oldest IDs from cache instead of List over 260k+ files (handlers_retention_tolerance.go). |
| **Aggregation retention** | When audit_event count > 100k, retention batches and shortened retention tier increase delete throughput (handlers_aggregation.go). |

---

## 2. High-impact recommendations (priority order)

### 2.1 Defer FlushKind on Create/Delete (object ops latency)

- **Problem:** Every Create (and Delete) blocks on `FlushKind(kind, 5s)` so List sees the new object. Profile: ~60%+ in syscall; queue drain + batch timeout dominate.
- **Recommendation:** **Option A** in OBJECT_OPERATIONS_PERFORMANCE: make FlushKind non-blocking for single-object Create/Delete. Enqueue index update and return; same process already sees in-memory index. Disk index and cross-process visibility become eventually consistent (worker persists in background).
- **Tradeoff:** New process or List that reads from disk before worker runs might miss the object until next flush. Mitigation: flush on timer or on next List for that kind.
- **Files:** `pkg/storage/content_addressable_storage_crud.go` (Create path), Delete path (index-remove done channel + FlushKind). Add a context key or storage option to “defer flush” for single ops, or make the wait optional with a short timeout (e.g. 100 ms) then return.

### 2.2 Change journal: use global storage provider

- **Problem:** `CreateChangeJournalEntry` calls `NewFileObjectStorage(projectRoot)` each time (change_journal.go line 69). Under load this builds a new storage instance per write.
- **Recommendation:** Use `GetGlobalStorageProviderCache().GetOrCreate(ctx, projectRoot)` and type-assert to `*FileObjectStorage` when needed (PERFORMANCE_BOTTLENECK_AUDIT §5).
- **Impact:** Reuses same HashRegistry/CAS caches and avoids repeated storage init.

### 2.3 Field registry and kind mapper: pre-warm at startup

- **Problem:** First `GetFieldsForKind(kind)` can trigger `LoadFields()` (all specs). First `GetStrategyForKind(ctx, kind)` can trigger `kindMapper.Initialize()` (directory/spec scan). OBJECT_OPERATIONS_PERFORMANCE profiles: ~26% spec load, ~14% field registry, ~12% kind mapper.
- **Recommendation:** Pre-warm at storage init or scheduler/CLI init: call `FieldRegistry.LoadFields()` once, and run `kindMapper.Initialize()` once (or ensure single init per process). Cache prewarm job already does Tier 1–2; ensure object-create path never pays full LoadFields on first touch.
- **Check:** No code path clears field registry or kind mapper cache on create/update (PRE_CHANGE_CHECKLIST: no LoadFields on hot path).

### 2.4 Batch CAS index updates (aggregate-audit and cleanup)

- **Problem:** AGGREGATE_AUDIT_CPU_PROFILES: `(*IDIndex).SetMapping` and `loadLocked` dominate; each update does load full index → merge one entry → save. O(n) index reads/writes per batch of changes.
- **Recommendation:** Use `SetMappings` (or equivalent batch API) where possible so index is loaded once and saved once per batch. When applying cleanup or aggregation results, collect all id→hash changes and apply in one go.

### 2.5 Audit event batching (reduce per-op cost)

- **Problem:** Each Create/Update can call `CreateAuditEventWithBuilder`; goroutine start and audit build show in CPU (OBJECT_OPERATIONS_PERFORMANCE §5).
- **Recommendation:** For bulk create/update, use existing audit buffer and flush once per batch where possible. Single-op path: keep minimal payload on hot path; defer heavy serialization or metadata to background.

### 2.6 HashRegistry outside FileObjectStorage (follow-up)

- **Problem:** verifyHashRegistrySaveForSystemObject, snapshot_manager, object_storage_file_helpers verify path, deferred_hash_update still call `NewHashRegistry` (PERFORMANCE_BOTTLENECK_AUDIT §4).
- **Recommendation:** Prefer passing `*FileObjectStorage` (or HashRegistryProvider) into these helpers so they use `f.newHashRegistry(...)` when available. Low frequency today; address when those paths become hot.

---

## 3. Scheduler and retention (operational)

- **High-volume cache for retention:** Ensure cache_prewarm or aggregation runs before retention_tolerance so the audit_event max_count fast path (QueryOldestByKind) is used; otherwise retention falls back to CAS or batched List.
- **Job count and LoadJobs:** Keep scheduler_job count bounded; avoid creating large numbers of one-off jobs (e.g. scan-tests) without cleanup. SCHEDULER_MEMORY_AND_HANG_ANALYSIS: 1500+ jobs can make LoadJobs slow and contribute to pool_creation_declined.
- **Timeouts:** Long-running jobs (SCH-002, retention) use max_runtime_seconds; ensure critical jobs bypass goroutine ceiling so they at least get submitted and timeout on their own terms.

---

## 4. How long until object counts are remedied?

**Short answer:** The performance recommendations (FlushKind defer, change_journal, pre-warm, batch index, etc.) **do not by themselves reduce object counts**. They reduce timeouts and contention so that jobs (retention, aggregation) are more likely to complete. **What actually reduces counts** is retention_tolerance max_count and audit_event_aggregation’s retention phase deleting audit_events.

**Why progress has stalled:** The retention fast path (delete by oldest IDs from the high-volume event cache) is **only used when the cache is populated**. The cache is built at the start of SCH-002 (audit_event_aggregation) with a **10s timeout**. With 260k+ events, the build does not finish in 10s, so the cache is never populated and retention stays on the slow path (~30 deletes per batch). So delete rate never catches up.

**Change made:** The aggregation handler’s cache build timeout is increased from 10s to **5 minutes** (or job deadline minus 1 min if sooner) so the first SCH-002 run can populate the cache. Once the cache is built, retention_tolerance can use `QueryOldestByKind` and delete up to 90k per run (750×120 batches).

**After rebuild and restart (with the new timeout):**

1. **First SCH-002 run** (e.g. next 15-min tick): Builds the high-volume cache (up to 5 min). If it completes, the cache is populated for the rest of the daemon’s lifetime.
2. **Next retention_tolerance run**: Uses the cache path; deletes up to 90k audit_events in one run (if over max_count).
3. **Subsequent SCH-002 runs**: Use the cache for their own retention phase and can delete up to 40k per run (critical tier when count > 100k).

**Rough timeline:** If the cache builds successfully on the first SCH-002 run, then within **2–4 retention/aggregation cycles** (about **2–8 hours**, depending on schedule) audit_event count can drop from 260k toward target. If creation rate is high, net progress is slower; the goal is delete rate > creation rate. If the cache still fails to build (e.g. job deadline too short), increase SCH-002’s `max_runtime_seconds` or add a dedicated cache-build step at daemon start (e.g. in cache_prewarm) with a long timeout.

---

## 5. Verification

- **CRUD baseline:** `ZQK_CRUD_PROFILE=1` with TestCRUDBaselineConsistency; compare baseline JSON and pprof before/after changes (OBJECT_OPERATIONS_PERFORMANCE).
- **System check:** system-check-cpu-profile-notes.md; run with `--cpu-profile` and inspect top/cum.
- **Aggregate-audit:** AGGREGATE_AUDIT_CPU_PROFILES.md; re-profile after batching index updates.
- **Policy:** Before each change, apply PRE_CHANGE_CHECKLIST.md (§2–3: response time, no blocking, no full cache clear on hot path).

- **Cache and retention:** After restart, confirm logs for "High-volume event cache built successfully" (SCH-002) and "Using high-volume cache for audit_event max_count" (retention). If absent, the cache is not populated and retention is still on the slow path.

### Failure rates (hash registry save timeouts)

- **What to monitor:** `grep rolled .zqk/scheduler/daemon.stdio` (or equivalent). Each line is a create/update that was rolled back because the hash registry save did not complete within the caller’s max wait (e.g. "hash registry save did not complete within 5m0s").
- **High rate means:** The save worker for that kind is not finishing one write within the timeout (one write = marshal + write + sync of the full registry; for 260k+ entries this can take minutes). Callers retry up to 3 times then roll back; the worker may still complete in the background.
- **Change made:** Large-registry save timeout was increased from 90s to **5 minutes** (same as small registries) so a single slow write does not cause spurious rollbacks. If rollback rate stays high after this, the worker is consistently taking >5m per write (disk or CPU bound); consider incremental or async hash registry persist as a future improvement.
