# Storage Backend Detection and Configuration v1.0

**Last Verified:** 2026-08-31


**Version:** 1.1.0  
**Created:** 2025-12-29  
**Updated:** 2026-07-29  
**Status:** Active (aligned to ADR)  
**Related:** [ADR-STORAGE-FILE-SSOT-GRAPH-PROJECTION-v1.0.md](./decisions/ADR-STORAGE-FILE-SSOT-GRAPH-PROJECTION-v1.0.md), Graph Backend Architecture Requirements

## Overview

ZQK selects a **storage topology** at process start via `StorageFactory` (`pkg/storage/storage_factory.go`).

**Normative product rule (2026-07-29):** the **filesystem CAS/stream plane is the durability source of truth**. Memgraph is an **optional projection / query accelerator**. See the ADR above.

Historical note: an earlier revision of this doc claimed “only one backend at a time.” Runtime later introduced **`HybridObjectStorage` dual-write** when a graph pool/socket is live. That dual-write is now classified as **`hybrid_legacy`** and is **not** the target Open-Core topology.

## Product modes

| Mode | Meaning | When |
|------|---------|------|
| **`file`** | `FileObjectStorage` only | **Default** / Open-Core / no graph deployable |
| **`file+projection`** (target) | File SSOT; graph rebuilt or updated as projection | Studio / large graphs |
| **`hybrid_legacy`** (current when graph up) | `HybridObjectStorage(graph primary, file secondary)` sync dual-write | Transitional — deprecate |
| **`graph_only`** | Not a product default | Experiments only |

## Storage Factory (today)

```go
factory, err := storage.NewStorageFactory(ctx, projectRoot)
storageProvider := factory.GetStorage()
```

### Detection Logic (current code)

1. **Graph available?**  
   Provider enabled, `ZQK_MOCK_GRAPH` / mock, or live `rpcpool.sock` dial succeeds.
2. **If graph available:**  
   `NewHybridObjectStorage(lazyGraph, fileStorage)` — **`hybrid_legacy`** (graph primary, file secondary, dual-write).
3. **Else:**  
   `FileObjectStorage` only — **`file`** mode (Open-Core / small-env path).

### Target Logic (ADR follow-up)

1. Default **`file`** unless explicitly opted into projection.
2. When projection enabled: **write file first**, then project to graph (sync or async); reads may prefer healthy graph with file fallback.
3. Keep **rebuild-from-file** as recovery when projection drifts.
4. Remove long-lived call sites that construct raw `FileObjectStorage` while the factory believes hybrid/graph is in force.

## Backend building blocks

### File / CAS / stream (`FileObjectStorage`)

- **Location:** `docs/process/` (+ stream / `.zqk` layouts per kind profile)
- **Role:** **Durability SSOT**
- **Cache:** Object ID / reverse-ref / listing indexes as applicable
- **Use case:** Default; no external dependencies

### Graph (`LazyGraphStorage` → `GraphObjectStorage` / pool)

- **Location:** Memgraph (Bolt) via scheduler RPC pool when available
- **Role:** Optional **projection** and relationship query accelerator
- **Use case:** Studio / GraphRAG — **not** required for Open-Core launch

### Hybrid legacy (`HybridObjectStorage`)

- Writes primary then secondary; reads primary with secondary fallback; lazy migrate secondary→primary on update.
- Comment in code still mentions “git-traceability” via secondary — under the ADR, **file is primary for durability**, so this wrapper’s **role order is legacy**.

## Integration Points

### CLI Processor

`Processor` uses `NewStorageFactory` for the default provider. Prefer that over ad-hoc `NewFileObjectStorage` unless intentionally file-only (tests, migration tools).

### Check Command

`check` still branches on file vs graph capabilities for cache pre-warm. With file-as-SSOT, **object ID / reverse-ref caches remain relevant** even when a graph projection exists.

## Configuration

```bash
# Historical / provider flags (see pkg/zqkenv)
export ZQK_GRAPH_ENABLED=true   # opt into graph connectivity when supported
export ZQK_GRAPH_HOST=localhost
export ZQK_GRAPH_PORT=7687
# … pool size, credentials as documented in graph-backend runbooks
```

Explicit **`storage.mode=`** config is an ADR implementation follow-up (not yet a stable public knob). Until then, “no graph socket / not enabled” ⇒ **`file`**.

### Detection Flow (target)

```
NewStorageFactory
    │
    ├─ mode file (default / Open-Core) ──► FileObjectStorage
    │
    └─ projection opted in and graph healthy
           │
           ├─ write: file first, then project
           └─ read: graph if healthy else file
```

### Detection Flow (current legacy when graph up)

```
Graph available? ──yes──► Hybrid(graph, file)  [hybrid_legacy]
                 └──no───► FileObjectStorage
```

## Benefits (ADR-aligned)

1. **Portable default** — small envs need no Memgraph artifact  
2. **One durability story** — git + CAS  
3. **Optional acceleration** — graph when worth the ops cost  
4. **Graceful degrade** — projection down ⇒ file still authoritative  

## Testing

```bash
# File-only (no graph socket / graph disabled)
./bin/zqk system check

# Studio with graph projection available (legacy hybrid until factory lands)
# ensure rpcpool.sock / provider per graph-backend runbooks
./bin/zqk system check
```

## See also

- [ADR-STORAGE-FILE-SSOT-GRAPH-PROJECTION-v1.0.md](./decisions/ADR-STORAGE-FILE-SSOT-GRAPH-PROJECTION-v1.0.md)
- [graph_backend_pivot_spec.md](./graph_backend_pivot_spec.md)
- [MEMGRAPH_BULK_OPS_GUIDE.md](./MEMGRAPH_BULK_OPS_GUIDE.md)
- [object-storage-provider-v1.0.md](./object-storage-provider-v1.0.md)
