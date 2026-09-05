# Vectorization Engine Architecture Blueprint

**Last Verified:** 2026-08-31


## 1. Structure-Aware Chunking
Documents are broken down into discrete semantic units (Chunks) and injected with structural metadata (Kind, Domain, Goal) prior to vectorization. Embeddings are L2-normalized to ensure they retain high "semantic density."

## 2. Chunk-Level Graph Integration
- **HNSW Index:** Leveraging MemGraph/Neo4j natively using an HNSW (Hierarchical Navigable Small World) index with Cosine Similarity.
- **Data Model:** Vectors are not stored on parent objects. Instead, we use a `(c:Chunk)-[:PART_OF]->(o:SystemObject)` relationship pattern.
- **Hierarchical Retrieval (Macro-to-Micro):** To prevent exponential Cartesian memory bloat during graph traversals, the system queries the `SystemObject` index first to isolate a bounded set of parent objects, then executes a localized vector search only across the chunks owned by those parent nodes.

## 3. Dynamic Query Router
To resolve the catastrophic recall failure vs. HNSW breakage dilemma, a Dynamic Query Router is implemented inside `QueryHybrid`:
- **High Selectivity (Restrictive Constraints):** Bypasses the HNSW index entirely. Executes a rapid structural graph traversal first (pre-filtering), then runs an exact, brute-force Cosine Similarity scoring across the tiny candidate set.
- **Low Selectivity (Broad Constraints):** Uses the HNSW index first with Over-sampled Post-Filtering (requesting 10x K nearest neighbors) to ensure sufficient high-quality semantic matches survive structural filtering.

## 4. Topological Context Sorting & Orchestration Flow
- **TPM Pipeline:** The `pkg/pipeline` orchestrator accurately tracks exact Tokens Per Minute (TPM) against external provider limits using a local tokenizer, preventing 429 backpressure.
- **Topological Sorting:** Prompt compilation uses a Topological Context Sort. The graph traverses `[:DEPENDS_ON]` edges to build a mandatory prerequisite list. Token budgets are strictly reserved for foundational dependencies first (ensuring Policies and core Rules are never dropped), before backfilling raw semantic chunks up to the TPM limit.

## 5. 3-Layer Testing Strategy
- **Unit:** Mocking the `MemoryStore` to validate token budgeting and prompt injection truncation.
- **Integration:** Localized MemGraph testcontainers to validate `RetrieveSemantically` and `QueryHybrid` Cypher syntax against the HNSW index.
- **Semantic Validation:** Seeding the graph with diverse noise data and executing natural language queries to scientifically assert the ranking precision of embeddings.
