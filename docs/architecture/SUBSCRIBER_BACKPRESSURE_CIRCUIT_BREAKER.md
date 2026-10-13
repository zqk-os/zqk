# Subscriber Backpressure, Adaptive Concurrency Throttling, and Circuit Breaking

## Executive Summary
In distributed event-driven systems and asynchronous swarm architectures, slow, blocked, or failing subscribers can easily destabilize the entire callback dispatcher. Without proactive backpressure buffering, concurrency throttling, and circuit-breaking isolation, a single misbehaving subscriber blocks dispatch loops, triggers unbounded memory consumption, and causes cascading failure cascades across the entire cluster.

This specification details the architecture, design, and mathematical mechanics of:
1. `ResilientSubscriber`: A resilient decorator wrapping any `Subscriber` with integrated backpressure queuing, concurrency limits, and circuit breaker trip guards (`cmd/zqk/callback/resilient_subscriber.go`).
2. `AdaptiveRingBufferQueue`: A thread-safe, bounded circular buffer supporting deterministic drop policies (`DropOldest` vs `DropNewest`).
3. `CircuitBreaker`: A three-state trip-and-reset circuit breaker (`Closed`, `Open`, `HalfOpen`) with probe recovery and adaptive cool-off intervals.

---

## Architectural Topology

```mermaid
flowchart TD
    subgraph CallbackBus ["Knowledge Kernel Callback Bus"]
        IncomingEvents["Callback Entries / Swarm Events"] --> ResilientSub["ResilientSubscriber (Decorator)"]
    end

    subgraph BackpressureLayer ["Adaptive Backpressure & Queueing"]
        ResilientSub --> CBCheck{"Circuit Breaker State"}
        CBCheck -->|Open (Shed Load)| FailFast["ErrCircuitBreakerOpen (Fast Reject)"]
        CBCheck -->|Closed / Half-Open| Queue["Bounded Ring Buffer Queue"]
        Queue -->|Capacity Reached| DropPolicy{"Drop Strategy"}
        DropPolicy -->|DropOldest| EvictOld["Evict Head & Enqueue Tail"]
        DropPolicy -->|DropNewest| EvictNew["Reject Incoming Entry (ErrQueueFull)"]
    end

    subgraph ConcurrencyLayer ["Concurrency Throttler"]
        EvictOld --> Semaphore["Worker Semaphore (Max Concurrency)"]
        Queue -->|Within Watermarks| Semaphore
        Semaphore --> Worker["Execute Inner Subscriber.Notify()"]
    end

    subgraph CircuitBreakerStateMachine ["Circuit Breaker Trip & Recovery"]
        Worker -->|Success| RecordSuccess["Record Success & Reset Failure Counter"]
        RecordSuccess --> TransitionClosed["State -> Closed"]
        Worker -->|Failure / Timeout| RecordFail["Increment Consecutive Failures"]
        RecordFail --> Threshold{"Failures >= Threshold?"}
        Threshold -->|Yes| TripOpen["State -> Open (Trip Breaker)"]
        TripOpen --> Timer["Cool-Off Interval (Sleep)"]
        Timer --> HalfOpen["State -> Half-Open (Canary Probe)"]
        HalfOpen --> Worker
    end
```

---

## 1. Bounded Backpressure & Drop Strategies

The `ResilientSubscriber` isolates the callback dispatcher from slow consumer processing speeds using a bounded in-memory buffer:

### Drop Strategies
1. **`DropOldest`**:
   - When queue capacity is reached, the oldest unconsumed element at the buffer head is discarded.
   - Ideal for real-time telemetry, terminal state streaming, and sensor feeds where freshness takes precedence over historical replay.
2. **`DropNewest`**:
   - Rejects the incoming element immediately, returning `ErrQueueFull`.
   - Protects existing in-flight work batches from being overwritten when strict processing order is required.

---

## 2. Concurrency Throttling & Watermark Limits

To prevent memory leaks and thread starvation under sudden event spikes:

1. **Semaphore-Bounded Concurrency**:
   - Limits simultaneous executions of the inner subscriber to `MaxConcurrency`.
   - Acquires a token before calling `Notify(ctx, entry)` and releases it upon return, preventing runaway goroutine proliferation.
2. **Execution Timeout**:
   - Enforces a per-invocation timeout deadline (`Timeout`).
   - If the subscriber does not complete within the deadline, execution is canceled with `context.DeadlineExceeded` and recorded as a failure.

---

## 3. Trip-and-Reset Circuit Breaking

The circuit breaker protects upstream systems when a subscriber is experiencing persistent outages:

### State Transitions
1. **`Closed` (Normal Operation)**:
   - All events are dispatched to the inner subscriber.
   - Failures increment the consecutive failure counter.
   - If consecutive failures reach `FailureThreshold` (default: 5), the breaker trips to `Open`.
2. **`Open` (Fault Isolation)**:
   - All incoming events are immediately rejected with `ErrCircuitBreakerOpen` without invoking the subscriber or enqueueing.
   - Starts a cool-off timer (`RecoveryTimeout`, default: 5s).
3. **`Half-Open` (Canary Recovery)**:
   - When the cool-off timer elapses, a single probe event is allowed through to test subscriber health.
   - If the canary succeeds, the breaker resets to `Closed`.
   - If the canary fails, the breaker trips back to `Open` for another recovery period.
