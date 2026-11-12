# ADR: Raft-Lite Leader Election & Consensus Heartbeat Engine v0.3.8

## Status
Accepted

## Date
2026-11-12

## Context
In multi-agent collaborative topologies, atomic coordination and conflict-free task scheduling require a deterministically designated coordinator or leader node per cluster.
Without clear consensus, multiple nodes may concurrently claim the leader role, dispatching conflicting tasks, duplicating Knowledge Kernel mutations, or corrupting state transitions.

In Cycle 138, we establish a lightweight, Raft-inspired leader election and consensus heartbeat engine (`RaftLeaderEngine`) in `pkg/community/raft_leader.go`.

The Raft-Lite engine must:
1. Model candidate, follower, and leader roles with monotonically increasing terms (`Term uint64`).
2. Require majority vote quorum (`majority = (clusterSize / 2) + 1`) to achieve leadership.
3. Grant time-bounded leader leases secured by periodic heartbeat renewals (`RenewHeartbeat`).
4. Force immediate leader demotion and step-down to follower when a higher term is discovered or when heartbeat lease quorum is lost.
5. Adhere to the declarative blank-check pattern using `strutil.ErrorOnBlank` and zero-duplication AST standards.

## Decision
1. Implement `RaftLeaderEngine` in `pkg/community/raft_leader.go`.
2. Model role states (`RoleFollower`, `RoleCandidate`, `RoleLeader`), vote requests, and heartbeat leases with monotonic terms.
3. Validate inputs using declarative helpers (`strutil.ErrorOnBlank`) avoiding procedural blank check anti-patterns.
4. Construct comprehensive unit test suite in `pkg/community/raft_leader_test.go` covering the Three-Fold Proof:
   - Static Floor Invariant: Rejection of zero terms, blank candidate/cluster IDs, cluster sizes < 1, and zero-value safety.
   - Operational Dynamic Proof: Candidate transitions, majority quorum vote gathering, leader lease issuance, and heartbeat renewal.
   - Negative Adversarial Boundary: Immediate step-down on encountering higher term, expired lease demotion, split-vote recovery, and invalid peer vote rejections.
5. Register this decision in Knowledge Kernel CAS via `zqk new decision --cas` and `zqk docman register`.

## Consequences
- Guarantees exactly-one-leader semantics across ephemeral swarm nodes.
- Eliminates split-brain execution when used alongside the Swarm Partition Detector.
- Preserves codebase hygiene and strict AST lint compliance.
