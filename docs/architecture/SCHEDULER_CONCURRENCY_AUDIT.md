# Scheduler and Storage Concurrency Audit

**Last Verified:** 2026-08-31


**Purpose:** Single place to see all bounded concurrency settings so we don’t create a configuration that only runs well on high-end, multi-core machines. Use when tuning for low/mid-tier hardware or when reviewing CPU usage after a scheduler dump.

**References:** unbounded-concurrency-fixes.md, list_count_concurrency.go, CLI_PERFORMANCE_AND_CONSISTENCY.md.

---

## 1. Summary: already CPU-aware or configurable

| Component | Default / cap | CPU-aware? | Env/config? |
|-----------|----------------|------------|-------------|
| Async validator workers | NumCPU×2, clamp [4, 32] | Yes | Overridable via `concurrency.SetGlobalConcurrencyConfig` |
| Async router workers | NumCPU, clamp [2, 16] | Yes | Same |
| Retention bulk delete workers | default 20, max 64 | No | **Yes:** `BULK_DELETE_WORKERS` per job env |

All other values below are **fixed constants** (no env, no CPU scaling). They are safe on low-core machines because they are **absolute caps**, not “per core,” but on 2–4 core machines you may want to lower some of them to reduce CPU contention.

---

## 2. Scheduler (pkg/scheduler)

| Constant | Location | Value | Notes |
|----------|----------|--------|--------|
| `globalGoroutineCap` | scheduler.go | 512 (default) | Process-wide goroutine budget (pools + one-off goroutines reserve from this). Overridable via `ZQK_SCHEDULER_GOROUTINE_CAP` (clamped to [128, 4096]). |
| `triggeredPoolSize` | scheduler.go | 64 (default) | Max concurrent job executions (cron, event, lifecycle, immediate). Overridable via `ZQK_SCHEDULER_TRIGGERED_POOL_SIZE` (clamped to [8, 256]). |
| `triggeredQueueSize` | scheduler.go | 512 | Queue depth for triggered work. |
| `maxConcurrentHydrations` | job_management.go | 16 | Job hydration workers (load jobs from storage at startup/reload). |
| `defaultBulkDeleteWorkers` | handlers_retention_tolerance.go | 20 | Retention job bulk-delete workers when `BULK_DELETE_WORKERS` not set. |
| `maxBulkDeleteWorkers` | handlers_retention_tolerance.go | 64 | Cap for `BULK_DELETE_WORKERS` env. |
| `GoroutineCountWarningThreshold` | pkg/concurrency/goroutine_ceiling.go | 1500 | Health monitor logs warning above this. |

**Recommendation for low-core:** Use `ZQK_SCHEDULER_TRIGGERED_POOL_SIZE` and/or `ZQK_SCHEDULER_GOROUTINE_CAP` to reduce concurrency on small machines (e.g. pool size 24–32). Defaults remain safe for high-core machines.

---

## 3. Storage: list/count and CAS (pkg/storage)

| Constant | Location | Value | Notes |
|----------|----------|--------|--------|
| `listCountMaxConcurrent` | list_count_concurrency.go | 16 (default) | Max concurrent file-heavy List or Count operations. Each operation can use up to N workers (see below). Overridable via `ZQK_LIST_COUNT_MAX_CONCURRENT` (clamped to [4, 64]). |
| `listMaxConcurrentReads` / `listMaxWorkers` / `maxWorkers` | list_count_concurrency.go, object_storage_file_list_main.go, object_storage_file_list_query.go | 64 (default) | Workers per single List or Count (readdir/open, parse). Now comes from `getListReadWorkers()` with `ZQK_LIST_READ_WORKERS` override (clamped to [4, 128]). |
| `casReadDirPoolWorkers` | content_addressable_storage_crud.go | 16 | CAS bucket-search ReadDir pool (already kept low for CPU; see INDEX_FIRST_LOW_CPU_SCAN_DESIGN.md). |
| `casReadDirPoolQueue` | content_addressable_storage_crud.go | 256 | Queue for CAS ReadDir tasks. |

**Recommendation for low-core:**  
- Lower `ZQK_LIST_COUNT_MAX_CONCURRENT` (e.g. 8) to reduce peak load when many jobs do list/count at once.  
- Lower `ZQK_LIST_READ_WORKERS` (e.g. 16 or 32) so each list/count operation uses fewer workers, reducing CPU and goroutine pressure.

---

## 4. Storage: high-volume cache build and bulk delete

| Constant | Location | Value | Notes |
|----------|----------|--------|--------|
| `workers` (cache build) | high_volume_event_cache.go | 64 (default) | Parallel workers when building cache from CAS index (Read created_at per ID). Overridable via `ZQK_HIGH_VOLUME_CACHE_BUILD_WORKERS` (clamped to [4, 128]). |
| BulkDeleteOptimized default | bulk_delete_optimized.go | 10 | Used when caller passes 0; retention uses getBulkDeleteWorkers (20 default, env-capped 64). |
| audit_aggregation BulkDeleteOptimized | audit_aggregation.go | 20 | Hardcoded worker count for parallel deletion in aggregation. |

**Recommendation for low-core:**  
- High-volume cache build: consider env `ZQK_HIGH_VOLUME_CACHE_BUILD_WORKERS` (default 64, e.g. 16 on small machines) so cache prewarm/aggregation doesn’t spike CPU.  
- Bulk delete is already tunable per job via `BULK_DELETE_WORKERS`; keep default 20 and max 64.

---

## 5. Transceiver (async router)

| Source | Value | Notes |
|--------|--------|--------|
| `concurrency.GetGlobalConcurrencyConfig().AsyncRouterMaxWorkers` | NumCPU, clamp [2, 16] | CPU-aware; used when creating async router. |
| Profile `max_workers` | e.g. 10 in profile_loader | Can override per profile. |

No change needed for low-core; already scaled.

---

## 6. Scheduler dump

Use **`zqk scheduler dump`** to capture goroutine dump, heap profile, and thread info to `.zqk/scheduler/diagnostics/`. Check scheduler-events.json for `operation=dump` `status=complete`. Inspect goroutine count and thread count over time via scheduler_health_metric to confirm we’re not creeping toward the ceiling on low-core machines.
