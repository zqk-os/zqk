# System check: cache-first, async population, quick CLI

**Version:** 1.0  
**Status:** Target architecture  
**Purpose:** Define the intended behavior for system check, pre-warm, object counts, and CLI so that validation is not run in the CLI path and long work is always async with callbacks.

## Flow by thread

### Object create | update | delete

- **Thread 1:** Update/invalidate **ID cache** — async, with callback when store completed.
- **Thread 2:** Update/invalidate **list cache** — async, with callback when store completed.
- **Thread 3:** **Enqueue validation** — validation queue worker farms out to workers that update the validation cache.

CLI returns after initiating these; no blocking on cache writes or validation.

### System check

- **Thread main:** Show **validation cache results** (or a not-ready message if cache is still populating).
- **Thread 1:** Trigger **scan worker** to update object counts and enqueue validations (async).

Main thread returns quickly with cached results; scan and validation run in background.

## Principles

1. **System check does not validate** — it only **returns validation results**, which should **mostly come from cache**. It may report "outstanding validations" when some objects are not yet in the validation cache.
2. **Pre-warm drives all caches** — the pre-warm that does directory scans (e.g. file backend) should **trigger** population of the object ID cache, list cache, and validation cache **asynchronously**.
3. **Object counts are quick** — if mtime has changed, diff object cache vs directory scan and trigger cache updates and validations only for what’s new (e.g. non–CLI-added objects). This makes it easy to see objects not yet in cache and to tell the user when there are outstanding validations.
4. **CLI never blocks on long work** — every CLI request is an async operation; if work is long-running, use a callback (e.g. scheduler). The CLI returns quickly. No long-running serial/blocking processes should flow through the CLI.

## Intended behavior

### System check

- **Reads** validation state cache (and object ID cache for scope).
- **Returns** cached validation results.
- **Does not run** validation in the CLI process.
- When some objects have no cached result (e.g. new files, or cache not yet populated): report **"outstanding validations: N"** (or similar) and optionally trigger async validation in the background (callback); CLI still returns quickly with what’s in cache.

### Pre-warm (e.g. file backend directory scan)

- Directory scan (or equivalent) **triggers** async population of:
  - **Object ID cache**
  - **List cache**
  - **Validation cache** (by enqueuing validation work; results written to cache when done)
- All of this is **async**; nothing blocks the CLI or the scheduler beyond submitting work.
- Pre-warm may be invoked by scheduler jobs (e.g. `cache_prewarm`) or on first access (quick-return + background job).

### Object count

- **Quick path:** Use cached object count when possible.
- **When mtime (or equivalent) indicates change:** Diff object cache vs directory scan. From the diff:
  - Trigger **async** cache updates and validations only for new/changed objects.
  - Expose "objects not yet in cache" and "outstanding validations" so the CLI can inform the user.
- Count itself returns quickly (from cache + diff result); any heavy work is async with callback.

### Create / update / delete (CLI)

- Invalidate object ID cache, list cache, and validation cache for affected IDs (current behavior).
- Optionally **enqueue async validation** for the affected objects so the validation cache is repopulated in the background; CLI returns immediately.
- No long-running validation in the CLI process.

## Relation to current implementation

- **Current:** `zqk system check` builds/loads object ID cache, discovers objects, enqueues validation, runs the async validator, and **waits** for completion (or timeout) before returning. So the CLI currently blocks until validation is done (or 30m timeout).
- **Target:** System check becomes a **cache read**: load validation cache (and object ID cache for scope), return cached results, report outstanding validations; validation runs elsewhere (pre-warm, scheduler job, or background callback).
- **Pre-warm today:** Scheduler `cache_prewarm` pre-warms spec/lifecycle/field registries and optionally object ID cache; it does **not** yet populate list cache or validation cache via directory scan. The **target** is for pre-warm (when it does a directory scan) to trigger async population of all three caches.
- **Validation cache invalidation:** On create/update/delete we invalidate the validation cache for affected IDs so that async validation (or next pre-warm) can repopulate; system check then reports up-to-date results from cache. See `validation_cache_sync.go` and `InvalidateValidationStateCacheForObjects`.

## References

- **`docs/architecture/CLI_PERFORMANCE_AND_CONSISTENCY.md`** — Authoritative CLI performance contracts (1 s max response, async with progress), caching rules (incremental; full populate only at scheduler init or user request), and definition of "works." All CLI and cache design must align.
- `docs/architecture/system-check-pipeline.md` — Current pipeline phases (to be aligned with cache-first behavior).
- `docs/architecture/system-check-data-flow.md` — Data flow and completion signals.
- `docs/architecture/CACHE_MANAGEMENT_STRATEGY.md` — Cache pre-warm strategy.
- `pkg/scheduler/handlers_cache_prewarm.go` — Cache pre-warm handler (object ID cache, specs, lifecycle, etc.).
- `cmd/zqk/system/validation_cache_sync.go` — Validation cache invalidation on create/update/delete.
