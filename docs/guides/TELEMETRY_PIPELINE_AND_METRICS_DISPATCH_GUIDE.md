# Telemetry Pipeline, Metric Export & Observability Subsystem Architectural & Operational Guide

## 1. Architectural Overview

The **Telemetry Pipeline, Metric Export & Observability Subsystem** (`pkg/telemetry`) provides end-to-end telemetry capture, anonymization, metric aggregation, and runtime performance observability across the ZQK Core runtime and multi-agent execution swarms (governed by POL-DEFAULT-142277d3).

In autonomous and distributed agent execution, observability must operate under strict non-negotiable operational invariants:
1. **Privacy-by-Default & Anonymization Invariant:** Zero sensitive data leakage. Community edition telemetry defaults to fail-closed opt-out. When opted in, all payloads undergo strict automated scrubbing of secrets, authentication tokens, email addresses, IP addresses, and user home directory paths prior to retention or export.
2. **Deterministic RSS Memory Cap Invariant:** Unbounded memory growth from telemetry buffering or diagnostic tracking is prohibited. The system enforces an OOM-safe Resident Set Size (RSS) memory cap that drops chunks and alerts callers under memory pressure before kernel OOM-killer intervention.
3. **Objective Kernel Metrics Collection:** Telemetry captures live system and process metrics—real RSS usage, process CPU seconds (user/system), network I/O counters, IPC latencies, and agent token usage—rather than synthetic approximations.
4. **Performance Drift Detection:** Anomaly monitoring detects execution latency or MTTR deviations exceeding baseline tolerances (e.g. >10%), dispatching alerts via the internal event bus.
5. **Distributed W3C Trace Propagation:** End-to-end request tracing is maintained across asynchronous boundary jumps using W3C Trace Context headers.

---

## 2. Component Topology & Architecture

The subsystem consists of seven cooperative primitives:

```mermaid
flowchart TD
    subgraph Ingestion["Telemetry Intake & Instrumentation"]
        Agent["Agent Swarm & Tool Invocation"]
        KernelRuntime["Knowledge Kernel Runtime"]
        Tracer["W3C Span Tracer (pkg/telemetry/tracer.go)"]
    end

    subgraph CorePipeline["pkg/telemetry Core Subsystem"]
        Anon["Payload Anonymizer\n(anonymizer.go)"]
        Collector["DiagnosticMetricsCollector\n(collector.go)"]
        Tracker["DefaultTracker\n(telemetry.go)"]
        Throughput["ThroughputAggregator\n(throughput_aggregator.go)"]
        CapGuard["MemoryCap & OOMSafeStreamer\n(mem_cap.go)"]
        Drift["DriftDetector\n(drift.go)"]
        Daemon["TelemetryDaemon & Compactor\n(daemon.go & compaction.go)"]
    end

    subgraph Sinks["Export & Alerting Sinks"]
        EventBus["Internal Event Bus\n(telemetry.drift)"]
        DiskExport["Secure File Export\n(paths.FilePerm600)"]
        UnifiedGraph["Unified Graph Nodes\n(Kernel CAS & Telemetry Ledger)"]
    end

    Agent --> Anon
    KernelRuntime --> CapGuard
    Tracer --> Tracker

    Anon --> Collector
    CapGuard --> Collector
    Tracker --> Throughput
    Tracker --> Drift

    Drift --> EventBus
    Collector --> DiskExport
    Daemon --> UnifiedGraph
```

### Component Breakdown
- **Payload Anonymizer (`anonymizer.go`):** Strips OAuth bearer tokens, API keys, GitHub tokens, Stripe secrets, JWTs, IPv4 addresses, email addresses, and `/Users/` or `/home/` file paths. Enforces global opt-in/opt-out toggling.
- **Diagnostic Metrics Collector (`collector.go`):** Bounded ring-buffer event accumulator and diagnostic system state snapshot taker. Supports synchronous/asynchronous sanitized exports and file writes using secure file permissions (`paths.FilePerm600`).
- **Memory Cap Guard & OOM Safe Streamer (`mem_cap.go`):** Proactively reads OS resident set size (`gopsutil/process`) at regular intervals. Drops stream items safely and raises pressure flags when RSS crosses configured thresholds (default: 300 MiB).
- **Default Tracker (`telemetry.go`):** High-throughput execution tracker recording tool run durations, exit codes, cache hit/miss ratios, IPC latencies, agent prompt/completion token consumption, and ghost drift MTTR using Prometheus-style histogram bucketing.
- **Throughput Aggregator (`throughput_aggregator.go`):** Sliding-window token throughput aggregator with deterministic clock injection, active worker accounting, and idle worker detection.
- **Drift Detector (`drift.go`):** Statistical variance and regression monitor comparing live performance observations against calibrated baselines. Emits `DriftEvent` instances to the event bus when deviation exceeds 10%.
- **Telemetry Daemon & Compactor (`daemon.go`, `compaction.go`):** Coordinates background aggregation of spans and metrics into unified graph nodes. Executes periodic disk cleanup of stale stream logs (`.json`, `.jsonl`, `.chunk`) based on age thresholds.

---

## 3. Telemetry Collection & Metrics Dispatch Lifecycle

The following sequence diagram illustrates the lifecycle from tool execution intake to anonymization, throughput aggregation, drift evaluation, and background persistence:

