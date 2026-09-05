# Graph Backend Pivot Specification

**Last Verified:** 2026-08-31


**Version:** 1.1.0  
**Status:** Approved (SSOT clarified 2026-07-29)  
**Date:** 2026-07-04  
**Updated:** 2026-07-29  
**Target File Path:** `docs/architecture/graph_backend_pivot_spec.md`  
**Normative topology:** [ADR-STORAGE-FILE-SSOT-GRAPH-PROJECTION-v1.0.md](./decisions/ADR-STORAGE-FILE-SSOT-GRAPH-PROJECTION-v1.0.md)

This specification document outlines ZQK's architectural transition from a legacy file-based YAML/JSON storage to a pluggable graph database backend, ensuring complete Git-traceability, high-performance querying, and GraphRAG operational readiness.

**Clarification (ADR 2026-07-29):** Filesystem CAS/stream remains the **durability source of truth**. Memgraph is an **optional projection / query accelerator**, not a second SSOT. Open-Core / small installs default to **file-only** (no Memgraph deployable required). Synchronous dual-write `HybridObjectStorage(graph, file)` is **`hybrid_legacy`** pending factory migration to file-first projection.

---

## 1. Pluggable Interface

ZQK adopts a clean, abstracted boundary between the high-level object storage operations and the low-level graph representations. This is achieved via unified interface definitions and a set of adaptive wrappers.

### Core Interface Contracts

*   **`ObjectStorageProvider`** (defined in `pkg/storage/object_storage_interface.go`): The primary application-facing interface for reading, writing, and querying ZQK specification objects. It abstractly exposes methods such as `Create`, `Read`, `Update`, `Delete`, `List`, `Query`, `Search`, `Exists`, `Count`, `Aggregate`, `GetRelated`, `GetPath`, `GetNeighbors`, `Move`, and `Rename`.
*   **`ObjectTransaction`** (defined in `pkg/storage/object_storage_interface.go`): Governs high-level transactional boundaries for atomic multi-object operations, supporting `Create`, `Read`, `Update`, `Delete`, `Commit`, and `Rollback`.
*   **`GraphProvider`** (defined in `pkg/graph/provider/interfaces.go`): The factory and capability discovery interface for graph database backends. It defines the connection pool initializer `CreatePool` and exports backend capability summaries (`GetCapabilities`, `SupportsFeature`).
*   **`ConnectionPool`** (defined in `pkg/graph/provider/interfaces.go`): Manages a pool of active connection instances, optimizing resources through connection reuse (`GetConnection` and `ReturnConnection`) and automatic transaction rollback on connection release.
*   **`GraphConnection`** (defined in `pkg/graph/provider/interfaces.go`): Represents a single active channel to the graph database. It exposes node and edge manipulation APIs (`CreateNode`, `GetNode`, `UpdateNode`, `DeleteNode`, `ListNodes`, `CreateEdge`, `GetEdge`, `UpdateEdge`, `DeleteEdge`, `ListEdges`), raw queries (`ExecuteQuery`), similarity search (`ExecuteVectorQuery`), and explicit transaction creation.
*   **`GraphTransaction`** (defined in `pkg/graph/provider/interfaces.go`): Defines transactional operations mirroring the node and edge mutators of `GraphConnection` in an explicit context, alongside nested transaction support via `BeginNestedTransaction`.

### Structural Wrappers & Adapters

To ensure performance, backward compatibility, and reliable execution patterns, ZQK implements five key wrappers:

1.  **Lazy Storage Wrapper (`pkg/storage/lazy_graph_storage.go`)**: Defers the initialization and acquisition of the graph connection pool until the first data operation is invoked. This design pattern ensures that fast, non-database CLI tasks (e.g., help commands, diagnostics) do not suffer from connection latency or blocking startup times. If execution is CLI-based and not the scheduler daemon itself, the wrapper attempts to connect to the JSON-RPC socket proxy, falling back to a direct pool if the daemon is unavailable.
2.  **Pool Aware Wrapper (`pkg/storage/object_storage_pool_wrapper.go`)**: Adapts `GraphObjectStorage` (which operates on a single connection context) to run safely within a connection pool environment. Every invocation under `ObjectStorageProvider` is mapped to a `pool.Execute` function block that handles connection acquisition, operation execution, and connection return. For transactions, it acquires a connection, starts a `GraphTransaction`, and yields a `poolAwareTransaction` wrapper which ensures that calling `Commit` or `Rollback` safely returns the connection to the pool.
3.  **Hybrid Wrapper (`pkg/storage/object_storage_hybrid.go`)** — **`hybrid_legacy`**: historically dual-writes with **graph as primary** and **file as secondary**, graph-first reads, and lazy file→graph migration on update. Partition locks (`sync.RWMutex` shards) serialize concurrent writes per object id. **Target (ADR):** file is durability primary; graph is projection — prefer file-first write + project, then retire graph-primary dual-write as the default. See [ADR-STORAGE-FILE-SSOT-GRAPH-PROJECTION-v1.0.md](./decisions/ADR-STORAGE-FILE-SSOT-GRAPH-PROJECTION-v1.0.md).
4.  **Scheduler Daemon RPC Proxy (`pkg/graph/rpcpool/`)**: Starts a background JSON-RPC server over a local Unix domain socket (`rpcpool.sock`). Rather than paying a connection penalty (TCP handshake, auth, session establishment) on every quick CLI run, ephemeral client invocations tunnel through the Unix socket using `rpcpool.ConnectClient()`, leveraging the persistent connections held active by the scheduler daemon.
5.  **MemGraph Backend (`pkg/graph/memgraph/`)**: The concrete graph database implementation. It communicates with MemGraph using the Bolt protocol via the official Neo4j Go driver (`neo4j-go-driver/v5`). It supports native Cypher queries, index setup, connection pooling, and vector search.

