# ADR-0048: Autonomous Provider Deadlock Detection & Lease Renewal v0.3.6

- **Title: Autonomous Provider Deadlock Detection & Lease Renewal v0.3.6**
- **Status: Accepted**
- **Date**: 2026-11-12
- **Author**: Community Core Team

## Context

When multiple community providers coordinate on shared system resources (file locks, network sockets, ephemeral state caches), lock inversion or unhandled error panics can create circular wait-for dependency graphs. Without active cycle detection and lease heartbeating, worker goroutines deadlock indefinitely, exhausting swarm resources.

## Decision

1. **Wait-For Graph Cycle Detection**:
   - Introduce `ProviderLockManager` maintaining directed wait-for dependency graphs across resource allocations.
   - Run depth-first search cycle detection prior to granting lock acquisitions, rejecting cyclic acquisitions with `ErrLockDeadlockDetected`.

2. **Heartbeated Leases & Automatic Expiry**:
   - Provide auto-renewing lease handles with configurable time-to-live timeouts.
   - If a provider process stalls or abandons a lease without renewal, the lock is automatically preempted and reassigned.

3. **Ontological Traceability & Quality Gate**:
   - Traceable directly to parent Community Ecosystem Extensibility goals and active execution priority plans.
   - Complies fully with `POL-DOC-001` character budgets and prohibition of internal cycle sprint narratives in public docs.

## Consequences

- Swarm executions are strictly protected against cyclic deadlocks.
- Stalled or abandoned worker locks expire deterministically, maintaining forward momentum.
- Multi-agent swarms operate with resilient mutual exclusion guarantees.
