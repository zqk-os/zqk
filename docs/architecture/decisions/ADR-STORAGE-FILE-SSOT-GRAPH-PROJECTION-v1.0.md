# ADR: File CAS as durability SSOT; Memgraph as optional projection

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Created:** 2026-07-29  
**Status:** Accepted  
**Decision Date:** 2026-07-29  

**Related:** Open-Core plan `[REDACTED-ID]` · persistence BLI `[REDACTED-ID]` · decision object `[REDACTED-ID]` · hybrid dual-write legacy (BLI-302 era)

## Context

ZQK can persist process objects via:

1. **Filesystem / CAS / stream** under the project tree (`docs/process/`, `.zqk/…`) — git-native, no extra deployable.
2. **Memgraph (Bolt)** via `GraphObjectStorage` — fast graph query / GraphRAG adjacency.

When a graph socket/provider is detected, `StorageFactory` currently wraps **`HybridObjectStorage(graph primary, file secondary)`**: synchronous dual-write, graph-first read with file fallback, and lazy file→graph migration on update (`pkg/storage/storage_factory.go`, `object_storage_hybrid.go`).

That topology caused:

- **2× write cost** and divergence when one side failed or paths bypassed hybrid (many CLI helpers open `FileObjectStorage` directly).
- **Show-stopping bulk/delete hangs** on the file secondary (CAS full-dir scans) while graph used set-delete — fixed in PR #1303 / #1305, but dual-write remains an architectural tax.
- **Stale docs** claiming “only one backend at a time” while code silently dual-writes.

**Open-Core / small environments** must not require Memgraph (Docker or standalone). Filesystem-only must remain a first-class, complete product mode.

The graph pivot spec already stated the right long-term intent: YAML/CAS remains Git-traceable truth; the DB is an indexed view (`graph_backend_pivot_spec.md` §2 Separation of Concerns). Runtime hybrid dual-write drifted from that.

## Decision

### 1. Durability source of truth = filesystem (CAS / stream profiles)

Process object **durability** for product and Open-Core is the **file/CAS/stream plane**. Git, portable bootstrap, and “no extra artifact” installs depend on this.

### 2. Memgraph is an optional projection / query accelerator

When present, Memgraph may accelerate relationship queries, GraphRAG, and bulk graph mutations. It is **not** a second independent SSOT. It must be **rebuildable from file**.

### 3. Product storage modes

| Mode | Factory behavior | Audience |
|------|------------------|----------|
| **`file`** | `FileObjectStorage` only | **Default** for Open-Core, laptops, CI without graph |
| **`file+projection`** (target) | Write **file first**; project to graph (sync or async); reads may use graph when healthy, fall back to file | Studio / large graphs |
| **`hybrid_legacy`** | Today’s `HybridObjectStorage(graph, file)` dual-write | Transitional only — deprecate |
| **`graph_only`** | Not a product default | Experiments / non-durable scratch only |

### 4. Default and Open-Core

- **Community / Open-Core default = `file`.** No Memgraph required for launch-ready candidate trees.
- Graph remains **opt-in** (env + live pool/socket), never a hard dependency of `system init` or portable bootstrap.

### 5. Implementation direction (normative for follow-up code)

1. Document and eventually **config-select** mode (env or project config); until then, treat current hybrid as **`hybrid_legacy`**.
2. Prefer **file-first write, then project** over graph-first dual-write.
3. Eliminate long-lived bypasses that open raw `FileObjectStorage` while the process believes hybrid/graph is primary — one wiring path via `StorageFactory`.
4. Provide / keep **rebuild projection from file** as the recovery path when graph drifts.
   - **Landed API:** `FileFirstProjectionStorage.RebuildProjectionFromSSOT` (opt-in via `ZQK_STORAGE_MODE=file+projection`).
   - **CLI sketch (not yet shipped):** `zqk system rebuild-graph-projection [--dry-run]` → unwrap factory to `*FileFirstProjectionStorage`, call `RebuildProjectionFromSSOT`, report `rebuiltCount`. TRACK: `[REDACTED-ID]` (or follow-on if B3 scope is factory bypasses only).
5. Keep bulk graph UNWIND / set-delete optimizations for when projection is enabled; keep file CAS fail-fast / no processDir dependents scan for the SSOT path.

## Consequences

**Positive**

- Small installs and Open-Core stay single-binary + tree; no mandatory graph deployable.
- One durability story for agents and humans (git + CAS).
- Graph outages degrade to file without “which copy is true?” debates.
- Aligns code direction with `graph_backend_pivot_spec.md` SSOT language.

**Negative / cost**

- Studio must accept projection lag if projection becomes async.
- Migrating off `hybrid_legacy` needs factory + call-site cleanup and a rebuild tool.
- Relationship-heavy queries without graph stay on file indexes / scans (acceptable for small envs).

**Non-goals**

- Does not remove Memgraph support from Studio.
- Does not change data-cell **profile** vocabulary (CAS vs stream vs future); those remain spec-declared ([ADR-DATA-CELL-SPEC-PIPELINE](../../decisions/ADR-DATA-CELL-SPEC-PIPELINE-v1.0.md)).

## Related documents

| Topic | Document |
|-------|----------|
| **Decision object** | `[REDACTED-ID]` |
| Backend detection (updated for this ADR) | [storage-backend-detection-v1.0.md](../storage-backend-detection-v1.0.md) |
| Graph pivot / GraphRAG | [graph_backend_pivot_spec.md](../graph_backend_pivot_spec.md) |
| Memgraph bulk ops | [MEMGRAPH_BULK_OPS_GUIDE.md](../MEMGRAPH_BULK_OPS_GUIDE.md) |
| Object storage interface | [object-storage-provider-v1.0.md](../object-storage-provider-v1.0.md) |
| Factory (code) | `pkg/storage/storage_factory.go` |
| Hybrid legacy (code) | `pkg/storage/object_storage_hybrid.go` |