---

## 2. GraphRAG Schema

ZQK organizes its knowledge graph using a unified GraphRAG schema design. This architecture enables multi-hop reasoning, semantic searching, and structured document retrieval.

### The Three-Layer Schema Architecture

```
+-------------------------------------------------------------+
|                     1. DOCUMENT LAYER                       |
|           [ :Document ] ---> raw YAML files & chunks        |
+-------------------------------------------------------------+
                               | (HAS_SOURCE)
                               v
+-------------------------------------------------------------+
|                      2. ENTITY LAYER                        |
|   [ :Goal ]   [ :Milestone ]   [ :Requirement ]   [ :etc ]  |
+-------------------------------------------------------------+
                               | (BELONGS_TO, IMPLEMENTS, etc.)
                               v
+-------------------------------------------------------------+
|                    3. RELATIONSHIP LAYER                    |
|             Ontological edges connecting entities           |
+-------------------------------------------------------------+
```

1.  **Document Layer**: Represents the raw source of truth. Each specification YAML file in the repository maps to a `:Document` node containing properties like `source_path`, full YAML `content` as a string, `chunk_id`, `chunk_index`, `chunk_text`, and file timestamps.
2.  **Entity Layer**: Represents the compiled semantic objects extracted from the document content. Each system kind (e.g., `goal`, `requirement`, `backlog_item`) translates to a node with a corresponding label (e.g., `:Goal`, `:Requirement`, `:BacklogItem`). These nodes contain structured properties and a high-dimensional float vector (`embedding`) representing their title, description, and context.
3.  **Relationship Layer**: Formulates explicit directed edges between the entities based on their reference definitions. Edges (e.g., `:BELONGS_TO`, `:IMPLEMENTS`, `:SUPPORTS`) carry metadata such as `source_field` and creation timestamps, establishing the topology required for graph traversals and RAG context extraction.

### Separation of Concerns

To avoid conflicts and scale horizontally, ZQK enforces clear separation of concerns:

*   **YAML Spec vs. DB Instance Separation**: YAML/CAS files remain the human-editable, Git-traceable **durability source of truth**. The database is an optional ephemeral **projection / indexed view** of those objects (rebuildable from file). Product modes: [ADR-STORAGE-FILE-SSOT-GRAPH-PROJECTION-v1.0.md](./decisions/ADR-STORAGE-FILE-SSOT-GRAPH-PROJECTION-v1.0.md).
*   **Namespaces**: Objects and references are scoped using namespace prefixes (e.g., `zqk_core:backlog_item`, `project_alpha:BLI-123`). This keeps multiple projects, environments, or standard templates isolated within the same database instance.
*   **Origin Project/System Tags**: Every node and edge is tagged with `origin_project` and `origin_system` metadata. This allows querying across the entire organization while maintaining strict filtering capabilities based on data source and ownership.

---

## 3. Ontology Definition

ZQK translating rules define how system object schemas map to nodes and relationships in the graph. The ontology maps specific YAML reference fields to directed graph edges.

### Ontology Mapping Table

| Source Object Kind | Source Label | Target Object Kind | Target Label | Reference Field | Relationship Type | Direction |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| `backlog_item` | `BacklogItem` | `priority_plan` | `PriorityPlan` | `priority_plan_ref` | `BELONGS_TO` | Outgoing |
| `backlog_item` | `BacklogItem` | `requirement` | `Requirement` | `requirement_refs` | `IMPLEMENTS` | Outgoing |
| `backlog_item` | `BacklogItem` | `goal` | `Goal` | `goal_refs` | `SUPPORTS` | Outgoing |
| `agent_task` | `AgentTask` | `backlog_item` | `BacklogItem` | `backlog_item_ref` | `ASSOCIATED_WITH` | Outgoing |
| `requirement` | `Requirement` | `goal` | `Goal` | `goal_refs` | `ALIGN_TO` | Outgoing |
| `priority_plan` | `PriorityPlan` | `goal` | `Goal` | `goal_refs` | `FOCUSES_ON` | Outgoing |

### Ontology Translation Rules

