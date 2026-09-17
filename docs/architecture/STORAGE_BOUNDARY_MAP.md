# Storage Architecture: Subpackage Boundary Map & One-Way Import Contract

**Last Verified:** 2026-08-31


**Status:** Approved Architectural Specification (2026-08-21)  
**Track:** `PRI-CEF-R12-GOD-PACKAGE-001` / `BLI-CEF-R12-BOUNDARY-MAP-001`  
**Requirement:** `REQ-CEF-R12-ARCH-GOD-STORAGE`  
**Milestone:** `MIL-CEF-R2-M2-ARCH`  
**Workstream:** `WS-CEF-ARCHITECTURE`  
**Auditor Finding Reference:** `F-ARCH-001` (Storage god-package decomposition)

---

## 1. Executive Summary & Rationale

Prior to Round 12, `pkg/storage` functioned as a monolithic god-package containing over 567 files spanning core interfaces, content-addressable storage (CAS), write-ahead logging (WAL), Memgraph/graph indices, caching layers, and high-level query orchestration. 

This document defines the strict, bounded subpackage taxonomy, responsibility boundaries, and acyclic one-way import hierarchy governing `pkg/storage`.

---

## 2. Target Subpackage Taxonomy & Layering

```
                     ┌───────────────────────────────────────┐
                     │              pkg/storage              │  Level 3 (Root Facade)
                     │  - Public API (NewFileObjectStorage)  │
                     │  - Backwards-compatible methods       │
                     └───────────────────┬───────────────────┘
                                         │ imports
               ┌─────────────────────────┼─────────────────────────┐
               ▼                         ▼                         ▼
  ┌─────────────────────────┐ ┌─────────────────────┐ ┌─────────────────────────┐
  │    pkg/storage/file     │ │  pkg/storage/graph  │ │    pkg/storage/cache    │  Level 2 (Engines)
  │ - Content Addressable   │ │ - Graph DB syncing  │ │ - In-memory object cache│
  │ - Blobs & FS writes     │ │ - Relationship index│ │ - TTL & LRU invalidation│
  │ - Shard partition layout│ │ - Graph traversals  │ │                         │
  └────────────┬────────────┘ └──────────┬──────────┘ └────────────┬────────────┘
               │                         │                         │
               │ imports                 │ imports                 │ imports
               ▼                         ▼                         │
  ┌─────────────────────────────────────────────────┐              │
  │                 pkg/storage/wal                 │              │  Level 1 (Durability)
  │  - Append-only write-ahead log & segment files  │              │
  │  - Crash recovery & replay state machines       │              │
  └────────────────────────┬────────────────────────┘              │
                           │                                       │
                           │ imports                               │ imports
                           ▼                                       ▼
  ┌─────────────────────────────────────────────────────────────────────────────┐
  │                              pkg/storage/core                               │  Level 0 (Domain)
  │  - Interfaces (ObjectStorage, GraphProvider, WALManager, CacheProvider)     │
  │  - Data Models, Filter descriptors, Structs, Error definitions              │
  │  - Zero dependencies on sibling storage packages                            │
  └─────────────────────────────────────────────────────────────────────────────┘
```

---

## 3. Layer Responsibility Matrix

| Subpackage | Layer | Primary Responsibility | Allowed Imports | Forbidden Imports |
| :--- | :---: | :--- | :--- | :--- |
| **`pkg/storage/core`** | 0 | Fundamental interfaces, domain types, filter structs, error definitions | Standard library, `pkg/objects`, `pkg/utils` | `pkg/storage`, `pkg/storage/{file,graph,wal,cache}` |
| **`pkg/storage/wal`** | 1 | Write-ahead logging, segment rolling, crash recovery, compaction | `pkg/storage/core`, standard library | `pkg/storage`, `pkg/storage/{file,graph,cache}` |
| **`pkg/storage/file`** | 2 | Content-addressable file storage, sharding, atomic writes, fsync | `pkg/storage/core`, `pkg/storage/wal` | `pkg/storage`, `pkg/storage/{graph,cache}` |
| **`pkg/storage/graph`** | 2 | Memgraph / Neo4j relationship indexing and graph queries | `pkg/storage/core`, `pkg/storage/wal` | `pkg/storage`, `pkg/storage/{file,cache}` |
| **`pkg/storage/cache`** | 2 | High-throughput in-memory caching, invalidation, TTL | `pkg/storage/core` | `pkg/storage`, `pkg/storage/{file,graph,wal}` |
| **`pkg/storage`** | 3 | Public facade, constructor `NewFileObjectStorage`, legacy forwarding | All `pkg/storage/*` subpackages | None (root aggregator) |

---

## 4. One-Way Dependency & Acyclic Invariants

1. **Strict Upward Aggregation**:
   - `pkg/storage` (Level 3) is the ONLY package permitted to import subpackages (`core`, `file`, `graph`, `wal`, `cache`).
   - Subpackages (`pkg/storage/*`) MUST NEVER import root `pkg/storage`.
2. **Zero Horizontal Peer Dependencies Between Engines**:
   - `pkg/storage/file`, `pkg/storage/graph`, and `pkg/storage/cache` must remain completely independent of one another.
3. **Core Isolation**:
   - `pkg/storage/core` must have ZERO dependencies on any other `pkg/storage/*` package.
4. **Automated Enforcement**:
   - Boundary rules are validated in CI and pre-commit hooks via AST import analysis (`BLI-CEF-R12-IMPORT-GATE-001`).
