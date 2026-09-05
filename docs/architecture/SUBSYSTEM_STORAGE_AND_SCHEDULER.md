# Storage & Scheduler Subsystems Architecture (K:F-DOC-003 / CRIT-CEF-R8K-DOC-003)

**Last Verified:** 2026-08-31


## 1. Storage Subsystem (`pkg/storage`)
The storage layer provides ACID-like persistence guarantees for spec-driven kernel objects using Content-Addressable Storage (CAS), Write-Ahead Logging (WAL), and atomic snapshots.

### Components
- **Content-Addressable Storage (CAS)**:
  - Objects are serialized to canonical YAML named by SHA-256 content hash (`docs/process/<kind>/<hash>.yaml`).
  - Immutable write paths ensure zero in-place mutations; updates write a new hash file and delete the old hash.
  - On Darwin/macOS, background sync queues manage durable `F_FULLFSYNC` operations asynchronously to prevent I/O blocking.
- **Write-Ahead Log (WAL)**:
  - Sequence-ordered mutation journal in `.zqk/wal/`.
  - Fast tail lookup (`ReadLastSeqFromTail`) inspects the bounded EOF window without full file replay.
- **Snapshot Engine**:
  - Full memory state serialized to `.zqk-state/system-state.csnap` via Snappy compression.
  - Retention policies keep 3 external prior snapshots in `~/zqk-csnap-backups/`.

---

## 2. Scheduler Subsystem (`pkg/scheduler`, `cmd/zqk/scheduler`)
The scheduler is an asynchronous workflow daemon executing recurring maintenance, background validations, and regression test bundles.

### Components
- **Trigger Queue & Batching Engine**:
  - Batches high-volume object validation events into unified execution sweeps.
- **Run Wrapper**:
  - Manages process groups, execution deadlines (`max_runtime_seconds`), and audit logging.
- **Pre-Commit Background Worker**:
  - Runs lint, policy, and integrity checks asynchronously, writing aggregated status to `.zqk/pre-commit/results.json`.
