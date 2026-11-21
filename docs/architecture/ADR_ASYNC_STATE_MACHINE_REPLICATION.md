# ADR: Asynchronous State Machine Replication & Merkle Log Compaction

## Status
Accepted

## Date
2026-11-20

## Context
In distributed multi-agent swarm deployments, peer nodes maintain replicated state machines driven by an append-only event log. As cluster uptime and event volume scale, naive log-based replication faces four core reliability challenges:
1. **Non-Deterministic Replay & Duplicate Delivery**: Asynchronous network delivery can reorder or duplicate events; without pure transition functions and idempotent application, replicas can diverge.
2. **Unbounded Log Growth**: Retaining the entire event history indefinitely exhausts memory and disk budgets.
3. **Slow Replica Catch-Up**: Lagging or newly joined nodes replaying millions of historical events experience unacceptable recovery latency.
4. **Silent State Divergence & Corrupted Snapshots**: Hardware faults, software bugs, or Byzantine peers can introduce subtle state drift or corrupted state transfers if cryptographic verification and isolation are absent.

To address these challenges, we introduce the Asynchronous State Machine Replication and Merkle Log Compaction subsystem in `pkg/community/state_replication.go`.

## Decision
1. **Deterministic State Transitions & Idempotence**:
   - Implement state transitions as pure, deterministic functions `(State, Event) -> (NextState, error)` with strict sequence monotonicity and schema invariant checks.
   - Track applied event sequences / deduplication watermarks so duplicate or replayed events are handled idempotently without side effects.
   - Reject malformed, out-of-order, or invariant-violating transitions fail-closed.

2. **Sliding-Window Log Compaction**:
   - Maintain a configurable sliding window of recent log entries above a compacted base sequence index.
   - Periodically prune log entries below the compaction watermark once a verified snapshot covers that prefix, bounding storage footprint while preserving recent tail entries for incremental replication.

3. **Merkle State Root Snapshots & Catch-Up**:
   - Compute deterministic cryptographic Merkle state roots (SHA-256 over canonically ordered state entries) at compaction boundaries and snapshot checkpoints.
   - Support fast catch-up replication for lagging nodes by transferring a Merkle-anchored snapshot followed by incremental replay of the post-snapshot log tail.
   - Verify snapshot payloads against their declared Merkle root before installation; reject any corrupted or mismatched snapshot fail-closed.

4. **Divergence Detection & Node Quarantine**:
   - Compare Merkle state roots and transition digests across peers during replication and checkpoint exchange.
   - Upon detecting a state root mismatch, non-deterministic transition divergence, or corrupted snapshot root, immediately quarantine the diverging node to halt further state propagation and isolate the fault.
   - Require clean snapshot resynchronization and verification before a quarantined node can rejoin active replication.

5. **Implementation Hygiene & Three-Fold Verification**:
   - Adhere to codebase hygiene standards, including declarative validation (`strutil.ErrorOnBlank`) for node and stream identifiers and structured concurrency (`goroutinelabels`) for background replication tasks.
   - Provide comprehensive test coverage in `pkg/community/state_replication_test.go` satisfying:
     - **Static Floor Invariant**: Pure transition determinism, idempotent event application, and state schema invariants.
     - **Operational Dynamic Proof**: Sliding-window log pruning, periodic Merkle state root calculation, and fast snapshot catch-up across lagging nodes.
     - **Negative Adversarial Boundary**: Detection of non-deterministic transitions, rejection of corrupted snapshot roots, and quarantine of diverging nodes.

## Consequences
- **Bounded Storage & Fast Recovery**: Sliding-window compaction caps log growth while Merkle snapshots enable rapid catch-up for lagging replicas.
- **Cryptographic Integrity**: Every compacted state boundary is anchored by a verifiable Merkle root, preventing silent corruption during snapshot transfer.
- **Fault Isolation**: Automated divergence detection and node quarantine contain faulty or non-deterministic replicas before they corrupt healthy cluster state.
