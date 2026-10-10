# Architecture Specification: Shockwave Subscriber and Invalidation Bus Event Cascading

## 1. Executive Summary & Problem Statement

In distributed multi-agent systems and asynchronous execution kernels, state mutations (e.g. task completions, requirement transitions, criteria satisfactions, and scheduler job outcomes) occur continuously. Historically, dependent agents and query subsystems relied on polling loops or timed refresh intervals to detect state changes. Polling introduces two critical failure modes:
1. **Agent Stalls & Resource Depletion**: Frequent disk and memory scans consume excessive CPU cycles and I/O bandwidth, causing latency spikes.
2. **Context Latency & Clock Drift**: Changes are only recognized after interval expiration, stalling high-velocity autonomous agent loops.

The Knowledge Kernel addresses this by introducing the **`ShockwaveSubscriber`**, a reactive callback component that bridges asynchronous scheduler callbacks directly to the kernel's **`InvalidationShockwaveBus`**. State changes immediately trigger reactive mutation invalidations, evicting stale caches and waking up downstream listeners across the kernel graph without polling overhead.

---

## 2. Component Architecture & Event Cascading Topology

```
                  +-----------------------------------------+
                  |  Asynchronous Callback Producer         |
                  |  (Scheduler, Subagent, Worker, Command) |
                  +-----------------------------------------+
                                       |
                                       v
                  +-----------------------------------------+
                  |       MultiSubscriberDispatcher         |
                  +-----------------------------------------+
                         |                    |
        +----------------+                    +----------------+
        |                                                      |
        v                                                      v
+-----------------------+                            +-----------------------+
|  FileLogSubscriber    |                            |  KernelWALSubscriber  |
|  (Audit Trails)       |                            |  (Lifecycle WAL)      |
+-----------------------+                            +-----------------------+
                                       |
                                       v
                  +-----------------------------------------+
                  |         ShockwaveSubscriber             |
                  |  - Interface: CallbackSubscriber        |
                  |  - Nil & Boundary Event Validation      |
                  |  - Semantic Kind & ID Resolution        |
                  |  - Non-Blocking Async / Sync Dispatch   |
                  +-----------------------------------------+
                                       |
                                       v
                  +-----------------------------------------+
                  |       InvalidationShockwaveBus          |
                  |  (storage.GetGlobalInvalidationBus())   |
                  +-----------------------------------------+
                      |                 |                 |
                      v                 v                 v
               +--------------+  +--------------+  +--------------+
               |  ParseCache  |  |  ListCache   |  | ObjectIDCache|
               +--------------+  +--------------+  +--------------+
                      |                 |                 |
                      +-----------------+-----------------+
                                       |
                                       v
                      [Reactive Cache Eviction & Listener Wakeup]
```

---

## 3. Core Contracts & Structural Implementation

### 3.1 Interface Compliance
`ShockwaveSubscriber` implements the canonical `CallbackSubscriber` (aliased to `Subscriber`) contract:

```go
type Subscriber interface {
    Name() string
    Notify(ctx context.Context, entry *CallbackEntry) error
}

type CallbackSubscriber = Subscriber
```

### 3.2 Constructor & Functional Options
The subscriber can operate in synchronous mode (default) or asynchronous buffered mode:

```go
// Synchronous initialization linking to the global or custom invalidation bus
sub := callback.NewShockwaveSubscriber(bus)

// Non-blocking asynchronous initialization with bounded ring-buffer capacity
asyncSub := callback.NewAsyncShockwaveSubscriber(bus, 256,
    callback.WithLogger(logger),
    callback.WithName("shockwave"),
)
defer asyncSub.Close()
```

---

## 4. Boundary Condition Handling & Resilience

### 4.1 Nil Event Rejection
To prevent undefined behavior, `Notify` rejects uninitialized entries and payloads early:
- `entry == nil` -> Returns `ErrNilCallbackEntry`
- `entry.Payload == nil` -> Returns `ErrNilPayload`
- Calling on an uninitialized receiver or bus -> Returns `errfmt.Errorf` / `ErrNilBus`
- Calling on a terminated subscriber -> Returns `ErrSubscriberClosed`

### 4.2 Semantic Kind and ID Inference
Payloads from disparate subsystems often omit explicit object kinds. `ShockwaveSubscriber` resolves them deterministically:
1. **Identifier Resolution**: Inspects `object_id`, then `id`, then `job_id`.
2. **Kind Inference**:
   - Inspects `objects.FieldKeyKind`.
   - If missing, calls `storage.InferKindFromID(objID)` matching canonical prefixes (`BLI-` -> `backlog_item`, `REQ-` -> `requirement`, `CRIT-` -> `criteria`, `GOAL-` -> `goal`, `PRI-` -> `priority_plan`, `MIL-` -> `milestone`).
   - If ID represents a scheduler job, falls back to `scheduler_job`.
   - Otherwise defaults to `unknown`.
3. **Operation Resolution**: Maps `"op"` to `MutationOpPut` or `MutationOpDelete`.

---

## 5. Concurrency Lifecycle & Zero-Leak Guarantees

### 5.1 Named Goroutine Supervision
In asynchronous mode, worker goroutines are registered via `goroutinelabels.NewGoroutine`:
```go
goroutinelabels.NewGoroutine("shockwave_subscriber_worker", "async invalidation shockwave worker").StartSimple(sub.runWorker)
```

### 5.2 Graceful Termination and Drain
When `Close()` is invoked:
1. Atomically sets the closed flag (`s.closed.CompareAndSwap(false, true)`).
2. Cancels context signaling the worker goroutine.
3. Drains remaining events in `eventCh` to ensure zero message loss.
4. Waits for `s.workerWg.Wait()` to complete, guaranteeing zero channel or goroutine leaks.

---

## 6. Verification Matrix

The implementation is verified by automated test suites in `cmd/zqk/callback/shockwave_subscriber_test.go`:
- `TestShockwaveSubscriber_InterfaceComplianceAndDefaults`: Asserts interface and naming invariants.
- `TestShockwaveSubscriber_NilEventAndPayloadRejection`: Verifies fail-closed rejection of nil inputs.
- `TestShockwaveSubscriber_KindInferenceResilience`: Verifies prefix-based kind deduction across ontology kinds.
- `TestShockwaveSubscriber_MissingObjectIDFallback`: Verifies fallback to `job_id` and `unknown`.
- `TestShockwaveSubscriber_AsyncNonBlockingDispatch`: Confirms non-blocking queueing, graceful drain, and closed rejection.
- `TestShockwaveSubscriber_ConcurrentLoadAndZeroLeaks`: Stresses the subscriber under 50 concurrent goroutines with `-race` enabled.
- `TestShockwaveSubscriber_DispatcherIntegration`: Proves seamless fanout when orchestrated by `MultiSubscriberDispatcher`.
