# Memgraph Bulk Operations: Performance & Best Practices

**Last Verified:** 2026-08-31


**Date:** 2026-07-29  
**Target:** ZQK Graph Storage / DataCell Performance Tuning  
**Status:** Reference Guide for Studio & Open-Core Batch Operations  
**Topology:** Graph bulk patterns apply when Memgraph projection is enabled. Durability SSOT remains filesystem — [ADR-STORAGE-FILE-SSOT-GRAPH-PROJECTION-v1.0.md](./decisions/ADR-STORAGE-FILE-SSOT-GRAPH-PROJECTION-v1.0.md). 

---

## 1. Overview & Architecture Considerations

Memgraph is an in-memory graph database built in C++. While its raw query execution speed is significantly faster than disk-backed graph databases (e.g., Neo4j), bulk mutations (inserts, updates, deletes) require careful memory and transaction management.

### Key Architectural Constraints
1. **MVCC Memory Overhead**: Memgraph uses Multi-Version Concurrency Control (MVCC). A single uncommitted transaction modifying hundreds of thousands of nodes/edges accumulates version chains in memory, causing high RAM allocation and potential lock contention.
2. **Supernode Locking**: Concurrent bulk writes targeting connected nodes ("supernodes") can trigger transaction retries or lock waiting.
3. **Cypher Parse Overhead**: Executing raw string Cypher statements per object creates execution plan parsing bottlenecks.

---

## 2. Recommended Batching & Chunking Boundaries

| Operation Type | Recommended Batch Size | Implementation Strategy |
|----------------|-----------------------|-------------------------|
| **Bulk Create / Insert** | $N = 1,000$ to $50,000$ | Parameterized `UNWIND $batch AS item` |
| **Complex Subgraph Mutations** | $N = 50$ to $500$ | Small transactional batches to prevent lock contention |
| **Bulk Delete (`DETACH DELETE`)** | $N = 500$ to $1,000$ | Client-side/Procedure ID chunk loops |
| **Schema Index Priming** | Pre-bulk | `CREATE INDEX ON :Label(property)` prior to batching |

---

## 3. Cypher Query Patterns & Best Practices

### A. Parameterized Ingestion via `UNWIND`
Do **not** execute individual `CREATE` statements in loops. Instead, pass a parameter array and unwind:

```cypher
UNWIND $batch AS row
CREATE (e:Entity {id: row.id, kind: row.kind})
SET e += row.properties
```

### B. Index Priming Before Bulk Operations
Executing `MERGE` or `MATCH` during bulk inserts without an index results in $O(N^2)$ full scans.
Always verify or create indexes prior to executing bulk workloads:

```cypher
CREATE INDEX ON :Entity(id);
```

### C. Safe Bulk Delete Pattern
Avoid global unbatched deletes (`MATCH (n) DETACH DELETE n`). Use parameterized ID chunks:

```cypher
UNWIND $ids AS target_id
MATCH (n:Entity {id: target_id})
DETACH DELETE n
```

**Studio equivalent (shipped):** `WHERE n.id IN $deleteIds` + `DETACH DELETE` in chunks of `graphBulkSetDeleteBatchSize` (1000), after `CREATE INDEX ON :Entity(id)`. Same intent; prefer the form that Memgraph plans as an index seek on your driver.

---

## 4. Summary Checklist for ZQK Graph Storage Layer

- [x] **Auto-Chunking**: Enforce batch limits ($N = 50$ legacy tx; $N = 1,000$ for node deletes; $N = 500$ UNWIND for BulkCreate/BulkUpdate).
- [x] **Index Pre-creation**: Ensure primary entity lookup properties (e.g. `:Entity(id)`) have active indexes.
- [x] **Parameterized Calls**: Ensure driver calls pass slice parameters over parameterized Cypher templates.

---

## 5. Studio implementation map (2026-07-29)

| Guide item | Code |
|------------|------|
| BulkCreate/Update UNWIND N=500 | `graphBulkUNWINDBatchSize` + `conn.ExecuteBatch` (`create_node` / `update_node`) |
| Set-delete N=1000 | `graphBulkSetDeleteBatchSize` + `detachDeleteEntitiesByIDs` |
| Index prime | `ensureEntityIDIndex` → `CREATE INDEX ON :Entity(id)` |
| Anti-pattern N×DeleteNode | Leaf `BulkDelete` uses ExecuteQuery set-delete only |
| Anti-pattern processDir dependents scan | `findDependents` fails closed if reverse-ref index not ready (LoadCache only) |

**Still open / follow-ups:** CAS index rebuild CLI for stale misses; optional BulkGet UNWIND; factory migration from `hybrid_legacy` to file-first projection ([ADR-STORAGE-FILE-SSOT-GRAPH-PROJECTION-v1.0.md](./decisions/ADR-STORAGE-FILE-SSOT-GRAPH-PROJECTION-v1.0.md)). TRACK: `[REDACTED-ID]`.
