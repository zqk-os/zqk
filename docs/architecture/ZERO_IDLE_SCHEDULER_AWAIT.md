# Architecture Specification: Zero-Idle Scheduler Subprocess Await and Reactive WAL Dispatch

## 1. Overview and Problem Statement
In automated distributed pipelines and autonomous agent swarms, tasks are regularly dispatched to the background scheduler daemon (`zqk scheduler trigger` or `submit`). Previously, waiting for task completion required external sleep polling or periodic status querying. This pattern led to high CPU overhead, prolonged latency, and worst of all, caused agent harnesses to transition into idle states (`waiting_for_input`).

To eliminate idle wait states, ZQK implements the **Zero-Idle Scheduler Subprocess Await** command:
```bash
zqk scheduler wait <job_id> [--timeout 5m] [--poll-interval 50ms] [--format json|table]
```
This CLI command directly hooks into the Knowledge Kernel's reactive lifecycle waker architecture, immediately resuming upon task completion with sub-millisecond response times.

---

## 2. Technical Architecture & Control Flow

```
┌────────────────────────────────┐
│   Caller / Agent Orchestrator  │
└───────────────┬────────────────┘
                │ zqk scheduler wait <job_id>
                ▼
┌────────────────────────────────┐
│       Wait Command Runner      │  (cmd/zqk/scheduler/scheduler_wait.go)
└───────────────┬────────────────┘
                │ lifecycle.AwaitJobWithWAL
                ▼
┌────────────────────────────────┐
│      JobWakerRegistry          │  (pkg/lifecycle/waker.go)
│  - Registers jobID channel     │
└───────────────┬────────────────┘
                │
                ├──────────────────────────────────┐
                │ Concurrent In-Memory             │ Background WAL Poller
                │ Dispatch                         │ (lifecycle_events.wal)
                ▼                                  ▼
┌────────────────────────────────┐      ┌─────────────────────────────┐
│  MultiSubscriberDispatcher     │      │   PollLifecycleWAL          │
│  - Dispatches to subscribers   │      │   - Reads WAL records       │
└───────────────┬────────────────┘      └──────────────┬──────────────┘
                │                                      │
                └───────────────┬──────────────────────┘
                                │ ch <- LifecycleEvent
                                ▼
┌────────────────────────────────┐
│      Wait Result Output        │  (Status, Duration, Scope, CompletedAt)
└────────────────────────────────┘
```

---

## 3. Core Capabilities & Invariants

### 3.1 Dual-Path Delivery (Memory + WAL Replay)
1. **In-Memory Fast Path**: If the job completion callback occurs within the same runtime supervisor process, `JobWakerRegistry.Dispatch` routes the `LifecycleEvent` directly to the awaiting channel in <1ms without hitting disk.
2. **Durable WAL Replay Path**: If the callback was processed in an external CLI process (`zqk callback notify`), the event is committed to `.zqk/wal/lifecycle_events.wal`. The background WAL listener detects the new record and dispatches it to the waker registry.

### 3.2 Concurrency & Bounded Contexts
- **Zero Raw Goroutines**: All asynchronous polling and dispatching execute via `goroutinelabels.NewGoroutine`.
- **Context Deadline Enforcement**: `--timeout` bounds the maximum wait window (default 5 minutes). When exceeded, `ErrWaitTimeout` is raised and all subscribed channels are cleanly unregistered without leaking resources.
- **Data-Race Free Memory**: All internal registry structures are protected by `sync.RWMutex`, verified clean under `go test -race`.

---

## 4. Verification & Testing Manifest
- Unit and integration tests in `cmd/zqk/scheduler/scheduler_wait_test.go`:
  - `TestSchedulerWait_DispatchImmediate`: Verifies immediate in-memory channel unblocking.
  - `TestSchedulerWait_WALReplay`: Verifies WAL event detection and waker completion.
  - `TestSchedulerWait_FormatJSON`: Validates JSON schema parity.
  - `TestSchedulerWait_Timeout`: Confirms bounded cancellation upon context expiration.
  - `TestSchedulerWait_ContextCanceled`: Confirms graceful cleanup on SIGINT / context cancel.
  - `TestSchedulerWait_FailedJobStatus`: Confirms non-zero exit code when job status is `failed`.
