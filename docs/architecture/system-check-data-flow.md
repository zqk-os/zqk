# System check data flow and lock sequencing

**Last Verified:** 2026-08-31


**Version:** 1.0  
**Status:** Active  
**Purpose:** Explain why the process could block indefinitely despite timeouts, and document the completion signal and wait points so we can avoid similar failures.

## Visual overview: System Check Flow

The following diagram shows the high-level flow between the CLI, cache proxy, Object Validator (validation queue and worker pool), and Cache Manager (caches, storage, and invalidation). Storage operations trigger callback notifications that enqueue validation; the Cache Manager’s **handle invalidate** keeps list/ID caches consistent after create/update/delete.

![System Check Flow](system-check-flow.png)

*Figure: System Check Flow — cache proxy, Object Validator (violation cache, validation queue, worker pool), and Cache Manager (output coordinator, cache read/write, storage provider).*

## Why we blocked indefinitely

The design intended to avoid indefinite blocking by:

- **Follower:** `select` on `checkDone` | `runCtx.Done()` | `time.After(30m)` so the CLI returns at most after 30 minutes.
- **Setup phase:** `select` on `setupDone` | `time.After(15s)` so the main check goroutine doesn’t wait forever for the four loaders.

Two things broke that:

1. **Single completion signal not guaranteed**  
   The follower blocks on `<-checkDone`. Only the **background check goroutine** sends to `checkDone`. If that goroutine **panics** (e.g. in `EnsureObjectIDCacheReady` at `atomic.Value.Store`), it never runs `checkDone <- checkErr`. So the follower never receives and waits until `time.After(30m)` — effectively an indefinite hang from the user’s point of view.

2. **Unbounded wait inside the check**  
   A **helper goroutine** runs `setupWg.Wait()` and then `close(setupDone)`. The **main** check path has a 15s timeout on `setupDone`, so it doesn’t block forever. Setup has since been changed to use a channel and a 15s deadline only (no Wait()), so no goroutine blocks forever. The process still exits when the main path eventually sends to `checkDone` (or panics and now, with the fix, sends on recover). So the main “indefinite block” was the missing send on panic, not the stuck helper — but the helper is still a latent unbounded wait and can show up in goroutine dumps.

**Fixes applied:** (1) Background check goroutine uses `defer` + `recover()` so it **always** sends to `checkDone` exactly once. (2) Setup uses channel-based completion with a 15s deadline only; no Wait(), so no indefinite block.

## High-level data flow

```
runCheck (entry)
    → runCheckAsyncWithFollow
          │
          ├─ start background goroutine (system_check_background)
          │     → runCheckAsyncWithContextAndOperationID
          │           → setup loaders (4) in parallel; each sends on loaderDone; watchdog collects 4 or 15s then close(setupDone)
          │           → select setupDone | 15s timeout | cancelCtx  [main path bounded]
          │           → setupAsyncValidationFunction → EnsureObjectIDCacheReady (cache build + warm)
          │           → startAsyncValidator, discoverAndEnqueueObjects, showValidationProgressWithMetrics
          │     → [on exit or panic] checkDone <- err   [must happen exactly once]
          │
          └─ select checkDone | runCtx.Done() | time.After(30m)   [follower; now always gets signal on panic]
```

- **Single completion signal:** `checkDone` (buffered, 1). Only the background goroutine may send. It must send exactly once (normal return or after recover from panic).
- **Follower** must not depend on the 30m timeout for the panic case; it should receive from `checkDone` as soon as the check finishes or panics.

## Wait points and timeouts

| Wait point | Where | Timeout? | Notes |
|------------|--------|----------|--------|
| `<-checkDone` | runCheckAsyncWithFollow | 30m | Follower; now receives on panic (recover sends). |
| (removed) | — | — | Setup phase no longer uses `WaitGroup`; uses channel `loaderDone` (buffer 4) and one watchdog goroutine with 15s deadline only. No indefinite wait. |
| `<-setupDone` or `<-time.After(15s)` | runCheckAsyncWithContextAndOperationID | 15s | Main path bounded. |
| ensureRunnerMu (lock) | EnsureObjectIDCacheReady → RunInLockWithLogger | No | Short critical section; panic observed at atomic.Store inside. |
| cache.mu (RLock) | Post-warm verify (WithRLockTimeout) | 5s | Bounded. |
| loader.Load(ctx) | EnsureObjectIDCacheReady → getEnsureRunner().Load(ctx) | Config (e.g. PublishChannel, WaitForCompletion) | pkg/loader Runner. |
| preInitWg.Wait() / wg.Wait() | warmCASIndexesFromCache | No | All workers started in same function; workers use semaphore and panic handlers. If a worker blocks, Wait blocks. |

