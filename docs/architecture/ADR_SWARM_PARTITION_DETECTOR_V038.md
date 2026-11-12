# ADR: Swarm Partition Detector & Split-Brain Quorum Resolver v0.3.8

## Status
Accepted

## Date
2026-11-12

## Context
In distributed multi-agent swarm environments, transient network partitions, routing failures, or process freezes can isolate nodes into separate cluster fragments.
If partitioned nodes in a minority network component continue accepting job dispatches, claiming Backlog Items, or mutating Knowledge Kernel objects, catastrophic split-brain state divergence occurs.

In Cycle 137, we introduce a deterministic Swarm Partition Detector and Lease Fencing mechanism (`SwarmPartitionDetector`) within `pkg/community/partition_detector.go`.

The partition detector must:
1. Enforce strict majority quorum calculations: `majority = (totalNodes / 2) + 1`.
2. Track peer heartbeat freshness within a configurable lease duration window.
3. Issue and renew time-bounded fencing leases (`LeaseFenceRecord`) guarded by unexpired quorum consensus.
4. Transition minority partition nodes immediately to `PartitionStatusMinorityFenced`, actively revoking write permissions and blocking task execution.
5. Uphold the AST hygiene mandate with declarative validation (`strutil.ErrorOnBlank`) and structured concurrency.

## Decision
1. Implement `SwarmPartitionDetector` and associated lease types in `pkg/community/partition_detector.go`.
2. Enforce declarative validation on all identifiers (`NodeID`, `ClusterID`, `PeerID`).
3. Provide comprehensive test suite `pkg/community/partition_detector_test.go` verifying the Three-Fold Proof:
   - Static Floor Invariant: Strict majority calculations for even and odd cluster topologies, default lease durations, zero-value safety.
   - Operational Dynamic Proof: Quorum maintenance across active peer heartbeats, lease issuance, unexpired lease verification, and automatic fence activation upon partition timeout.
   - Negative Adversarial Boundary: Immediate rejection of writes in minority partitions, expired lease revocation, invalid/blank identifier rejections, and invalid cluster sizes.
4. Register the architectural decision into the Knowledge Kernel CAS via `zqk new decision --cas` and `zqk docman register`.

## Consequences
- Prevents split-brain mutations across fragmented agent clusters.
- Enforces strict safety barriers during network partitions.
- Provides predictable quorum state for higher-order consensus and leader election protocols in v0.3.8.
