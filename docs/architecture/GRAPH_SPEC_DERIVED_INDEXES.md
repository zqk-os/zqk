# Graph indexes derived from object specs

**Last Verified:** 2026-08-31


**Status:** Active (GFS P2a)  
**TRACK:** `BLI-1785825620087289000-ecb2b2c1` (design) · `BLI-1785825621805072000-d4e0e08e` (implementation)  
**Related:** [ADR-STORAGE-FILE-SSOT-GRAPH-PROJECTION-v1.0.md](./decisions/ADR-STORAGE-FILE-SSOT-GRAPH-PROJECTION-v1.0.md), [DATA_ORIGINATION_PIPELINE_VISION.md](./DATA_ORIGINATION_PIPELINE_VISION.md)

## Intent

Object specs under `.zqk/specs/objects/` are the **single schema plane**. MemGraph must not invent a parallel field contract. Indexes and (later) constraints / typed edges are **derived** from that plane so query performance tracks kinds humans already declared.

## Current behavior

| Mechanism | Source | Notes |
|-----------|--------|--------|
| `:Entity(id)` | `ensureEntityIDIndex` | Bulk delete set-path |
| `:<KindLabel>(id)` per kind | `EnsureSpecDerivedIndexes` | Idempotent `CREATE INDEX`; kind list from spec index |
| Node properties | Runtime object maps | Validated by shared Go validators on write |
| `*_refs` edges | Not yet | Still properties; future DDL pass |

## Target shape

1. **Labels** — `toLabel(kind)` + `Entity` (unchanged).
2. **Indexes** — `id` on `Entity` and each kind label present in the project spec index.
3. **Later** — unique constraints where specs declare identity; relationship types for `*_ref` / `*_refs` fields (projection of file SSOT, rebuildable).

## Invocation

- `FileFirstProjectionStorage.RebuildProjectionFromSSOTDetailed` calls `EnsureSpecDerivedIndexes` after upsert/orphan purge.
- Operators: `zqk system rebuild-graph-projection` (file+projection mode).

## Non-goals

- MemGraph as durability SSOT.
- Per-backend lifecycle rules that diverge from file create/update membranes.
