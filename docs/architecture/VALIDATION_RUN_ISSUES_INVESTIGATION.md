# Validation Run Issues Investigation

When `zqk system check` (async validation) finishes, you may see:

```
=== Validation run issues (check logs for details) ===
⚠️  This run had issues that may affect results. Review logs if counts or cache seem wrong:

  • Worker stop timeout: one or more workers did not stop in time; they may have been stuck.
  • Cache save timeout: validation state cache may not have been saved. Next run may re-validate more objects.
```

This doc explains what these mean, where to look for details, and how to mitigate.

## Where the flags are set

- **Source:** `pkg/validation/async_validator_lifecycle.go` — during `Stop()`.
- **Output:** `cmd/zqk/system/check_impl_output.go` uses `ValidationRunIssues` (from `GetRunIssues()` after `Stop()`) to render the section in `output_results` / `outputTable`.

## 1. Worker stop timeout

**Meaning:** When shutting down the async validator, the main goroutine waits for all validation workers to finish (via `av.wg.Wait()`) with a **timeout** (default **60s**). If that timeout is reached before all workers exit, `WorkerStopTimedOut` is set.

**Causes:**

- Many objects still in the queue when stop was requested, so workers need more than 60s to drain.
- Workers blocked on I/O (slow disk, NFS), locks (e.g. validation state cache, object storage), or long-running validation for a single object.
- High CPU load so each validation task takes longer.

**Where to look:**

- **Validation component log:** `.zqk/logs/components/validation.events.json`  
  Search for:
  - `"Timeout waiting for workers to stop"` — confirms the event.
  - `active_workers`, `queue_size`, `active_goroutines` at timeout.
  - `"Worker still active after stop timeout"` with `worker_id` and `state` (which phase the worker was in).
- **Config:** Worker stop timeout is set by `AsyncValidatorConfig.WorkerStopTimeout` (default 60s). System check currently uses defaults via `GetAsyncValidator()` in `cmd/zqk/system/async_check.go` (no custom config passed).

**Mitigation:**

- For large repos or slow disks, use a validator created with a custom `AsyncValidatorConfig` that sets a larger `WorkerStopTimeout` (e.g. 90s or 120s). Today this requires a code change where the validator is created (e.g. in `GetAsyncValidator` or `runCheckAsyncWithContextAndOperationID`) to pass opts.
- Reduce concurrent load (e.g. lower validator worker count) so each worker finishes sooner under resource contention.

## 2. Cache save timeout

**Meaning:** After workers are considered stopped, the validator saves the validation state cache to disk (`.zqk/cache/validation_cache.json`) with a **timeout**. The timeout is **scaled**: base **20s** + **6s per 1000 cached states**, capped at **3 minutes**. If the save (copy under lock, JSON marshal, file write + lock) does not complete within that time, `CacheSaveTimedOut` is set.

**Causes:**

- **Large state count:** JSON marshal and file write can take a long time (see `docs/architecture/memory-explosion-analysis.md`: timeouts observed with 7265–15373 states; save timeout was 1m2s–1m50s).
- Slow or contended disk, or file locking (another process holding the lock file).
- Lock file logic in `pkg/validation/state_cache.go` (stale lock detection, retries) can add delay.

**Where to look:**

- **Validation component log:** `.zqk/logs/components/validation.events.json`  
  Search for:
  - `"Timeout saving validation cache"` — includes `timeout`, `state_count`, and a short diagnostic.
- **Cache file:** `.zqk/cache/validation_cache.json` — check size and modification time. If the run timed out, the file may be from a previous run or not updated.
- **Lock file:** `.zqk/cache/validation_cache.json.lock` — if it exists and is old (>30s), another process may have hung while holding the lock.

**Mitigation:**

- **Increase cache save timeout** for large caches by passing a custom `AsyncValidatorConfig` with a larger `CacheSaveTimeout` (base is 20s; scaling still applies in lifecycle).
- **Reduce cache size over time:** Excluded kinds (e.g. audit_event, metrics, zqk_session) are not persisted; ensure high-volume kinds stay excluded. Longer-term, consider incremental or periodic cache saves (see memory-explosion-analysis.md).
- **Check disk and locks:** Ensure `.zqk/cache/` is on a fast, local filesystem and no other process is holding the lock for a long time.

## Quick checks after seeing the message

1. **Validation component log**  
   From repo root:
   ```bash
   # Worker stop details
   grep -E "Timeout waiting for workers|Worker still active after stop" .zqk/logs/components/validation.events.json

   # Cache save details
   grep -E "Timeout saving validation cache|Failed to save validation cache" .zqk/logs/components/validation.events.json
   ```

2. **Cache size**  
   - State count is logged at timeout; you can also inspect cache file size:
     `ls -la .zqk/cache/validation_cache.json`
   - Approximate state count: `jq '.states | length' .zqk/cache/validation_cache.json` (if file exists and is valid JSON).

3. **Stale lock**  
   `ls -la .zqk/cache/validation_cache.json.lock` — if present and old, consider removing after ensuring no other zqk process is running.

## Root cause: timeout context was already cancelled (fixed)

**Worker stop and cache save timeouts were not actually waiting.** In `Stop()`, the code did:

1. `av.cancel()` and `close(av.shutdown)` (so `av.ctx` is cancelled).
2. `stopCtx, stopCancel := context.WithTimeout(av.ctx, workerTimeout)` and later `saveCtx, saveCancel := context.WithTimeout(av.ctx, saveTimeout)`.

Because `av.ctx` is already cancelled, any context derived from it is also cancelled. So `stopCtx.Done()` and `saveCtx.Done()` were already closed, and the `select` in `Stop()` immediately took the timeout branch instead of waiting up to 60s (workers) or the scaled cache timeout. Result: we always reported "Worker stop timeout" and "Cache save timeout" even when workers and cache save would have completed in a few seconds.

**Fix:** Use `context.Background()` as the parent for both timeout contexts in `Stop()` so we actually wait the intended duration. See the code change in `pkg/validation/async_validator_lifecycle.go`.

## References

- `pkg/validation/async_validator_lifecycle.go` — Stop(), timeouts, and `GetRunIssues()`.
- `pkg/validation/async_validator_config.go` — Default timeouts and scaling notes.
- `pkg/validation/state_cache.go` — Save() (lock, marshal, write).
- `docs/architecture/memory-explosion-analysis.md` — Cache save timeouts and state counts.
- `cmd/zqk/system/async_check.go` — `GetAsyncValidator()` (currently no custom config).
