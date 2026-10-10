# Architecture Specification: Reactive WAL Lifecycle Callback Listener and Zero-Idle Waker

## 1. Overview and Problem Statement
In multi-agent and autonomous distributed operations, background processes (such as scheduled tasks, long-running test suites, compilation steps, or subagent tasks) execute out-of-band. Historically, callers and agent orchestrators resorted to polling loops (`time.Sleep` or periodic tickers), which either wasted CPU cycles or caused agents to yield control and enter idle wait states (`waiting_for_input`).

To guarantee continuous autonomous execution and zero idleness, the Knowledge Kernel implements the **Reactive WAL Lifecycle Callback Listener and Zero-Idle Waker**. When background scheduler jobs complete, their callback execution synthesizes an `EventTypeSchedulerCallback` event into the write-ahead log (`lifecycle_events.wal`). The Lifecycle Listener immediately processes this record and dispatches it to registered `JobWakerRegistry` listeners, unblocking waiting tasks reactively with sub-millisecond latency.

## 2. System Architecture

```
   ┌───────────────────────┐
   │ Background Scheduler  │
   │      Execution        │
   └──────────┬────────────┘
              │  --callback-completion "zqk callback notify"
              ▼
   ┌───────────────────────┐
   │   Callback Processor  │
   └──────────┬────────────┘
              │  MultiSubscriberDispatcher
              ▼
   ┌───────────────────────┐
   │  KernelWALSubscriber  │
   └──────────┬────────────┘
              │  Append(EventTypeSchedulerCallback)
              ▼
   ┌───────────────────────┐
   │  Lifecycle Event WAL  │  (.zqk/wal/lifecycle_events.wal)
   └──────────┬────────────┘
              │  ReplayFromCursor / Poller
              ▼
   ┌───────────────────────┐
   │   Lifecycle Listener  │
   └──────────┬────────────┘
              │  Wake(ev)
              ▼
   ┌───────────────────────┐
   │   JobWakerRegistry    │
   └──────────┬────────────┘
              │  ch <- ev (non-blocking)
              ▼
   ┌───────────────────────┐
   │  Reactive Agent Task  │  (Zero-Idle AwaitJob)
   └───────────────────────┘
```

## 3. Core Components

### 3.1 EventTypeSchedulerCallback
A first-class lifecycle event type in `pkg/lifecycle`:
```go
const (
    EventTypeSchedulerCallback EventType = "scheduler_callback"
)
```
Carries the `job_id` in `ev.ID`, target status (`completed` or `failed`) in `ev.ToStatus`, and execution metadata in `ev.Scope`.

### 3.2 JobWakerRegistry
Thread-safe waker coordinator in `pkg/lifecycle/waker.go`:
- `Register(jobID string) (<-chan *LifecycleEvent, func())`: Registers a channel for a specific job ID.
- `RegisterWildcard() (<-chan *LifecycleEvent, func())`: Registers for all scheduler callback events.
- `Dispatch(ev *LifecycleEvent) int`: Non-blocking broadcast to all matching registered wakers.
- `AwaitJob(ctx context.Context, jobID string) (*LifecycleEvent, error)`: Blocks reactively until the specific job completes or the context expires, cleaning up channel registration on return.
- `AwaitJobWithWAL(ctx, projectRoot, jobID, pollInterval)`: Integrates continuous WAL polling with reactive waker dispatch.

### 3.3 Lifecycle Listener Integration
`Listener` implements `CallbackWaker` registration:
- In `NewListener`, automatically registers `GetGlobalJobWakerRegistry()`.
- In `processEvent()`, matches `EventTypeSchedulerCallback` and dispatches directly to all registered `CallbackWaker` instances.

## 4. Concurrency, Reliability, and Error Boundaries
1. **Non-Blocking Delivery**: Dispatches to buffered channels using `select { case ch <- ev: default: }`, preventing slow or hanging consumers from blocking the WAL replay thread.
2. **Context and Timeout Safety**: `AwaitJob` strictly respects context deadlines (`context.WithTimeout`), returning `context.DeadlineExceeded` without leaking goroutines or channels.
3. **Idempotent Cleanup**: Waker unregistration functions are safe to invoke multiple times and immediately remove dead channels from the active registry.
4. **Race-Free Concurrency**: Verified under the Go race detector (`go test -race ./pkg/lifecycle/...`) across high-concurrency worker fan-out.
