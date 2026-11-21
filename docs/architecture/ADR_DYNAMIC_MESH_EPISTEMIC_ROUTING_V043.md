# ADR-0152: Autonomous Swarm Dynamic Mesh Topologies & Epistemic P2P Routing v0.4.3

## Status
Accepted (Milestone v0.4.3, Cycle 152)

## Date
2026-11-20

## Context
As autonomous agent swarms scale across heterogeneous hosts, regions, and epistemic domains, static peer-to-peer (P2P) overlays degrade under link congestion, asymmetric bandwidth, and partial network partitions. Naive hop-count routing ignores both physical transport quality (latency and throughput) and epistemic proximity between collaborating nodes, while unbounded neighbor tables risk memory and gossip amplification.

In Cycle 152, we introduce an adaptive P2P overlay and epistemic router (`DynamicMesh`) in `pkg/community/dynamic_mesh.go` to dynamically reconfigure swarm mesh topologies, compute composite shortest-path routes, and isolate partition blackholes with observable fallback telemetry.

## Decision

1. **P2P Graph & Neighbor Limit Floor Invariants (`pkg/community/dynamic_mesh.go`)**:
   - Maintain a thread-safe (`sync.RWMutex`) directed adjacency graph (`adj`) of `MeshNode` vertices and `LinkMetrics` edges.
   - `NewDynamicMesh(MeshConfig)` enforces `MaxNeighbors > 0` (`ErrInvalidMaxNeighbors`) and finite, non-negative routing weights (`ErrInvalidLinkWeight`), defaulting to `LatencyWeight = 1.0`, `BandwidthWeight = 100.0`, and `EpistemicWeight = 10.0` when all three weights are zero.
   - `RegisterNode` and `RemoveNode` validate node identifiers declaratively via `strutil.ErrorOnBlank` (`ErrInvalidNodeID`), reject duplicate registrations (`ErrNodeAlreadyExists`), and enforce positive per-node `MaxNeighbors`.
   - `UpsertLink` enforces per-node fan-out bounds (`ErrMaxNeighborsExceeded`) when adding new edges while allowing in-place metric updates, and rejects self-routes (`ErrSelfRoute`), unregistered endpoints (`ErrNodeNotFound`), or invalid link metrics (`ErrInvalidLinkWeight`, requiring finite `LatencyMs > 0`, finite `BandwidthMbps > 0`, and finite `EpistemicProximity >= 0`).

2. **Composite Epistemic + Latency + Bandwidth Shortest-Path Routing**:
   - `ComputeRoute(src, dst)` runs a min-heap Dijkstra search (`dijkstraLocked`) minimizing composite edge cost:
     `cost = (LatencyWeight * LatencyMs) + (BandwidthWeight / BandwidthMbps) + (EpistemicWeight / (1.0 + EpistemicProximity))`.
   - Produces a `MeshRoute` containing ordered `Hops`, `TotalCost`, `TotalLatencyMs`, `MinBandwidthMbps`, and `FallbackUsed`.

3. **Dynamic Topology Adaptation & Distance Matrix**:
   - Every topology mutation (`RegisterNode`, `RemoveNode`, `UpsertLink`, `RemoveLink`, `MarkLinkBlackholed`) invokes `recomputeMatrixLocked()` to refresh the all-pairs shortest-path `distanceMatrix` (recording `+Inf` for unreachable pairs).
   - `Snapshot()` and `DistanceMatrix()` return deep-copied, race-free views (`TopologySnapshot`) with deterministically sorted node lists and live `MeshTelemetry`.

4. **Partition Blackhole Detection & Fallback Telemetry**:
   - `isLinkBlocked` classifies a link as blocked when `Blackholed == true` or when `BlackholeLatencyThresholdMs > 0` and `LatencyMs >= BlackholeLatencyThresholdMs`.
   - `MarkLinkBlackholed` toggles explicit blackhole state on existing links and triggers matrix recomputation.
   - When `ComputeRoute` succeeds by routing around one or more blocked links, `MeshRoute.FallbackUsed` is set to `true` and `MeshTelemetry.FallbackReroutes` is incremented. When no unblocked path exists and a blocked link was encountered, `ComputeRoute` increments `RouteFailures` and `BlackholesDetected` and returns `ErrPartitionBlackhole` (or `ErrDestinationUnreachable` when no blackhole was involved).

## Consequences
- **Positive**:
  - Swarms automatically favor low-latency, high-bandwidth, epistemically aligned paths while honoring strict per-node neighbor caps.
  - Partition blackholes are isolated deterministically with fallback rerouting and structured telemetry counters (`RouteComputations`, `RouteFailures`, `FallbackReroutes`, `BlackholesDetected`).
- **Negative**:
  - Eager all-pairs distance matrix recomputation on topology mutations incurs `O(V * (V + E) log V)` work per mutation, trading write-path CPU for constant-time `DistanceMatrix` and `Snapshot` reads.
