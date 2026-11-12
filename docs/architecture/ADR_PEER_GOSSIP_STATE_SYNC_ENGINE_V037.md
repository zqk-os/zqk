# ADR: Multi-Cluster Peer Gossip & State Synchronization Engine v0.3.7

## Status
Accepted

## Date
2026-11-12

## Context
As the ZQK Knowledge Kernel scales across federated swarms, edge clusters, and decoupled agent processes, a central coordinating bus introduces single points of failure, network bottlenecks, and high latency.
To maintain high availability and eventual consistency, swarm nodes must exchange cluster topology, active worker health, and lease state through a lightweight, peer-to-peer gossip protocol.

In Cycle 136, we introduce a deterministic Multi-Cluster Peer Gossip & State Synchronization Engine (`PeerGossipSyncEngine`) within `pkg/community/peer_gossip.go`.

The gossip engine must:
1. Maintain a dynamic membership table of active cluster peers with monotonic sequence numbers (epochs).
2. Propagate state digests and membership events through anti-entropy epidemic dissemination.
3. Resolve split-brain or divergent version conflicts deterministically using epoch ordering and highest sequence precedence.
4. Enforce declarative input validation (`PeerID`, `ClusterID`, `Digest`) using `strutil.ErrorOnBlank`.
5. Maintain structured concurrency and goroutine safety via `goroutinelabels`.

## Decision
1. Implement `PeerGossipSyncEngine` and peer state data structures in `pkg/community/peer_gossip.go`.
2. Provide declarative validation using `strutil.ErrorOnBlank` for all identifiers and payloads.
3. Provide comprehensive test suite `pkg/community/peer_gossip_test.go` verifying the Three-Fold Proof:
   - Static Floor Invariant: Peer configuration defaults, membership initialization, zero-value safety.
   - Operational Dynamic Proof: Multi-node peer discovery, message dissemination across gossip rounds, convergence to unified state digest.
   - Negative Adversarial Boundary: Blank node/cluster IDs, partitioned node isolation, duplicate message deduplication, and stale epoch rejection.
4. Register the architectural decision into the Knowledge Kernel CAS via `zqk new decision --cas` and `zqk docman register`.

## Consequences
- Eliminates centralized coordinator bottlenecks for multi-cluster state distribution.
- Enables resilient, self-healing peer topologies that seamlessly handle node churn and network partitions.
- Maintains strict adherence to ZQK Knowledge Kernel architectural mandates and declarative code hygiene.
