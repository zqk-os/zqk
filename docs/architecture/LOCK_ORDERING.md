# Lock Ordering

**Last Verified:** 2026-08-31


**Purpose:** Prevent deadlocks when multiple mutexes (or lock resources) are used. This document defines the global lock order and practices for the zqk codebase. Enforce in code review (POL-ARCH-004).

## Principles

1. **Single global order** — When code must hold more than one lock, always acquire in the same order (e.g. A before B). Document any new ordering in this file.
2. **Prefer one lock at a time** — Copy data out from under one lock, release it, then acquire the next. Avoid holding multiple in-process mutexes simultaneously when possible.
3. **Use RunInLock / RunInRLock** — All in-process critical sections should use `pkg/concurrency` wrappers (see `pkg/concurrency/README.md`). No raw `Lock()`/`Unlock()` in production except where explicitly documented.

## In-Process Mutexes (by package)

These are the main mutexes that protect shared state. When a code path needs to take more than one, acquire in **lexicographic order by (package, mutex name)** unless a specific exception is documented below.

| Package | Mutex / Location | Notes |
|---------|------------------|--------|
| `pkg/concurrency` | `globalConfigMu` (config.go) | Global concurrency config; short sections only. |
| `pkg/storage` | `FileObjectStorage.unregisteredHashRegistriesMu` (object_storage_file.go) | Hash registry registration; no I/O under lock. |
| `pkg/storage` | `globalListCache.mu` (list_cache.go) | List cache get/invalidate; in-memory only. |
| `pkg/storage` | `HighVolumeEventCache.mu` (high_volume_event_cache.go) | High-volume event cache (audit_event, metrics); RLock for reads, Lock for writes; see **High-volume event cache concurrency** below. |
| `pkg/storage` | `FileObjectStorage.walMu` (object_storage_file.go) | WAL append (Create/Update/Delete) and TryCompactWAL. When taken with ObjectWriteBuffer.mu, acquire **walMu first** (append path holds walMu then calls buffer Enqueue). Use RunInLockWithLogger. |
| `pkg/storage` | `ObjectWriteBuffer.mu` (object_write_buffer.go) | Write-behind queue and back-pressure (cond var). **Exception:** raw Lock/Unlock with cond.Wait() for back-pressure; RunInLock cannot be used where the callback calls cond.Wait(). When taken with FileObjectStorage.walMu, acquire walMu first. |
| `pkg/storage` | `ObjectWriteBehindWorker.mu` (object_write_behind_worker.go) | Worker running flag and lastCompactTime; use RunInLock. |
| `pkg/storage` | `FileTransactionCoordinator.mu` (file_transaction_coordinator.go) | Transaction lock set; see **File transaction ordering** below. |
| `pkg/storage` | `indexQueue.mu` (cas_index_write_queue_worker.go) | CAS index queue; snapshot under RLock, then release before file lock. |
| `pkg/storage/id_generation` | `batch_generator.mu` (batch_generator.go) | Sequence dir and state; use RunInLock/RunInRLock. |
| `pkg/mcp` | `Server.toolsMu` (server) | Tool registration and list; RLock for read, Lock for write. |
| `pkg/mcp` | `Server.clientsMu` (server_lifecycle.go) | Client set; TryLock used in shutdown path (intentional). |
| `pkg/mcp` | `ProcessGroupManager.mu` (process_group_manager.go) | Tracked goroutines and subprocesses. |
| `pkg/scheduler` | `PackageConcurrencyLimiter.mu` (package_concurrency_limiter.go) | Per-package semaphore map; used only inside executeJob for run_wrapper; never held together with conflict manager or other in-process mutexes. RunInLockWithLogger. |
| `pkg/scheduler` | `streamingOutputWriter.mu` (streaming_output.go) | Per-writer ring buffer for command stdout/stderr preview; not held with any other in-process mutex. RunInLock. |

## File Transaction Ordering (storage)

When using **FileTransactionCoordinator** for multi-file transactions:

1. **Transaction mutex (`Transaction.mu`)** — Hold only while reading or updating the transaction’s list of operations and file paths. Release before calling the coordinator.
2. **Coordinator mutex (`FileTransactionCoordinator.mu`)** — Hold only to check or update the coordinator’s map of acquired locks. Release before acquiring any **file-based** lock (e.g. `AcquireLock(lockPath)`).
3. **File locks (by path)** — Paths are **sorted** before acquisition so all callers acquire file locks in the same order. See `AcquireLocks` in `file_transaction_coordinator.go`.

So the order is: **Transaction.mu → (release) → Coordinator.mu → (release) → file locks in sorted path order**. Never hold the coordinator mutex while blocking on a file lock.

## Cross-Process / File-Based Locks

Advisory file locks (e.g. `*.txn.lock`, CAS index `.lock`) are ordered by **lock file path** (lexicographic). Code that acquires multiple file locks must sort the paths and acquire in that order. The file transaction coordinator already does this.

## Adding New Locks

When introducing a new mutex or lock resource:

1. Add it to the table above (or the appropriate “ordering” section) with package and location.
2. If it is ever taken together with another lock, define the order (e.g. “A before B”) and document it here.
3. Prefer designs that avoid holding two in-process mutexes at once (copy under one lock, release, then take the other).

## High-Volume Event Cache Concurrency (storage)

The `HighVolumeEventCache` uses a single `sync.RWMutex` (`c.mu`) for all operations. When used with other locks:

1. **Storage locks first**: If both cache and storage locks are needed, acquire storage locks first (e.g., `FileObjectStorage` locks), then cache lock.
2. **Copy data out**: Prefer copying data out from under one lock, releasing it, then acquiring the next lock.
3. **No I/O under lock**: Never hold the cache lock during file I/O operations (see `SaveCache` pattern).

See `docs/process/architecture/high-volume-event-cache-concurrency.md` for detailed concurrency patterns.

## Related

- **pkg/concurrency/README.md** — RunInLock, RunInRLock, WithLockCtxLogger; when to use which.
- **docs/process/architecture/high-volume-event-cache-concurrency.md** — High-volume event cache concurrency patterns.
- **docs/process/refactoring/CODEBASE_EVALUATION_AND_REMEDIAL_PLAN.md** §4 — Raw lock sites and lock ordering action.
- **POL-ARCH-004** — Lock ordering enforced in code review.
