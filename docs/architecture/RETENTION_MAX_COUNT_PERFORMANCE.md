# Retention max_count Performance

## Evidence: abysmal delete performance

The file **`max_count.txt`** (project root) captures real retention runs that showed:

- **Per-batch cost:** Each "batch" of enforce max_count for `audit_event` (with `protect_statuses: ["pending"]`) was doing a **full List** over all ~85k objects. That List path (stream-backed + status filter) does not use the CAS `OldestIDs` fast path; it merges CAS IDs + stream IDs, then **reads every object** to filter by status and sort by `created_at`, then returns the first 5k. So **each batch = ~85k reads**.
- **Observed timings:** Early batches ~1 min each; later batches degraded to **15–20 minutes** per batch (e.g. batch 32 at 13:29 → batch 35 at 13:48; batch 38 at 14:21 → batch 40 at 14:22). A full 40-batch run could take **hours** to delete ~17k events.
- **Runs with almost no progress:** Many runs showed batch 1 (0 deleted), batch 2 (1–2 deleted), then stop—same 85k scanned repeatedly with minimal deletable candidates.

## Root cause

For **stream-backed** `audit_event` with **`protect_statuses`** set:

1. The fast path that uses **CAS index `OldestIDs`** is disabled (it does not filter by status).
2. The handler uses **batched List** (limit 5k, filter `status $nin protect_statuses`, sort by `created_at`).
3. **List** for that kind + filter does **not** use `OldestIDs` (see `object_storage_file_list_main.go`: `!StreamStorageEnabledForKind(filter.Kind)` for that branch).
4. So each List call: builds full ID set (CAS + stream), then **reads all objects** in parallel, filters by status, sorts, returns first 5k. Cost = O(total count) reads **per batch**.

So we do **N batches × 85k reads** per run instead of one index/cache lookup for "oldest deletable" IDs.

## Mitigations (implemented or planned)

- **CAS batch delete:** Transaction commit uses `cas.BatchDelete(ids)` for all-deletes so we don’t pay N× (wait + FlushKind). Deletes themselves are fast once we have IDs.
- **List Limit + OldestIDs:** For **non–stream-backed** kinds with sort by `created_at`, List uses `OldestIDs(offset+limit)` so we read only that many objects. Stream-backed is excluded so this doesn’t help audit_event with protect_statuses.
- **High-volume cache with status:** When the high-volume event cache is **built** (index + stream), we populate **status** on each entry. Retention can then call **`QueryOldestByKindExcludingStatus(kind, limit, protectStatuses)`** to get candidate IDs from the cache without any full List. After restart, cache may be loaded from disk (no status in v2 minimal payload); then we fall back to batched List until the next cache build.

## References

- `max_count.txt` – sample log of slow runs.
- `pkg/scheduler/handlers_retention_tolerance.go` – `enforceMaxCount`, `enforceMaxCountBatchedList`.
- `pkg/storage/object_storage_file_list_main.go` – List path and `OldestIDs` branch (stream-backed excluded).
- `docs/architecture/HIGH_VOLUME_EVENT_INDEXES.md` – index/cache contract for Count and OldestIDs.
