# ADR-0042: Autonomous Provider Health Telemetry & Failure Circuit Breaker Architecture v0.3.6

- **Title: Autonomous Provider Health Telemetry & Failure Circuit Breaker Architecture v0.3.6**
- **Status: Accepted**
- **Date**: 2026-11-12
- **Author**: Community Core Team

## Context

When community extensions encounter upstream latency spikes, network partitions, or repeated fatal crashes, continuing to route execution requests to the failing provider degrades host performance and clogs kernel dispatch queues. A resilient circuit breaker pattern is required to automatically monitor health telemetry, trip to an open failure state upon reaching error thresholds, and safely probe for recovery.

## Decision

1. **Circuit Breaker State Machine**:
   - Every registered provider maintains an isolated `ProviderCircuitBreaker` with three canonical states: `Closed` (normal operation), `Open` (tripped, fail-fast rejection), and `HalfOpen` (limited canary probing).
   - Configurable thresholds: `ConsecutiveFailuresThreshold` (default 5), `OpenDuration` (default 30s), and `ProbeSuccessThreshold` (default 2).

2. **Health Telemetry & Metrics Aggregation**:
   - Track total invocations, successful executions, error counts, consecutive failures, and last failure timestamps atomically.
   - Failures trip the circuit open immediately when thresholds are breached, returning `ErrCircuitOpen` without invoking failing external code.

3. **Ontological Traceability & Quality Gate**:
   - Traceable directly to parent Community Ecosystem Extensibility goals and active execution priority plans.
   - Strict adherence to `POL-DOC-001` character budgets and prohibition of internal sprint references in public guides.

## Consequences

- Flaky or degraded community extensions fail-fast without cascading failures across the core kernel.
- Automatic recovery probing self-heals transient provider outages without requiring operator intervention.
- The open-core extension architecture provides carrier-grade operational resilience.
