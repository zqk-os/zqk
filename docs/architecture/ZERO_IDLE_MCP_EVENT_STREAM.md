# Architecture Specification: Zero-Idle MCP Event Stream and Real-Time Agent Notification Mesh

## 1. Overview and Problem Statement

In distributed multi-agent systems and developer IDE integrations, agent harnesses and client orchestrators execute background tasks, async scheduler jobs, and criteria verification workflows. Historically, client harnesses relied on polling loops (`zqk status`, `sleep`, or recurring tool calls) to detect when background tasks concluded.

This polling pattern introduced two major architectural failure modes:
1. **Prolonged Latency & CPU Overhead**: Polling intervals created artificial delays (often seconds) between job completion and agent wake-up.
2. **Idle State Traps (`waiting_for_input`)**: When autonomous agents cease tool execution while waiting for long-running jobs, modern agent platforms transition into idle wait states, interrupting the continuous execution loop.

To eliminate idle wait states and enable instantaneous wake-up, ZQK implements the **Zero-Idle MCP Event Stream and Real-Time Agent Notification Mesh**. This streaming mesh bridges the Knowledge Kernel's `JobWakerRegistry` and `LifecycleEventWAL` directly to connected Model Context Protocol (MCP) clients, broadcasting real-time JSON-RPC notifications the instant background operations complete.

---

## 2. Technical Architecture & Control Flow

```
┌─────────────────────────────────────────────────────────────┐
│                 Kernel Execution Engines                    │
│   (Scheduler Daemon, Worker Swarms, Verification Suites)    │
└──────────────┬───────────────────────────────┬──────────────┘
               │ Dispatch                      │ Append
               ▼                               ▼
┌──────────────────────────────┐ ┌────────────────────────────┐
│   JobWakerRegistry (Memory)  │ │ LifecycleEventWAL (Disk)   │
│   - Fast path (<1ms latency) │ │ - Multi-process durability │
└──────────────┬───────────────┘ └─────────────┬──────────────┘
               │ wakerCh                       │ walReplay
               └───────────────┬───────────────┘
                               ▼
┌─────────────────────────────────────────────────────────────┐
│                    WakerEventStreamer                       │  (pkg/mcp/waker_stream.go)
│  - Wildcard waker registration                              │
│  - Bounded ring deduplication (Seq / ID:Status)             │
│  - Non-blocking translation & buffer overrun protection     │
└──────────────┬───────────────────────────────┬──────────────┘
               │                               │
               │ Emit(Event)                   │ BroadcastMessage(...)
               ▼                               ▼
┌──────────────────────────────┐ ┌────────────────────────────┐
│    MCP EventEmitter          │ │  MCP Server Connection     │  (pkg/mcp/server.go)
│    (Pub/Sub Type Index)      │ │  Message Queue Dispatcher  │
└──────────────┬───────────────┘ └─────────────┬──────────────┘
               │                               │
               ├───────────────────────────────┤
               │ JSON-RPC 2.0 Notifications    │
               ▼                               ▼
┌─────────────────────────────────────────────────────────────┐
│                  Connected MCP Clients                      │
│   (IDEs: Cursor / Antigravity, CLI Proxies, Subagent Pods)  │
│   - notifications/event   (type: "task_waker")              │
│   - notifications/message (Zero-idle wake trigger)          │
└─────────────────────────────────────────────────────────────┘
```

---

## 3. Streaming Protocol & Notification Formats

The streaming mesh delivers notifications using the standard JSON-RPC 2.0 protocol over stdio and TCP/SSE transports. All notifications are fire-and-forget (`id` omitted) to prevent head-of-line blocking.

### 3.1 `notifications/event` (Structured Event Stream)

Clients subscribed via `events/subscribe` receive typed, structured event payloads:

