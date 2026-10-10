# Architecture Specification: Event-Driven Multi-Subscriber Callback Mesh

## 1. Overview and Problem Statement
In distributed agent workflows and asynchronous job executions, long-running processes (e.g., scheduler jobs, subagent executions, continuous audits) emit lifecycle completion and status transitions. Historically, execution harnesses resorted to idle sleep loops or synchronous waiting blocks, leading to agent stalls (`waiting_for_input` halts) and high latency.

The Knowledge Kernel employs an event-driven architecture where completion events act as signals of intent that the system metabolizes into reactive execution. To support multiple simultaneous consumers—including file-based audit logging, Knowledge Kernel write-ahead logging (WAL), and reactive invalidation shockwaves—ZQK introduces the `MultiSubscriberDispatcher`.

## 2. Dispatcher Architecture

The `MultiSubscriberDispatcher` decouples callback intake from consumption using the `Subscriber` interface:

```go
type Subscriber interface {
    Name() string
    Notify(ctx context.Context, entry *CallbackEntry) error
}
```

### Core Built-in Subscribers
1. **FileLogSubscriber (`file_log`)**: Formats and flushes human-readable, JSON, or JSONL logs to designated audit log files (`system.log` or custom paths).
2. **KernelWALSubscriber (`kernel_wal`)**: Synthesizes durable `lifecycle.LifecycleEvent` records from the callback payload (e.g. status transitions, criteria satisfaction, job completion) and appends them to the append-only `LifecycleEventWAL`.
3. **ShockwaveSubscriber (`shockwave`)**: Propagates storage and object `MutationEvent` shockwaves across the `InvalidationShockwaveBus`, waking up reactive listeners, evicting stale in-memory index caches, and cascading downstream transitions without polling.
4. **FuncSubscriber (`custom`)**: Enables dynamic runtime registration of functional closures and reactive channels for programmatic integration and unit verification.

## 3. Concurrency and Error Isolation
- **Managed Concurrency**: Dispatches fan out concurrently across all registered subscribers using `goroutinelabels.NewGoroutine(...)` to ensure tracking under runtime supervision.
- **Fail-Safe Isolation**: An error or latency spike in one subscriber (e.g., log disk write pressure) does not block or cancel other subscribers. Errors are aggregated and reported while preserving execution guarantees across all healthy subscribers.

## 4. Kernel WAL Event Synthesis Mapping
Callback payloads are synthesized into canonical `lifecycle.LifecycleEvent` records according to the following mapping:
- If `criteria_id` is present: maps to `lifecycle.EventTypeCriterionSatisfied`.
- If `status` / `from_status` are present: maps to `lifecycle.EventTypeStatusTransition`.
- For general scheduler tasks: maps to `scheduler_callback` with `completed` or `failed` target status and scoped job metadata.
