# Metrics Subsystem & Telemetry Pipeline Operational Architecture Guide (`pkg/metrics`)

## Executive Summary

The `pkg/metrics` subsystem provides a high-throughput, low-latency telemetry collection, in-memory sampling, timeseries persistence, and aggregation engine for the **ZQK Knowledge Kernel**.

In high-concurrency multi-agent orchestration and large-scale automated test suites, unrestricted disk persistence of individual metric events quickly exhausts file descriptors, degrades disk I/O, and causes lock contention across transactional boundaries. Conversely, production operations and observability dashboards demand accurate, near-real-time telemetry across project convergence, resource utilization, and operational lifecycles.

The `pkg/metrics` subsystem resolves this tension by providing:
1. **In-Memory Sampling & Micro-Batching:** High-frequency events (e.g., entity mutations, audit traces, cache operations) are accumulated into bounded in-memory batches and periodically flushed or aggregated into persistent `base_metric` records.
2. **Component Taxonomy:** Standardized abstraction primitives including scalar aggregators, list aggregators, status-history aggregators, and Prometheus histogram wrappers.
3. **Decoupled Concurrency Governance:** Strict concurrency controls utilizing bounded worker pools, lock timeouts (`RunInLockWithLogger`), and non-blocking asynchronous persistence through `goroutinelabels`.
4. **Append-Only Timeseries Engine:** A dedicated binary timeseries engine (`pkg/metrics/timeseries.go` and `pkg/metrics/tsdb`) utilizing zigzag varint delta-compression for time and values.
5. **Seamless Telemetry Integration:** Interoperability with `pkg/telemetry` dispatchers and `pkg/metricpack` composition roots.
6. **Strict Statement Coverage Invariant:** High unit and integration verification floor (>=85.0% statement coverage, zero data races under `go test -race`).

---

## Architecture & Component Taxonomy

```
                   ┌──────────────────────────────────────────────┐
                   │             Event / Mutation Site            │
                   │ (CLI, Pipeline, CAS Cache, Graph Provider)   │
                   └──────────────────────┬───────────────────────┘
                                          │
                                          ▼
                   ┌──────────────────────────────────────────────┐
                   │        MetricPipeline (Sample / Record)       │
                   └──────────────────────┬───────────────────────┘
                                          │
                  ┌───────────────────────┴───────────────────────┐
                  ▼                                               ▼
     ┌────────────────────────┐                      ┌────────────────────────┐
     │  SamplerRegistry       │                      │  Aggregators & Views   │
     │  - Max Samplers: 64    │                      │  - Scalar Aggregator   │
     │  - System Fallback     │                      │  - List Aggregator     │
     │  - Per-Kind Batching   │                      │  - Status History      │
     └────────────┬───────────┘                      └───────────┬────────────┘
                  │                                               │
                  ▼                                               ▼
     ┌────────────────────────┐                      ┌────────────────────────┐
     │  In-Memory Sampler     │                      │  MetricFactory         │
     │  - Bounded Batches     │                      │  - Async Spec Builder  │
     │  - Ticker Flush        │                      │  - CAS / Storage Plane │
     └────────────┬───────────┘                      └───────────┬────────────┘
                  │                                               │
                  └───────────────────────┬───────────────────────┘
                                          │
                                          ▼
                   ┌──────────────────────────────────────────────┐
                   │   Storage Engine & Binary TimeSeries DB      │
                   │   - CAS Object Storage (base_metric)         │
                   │   - Delta-Compressed Timeseries Chunks       │
                   └──────────────────────────────────────────────┘
```

### Component Taxonomy

1. **MetricPipeline (`pipeline.go`):**
   - Coordinates event collection, trait-based metric discovery, and aggregator routing.
   - Manages subscriber callbacks and integrates with the lazy-initialized `TraitRegistry`.
   - Routes high-frequency events through `SamplerRegistry.GetOrCreatePipelineSampleSampler()`, avoiding one ticker goroutine per object kind.

2. **SamplerRegistry (`sampler_registry.go`):**
   - Central registry for configured and on-demand `Sampler` instances.
   - Enforces a distinct sampler ceiling (`maxSamplers = 64`) to prevent unbounded goroutine leaks from background flush tickers.
   - Transparently aliases overflowing system metrics to a shared fallback system sampler.

3. **Sampler & SamplerBatch (`sampler.go`):**
   - Accumulates events per batch key (by object ID, target kind, or global partition).
   - Enforces batch ceilings (`maxSamplerBatchKeys = 1000`) and evicts the oldest batch upon overflow to ensure strictly bounded memory footprint.
   - Periodic background flushes via dedicated, named ticker goroutines (`flushTickerLoop`).

4. **Specialized Aggregators:**
   - **Scalar Aggregator (`scalar_aggregator.go`):** Computes numeric aggregations (`sum`, `avg`, `min`, `max`, `count`) over numeric fields.
   - **List Aggregator (`list_aggregator.go`):** Evaluates frequency distributions, distinct value sets, and element occurrences across array attributes.
   - **Ordered List Aggregator (`ordered_list_aggregator.go`):** Tracks sequence preservation, transition paths, and average sequence lengths.
   - **Status History Aggregator (`status_history_aggregator.go`):** Analyzes state machine transitions, state duration distributions, and identifies frequent transition pathways.

5. **Binary Timeseries Engine (`timeseries.go`, `timeseries_query.go`, `pkg/metrics/tsdb`):**
   - Stores chronological datapoints using varint timestamp deltas and zigzag varint value deltas.
   - Partitions into hourly or daily chunk files (`<series>_<timestamp>.chunk`) within dedicated series directories.
   - Provides time-bucketed rollup aggregations (`Rollup`), chronological range queries (`QueryRangePoints`), and streaming feeds (`AmbientFeed`).

