# ADR-0045: Autonomous Provider Metrics Rollup & Anomaly Quarantine v0.3.6

- **Title: Autonomous Provider Metrics Rollup & Anomaly Quarantine v0.3.6**
- **Status: Accepted**
- **Date**: 2026-11-12
- **Author**: Community Core Team

## Context

As community providers scale across multi-agent swarms, operational telemetry must aggregate invocation volume, latency distributions, and error ratios. When a faulty provider encounters a burst of errors or severe latency degradation, it risks exhausting swarm worker resources or poisoning downstream task pipelines. Autonomous anomaly detection and instant quarantine isolation are essential to shield the kernel.

## Decision

1. **Rolling Telemetry & Metric Rollup**:
   - Introduce `ProviderMetricsCollector` tracking total invocations, successes, error counts, rolling latency moving averages, and percentile buckets.
   - Aggregate metrics per provider ID with thread-safe atomic primitives.

2. **Automated Anomaly Detection & Quarantine**:
   - Compute real-time health scores and error rate thresholds (defaulting to max 50% error rate over a minimum sample window).
   - If anomalous failure thresholds are crossed, transition provider state to `Quarantined`, blocking all non-probe invocations until explicitly remediated or circuit reset.

3. **Ontological Traceability & Quality Gate**:
   - Traceable directly to parent Community Ecosystem Extensibility goals and active execution priority plans.
   - Complies fully with `POL-DOC-001` character budgets and prohibition of internal cycle sprint narratives in public docs.

## Consequences

- Rogue or degraded providers are rapidly isolated before impacting adjacent workloads.
- Operators and swarms gain transparent telemetry on provider reliability and performance.
- Fault domain containment guarantees overall kernel stability.
