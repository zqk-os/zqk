# CAS list vs get consistency: one source of truth

**Last Verified:** 2026-08-31


**Related ADR:** [ADR-CAS-PENDING-VISIBILITY-LAYER-v1.0.md](./decisions/ADR-CAS-PENDING-VISIBILITY-LAYER-v1.0.md) · `[REDACTED-ID]` · `REQ-CAS-PENDING-001` · plan `[REDACTED-ID]`.

## Pending visibility layer (target — normative)

Cross-process **read-your-writes** must not depend on GET waiting for async durable-index merge. After a mutation reports **success**, a subsequent process must resolve the id without an O(n) directory scan.

**GET order (target):**

1. **Pending / creation visibility cache** — small, cross-process-visible `id → hash` published **before** success returns (WAL-intent durable + id/hash known on write-behind paths).
2. **In-memory CAS index** — same process (`setIndexMappingInMemory`).
3. **On-disk CAS index** — shared durable locator.
4. **Not found** — fail-closed on the hot path (no full-kind scan). TRACK: `[REDACTED-ID]`.

**Writer contract:** success ⇒ pending entry visible to other processes. Background work promotes pending → durable index; pending entries evict once the disk index confirms the same mapping.

**Shutdown contract:** reject new writes → dump pending to durable index → drain write-behind/WAL → exit. Force kill remains residual risk; restart reconciles WAL (+ any pending that reached disk).

**Not the primary fix:** get-on-miss full scans, blanket `recover-cas` / `fix-hash-mismatches`, or agent recreate-on-not-found.

## Problem (list vs get; historical get scan)

- **object get** (current hot path): `getObjectFilePath` → on-disk/in-memory index miss ⇒ **not found** when CAS is available (fail-closed). Cold/broken CAS-only paths may still discover via scan.
- **object list** can show fewer objects than exist on disk (storage uses **CAS index** via `cas.ListIDs()`; index may be empty or stale).

Historically get had a discovery fallback (scan on miss); that conflicted with hot-path latency and was removed for CAS-available kinds. List still uses the index only. When the durable index lags a successful write and **pending is missing**, get and list both under-report until flush/refresh — agents then fork ids. The pending layer closes the get gap; list disparity refresh remains for bulk alignment.

## Root cause

- **Source of truth** for content is **files on disk**.
- The **CAS index** (id → hash) is a **durable locator cache**. It is populated by: (1) Create/Update/Delete (in-memory + async write queue), (2) warmCASIndexesFromCache, (3) **background refresh** when list/count detect index-vs-disk disparity, (4) **target:** promotion from the pending visibility layer.
- **List** must not block on a full scan. Per cache-first async design: return current cache; detect disparity; trigger **background** refresh so the next list (and other caches) see the updated index.

## Consistency model

- **Same process:** Create/Update/Delete update the in-memory index; List sees them. Write queue persists to disk; CLI flushes on exit.
- **Cross-process (target):** Writers publish to the **pending visibility layer** at success; readers check pending before the on-disk index. Durable index flush (CLI on exit; scheduler after scheduler_job writes; shutdown dump) remains required for long-term consistency. When the durable index is still stale and pending has already been promoted/evicted, **disparity detection + background refresh** corrects list/count for all CAS kinds.
- **Empty index:** List/count call **EnsureCASIndexPopulatedFromScan(kind)** **synchronously** so we have something to return; then use the refreshed index.
- **Non-empty index, disparity:** Compare on-disk count with `len(casIDs)`. To avoid scanning on every list, we **cache** the last disk count and the **mtime(s)** of the kind dir (and, for bucketed kinds, **each bucket subdir**) when we last counted. If mtimes haven't changed, we use the cached count (no ReadDir of file contents). If mtimes differ or cache miss, we run **countHashNamedFilesInKindDir** (ReadDir only, no YAML), update the cache, then compare. Bucket mtimes are required because new files in an existing bucket don't change the parent kind dir's mtime. If count ≠ len(casIDs), **trigger background** **EnsureCASIndexPopulatedFromScan(kind)**. List/count return **immediately** with the current index. One refresh per project+kind at a time (deduplicated).

This applies to **all CAS kinds** (scheduler_job, backlog_item, audit_event, etc.), not only scheduler_job. No kind-specific sync scan.

## Implementation (done)

