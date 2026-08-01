# CLI object create / update / delete — why the command can pause (bounded)

**Status:** Operational reference  
**Audience:** Operators, agents running bulk `zqk object` commands

## Not the scheduler guard (for `delete`)

`object delete` is classified as **`schedulerRequirementNone`** in `cmd/zqk/object/scheduler_guard.go`: the **scheduler daemon is not required** for delete to proceed. **`--allow-degraded` does not change delete behavior** via that guard (it only affects commands that require or optionally use the scheduler, e.g. `list`, `count`, `bulk*`).

If a bulk delete script appeared to “need” `--allow-degraded`, that was likely **coincidence** (retry after a slow flush, or a shorter second run), not scheduler gating.

## What actually blocks: write-behind durability + listing indexes

After a successful `Storage().Delete`, `object delete` **waits** so the next process sees CAS changes immediately:

1. **`EnsureCLIObjectMutationVisibleForProvider`** — uses **`DurabilityFlushContext()`**, which is a **`context.WithTimeout(..., 45*time.Second)`** (`pkg/storage/object_storage_cli_durability.go`). This drains the write-behind buffer and flushes per-kind listing index work under that deadline.
2. **`FlushAllListingIndexesForProjectRoot`** — additional flush so list mappings are consistent; uses a **short timeout** (currently **5s** total budget via `FlushAllListingIndexesForProjectRootWithTimeout` from `pkg/storage/cas_test_helpers.go`).

So the CLI is **not hanging indefinitely**: it is waiting for **bounded** I/O and index work. Under heavy WAL backlog or slow disk, a single delete can approach the **~45s** durability window (and fail with a timeout error if the buffer cannot drain in time).

## Mitigations for operators

- **Expect multi-second latency** after mutations on busy repos; this is **by design** for cross-process consistency.
- When total flush time **≥ ~2s**, the object CLI emits an **Info** log: `object CLI: durability flush slower than threshold …` with `ensure_visible`, `listing_index_flush` (delete only), and `total` durations—so long waits are **visible** in normal logging output, not silent.
- Use **`--timeout`** on the root `zqk` command if your wrapper kills the process too aggressively (default `0` lets the CLI auto-calculate; set explicitly for long runs, e.g. `90s` for batch scripts).
- **Do not** assume `--allow-degraded` fixes slow `object delete`; address **disk contention**, **WAL size**, or **run fewer concurrent mutators** instead.

## Related code

- `cmd/zqk/object/delete.go` — post-delete flush sequence  
- `cmd/zqk/object/mutation_durability_logging.go` — `logSlowCLIObjectMutationFlush`, `slowCLIObjectMutationFlushThreshold`  
- `pkg/storage/object_storage_cli_durability.go` — `DurabilityFlushContext`, `EnsureCLIObjectMutationVisible`  
- `cmd/zqk/object/scheduler_guard.go` — `requirementForObjectCommand`
