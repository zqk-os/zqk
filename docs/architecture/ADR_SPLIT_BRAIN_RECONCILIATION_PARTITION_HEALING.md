# ADR: Deterministic Split-Brain Reconciliation and Network Partition Healing

## Status
Accepted

## Date
2026-11-20

## Context & Motivation
In distributed ZQK swarm deployments, transient network partitions can isolate cluster subsets into disconnected islands. While partitioned, isolated sub-clusters may continue accepting mutations or diverging in local state, creating a split-brain topology with distinct branch histories and causal timelines. When the underlying mesh heals and nodes reconnect, naive last-writer-wins or ad-hoc state merging risks silent data loss, accepting stale minority rollbacks, or admitting forged causal histories.

In Cycle 154, we introduce the Deterministic Split-Brain Reconciliation and Network Partition Healing subsystem (`pkg/community/partition_reconciler.go`) to provide deterministic partition detection, divergent quorum detection, and conflict-free state unification during mesh heal events.

## Decision & Architecture

1. **Branch History Log & Vector Clock Invariants (Static Floor)**:
   - Every reconciler configuration and partition branch descriptor must satisfy strict static structural invariants before any reconciliation pass executes:
     - Local cluster identities and node identifiers must be non-empty.
     - Quorum thresholds must be strictly positive and mathematically consistent with total cluster membership (`quorum > total_nodes / 2`).
     - Branch history logs must carry non-empty, monotonic causal vector clocks and valid cryptographic state hashes (e.g., SHA-256 state digests).
     - Malformed, empty, or mathematically inconsistent partition descriptors fail closed immediately.

2. **Strict Majority Quorum Arbitration**:
   - Upon network reconnection, competing partition branches are evaluated against the cluster's strict majority quorum threshold.
   - A branch is eligible to serve as the authoritative winning baseline only if its attesting voter set satisfies strict majority quorum (`len(voters) >= quorum_threshold`).
   - When comparing causally ordered branches with valid quorums, causal vector clock dominance determines the authoritative base; concurrent divergent branches with valid state proofs are ordered deterministically by causal rank and canonical digest tie-breaking so every node converges to the exact same winning branch.

3. **Deterministic Delta Unification**:
   - Once the winning majority branch is selected, non-conflicting state deltas from reconciled branches are unified deterministically without data loss.
   - Operations are applied in causal vector-clock order, with deterministic key-level conflict resolution for concurrent updates and idempotent replay protection across the branch history log.
   - Post-reconciliation state produces a unified causal vector clock (component-wise maximum across merged timelines) and a verifiable deterministic state hash.

4. **Sub-Quorum & Stale Rollback Rejection**:
   - Reconciliation requests originating from minority/sub-quorum branches (`len(voters) < quorum_threshold`) are rejected immediately.
   - Stale partition branches whose vector clocks are strictly dominated by the current healed state, or branches containing forged, non-monotonic, or tampered vector clocks/state hashes, are aborted fail-closed to prevent state regression or Byzantine rollback.

## Verification & Three-Fold Proof Strategy
Verification is anchored in `pkg/community/partition_reconciler_test.go`, structured around the Three-Fold Proof:
1. **Static Floor Invariant**: Validates that empty cluster identities, invalid quorum thresholds, empty branch vector logs, and malformed state hashes are rejected fail-closed at construction and input validation boundaries.
2. **Operational Dynamic Proof**: Simulates multi-node network partitions with divergent branch histories, verifies strict majority quorum arbitration upon mesh heal, and confirms deterministic, zero-loss delta unification across reconnected nodes.
3. **Negative Adversarial Boundary**: Exercises minority/sub-quorum branch reconciliation attempts, stale partition rollback attempts, and forged/tampered vector clocks, asserting immediate rejection without mutating authoritative cluster state.

## Consequences
- Guarantees deterministic, convergence-safe split-brain recovery across healed mesh topologies without manual intervention.
- Prevents minority partitions or stale nodes from overwriting or rolling back majority-committed state.
- Establishes verifiable causal and cryptographic invariants for all partition reconciliation transitions.