---

## Thread Safety, Concurrency & Atomic Update Invariants

### 1. Named Goroutine Hygiene
All background goroutines in the subsystem are minted using `goroutinelabels.NewGoroutine(...)` or structured runner closures. Anonymous `go func()` invocations are prohibited to maintain strict stack trace visibility during profiling and diagnostics.

### 2. Lock Safety with Deadlock Protection
Registry and batch mutations utilize `concurrency.RunInLockWithLogger` with explicit lock names:
- `LockNameSamplerRegistryRegisterCheck` / `LockNameSamplerRegistryRegisterStore`
- `LockNameSamplerRegistryGetOrCreateCheck` / `LockNameSamplerRegistryGetOrCreateStore`
- `LockNameSamplerGetOrCreateBatch` / `LockNameSamplerBatchAddEvent`

Lock contention is logged to structured loggers when thresholds are exceeded.

### 3. Resource Bounds & Goroutine Cap Invariant
- **Max Samplers (`maxSamplers = 64`):** Prevents goroutine explosion in environments where hundreds of distinct kinds are processed. When capacity is reached, system metrics alias to the shared fallback sampler, while non-system registrations fail closed.
- **Max Batch Keys (`maxSamplerBatchKeys = 1000`):** Prevents memory leaks under high-cardinality entity IDs by evicting the oldest batch when the ceiling is breached.
- **Max Payload Size (`maxValidationMetricsJSONLen = 256 * 1024`):** Caps serialized JSON payloads embedded into `base_metric` instances at 256KB to avoid stream bloat.

### 4. Zero PII & Zero Nanosecond Literal Invariant
Telemetry payloads and generated documentation must never expose Personally Identifiable Information (PII) or internal nanosecond graph identifiers in unredacted surfaces.

---

## Telemetry Export Pipeline & Subsystem Integration

### Integration with `pkg/telemetry`
The metrics subsystem functions as an intake sensor for the wider telemetry pipeline:
1. `pkg/metrics` captures atomic counters, latencies, and operational event batches.
2. `pkg/telemetry` ingests these metrics through unified export collectors, applying cryptographic masking, field redaction, and optional OpenTelemetry / Prometheus bridge formatting.
3. In-memory circular buffers in `pkg/telemetry` decouple high-frequency kernel mutations from external scrape or push intervals.

### Integration with `pkg/metricpack`
`pkg/metricpack` serves as the composition root binding object specs, lifecycle definitions, and ontological traits for all metric entity kinds (including `base_metric`, `scalar_metric_sampler`, `list_metric_sampler`, and `status_history_metric_sampler`).

When `pkg/metrics` instantiates a persistent metric object:
- It leverages `instance_builders.NewForKind(objects.KindBaseMetric, ...)` to guarantee canonical schema adherence.
- It attaches provenance metadata including `first_seen`, `last_seen`, `window_start`, `window_end`, `collection_count`, and `object_count`.
- Storage persistence occurs asynchronously via `PersistBaseMetricInstance` with a bounded timeout (60s), ensuring the caller's execution path is never blocked.

---

## Operational Runbooks, Failure Modes & Edge Case Mitigation

### Runbook 1: High Goroutine Count or Ticker Leakage
**Symptom:** Memory inspection or runtime profiles indicate accumulating goroutines in `flushTickerLoop`.
**Cause:** Unbounded invocation of `NewSampler` or bypassing `SamplerRegistry.StopAll()`.
**Mitigation:**
1. Verify that `SamplerRegistry` is maintained as a singleton or managed lifecycle component via `MetricPipelineForProject`.
2. In test harnesses, register cleanup functions:
   ```go
   reg := NewSamplerRegistry(storageProvider)
   t.Cleanup(func() { _ = reg.StopAll() })
   ```
3. Ensure custom callers do not bypass the registry to spawn independent samplers with unmanaged tickers.

### Runbook 2: Metrics Not Appearing in Test Suites
**Symptom:** Unit tests asserting `base_metric` persistence fail to find created objects.
**Cause:** Default test environment isolation suppresses metric persistence to prevent lock contention and test pollution.
**Mitigation:**
Opt into metrics recording within the test setup:
```go
metricsrecording.EnterAllowRecording()
t.Cleanup(metricsrecording.LeaveAllowRecording)
```
Or execute the test command with the environment variable set:
```bash
ZQK_TEST_METRICS_RECORDING=1 go test ./...
```

### Runbook 3: Timeseries Chunk File Corruption or Invalid Tail
**Symptom:** `readChunk` reports unexpected EOF or zigzag delta overflow.
**Cause:** Sudden process termination during an incomplete buffer flush.
**Mitigation:**
1. The reader gracefully treats unexpected EOF as a clean termination of the readable stream, preserving all valid preceding points.
2. If corrupted chunks occur from external filesystem writes, inspect the series directory (`.zqk/metrics/timeseries/<series>/`) and quarantine files failing timestamp parsing or delta validation.
3. For continuous daemon writes, ensure `TimeSeriesWriter.Close()` is called during shutdown lifecycles to flush the `bufio.Writer` and sync the file handle.

### Runbook 4: Sampler Registry Reached Capacity Warning
**Symptom:** Logs emit warnings stating `sampler registry at capacity (64), cannot register sampler`.
**Cause:** Dynamic registration of distinct object kinds and fields exceeded the 64-sampler threshold.
**Mitigation:**
1. Configure sampler profiles in `.zqk/metrics/sampler_config.yaml` to group multiple object kinds under shared profiles or wildcard registrations.
2. Utilize the shared pipeline sampler (`mp.Sample(event)`) which aggregates through a single unified sampler rather than minting dedicated samplers per kind.
