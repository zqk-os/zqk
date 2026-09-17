# Object WAL, file descriptors, and related anti-patterns

**Last Verified:** 2026-08-31


This document records **stability and correctness** rules for the object write-ahead log (`.zqk/wal/object.wal`) and for **component log files** (e.g. `validation-events.json`) so we do not leak file descriptors or leave multiple open handles to the same path for different inodes.

## Object WAL

### How the live process uses the WAL

- `FileObjectStorage` opens an `ObjectWAL` and the `ObjectWriteBehindWorker` appends, replays, and (when safe) compacts via `TryCompactWAL`, which **closes the WAL** before running `CompactWAL` in-process.
- **Renaming** the WAL file (as `CompactWAL` does) while **another** file descriptor in a different process still has the same path open for write can leave that descriptor attached to an **unlinked** inode. Many such events show up in `lsof` as many lines for the same path with different `NODE` (inode) values.

### Do

- Rely on **in-process** compaction: the write-behind worker’s `TryCompactWAL` path, or normal operation, for steady-state size control.
- If you run **`zqk system compact-wal` (PRUNED)** from the CLI: ensure the **scheduler daemon is stopped** first, **or** pass **`--force`** after reading the warning. The command defaults to refusing when `IsSchedulerRunning` is true.

### Don’t

- Run standalone **`zqk system compact-wal` (PRUNED)** against a **live** daemon project without `--force`, expecting it to be “always safe”; that pattern was the source of misleading operator guidance.

## Validation and component loggers

### Decision-context loggers

- `GetLoggerFromDecisionContext` builds a **`decisionContextLogger`** with its **own** destination cache per instance. Opening `*-events.json` creates **OS-level** file descriptors (often via `SizeBasedRollingWriter`).
- **Every** short-lived **`AsyncValidator`** created for logging must **`Stop()`** after use so the shutdown pipeline runs **`TryCloseLoggerDestinations`** and releases those FDs.

### Scheduler validation scanner (`EnqueueAll`)

- Create **`AsyncValidator`** only within a **`defer validator.Stop()`** scope (or reuse a singleton process-wide validator). Starting the validator without stopping leaks component log FDs on each Tier-4 enqueue pass.

### Don’t

- Instantiate **`AsyncValidator`** repeatedly (e.g. scheduler hooks, tests) without **`Stop()`** when the validator actually started.

## Operational checks

- Inspect duplicate path handles:  
  `lsof -p <pid> | sort | uniq -c` on suspicious paths (`object.wal`, `*-events.json`).
- For heap attribution, enable **`ZQK_PPROF=1`** on the daemon and use **`go tool pprof`** on `/debug/pprof/heap`; object counts alone do not explain RSS.

## References

- `pkg/storage/object_wal.go` — `CompactWAL`, rename semantics.
- `pkg/storage/object_storage_file.go` — `TryCompactWAL`.
- `pkg/logging/logger.go` — `TryCloseLoggerDestinations`.
- `cmd/zqk/system/compact_wal.go` — CLI guard vs scheduler.
- `cmd/zqk/system/validation_scanner_scheduler.go` — `EnqueueAll` lifecycle.