```json
{
  "jsonrpc": "2.0",
  "method": "notifications/event",
  "params": {
    "event": {
      "type": "task_waker",
      "timestamp": "2026-10-10T09:15:30.123456Z",
      "message": "Scheduler callback waker event: scheduler_job job-8491 status=completed",
      "fields": {
        "job_id": "job-8491",
        "task_id": "job-8491",
        "event_type": "scheduler_callback",
        "kind": "scheduler_job",
        "from_status": "in_progress",
        "to_status": "completed",
        "seq": 1042,
        "scope": {
          "exit_code": "0",
          "runtime_ms": "1420"
        }
      },
      "severity": "info",
      "priority": "high"
    }
  }
}
```

### 3.2 `notifications/message` (Real-Time Console Broadcast)

To support MCP clients that render console messages or lack explicit event subscription logic, the streamer simultaneously broadcasts a `notifications/message` notification:

```json
{
  "jsonrpc": "2.0",
  "method": "notifications/message",
  "params": {
    "message": "Scheduler callback waker event: scheduler_job job-8491 status=completed",
    "message_type": "task_waker",
    "priority": "high",
    "timestamp": "2026-10-10T09:15:30Z"
  }
}
```

---

## 4. Client Subscription Model

Clients control their event delivery using standard MCP event management methods:

### 4.1 `events/list`
Discovers supported event types, including `task_waker` and `lifecycle.event`:

```json
// Request
{"jsonrpc": "2.0", "id": 1, "method": "events/list"}

// Response
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "eventTypes": [
      "log.debug",
      "log.info",
      "log.warn",
      "log.error",
      "tool.started",
      "tool.completed",
      "tool.failed",
      "action.required",
      "task_waker",
      "lifecycle.event"
    ],
    "subscriberCount": 2
  }
}
```

### 4.2 `events/subscribe`
Registers an active subscription. Clients can subscribe to `task_waker` specifically or omit `eventTypes` to receive all system notifications:

```json
// Request
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "events/subscribe",
  "params": {
    "eventTypes": ["task_waker", "lifecycle.event"]
  }
}

// Response
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "subscriptionId": "sub_1791621980000000000",
    "eventTypes": ["task_waker", "lifecycle.event"]
  }
}
```

### 4.3 `events/unsubscribe`
Tears down a subscription and releases all associated buffers:

```json
// Request
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "events/unsubscribe",
  "params": {
    "subscriptionId": "sub_1791621980000000000"
  }
}

// Response
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "message": "Unsubscribed successfully"
  }
}
```

---

## 5. Dual-Path Delivery & Concurrency Invariants

### 5.1 In-Memory Fast Path vs. Durable WAL Replay
1. **In-Memory Fast Path (<1ms)**: If background execution runs in the same supervisor process, `JobWakerRegistry.Dispatch` routes directly to the streamer's wildcard channel.
2. **Durable WAL Replay Path**: If an external CLI process (e.g. `zqk callback notify` or independent test runner) updates state, the event is appended to `.zqk/wal/lifecycle_events.wal`. The background WAL poller reads the event and streams it.
3. **Idempotent Deduplication**: A 30-second TTL bounded memory cache indexes recently streamed events (`seq` or `id:type:to_status`), preventing duplicate notifications if both fast-path and WAL replay observe the same event.

### 5.2 Boundary & Error Handling
- **Buffer Overrun Protection**: All client notifications are enqueued via bounded `MessageQueue` instances. If a slow or blocked client exhausts its queue buffer, messages are intentionally dropped with metrics recording (`mcp_queue_dropped_total`), ensuring slow consumers never block the server or other clients.
- **Graceful Disconnects**: Disconnected or dead client writers are detected during enqueue or write timeouts. Subscriptions tied to dropped writers are automatically cleaned up via `unsubscribeWriterSubscriptions`.
- **Zero Goroutine and Channel Leaks**:
  - Background workers execute strictly via `goroutinelabels.NewGoroutine`.
  - Calling `Stop()` cancels context, immediately unregisters channels from `JobWakerRegistry`, and blocks on `sync.WaitGroup` until all goroutines exit cleanly.
  - Server shutdown hooks (`server.RegisterShutdownHook`) automatically invoke `Stop()`.
