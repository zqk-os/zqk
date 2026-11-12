# ADR: Swarm Fleet Heartbeat Telemetry Aggregator & Node Health State Ingestion v0.3.7

## Status
Accepted

## Date
2026-11-12

## Context
As the ZQK knowledge kernel orchestrates increasingly distributed autonomous swarms (across multiple agent processes, CI workers, and edge nodes), the system requires real-time observability over swarm fleet health.
In Cycle 134, we establish a deterministic, thread-safe Swarm Fleet Heartbeat Aggregator (`SwarmFleetAggregator`) within `pkg/community/fleet_heartbeat.go`.

The aggregator must:
1. Ingest heartbeat pings containing agent identity, role, timestamp, capacity, and current workload.
2. Enforce declarative input validation using `strutil.ErrorOnBlank` or `stringutil.ErrorOnBlank` (rejecting imperative `strings.TrimSpace` boilerplate in accordance with POL-WORKFLOW and `zqk-vet`).
3. Support configurable stale timeout thresholds to reliably identify dead, paused, or partitioned nodes.
4. Calculate fleet-level aggregate health statistics (total nodes, healthy count, stale count, aggregate utilization ratio, status classification).
5. Maintain strict concurrency hygiene and zero allocations on steady-state heartbeat reads.

## Decision
1. Implement `FleetHeartbeat` and `SwarmFleetAggregator` in `pkg/community/fleet_heartbeat.go`.
2. Validate required string fields (`NodeID`, `Role`) using `strutil.ErrorOnBlank`.
3. Provide comprehensive test suite `pkg/community/fleet_heartbeat_test.go` verifying:
   - Static Floor Invariant (zero-value defaults, valid config thresholds)
   - Operational Dynamic Proof (concurrent heartbeats, aggregation, prune/stale transitions)
   - Negative Adversarial Boundary (blank IDs, zero/negative timeouts, stale lookups)
4. Register the architectural decision into the Knowledge Kernel CAS via `zqk new decision --cas` and `zqk docman register`.

## Consequences
- Enables upstream schedulers and swarms to query live cluster health before dispatching heavy token workloads.
- Eliminates silent node drops and enforces clean declarative error handling throughout the fleet subsystem.