*   **Nodes**: Each YAML file is mapped to a node. The node's primary label corresponds to the camel-cased `kind` attribute (e.g., `backlog_item` becomes `BacklogItem`).
*   **Attributes**: Top-level YAML scalar and list fields (excluding references) are stored as node properties (e.g., `title`, `status`, `priority_tier`, `created_at`).
*   **Edges**: Fields ending in `_ref` (scalar) and `_refs` (array) are parsed. The validator resolves the target identifier to a destination node ID and constructs a directed edge from the source node to the target node using the mapped relationship type.

---

## 4. Migration & Verification Strategy

Migrating from file-based JSON/YAML storage to the graph backend is executed via a structured pipeline designed for validation, data integrity, and rollback readiness.

### Three-Phase Migration Process

1.  **Phase 1: Ingestion & Document Creation**: The migration tool scans the repository paths (excluding `_internal/` configurations), reads the YAML files, and creates `:Document` nodes with full file content and paths in the database.
2.  **Phase 2: Entity Compilation**: The tool parses each document's YAML contents, validates it against the object schema, combines descriptive text blocks, computes a vector embedding, and creates the `:Entity` node linked to its document via a `:HAS_SOURCE` relationship.
3.  **Phase 3: Relationship Stitching**: The tool iterates over the extracted reference properties, validates that the targets exist, and writes the directed relationship edges according to the system ontology.

### Integrity & Fallback Mechanisms

*   **Consistency Checks**:
    *   *Volume Match Check*: After migration, the count of `:Entity` nodes must exactly match the number of source YAML files.
    *   *Reference Integrity Check*: Unresolved references (pointing to non-existent IDs) are logged. Depending on configuration (`strict` vs `lenient`), the migration will halt or complete with warnings.
    *   *Directional Verification*: Verification queries confirm that relationships contain correct labels and travel in the appropriate direction (e.g., checking that a `BacklogItem` points to a `PriorityPlan`, not vice-versa).
*   **Schema Validation**: Every migrated object is validated against its schema specification (e.g., matching the `schema_version` requirements) before insertion. Parse failures or schema violations abort the transaction.
*   **Fallback Mechanisms**:
    *   *Read/Write Fallback*: If the graph database becomes unreachable or suffers a fatal query error during runtime, the application falls back to read operations on the local file system.
    *   *Transaction Rollback*: Migration runs within an explicit transactional block. Any failure during Phase 1, 2, or 3 aborts the migration, performing a full rollback in the database to maintain consistency. The local file system remains the immutable source of truth.

---

## 5. Architectural Gaps, Implementation Inconsistencies, and Unhandled Edge Cases

The transition to a pluggable graph database backend introduces several architectural challenges, system constraints, and edge cases that require ongoing management.

### Identified Gaps & Inconsistencies

1.  **Simulated Nested Transactions**: MemGraph does not natively support nested transactions (savepoints) via its Bolt driver interface. ZQK's `BeginNestedTransaction` interface method simulates nesting by tracking parent transaction pointers in Go memory (`parent *memgraphTransaction`). However, this simulation lacks true database-level isolation. A rollback at the simulated child level triggers a rollback of the entire parent transaction, failing to provide SQL-like partial rollback functionality.
2.  **Stateful Transaction Resource Leaks**: To minimize latency, the ephemeral CLI uses a lazy-loading connection client linked via Unix JSON-RPC to the persistent background scheduler daemon. If the CLI client crashes, terminates unexpectedly, or is forcefully killed (e.g., `SIGKILL`) while holding an active transaction, the Unix socket closes. However, the background daemon may not instantly detect the client's abrupt exit, potentially leaving the Bolt transaction active and locking connection pool resources on the database side.
3.  **Conceptual RDF SPARQL Translation**: While the interfaces `GraphProvider` and `GraphConnection` contain definitions for `QueryLanguageSPARQL` and `FeatureSPARQLQuery`, the actual translation pipeline from ZQK's abstract query model to SPARQL syntax remains conceptual. Only Cypher query generation is fully implemented; RDF backends are currently unsupported in practice.
4.  **Asynchronous Relationship Stitching**: Real-time CRUD operations modify node attributes immediately. However, constructing and updating relationship edges is deferred to asynchronous sync/migration pipelines. This delay introduces a temporary inconsistency where a node has updated a `priority_plan_ref` field, but the corresponding graph edge is not created until the next pipeline synchronization loop runs.
5.  **Namespace Validation Leniency**: To accommodate legacy configuration and migration data, the namespace validator implements fallback rules. If a reference does not have a namespace prefix (e.g., `BLI-101` instead of `project:BLI-101`), the validator falls back to the default or active project namespace. While this maintains backward compatibility, it introduces collision risk if identical IDs are present across different aggregated systems.
6.  **DB Provider Vector Search Dependency**: Similarity and vector searches (via `ExecuteVectorQuery`) depend heavily on specific vector indexes implemented by the graph provider. Because MemGraph, Neo4j, and Mock databases handle vector indexing and search operators differently, ZQK requires backend-specific execution branches, making the system dependent on DB-provider features and forcing slow in-memory vector calculations when running against Mock providers.
