# ADR: Distributed Ephemeral Lock Leasing & Swarm Barrier Engine v0.3.8

## Status
Accepted

## Date
2026-11-12

## Context
In concurrent multi-agent swarm environments, multiple agents may compete for mutually exclusive resources such as integration branch checkouts, Knowledge Kernel CAS mutation tokens, or release tag publications.
Without atomic, time-bounded distributed lock leasing, concurrent workers risk race conditions, conflicting file writes, or orphaned locks if an agent crashes midway through execution.

In Cycle 139, we introduce a lightweight, in-memory and cluster-coordinated Ephemeral Lock Leasing & Barrier Engine (`EphemeralLockManager`) in `pkg/community/lock_lease.go`.

The lock lease manager must:
1. Guarantee mutual exclusion: exactly one client holds an active lock on a resource key at any instant.
2. Grant monotonic fencing tokens (`FencingToken uint64`) with every acquisition to detect and drop out-of-order write attempts.
3. Enforce time-bounded lease TTLs with heartbeat keepalive extensions (`ExtendLease`).
4. Automatically reclaim expired locks held by failed or partitioned agents without deadlocking subsequent acquirers.
5. Adhere strictly to declarative input validation using `strutil.ErrorOnBlank` and zero-duplication AST principles.

## Decision
1. Implement `EphemeralLockManager` in `pkg/community/lock_lease.go`.
2. Provide declarative error validation rejecting blank resource keys, blank client IDs, and non-positive TTLs.
3. Provide robust concurrent lock acquisition, lease extension, and release mechanisms guarded by reader-writer locks.
4. Construct comprehensive unit test suite in `pkg/community/lock_lease_test.go` covering the Three-Fold Proof:
   - Static Floor Invariant: Rejection of blank resource keys, blank client IDs, non-positive TTLs, and zero-value safety.
   - Operational Dynamic Proof: Exclusive lease granting, monotonic fencing token progression, heartbeat keepalive renewal, and explicit release.
   - Negative Adversarial Boundary: Immediate rejection of concurrent acquisitions while lease is unexpired, safe auto-reclaim of expired leases by competing nodes, and unauthorized release/extension protection.
5. Register this decision into Knowledge Kernel CAS via `zqk new decision --cas` and `zqk docman register`.

## Consequences
- Prevents resource contention and race conditions among concurrent swarm workers.
- Guarantees deadlock freedom via automatic expiration of orphaned agent locks.
- Strengthens overall cluster reliability and consensus integrity in v0.3.8.
