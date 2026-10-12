# Distributed Event Mesh Telemetry Aggregation and Adaptive Backpressure Throttling

## Overview

The Distributed Event Mesh Telemetry and Backpressure subsystem (`cmd/zqk/callback/mesh_telemetry_backpressure.go`) provides unified, multi-dimensional execution observability and adaptive flow control for asynchronous event subscribers across the Knowledge Kernel. It combines bounded sliding-window time-series bucketing with credit-based token bucket rate limiting and AIMD (Additive Increase / Multiplicative Decrease) dynamic congestion control to protect against queue starvation, memory exhaustion, and distributed event storms.

## Architecture

```
+-----------------------------------------------------------------------------------+
| Distributed Event Mesh Telemetry & Backpressure Flow                              |
|                                                                                   |
|  +--------------------+         Acquire()        +-----------------------------+  |
|  | Event Dispatcher   | -----------------------> | MeshBackpressureThrottler   |  |
|  +--------------------+                          | (Credit Budgeting & AIMD)   |  |
|            |                                     +--------------+--------------+  |
|            | Notify()                                           |                 |
|            v                                                    v                 |
|  +--------------------+    Dimensional Snapshots +-----------------------------+  |
|  | MeshTelemetryAgg   | <----------------------- | Subscriber Worker Pools     |  |
|  | (SlidingWindowBuf) |                          | (Policy: Reject|Wait|Drop)  |  |
|  +--------------------+                          +-----------------------------+  |
+-----------------------------------------------------------------------------------+
```

## Core Components

### 1. `SlidingWindowBuffer` & Time-Series Bucketing
Aggregates high-throughput telemetry metrics across sliding sub-buckets:
- **Discretized Sub-Buckets**: Configurable window durations (default 10s) and bucket counts (default 10) providing sub-second temporal resolution.
- **Latency Percentile Quantization**: Maintains discretized latency histograms enabling accurate estimation of P50, P95, and P99 latency percentiles without unbounded memory allocation.
- **Throughput Rate Normalization**: Computes sliding-window throughput (`TotalDispatched / WindowDuration`) and drop rates (`DropRate() = TotalDropped / TotalDispatched`) dynamically.

### 2. `MeshTelemetryAggregator`
Subscriber-aware dimensional telemetry store:
- **Multi-Dimensional Tagging**: Slices metrics by topic, broker, subscriber pool, and execution status (`success`, `dropped`, `error`).
- **CallbackSubscriber Integration**: Implements the canonical `CallbackSubscriber` interface (`Notify()`) to observe wire events without adding dispatch latency.
- **Credit Debt Telemetry**: Exposes system-wide credit debt and queue buffer utilization ratios for ambient health monitoring.

### 3. `MeshBackpressureThrottler`
Credit-based adaptive flow control engine:
- **Credit Budgets**: Individual subscriber pools maintain independent initial, current, and maximum credit ceilings.
- **Backpressure Policies**:
  - `PolicyReject`: Returns `ErrCreditExhausted` immediately when credits drop to zero.
  - `PolicyWait`: Blocks caller until credits are replenished or context expires.
  - `PolicyDropOldest`: Drops the oldest enqueued event to admit high-priority new arrivals.
- **Replenishment & Acknowledgment**: Callers replenish credits via `Replenish()` or notify completed dispatches via `Acknowledge(credits, latency)`.

### 4. `AdaptiveRateLimiter` (AIMD Congestion Control)
Dynamic rate regulation adapting to downstream subscriber health:
- **Additive Increase**: Gradually expands rate limits by a constant step upon consecutive successful acknowledgments below latency thresholds.
- **Multiplicative Decrease**: Swiftly halves rate limits upon credit exhaustion, dropped events, or SLA latency violations.
- **Ceiling and Floor Bounds**: Clamps rates strictly between configurable minimum and maximum bandwidth limits.

## Boundary & Error Handling Specifications

1. **Invalid Configuration Guards**: Rejects zero or negative window durations, bucket counts, or credit limits fail-closed (`ErrInvalidConfiguration`, `ErrInvalidCreditAmount`).
2. **Missing Pool Protection**: Operations targeting unregistered subscriber pools return `ErrSubscriberNotFound` without panicking.
3. **Graceful Shutdown Invariant**: Closing the throttler or controller unblocks waiting callers immediately with `ErrThrottlerClosed`.
4. **Saturation & Clamping**: Credit replenishment cannot exceed `MaxCredits`, preventing unbounded buffer inflation.