```mermaid
sequenceDiagram
    autonumber
    participant Agent as Agent Execution
    participant Tracer as SpanTracer
    participant Anon as Anonymizer
    participant Collector as MetricsCollector
    participant MemCap as MemoryCapGuard
    participant Tracker as TelemetryTracker
    participant Bus as EventBus
    participant Daemon as TelemetryDaemon

    Agent->>Tracer: StartSpan(ctx, "tool_exec")
    Tracer-->>Agent: ctxWithTrace, span
    Agent->>Anon: SanitizePayload(rawOutput)
    Anon-->>Agent: sanitizedOutput
    Agent->>Collector: RecordEvent("tool_complete", sanitizedMetrics)
    Collector->>MemCap: IsExceeded()
    alt Under Memory Budget
        MemCap-->>Collector: false
        Collector->>Collector: Buffer Event (Prune Oldest if > Cap)
    else Over Memory Budget
        MemCap-->>Collector: true
        Collector->>Collector: Drop or Enforce Cap
    end
    Agent->>Tracker: RecordExecution(tool, duration, exitCode)
    Agent->>Tracker: RecordAgentTokenUsage(agent, inTokens, outTokens)
    Tracker->>Tracker: Observe Histograms & Ratios
    Tracker->>Bus: CheckDrift(baseline, current)
    opt Deviation > 10%
        Bus-->>Bus: Publish("telemetry.drift", DriftEvent)
    end
    Agent->>Tracer: span.End()
    Note over Daemon: Periodic Aggregation Cycle
    Daemon->>Collector: Flush()
    Daemon->>Daemon: CompactOldSegments(roots, maxAge)
```

---

## 4. Memory Cap & Buffer Eviction State Machine

The memory management layer enforces hard RSS limits to prevent Out-Of-Memory termination during high-volume agent sessions:

```mermaid
stateDiagram-v2
    [*] --> Healthy: NewMemoryCap(limitBytes, strict)

    state Healthy {
        [*] --> Sampling
        Sampling --> WithinBudget: RSS <= Limit
        WithinBudget --> Sampling: Tick Interval (500ms)
    }

    state WarningPressure {
        [*] --> ApproachingCap: RSS > 90% Limit
        ApproachingCap --> EvictingOldest: Buffer Trimming
    }

    state ExceededHardCap {
        [*] --> CapTripped: RSS > Limit
        CapTripped --> DroppingChunks: OOMSafeStreamer returns nil
        CapTripped --> FlagOOM: ooccur = true
    }

    Healthy --> WarningPressure: Memory Growth
    WarningPressure --> ExceededHardCap: RSS exceeds limit
    ExceededHardCap --> WarningPressure: Garbage Collection / Segment Purge
    WarningPressure --> Healthy: RSS drops below 90%
    ExceededHardCap --> [*]: Close()
```

### State Semantics
- **Healthy:** Current RSS is strictly below the configured limit. Streaming chunks are processed with `OOMGuarded: true`.
- **Warning Pressure:** Current RSS approaches the boundary threshold. In-memory metric buffers prune oldest events via sliding FIFO slices.
- **Exceeded Hard Cap:** Current RSS crosses the configured threshold. `OOMSafeStreamer` safely drops payloads, increments `failed` counters, sets `ooccur = true`, and preserves the host process from OS OOM kill.

---

## 5. Concurrency & Thread-Safety Guarantees

All primitives in `pkg/telemetry` are architected for thread safety under high concurrency:
- **`sync.RWMutex` Protection:** `DiagnosticMetricsCollector`, `ThroughputAggregator`, `InMemoryHook`, `Daemon`, and `DefaultTracker` encapsulate all mutable maps and slices behind granular read/write mutexes.
- **Thread-Safe Snapshotting:** Methods such as `Flush()`, `Snapshot()`, `GetCacheHitRatios()`, `GetSummary()`, and `Workers()` generate isolated deep copies of in-flight data, allowing concurrent reads without blocking writer threads.
- **Named Goroutine Lifecycle Governance:** Background routines (such as the memory cap monitor in `NewMemoryCap` and asynchronous export in `AsyncExport`) are launched exclusively via `goroutinelabels.NewGoroutine`, ensuring strict labeling, panic recovery, and thread attribution.
- **Zero Race Condition Invariant:** Concurrency safety is continuously verified with `-race` enabled test matrices (`TestTelemetry_ConcurrencySafety`, `TestThroughputAggregator_ConcurrentSafety`).

---

## 6. Verification & Three-Fold Proof Breakdown

The telemetry subsystem is rigorously validated under the Three-Fold Proof:

| Dimension | Proof Type | Verification Target & Invariant | Status |
| :--- | :--- | :--- | :--- |
| **Static Floor** | Statement Coverage Floor | Statement coverage strictly $\ge 85.0\%$ across all routines in `pkg/telemetry` (target: 100.0%, achieved: 96.8%). | **VERIFIED** |
| **Operational Dynamic** | Functional Behavior Proof | Live telemetry intake, PII scrub verification, sliding-window throughput accounting, memory cap enforcement, and W3C traceparent formatting. | **VERIFIED** |
| **Negative Boundary** | Adversarial Hardening | Opt-out fail-closed rejections, empty event name validation, stopped collector guards, ring buffer overflow pruning, and invalid traceparent formatting rejections. | **VERIFIED** |

### Execution Commands
```bash
# Targeted test execution with statement coverage
ZQK_ALLOW_FOREGROUND_GO_TEST=1 go test -v -timeout 30s -cover ./pkg/telemetry

# Verification engine suites
./bin/zqk-vet -suite hygiene,tree

# Open-core release payload verification
sh scripts/open-core/check-public-release-payload.sh .
```