## Object count vs system check scope

**Why “Validating N objects” can be much lower than `zqk object count` total:**

- **`zqk object count`** uses the field registry’s **all kinds** and calls `storage.Count()` per kind. That total includes every kind in the system (e.g. audit_event, command_metric, mcp_session, backlog_item, requirement, …), so the total can be in the tens of thousands.
- **`zqk system check`** validates only objects that appear in the **object ID cache**. The cache is built from **`docs/process`** (kind discovery + scan of kind directories). So the check only validates that subset of objects (typically process-domain kinds and whatever else has files under docs/process at cache build time). That number is often in the thousands.

So both numbers are correct for what they measure: count = “all objects in system (all kinds)”; check = “objects in the object ID cache.” The count CLI now includes a scope note to that effect.

## Lock ordering (relevant to cache path)

No single global lock order is mandated; the following is for understanding deadlocks:

1. **ensureRunnerMu** (check_cache) — held around runner reset and currentProjectRoot.Store.
2. **cache.mu** (check_cache) — RLock for post-warm verify and diagnostics; write lock during BuildCache.
3. **Loader internals** (pkg/loader) — loadDone channel and atomics; no mutex shared with cache.

Avoid holding ensureRunnerMu or cache.mu across slow I/O or coordinator calls. Progress emission is already off the hot path (goroutine or non-blocking).

## Diagram (Mermaid): completion and panic path

```mermaid
sequenceDiagram
    participant Follower
    participant BG as Background goroutine
    participant Check as runCheckAsyncWithContextAndOperationID
    participant Setup as Setup loaders (4)
    participant Cache as EnsureObjectIDCacheReady / warm

    Follower->>BG: StartWithContext(run)
    BG->>Check: run
    Check->>Setup: Start 4 loaders; go func() { setupWg.Wait(); close(setupDone) }
    Note over Check: select setupDone | 15s | cancel
    Check->>Cache: buildObjectIDCacheIfNeeded → EnsureObjectIDCacheReady
    alt Panic in Cache (e.g. atomic.Store)
        BG->>BG: recover() → checkErr = panic error
        BG->>Follower: checkDone <- checkErr (defer)
    else Normal
        BG->>Follower: checkDone <- checkErr (defer)
    end
    Follower->>Follower: return err (no 30m wait)
```

## Recommendations and implementation status

| # | Recommendation | Status |
|---|----------------|--------|
| 1 | Panic recovery in the background goroutine so `checkDone` is always sent | **Done** — `check_follower.go`: `defer recover()` + `defer checkDone <- checkErr`. |
| 2 | No indefinite block in setup: bounded wait so a stuck loader never leaves any goroutine blocking forever | **Done** — `async_check.go`: setup uses a channel (`loaderDone`) and a single watchdog goroutine that collects 4 signals or hits 15s deadline then closes `setupDone`; no `Wait()` so no path blocks indefinitely. |
| 3 | Fix the root cause of the panic in `EnsureObjectIDCacheReady` (atomic.Value type / nil) | **Done** — `check_cache.go`: `progressNotifier` now always stores `*progressNotifierHolder`; no alternating concrete types. |
| 4 | When changing flow: ensure exactly one send to `checkDone` (or equivalent) | **Process** — Documented here and in code comments; enforce in review. |

## References

- `docs/architecture/system-check-flow.png` — Visual overview diagram (cache proxy, Object Validator, Cache Manager, validation queue).
- `docs/architecture/system-check-pipeline.md` — Pipeline phases and rules.
- `docs/architecture/system-check-architecture-alignment.md` — Implementation alignment.
- Goroutine/CPU profile notes and panic location: see system-check run output or daemon logs (e.g. `.zqk/logs/` when present).
- Code: `cmd/zqk/system/check_follower.go` (runCheckAsyncWithFollow, checkDone, recover), `async_check.go` (setupWg, setupDone), `check_cache.go` (EnsureObjectIDCacheReady, ensureRunnerMu, cache.mu).
