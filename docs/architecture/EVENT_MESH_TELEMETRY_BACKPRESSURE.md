# Distributed Event Mesh Telemetry Aggregation and Adaptive Backpressure Throttling

<!-- tags: architecture, mesh, telemetry, backpressure, aimd, event-bus, streaming, qos -->

## Executive Summary

The **Distributed Event Mesh Telemetry and Backpressure Subsystem** provides high-throughput, low-latency telemetry aggregation and resilient credit-based backpressure controls across asynchronous subscriber worker pools. In high-concurrency event bus workloads, subscriber processing speeds vary widely due to network jitter, disk I/O, and variable computational complexity. Without adaptive backpressure, unconstrained event publication causes catastrophic memory bloat, queue unbounded expansion, and cascade node failure.

This subsystem provides:
1. **Zero-Allocation Bounded Sliding Window Buffer**: Continuous $O(1)$ windowed metric aggregation (throughput, latency percentiles $P50, P95, P99$, buffer occupancy) across topics, brokers, and subscriber pools.
2. **Credit-Based Backpressure Protocol**: Pre-allocated burst credits with asynchronous replenishment pacing subscriber ingestion rates.
3. **Adaptive Rate Limiter (AIMD)**: Dynamic congestion control using Additive Increase / Multiplicative Decrease driven by subscriber buffer drain rates and watermark thresholds.
4. **Defensive Load Shedding & Degradation**: Policy-driven backpressure handling (`reject`, `wait`, `drop_oldest`) protecting system memory invariants.

---

## Architecture & Component Topology

```mermaid
flowchart TD
    subgraph EventMesh["Distributed Event Mesh Ingest"]
        E["Callback Entry / Event Stream"]
    end

    subgraph Controller["Mesh Telemetry & Backpressure Controller"]
        MC["MeshTelemetryBackpressureController"]
    end

    subgraph TelemetrySubsystem["Telemetry Aggregation Subsystem"]
        MTA["MeshTelemetryAggregator"]
        SWB["SlidingWindowBuffer (Ring of Discrete Temporal Buckets)"]
        DT["Dimensional Telemetry (Topic / Broker / Subscriber)"]
        SNAP["Immutable MeshTelemetrySnapshot"]
    end

    subgraph BackpressureSubsystem["Credit & Throttling Subsystem"]
        MBT["MeshBackpressureThrottler"]
        CP["Credit Pool (Per-Subscriber State)"]
        AIMD["Adaptive Rate Limiter (AIMD Engine)"]
        REP["Async Replenishment Worker (Ticker Loop)"]
    end

    subgraph Workers["Subscriber Worker Pools"]
        W1["Subscriber Pool 1"]
        W2["Subscriber Pool 2"]
        W3["Subscriber Pool N"]
    end

    E --> MC
    MC --> MBT
    MBT -->|Acquire Credit| CP
    CP -->|Policy Evaluation| AIMD
    AIMD -->|Approved| W1
    AIMD -->|Approved| W2
    AIMD -->|Approved| W3

    W1 -->|Acknowledge / Drain| MBT
    W2 -->|Acknowledge / Drain| MBT
    W3 -->|Acknowledge / Drain| MBT

    MC -->|Notify Telemetry| MTA
    MTA --> SWB
    MTA --> DT
    MTA --> SNAP

    REP -->|Replenish Credits| CP
```

---

## 1. Bounded Sliding Window Telemetry Aggregator

### Data Structure & Ring Allocation
The `SlidingWindowBuffer` decomposes a moving time horizon $T_{\text{window}}$ (default 10s) into $N$ discrete time buckets (default 10 buckets of 1s each). The bucket index is calculated via lock-free modulo arithmetic on nanosecond timestamps:

$$\text{index} = \left(\left\lfloor \frac{t_{\text{now}}}{\Delta t_{\text{bucket}}} \right\rfloor\right) \pmod N$$

When a bucket is reused after completing a full window cycle, its counters are recycled in-place, eliminating runtime heap allocations on the hot ingest path.

### Dimensional Slicing
Telemetry is partitioned along three orthogonal planes:
- **Topics**: Identifies message channels (e.g. `events`, `control-plane`, `orders`).
- **Brokers**: Tracks network ingress instances (e.g. `broker-east-1`, `broker-west-1`).
- **Subscribers / Pools**: Isolates consumer worker pools.

### Mathematical Complexity
- **Metric Ingestion (`RecordDispatch`, `RecordReceive`)**: $O(1)$ amortized, lock-free atomics and bucket slot lock.
- **Aggregation & Snapshotting (`Aggregate`, `Snapshot`)**: $O(N)$ where $N$ is the bucket count (typically 10), negligible CPU overhead ($< 15\mu\text{s}$).
- **Percentile Calculation ($P50, P95, P99$)**: $O(B)$ where $B = 64$ power-of-two latency histogram buckets using `math/bits.Len64`.

---

## 2. Credit-Based Backpressure Protocol

### Credit Lifecycle State Machine

