# Fast Path and Cache Fallback Checklist

**Last Verified:** 2026-08-31


**Purpose:** When List, Count, or another read path has a **fast path** that uses a cache (or partial index), ensure that **empty or error from the fast path does not become the final answer** unless the underlying source truly has no data. Use this checklist whenever adding or changing a cache-based fast path so all variants share the same fallback contract.

**Canonical location:** `docs/architecture/FAST_PATH_AND_CACHE_FALLBACK_CHECKLIST.md`  
**Referenced from:** `docs/architecture/PRE_CHANGE_CHECKLIST.md` (section 16).

---

## Contract

**List/Count with filters must not return 0 solely because the cache returned 0.**  
When the cache (or high-volume event cache, or partial index) is used as a fast path and returns no IDs or empty:

- **Fall through** to the full path (e.g. CAS index + disk read, or full scan) so the result reflects actual storage.
- Do **not** return empty immediately from the fast path unless the design explicitly treats "cache says 0" as the intended answer (e.g. after invalidation and no fallback).

---

## Checklist (when adding or changing a cache-based fast path)

- [ ] **Fallback when empty:** If the fast path returns 0 (or too few) results, does execution fall through to a full path (e.g. full CAS list, index + filter) instead of returning empty?
- [ ] **Same contract for all variants:** For the same logical operation (e.g. "audit_event list with created_at filter"), do **all** variants (e.g. time-window `$gte`/`$lte`, "older than" `$lt`/`$lte`) use the same rule? If one variant falls through when cache returns 0, the others should too unless documented otherwise.
- [ ] **Error handling:** If the fast path errors (e.g. cache unavailable), does the code fall through to the full path or surface the error? Prefer fallback so the user gets a result when storage has data.
- [ ] **Tests:** Is there a test or scenario that asserts: "when cache returns 0 (or is empty), the result comes from the full path and is non-empty when storage has matching data"? Adding this guards against regressions.

---

## Example: audit_event List with created_at

- **Time-window path** (`$gte` + `$lte`): Uses high-volume event cache `QueryByTimeWindow`. When `len(eventIDs) == 0`, code must **fall through** to full CAS path (index + read + filter by created_at). See `pkg/storage/object_storage_file_list_main.go`.
- **Older-than path** (`$lt` / `$lte` only): Uses cache `QueryOlderThan`. When cache returns 0, code **falls through** to full CAS path (same file). Both paths must behave consistently.

---

## References

- **PRE_CHANGE_CHECKLIST.md** — Section 14 points here for fast path / cache fallback.
- **CACHE_MANAGEMENT_STRATEGY.md** — Incremental vs full refresh; fast path should have a defined fallback.
- **CLI_PERFORMANCE_AND_CONSISTENCY.md** — Response time and caching rules; fallback may be slower but must still return correct results.
