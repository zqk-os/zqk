# ADR: Autonomous Knowledge Pruning, Semantic Deduping & Obsolete Graph Compaction

## Status
Accepted

## Date
2026-11-20

## Context
As autonomous multi-agent workflows continuously mint, evolve, and supersede entities in the Knowledge Kernel Content-Addressable Store (CAS), disconnected subgraphs, orphaned nodes, semantic duplicates, and circular reference chains accumulate over time.
Without deterministic pruning and compaction, graph traversal latency degrades, storage footprints grow unboundedly, and semantic duplication fragments context retrieval across agent swarms.

In Cycle 155, we introduce an Autonomous Knowledge Pruning, Semantic Deduping, and Obsolete Graph Compaction architecture within `pkg/community/knowledge_pruner.go`.

The knowledge pruner must:
1. **Orphan Entity Detection**: Perform deterministic reachability analysis from protected root compass entities (such as active Goals, Milestones, and Priority Plans) to identify unreachable or dangling CAS nodes.
2. **Cycle-Breaking Graph Traversal**: Track visited nodes and active recursion paths during graph traversal so circular reference topologies are detected, safely broken, and never cause infinite recursion or stack exhaustion.
3. **Semantic Deduping**: Identify and collapse semantically redundant knowledge entities into canonical surviving nodes while remapping inbound lineage references and preserving provenance.
4. **Cascading Pruner & Graph Compaction**: Execute non-destructive, dependency-ordered cascading garbage collection with deterministic compaction plans and strict root-protection invariants so root compass objects are never pruned.
5. **Declarative Validation & Hygiene**: Uphold AST hygiene standards with declarative input validation (such as `strutil.ErrorOnBlank`) and clean package boundaries in `pkg/community`.

## Decision
1. Implement the orphan detection, cycle-breaking graph traversal, semantic deduplication, and cascading pruner/compaction engine in `pkg/community/knowledge_pruner.go`.
2. Pin root compass objects as immutable traversal anchors so they are unconditionally protected from pruning or compaction.
3. Structure pruning as a deterministic, idempotent pipeline: graph ingestion and structural validation, reachability and cycle-breaking traversal, semantic deduplication, topological cascade planning, and safe CAS compaction.
4. Provide a comprehensive unit test suite in `pkg/community/knowledge_pruner_test.go` verifying the Three-Fold Proof:
   - **Static Floor Invariant**: Structural validation of graph nodes, lineage edges, and required invariant fields, alongside zero-value and blank-identifier safety.
   - **Operational Dynamic Proof**: Deterministic runtime traversal that accurately detects orphaned entities, collapses semantic duplicates, and executes safe cascading pruning and compaction.
   - **Negative Adversarial Boundary**: Safe detection and breaking of circular reference graphs without infinite recursion, strict enforcement of the root-protection invariant, and rejection of malformed or invalid inputs.
5. Register this architectural decision via `zqk docman register` and link it to the governing requirement in the Knowledge Kernel.

## Consequences
- Prevents unbounded Knowledge Kernel CAS growth and graph entropy across long-running autonomous execution loops.
- Eliminates infinite-recursion hazards when encountering circular reference graphs.
- Guarantees strategic continuity by ensuring root compass entities remain protected during automated compaction.