- **countHashNamedFilesInKindDir(kind)** in `object_storage_file_discovery.go`: fast count of hash-named files in the kind dir (and bucket subdirs). ReadDir only; no YAML reads.
- **getKindDirMtimes(kind)** in `object_storage_file_discovery.go`: returns kind dir mtime and, for bucketed kinds, each bucket subdir's mtime (UnixNano). Used to avoid re-counting when nothing changed.
- **getCachedDiskCountOrScan(kind)** in `object_storage_file_discovery.go`: returns disk count. If cached entry exists and kind dir + all bucket mtimes match, returns cached count. Otherwise runs countHashNamedFilesInKindDir and stores cache entry (diskCount, kindDirMtime, bucketMtimes).
- **triggerBackgroundCASIndexRefresh(kind)** in `object_storage_file_discovery.go`: if not already refreshing this project+kind, start a goroutine that runs **EnsureCASIndexPopulatedFromScan(kind)**. Deduplication via `casIndexRefreshInProgress` (sync.Map keyed by projectRoot+kind).
- **List path** (`object_storage_file_list_main.go`): get `casIDs`. If `len(casIDs) == 0`, sync **EnsureCASIndexPopulatedFromScan** and re-fetch. Else, `diskCount := countHashNamedFilesInKindDir(filter.Kind)`; if `diskCount != len(casIDs)`, **triggerBackgroundCASIndexRefresh(filter.Kind)**. Use current `casIDs` for the response.
- **Count path** (`object_storage_file_list_query.go`): same logic in **countFromCASIndex** so count and list stay aligned.
- **EnsureCASIndexPopulatedFromScan(kind)**: full scan (hash-named files, read `id` from YAML), **SetMappings** (load index, merge, save to disk). Used when index is empty (sync) and when disparity triggers background refresh (async).
- **Writers flush:** CLI flushes on exit; scheduler flushes `scheduler_job` after update/disable/operation execution so the index file is updated and disparity is less likely.

## Relation to other caches

Per **system-check-cache-first-and-async** and the design to keep id cache, list cache, and validation cache current with minimal blocking: the CAS index is the “id/list” source for CAS kinds once pending has been promoted. Disparity detection + background refresh updates this cache without blocking the CLI. When the background scan completes, the updated index is persisted; subsequent list/count and Get use it. No swap of in-memory caches is required for the CAS index itself — a single index is updated in place by **SetMappings** (load/merge/save), and the in-memory CAS instance already holds that index, so the next read sees the new data. For validation and object-ID caches, pre-warm and async population follow their own flows; CAS index refresh keeps the durable locator aligned with disk for **all** CAS kinds. The **pending visibility layer** is a separate, short-lived cache in front of that locator for read-your-writes — see the ADR.

---

## Operational hazards: `recover-cas`, `fix-hash-mismatches`, and the scheduler

These commands exist for **narrow** repair cases. Used broadly—especially while the **scheduler daemon** is running—they can **remove index rows**, **contend on the same index lock files** as live writers, and make **`object list` / `object get` disagree with YAML on disk** until indexes are reconciled. That looks like “the scheduler died” or “CVS objects vanished” even when the hash-named files are still present.

### `zqk system recover-cas`

- **What it does (typical):** For an id in the CAS index, if the **hash-named file** the index points at is **missing**, it may **remove the id from the index** (default options), or try ID-based recovery paths.
- **Hazard:** After a normal **`object update`**, content often moves to a **new** hash filename. The index should move with it. If the index is **wrong or lagging** but a **new** `*.yaml` hash file **does** exist, `recover-cas` can still look like “hash file missing” for the **old** hash and **drop the mapping**. The object may still exist on disk under the **new** name, but **`list` stops showing it** until the index is repopulated (e.g. `EnsureCASIndexPopulatedFromScan`, disparity refresh, or consistent manual repair).
- **Guidance:** Do **not** run `recover-cas` on a specific id as a first step when troubleshooting “get not found”—first confirm whether a **hash-named** file under the kind directory still contains that **`id:`**. Prefer **`zqk system check`** (and **`--auto-fix`** only when appropriate to your tier policy) or **wait for background CAS refresh** after writes. Use `recover-cas` when you have evidence the index row is stale **and** there is no valid on-disk blob, or per runbook.

### `zqk system fix-hash-mismatches` (PRUNED)

- **`--kind <kind>` with no object IDs:** The command **lists every id of that kind** and runs the strategy **per id**. For large kinds this is slow, contends on indexes, and is easy to misuse.
- **`--strategy force-reindex`:** Today this strategy **delegates to remove-stale-index** (implementation in `pkg/storage/hash_mismatch_fix_strategy.go`)—it **removes index entries**, it does **not** perform a full “rebuild index from all YAML” in one safe step. Running it across **all** `convergence_session` objects is effectively **mass index stripping**, not a gentle heal.
- **Guidance:** Treat **`fix-hash-mismatches` as last-resort, id-scoped repair** (explicit object IDs or a small input file). **Do not** run **`--kind convergence_session`** (or other small, governance-critical kinds) with **blanket** strategies without a runbook. Prefer **`zqk system check --auto-fix`** (per command help and tier policy) after understanding the mismatch. If convergence sessions disappear from **`object list`** but files remain under `.zqk/process/convergence_sessions/<hash>.yaml`, reconcile the **kind index** with **on-disk hash files** (and avoid concurrent scheduler + CLI hammering the same index).

### Scheduler interaction

- The scheduler and interactive **`zqk`** CLIs share the same **`.zqk/process/...`** CAS index files. Long-running or **whole-kind** repair commands can **hold or retry locks** and **time out** while the daemon is also writing. If the system looks wedged after a repair attempt, **restart the scheduler** after indexes are consistent, and avoid overlapping **bulk** `fix-hash-mismatches` with heavy scheduler job churn.