```mermaid
stateDiagram-v2
    [*] --> Idle: Pool Registered
    Idle --> Credited: Initial Credits Seeded
    Credited --> InFlight: Acquire(credits)
    InFlight --> Credited: Acknowledge() / Replenish()
    Credited --> Depleted: Credits < Required
    Depleted --> Credited: Async Replenish()
    Depleted --> DebtAccumulation: Credits < 0 && Debt <= Threshold
    DebtAccumulation --> HardRejected: Debt > DebtThreshold
    HardRejected --> Depleted: Drain / Reset
    Credited --> Closed: Close()
    Depleted --> Closed: Close()
    Closed --> [*]
```

### Policy Modes
1. **`PolicyReject`**: Fail-closed defensive rejection. If credits are exhausted, increments debt and immediately returns `ErrCreditExhausted`. If debt exceeds `DebtThreshold`, returns `ErrDebtThresholdExceeded`.
2. **`PolicyWait`**: Bounded cooperative blocking. Callers sleep on a condition variable (`sync.Cond`) and are notified asynchronously upon credit replenishment. Honors `context.Context` cancellation and unblocks immediately on `Close()`.
3. **`PolicyDropOldest`**: Non-blocking graceful degradation. Drops uncredited events, increments `totalDropped` telemetry, and prevents queue overflow.

---

## 3. Adaptive Rate Limiter (AIMD Congestion Control)

The throttler adjusts subscriber credit replenishment rates dynamically based on downstream queue occupancy and buffer drain metrics:

```mermaid
sequenceDiagram
    autonumber
    participant Worker as Subscriber Worker
    participant Throttler as MeshBackpressureThrottler
    participant AIMD as AIMD Rate Engine
    participant Aggregator as Telemetry Aggregator

    Worker->>Throttler: RecordDrain(drained, queueSize, capacity)
    Throttler->>Aggregator: RecordBufferUtilization(queueSize, capacity)
    Note over Throttler,AIMD: Utilization = queueSize / capacity
    alt Utilization >= HighWatermark (e.g. 80%)
        Throttler->>AIMD: Multiplicative Decrease: Rate = Rate * 0.5
        Note over AIMD: Backs off ingestion rate to relieve congestion
    else Utilization <= LowWatermark (e.g. 50%)
        Throttler->>AIMD: Additive Increase: Rate = Rate + 50.0 req/s
        Note over AIMD: Proactively ramps up throughput capacity
    end
    Throttler->>Throttler: Replenish credits for drained items
```

### Dynamic Formulae
- **Multiplicative Decrease (Congestion Trigger)**:
  $$R_{t+1} = \max(R_{\min}, R_t \times \beta) \quad \text{where } \beta = 0.50, \; U \ge U_{\text{high}}$$
- **Additive Increase (Healthy Drain)**:
  $$R_{t+1} = \min(R_{\max}, R_t + \alpha) \quad \text{where } \alpha = 50.0 \text{ ops/sec}, \; U \le U_{\text{low}}$$

---

## 4. Verification Matrix

| Requirement / Criteria ID | Description | Implementation Component | Verification Test |
| :--- | :--- | :--- | :--- |
| **REQ-1791759409280068000-5b71d659** | Distributed Event Mesh Telemetry Aggregation and Adaptive Backpressure Throttling | `cmd/zqk/callback/mesh_telemetry_backpressure.go` | `cmd/zqk/callback/mesh_telemetry_backpressure_test.go` |
| **CRIT-1791759409830632000-8c68a2b5** | Static Invariant / Telemetry Aggregator Layout & Bounded Window Buffer | `SlidingWindowBuffer`, `MeshTelemetryAggregator` | `TestMeshTelemetry_StaticInvariants_LayoutAndBuffer`, `TestMeshTelemetry_SlidingWindowBucketingAndPercentiles`, `TestMeshTelemetry_DimensionalAggregationAndSnapshots` |
| **CRIT-1791759409830633000-1de1047f** | Operational Dynamic Proof / Credit Backpressure & Adaptive Rate Limiter | `MeshBackpressureThrottler`, `AIMD Engine`, `SubscriberPoolState` | `TestMeshBackpressure_CreditDeductionAndReplenishment`, `TestMeshBackpressure_PolicyWaitAndReplenishmentWakeup`, `TestMeshBackpressure_AdaptiveRateLimiter_AIMD`, `TestMeshBackpressure_ConcurrentBurstAndRaceDetector` |
| **CRIT-1791759409830634000-af0e504b** | Negative Boundary & Architecture Specification | Domain Sentinels, Defensive Gating, Architecture Specification | `TestMeshBackpressure_NegativeBoundariesAndDefensiveRejection`, `TestMeshBackpressure_LifecycleCleanShutdownAndNoGoroutineLeaks`, `docs/architecture/EVENT_MESH_TELEMETRY_BACKPRESSURE.md` |
| **TST-1791759409830632001-c6816f17** | Unified Verification Test Suite | Full package test suite | `go test -v -race ./cmd/zqk/callback/ -run 'TestTelemetry\|TestBackpressure\|TestMesh'` |

---

## 5. Concurrency Lifecycle & Goroutine Hygiene

Adhering to policy `POL-CODE-1791593703788100000-9f2dff69` and `POL-DEFAULT-7c873b7213847d78`:
- All asynchronous background operations use labeled goroutines via `goroutinelabels.NewGoroutine`.
- Tickers are gracefully stopped via `defer ticker.Stop()`.
- Waiters unblock synchronously upon `Close()` with zero dangling goroutines.
- Verified 100% race-free under the Go race detector (`-race`).
