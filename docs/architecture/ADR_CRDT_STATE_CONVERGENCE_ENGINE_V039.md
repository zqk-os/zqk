# ADR-CRDT-STATE-CONVERGENCE-ENGINE-V039: Adaptive Conflict Resolution & Deterministic CRDT State Convergence Engine

## Status
Accepted

## Context
In decentralized, multi-region agent swarms, concurrent mutations to shared kernel state, memory graphs, and task manifests occur across isolated network partitions. When partitions heal or gossip sync occurs, concurrent modifications cannot rely on centralized serializable locks without catastrophic latency penalties and single points of failure.

Previous iterations relied on simple vector clock causal comparisons, which identify concurrency but do not provide autonomous, conflict-free state resolution when concurrent updates do not possess an unambiguous causal ancestor.

To achieve continuous autonomous progression (CAP) across distributed multi-cluster deployments, the ZQK runtime requires a deterministic, commutative CRDT (Conflict-Free Replicated Data Type) convergence engine with Last-Write-Wins Element-Set (LWW-Element-Set) semantics and anti-entropy delta reconciliation.

## Decision
1. **CRDT Data Structures**:
   - `LWWRegister`: Represents a scalar attribute guarded by a monotonic logical timestamp and node identifier tie-breaker for deterministic total ordering.
   - `LWWElementSet`: Provides an add-set and remove-set with bias towards concurrent additions or removals according to deterministic policy, guaranteeing commutativity ($A \cup B = B \cup A$) and associativity ($A \cup (B \cup C) = (A \cup B) \cup C$).
   - `CRDTDocument`: A structured document state container encapsulating typed LWW registers and element sets with an monotonic version vector.
2. **Delta & Full State Anti-Entropy Synchronization**:
   - Nodes can broadcast either granular deltas (`CRDTDelta`) for low-bandwidth incremental gossip or full document states (`CRDTDocument`) for catch-up synchronization.
   - Merging operations are purely functional, idempotent, and commutative, guaranteeing that any two nodes that have received the same set of updates will converge to the exact same state regardless of arrival order.
3. **Declarative Hygiene & Boundary Enforcement**:
   - All string and ID validations adhere strictly to declarative helpers (`strutil.RequireNonBlank` / `strutil.ErrorOnBlank`), avoiding imperative procedural checks.
   - Malformed payloads, negative or zero timestamps, and nil delta updates are rejected fail-closed with explicit error types, maintaining local document invariants intact.

## Consequences
- **Positive**:
  - Deterministic state convergence across arbitrary partition topologies and high network latency.
  - Zero coordination or distributed locking required during local writes.
  - Provable commutativity and idempotency verified via automated property testing.
- **Negative**:
  - Tombstone tracking in LWW-Element-Sets incurs memory overhead until periodic garbage collection/compaction sweeps occur.
