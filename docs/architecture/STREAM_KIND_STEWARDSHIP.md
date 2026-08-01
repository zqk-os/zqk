# Per-kind stream stewardship

**Status:** Implemented (enqueue + execution contract)  
**Audience:** Operators and engineers extending high-volume stream maintenance.  
**Related:** [STREAM_STORAGE.md](./STREAM_STORAGE.md), `pkg/storage/stream_stewardship.go`, REQ-STREAM-001.

---

## Intent

Each **stream-backed object kind** has a distinct **steward** responsibility: registry compaction, orphan segment GC, and (when enabled) runtime-delta backfill/overlay GC for that kind’s paths. **Orchestration** stays centralized — one maintenance wake (aggregate → retention → post-retention stewardship) — but **work and observability are per kind** so:

- A hot kind cannot **conceptually** crowd out others (each kind is a bounded unit of work).
- **Low-volume** kinds still run the **same code path**; compact/GC on an almost-empty tree is cheap, so we do not special-case “tiny kinds” in a way that breaks when volume grows later.
- **Envelope-tick drain** sees **one JSONL record per kind per phase** (`stream_steward_kind`), not a single opaque “compact everything” line.

---

## Scheduler contract

After retention tolerance succeeds, **`storage.PostRetentionStreamStewardship`**:

1. Logs spec/profile drift once (all kinds).
2. For each enabled stream kind (**sorted** for determinism): **`RunPostRetentionStreamKindStewardship`** → then **`datacell.EnqueueStewardMaintenance`** with **`MaintenanceOpStreamStewardKind`** and detail `c=<cycle>|k=<kind>|p=segments`.
3. For each runtime-delta-enabled kind (**sorted**): backfill + overlay GC → enqueue same op with `p=runtime_delta`.

Detail is truncated to **`StewardEnqueueDetailMaxBytes`** (512) by `EnqueueStewardMaintenance`.

---

## Not duplicated flows

Retention **policy** remains **`retention_tolerance.yaml`** + **`RetentionToleranceHandler`**. Physical hygiene remains **`CompactStreamRegistryForKind`**, **`GCOrphanedStreamSegmentsForKind`**, runtime-delta helpers — **unchanged primitives**, only **scheduling and enqueue granularity** evolved.
