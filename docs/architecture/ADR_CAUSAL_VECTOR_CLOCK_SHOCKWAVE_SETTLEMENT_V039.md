# ADR: Distributed Causal Vector Clock & Epistemic Shockwave Settlement Engine v0.3.9

## Status
Accepted

## Context
In distributed multi-agent swarms operating across independent nodes, clusters, or network partitions, status transitions (e.g., flipping Goal, Milestone, Priority Plan, and BLI lifecycles) propagate asynchronously as epistemic shockwaves. Physical wall-clock timestamps are susceptible to clock skew and NTP drift, which can cause out-of-order execution, race conditions, or conflicting status updates when network partitions heal.

To guarantee eventual consistency, deterministic convergence, and causal ordering across the distributed swarm, the community kernel requires a pure-Go, dependency-free causal vector clock primitive and an idempotent shockwave settlement engine.

## Decision
1. **Causal Vector Clock Primitive (`VectorClock`)**:
   - Each node maintains an integer counter map indexed by node ID (`map[string]uint64`).
   - Supports atomic local ticks (`Tick(nodeID)`), point-to-point message stamping, and monotonic pairwise merging (`Merge(other)` where each component becomes `max(v[i], other[i])`).
   - Provides strict partial ordering comparison: `Compare(other) -> RelationConcurrent, RelationBefore, RelationAfter, RelationEqual`.
   - Enforces fail-closed validation: rejects blank node IDs and non-positive increments.

2. **Epistemic Shockwave Settlement Engine (`ShockwaveSettler`)**:
   - Receives shockwave event envelopes bearing source node ID, target entity ID, transition status, and vector clock stamp.
   - Rejects or idempotently drops stale or duplicate events where incoming vector clock is causally dominated by the currently settled state (`RelationBefore` or `RelationEqual`).
   - Accepts and settles causal successors (`RelationAfter`) and applies deterministic tie-breaking (lexicographical node ID ordering) for concurrent (`RelationConcurrent`) events.
   - Enforces clock skew safety bounds to protect against adversarial or misconfigured nodes.

3. **Verification Matrix**:
   - Verified against requirement contracts and dedicated unit test suites.
   - Satisfies the Three-Fold Proof:
     - Static floor invariant: immutable vector clock representation, ordering invariants, and type validation.
     - Operational dynamic convergence proof: concurrent events across simulated nodes settle to deterministic states.
     - Negative adversarial boundary fencing: malformed payloads, negative counter advances, and extreme clock skews are rejected cleanly.

## Consequences
- Guarantees deterministic, conflict-free state convergence across all multi-agent swarm partitions.
- Eliminates reliance on synchronized physical wall clocks for distributed status settlements.
- Pure Go implementation guarantees 0 external dependencies and sub-millisecond settlement latency.
