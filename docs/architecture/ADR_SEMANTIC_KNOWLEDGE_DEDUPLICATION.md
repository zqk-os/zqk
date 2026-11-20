# ADR: Semantic Knowledge Deduplication & Graph Collision Collapser

## Status
Accepted

## Date
2026-11-20

## Context
In autonomous multi-agent knowledge graphs, concurrent or iterative ingestion frequently produces synonymic or redundant kernel entities that represent the same underlying concept under slightly different titles, descriptions, or vector projections. Left unchecked, these near-duplicate nodes fragment graph traversal, inflate Content-Addressable Storage (CAS) footprints, and dilute semantic retrieval precision.

Conversely, aggressive or naive merging risks collapsing genuinely distinct entities that happen to share surface-level vocabulary or occupy diverging parent branches in the ontological hierarchy.

In Cycle 156, we establish the Semantic Knowledge Deduplication and Graph Collision Collapser architecture in `pkg/community/semantic_dedup.go` to deterministically detect synonymic or redundant entities and collapse them into canonical CAS nodes while preserving structural divergence.

The deduplication and collision-collapsing engine must:
1. Enforce static structural integrity on all semantic embedding vectors and collision descriptors prior to similarity evaluation.
2. Combine continuous vector cosine similarity with lexical Levenshtein distance scoring to evaluate semantic equivalence.
3. Deterministically collapse verified duplicates into a single canonical CAS node with reproducible tie-breaking and alias tracking.
4. Preserve adversarial and structural boundaries so entities with distinct semantic signatures or diverging parent branches are never falsely collapsed.

## Decision
1. **Vector and Descriptor Floor Invariants (`pkg/community/semantic_dedup.go`):**
   - Reject empty or zero-magnitude embedding vectors, mismatched vector dimensions, and invalid or out-of-bounds normalized similarity thresholds (`[0.0, 1.0]`).
   - Require invariant schema fields (non-blank entity identifiers, descriptors, and parent-branch lineage metadata) before admitting nodes into collision evaluation.
2. **Hybrid Cosine + Levenshtein Similarity Scoring:**
   - Compute normalized vector cosine similarity over entity embeddings alongside normalized Levenshtein string similarity over canonicalized titles/descriptors.
   - Combine both signals via bounded, deterministic weighting to produce a composite collision score that captures both conceptual equivalence and lexical proximity.
3. **Deterministic Canonical Node Collapse:**
   - When candidate entities meet or exceed the calibrated collision threshold without violating structural guards, collapse redundant nodes into a single canonical CAS node.
   - Maintain deterministic canonical selection (e.g., lexicographical or lineage-ordered primary selection) and record merged aliases so downstream graph references resolve cleanly.
4. **Parent-Branch Divergence Preservation:**
   - Enforce an explicit divergence guard: entities anchored to diverging parent branches or exhibiting distinct semantic signatures below threshold boundaries are strictly isolated and never collapsed, preventing false-positive graph corruption.
5. **Three-Fold Verification & Kernel Registration:**
   - Cover the implementation with unit tests in `pkg/community/semantic_dedup_test.go` spanning static floor invariants, operational dynamic collapse, and negative adversarial boundary rejection.
   - Register this architectural decision in the Knowledge Kernel CAS via `zqk docman register` and link it to the governing requirement.

## Consequences
- Eliminates synonymic node bloat and keeps the Knowledge Kernel CAS compact and deterministic.
- Prevents false-positive merges across distinct ontological branches through explicit parent-branch divergence checks.
- Provides auditable, reproducible canonicalization for concurrent multi-agent knowledge ingestion.
