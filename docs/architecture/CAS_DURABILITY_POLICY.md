# CAS durability policy (per-OS)

**Last Verified:** 2026-08-31


**Tracks:** `BLI-CEF-R2-REL-CAS-FSYNC` / `REQ-CEF-R2-REL-CAS-FSYNC` / `CRIT-CEF-R2-REL-CAS-FSYNC-A`  
(prior: `BLI-CEF-REL-CAS-DURABILITY` / `REQ-CEF-REL-001` / `CRIT-CEF-REL-001A` / `CRIT-CEF-REL-001B`)

## Decision

Filesystem CAS **object** publishes (`writeFileWithSync`) use **platform-specific** durability:

| Platform | Policy | Rationale |
|----------|--------|-----------|
| **macOS (APFS)** | No application-level `fsync` / `F_FULLFSYNC` on temp write + hardlink | `os.File.Sync()` maps to `F_FULLFSYNC` and can stall for seconds–minutes under Time Machine / Spotlight load (see `cas_no_sync_stall_test.go`). APFS journaling makes the rename/hardlink durable for alpha macOS targets. Residual: power-loss before journal commit is accepted; process-kill visibility is covered by `TestCAS_WriteFileWithSync_VisibleAfterChildKill`. |
| **Linux / other** | `fsync` the temp file **before** close/hardlink, then `fsync` the parent directory | GoReleaser / Linux is a release target. POSIX `fsync` is not `F_FULLFSYNC`; it closes the buffered-only crash window that CEF E:F-REL-001 flagged. Fail closed if either sync returns an error. |

CAS **index** saves still `Sync()` the temp index on all platforms (one file per kind, not the 327-object hot path).

The **hash registry** remains a derived/rebuildable cache (`system check --auto-fix`) and does **not** fsync in `processSave`.

## Implementation anchors

- `pkg/storage/cas_publish_sync.go` — testable hooks
- `pkg/storage/cas_publish_sync_darwin.go` / `cas_publish_sync_other.go` — OS implementations
- `pkg/storage/content_addressable_storage_file.go` — `writeFileWithSync`
- `pkg/storage/hash_registry.go` — `processSave` omits `file.Sync()`
- Regression: `pkg/storage/cas_no_sync_stall_test.go`, `pkg/storage/cas_publish_sync_test.go`

## What this is not

- Not a claim of battery-backed enterprise durability SLAs.
- Not a substitute for backup / release checksums (`SECURITY.md`, `install.sh` fail-closed checksums).
- Graph/Memgraph remains a **projection**, not durability SSOT ([ADR-STORAGE-FILE-SSOT-GRAPH-PROJECTION](./decisions/ADR-STORAGE-FILE-SSOT-GRAPH-PROJECTION-v1.0.md)).
