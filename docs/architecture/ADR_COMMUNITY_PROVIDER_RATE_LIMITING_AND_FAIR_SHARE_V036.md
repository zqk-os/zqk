# ADR-0047: Community Provider Rate Limiting & Fair-Share Scheduling v0.3.6

- **Title: Community Provider Rate Limiting & Fair-Share Scheduling v0.3.6**
- **Status: Accepted**
- **Date**: 2026-11-12
- **Author**: Community Core Team

## Context

When multiple concurrent agent swarm workers execute tasks utilizing community provider adapters, unconstrained burst requests can trigger provider API rate limits or starve other providers of shared CPU and memory resources. To guarantee equitable resource distribution and prevent starvation, a centralized token-bucket rate limiter and fair-share execution queue are required.

## Decision

1. **Token-Bucket Burst Regulation**:
   - Introduce `ProviderRateLimiter` implementing standard token bucket replenishment with configurable burst depth and replenishment rates per provider.
   - Enforce non-blocking try-acquire and context-aware waiting semantics for incoming invocations.

2. **Fair-Share Weighted Execution Dispatcher**:
   - Provide `ProviderFairShareDispatcher` distributing execution slots across active providers using round-robin and weighted priority queues.
   - Prevent any single provider from monopolizing worker goroutine execution bandwidth.

3. **Ontological Traceability & Quality Gate**:
   - Traceable directly to parent Community Ecosystem Extensibility goals and active execution priority plans.
   - Complies fully with `POL-DOC-001` character budgets and prohibition of internal cycle sprint narratives in public docs.

## Consequences

- Swarm executions respect upstream provider rate limits gracefully without dropped tasks.
- Low-volume providers are guaranteed fair execution latency alongside high-throughput providers.
- Reinforces kernel stability and multi-agent swarm reliability.
