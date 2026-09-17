# Hive Mind Memory Architecture

**Last Verified:** 2026-08-31


## Overview
The 'Hive Mind Memory' component provides a unified storage layer for ZQK, merging graph-based structural relationships with vector-based semantic retrieval. This enables agents to query both concrete links (e.g., "Which spec is this requirement derived from?") and abstract context (e.g., "Find similar architectures to this system").

## Storage Schema

### Graph Layer (Neo4j/MemGraph)
Maintains the core ontology and structural relationships.
- Nodes: `Object`, `Relationship`, `Policy`, `Actor`.
- Properties: Standardized ZQK metadata, plus `vector_id` (a UUID linking to the Vector index).

### Vector Layer (Integrated)
Stores embeddings for textual content, doc snippets, and code chunks.
- Data Structure: `(VectorID, EmbeddingData, MetaDataLink)`
- Index: HNSW (Hierarchical Navigable Small World) for sub-millisecond retrieval.

## Agentic Retrieval Interface

Defined in `pkg/hivemind`:

```go
type MemoryStore interface {
    // RetrieveSemantically performs K-Nearest Neighbors search
    RetrieveSemantically(ctx context.Context, query string, k int) ([]MemoryResult, error)
    
    // RetrieveGraphContext fetches structural neighbors for a given ID
    RetrieveGraphContext(ctx context.Context, id string, depth int) (*GraphSubgraph, error)
    
    // QueryHybrid performs a combined semantic-structural query
    QueryHybrid(ctx context.Context, query string, constraints HybridConstraints) ([]MemoryResult, error)
}
```

## Synchronization Strategy
1. **Write-Through Dual-Write:** Any object creation/update through `zqk object create/update` triggers a synchronous write to the Graph store.
2. **Asynchronous Indexing:** An internal worker processes the new object text, generates an embedding, and updates the Vector index, ensuring the main request flow remains non-blocking.
3. **Consistency Check:** A background daemon (the `HiveMindSynchronizer`) runs periodically to reconcile graph `vector_id` pointers with existing vector indices.

## Testing Strategy
- **Vector Accuracy:** Use cosine similarity benchmarks on known ground-truth datasets (e.g., FAQ pairs).
- **Graph Connectivity:** Unit tests to ensure that inserting an object creates the expected edges and maintains referential integrity.
- **End-to-End Hybrid:** Scenarios that insert a new object and immediately attempt retrieval using a combination of semantic similarity and graph traversal depth to ensure atomic availability.
