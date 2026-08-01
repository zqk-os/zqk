# Performance Bottleneck Audit: Uncached / Per-Operation Heavy Resources

**Purpose:** Identify places that create goroutines, heavy structs, or expensive resources per request or per operation (rather than reusing a cached or shared instance). Such patterns can cause goroutine and memory growth under load.

**Status:** Initial audit 2026-02-25. HashRegistry and audit_events paths fixed; remaining items documented for follow-up.

---

## 1. Fixed: HashRegistry per (kind, dir)

**Location:** `pkg/storage/object_storage_file_hash_registry.go`, `object_storage_file.go`

**Issue:** Every create/update/delete/move (and CAS verify) called `newHashRegistry(ctx, kind, dir)`, which created a new `HashRegistry` and triggered `startSaveWorker` (a goroutine) on first `Save()`. Under load this produced hundreds of registries and goroutines.

**Fix:** Cache `*HashRegistry` per `(kind, dir)` in `FileObjectStorage.hashRegistryCache` (`ResourceCache[*HashRegistry]`). Key: `hashRegistryCacheKey(kind, dir)`. Same (kind, dir) reuses one registry and at most one save worker per kind/dir.

**Also fixed:** `pkg/storage/audit_events.go` — `WriteSystemObjectAndRegisterHash` now uses `fileStorage.newHashRegistry(ctx, kind, kindDir)` when `fileStorage != nil`, so system object writes (audit events, etc.) share the cached registry instead of creating a new one each time.

---

## 2. Fixed: CAS per kind (already cached)

**Location:** `pkg/storage/object_storage_file_cas.go`

**Status:** Already uses `f.casCache.GetOrCreate(ctx, kind, ...)` to cache `*ContentAddressableStorage` per kind. No change needed.

---

## 3. Fixed: Storage provider (global cache)

**Location:** `pkg/storage/audit_events_helper.go`, change notification, scheduler

**Status:** Scheduler and change notification use `GetGlobalStorageProviderCache().GetOrCreate(ctx, projectRoot)` (or equivalent) so they do not create a new `NewFileObjectStorage` per update. No additional change needed.

---

## 4. Follow-up: NewHashRegistry outside FileObjectStorage

**Locations:**

| File | Usage | Notes |
|------|--------|------|
| `pkg/storage/audit_events.go` | `verifyHashRegistrySaveForSystemObject` creates `NewHashRegistry` for read-back verification | Low-frequency debug path; nolint unused. Optional: pass `*FileObjectStorage` and use `newHashRegistry` if available. |
| `pkg/storage/snapshot_manager.go` | `NewHashRegistry` in loop over snapshot objects for hash lookup | Recovery/verification path; could accept a cache or reuse by (kind, dir) if this path becomes hot. |
| `pkg/storage/object_storage_file_helpers.go` | `NewHashRegistry(registry.ctx, ...)` for verify path | Same as above; consider reusing from cache when storage is FileObjectStorage. |
| `pkg/storage/deferred_hash_update.go` | `NewHashRegistry` for deferred update | If this runs in a hot path, consider obtaining registry from storage cache. |

**Recommendation:** Prefer passing `*FileObjectStorage` (or a `HashRegistryProvider` interface) into these helpers so they can call `f.newHashRegistry(...)` when available; otherwise keep `NewHashRegistry` for non-file or test-only paths.

---

## 5. Follow-up: NewFileObjectStorage in change_journal

**Location:** `pkg/storage/change_journal.go` (around line 69)

**Issue:** `CreateChangeJournalEntry` calls `NewFileObjectStorage(projectRoot)` to get storage for writing the journal entry. If this is invoked frequently, each call builds a new storage instance (loaders, caches, etc.).

**Recommendation:** Use `GetGlobalStorageProviderCache().GetOrCreate(ctx, projectRoot)` and type-assert to `*FileObjectStorage` when CAS routing is needed, so the same provider (and thus same HashRegistry/CAS caches) is reused.

---

## 6. List / Count / Search concurrency (already mitigated)

**Location:** `pkg/storage/list_count_concurrency.go`, list/count/search implementations

**Status:** List/count use a bounded semaphore (`listCountMaxConcurrent` increased to 16); list workers use a fixed worker pool; context cancellation and closer patterns are in place. No uncached “per call” heavy resource; contention is bounded.

---

## 7. Other goroutine spawns (reference)

Production code that starts goroutines (for awareness, not necessarily bottlenecks):

- **HashRegistry.startSaveWorker** — Now one per cached registry (kind, dir); on-demand, exits after idle.
- **List CAS result closer** — One goroutine per list call to close the results channel after workers finish; bounded by list concurrency.
- **Cache prewarm tier-3** — Fixed so tier-3 tasks use a timeout context and the wait goroutine does not block forever.
- **Scheduler job pool** — Fixed size (e.g. 64 workers); not per-request.
- **Validation state cache flusher, async scanner** — One per cache/validator instance.

---

## 8. Audit checklist for new code

When adding or touching code that:

- Creates a `HashRegistry`, `ContentAddressableStorage`, or `FileObjectStorage` → use the existing cache (or a new cache keyed by identity) so the same logical resource is reused.
- Starts a goroutine per request or per item → prefer a bounded pool or a single long-lived worker with a channel.
- Loads specs or heavy config on every call → use lazy load + cache (e.g. `ResourceCache`, `sync.Once`).

See also: [SCHEDULER_OVERLOAD_AND_TIMEOUT.md](./SCHEDULER_OVERLOAD_AND_TIMEOUT.md) (§5 long term), [CLI_PERFORMANCE_AND_CONSISTENCY.md](./CLI_PERFORMANCE_AND_CONSISTENCY.md), [unbounded-concurrency-fixes.md](./unbounded-concurrency-fixes.md).
