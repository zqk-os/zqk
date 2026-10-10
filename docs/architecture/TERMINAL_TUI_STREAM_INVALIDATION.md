# Architecture Specification: Real-Time Event-Driven Terminal & TUI Stream Invalidation

## 1. Executive Summary & Problem Statement

In asynchronous CLI, scheduler, and multi-agent systems, long-running workloads emit continuous lifecycle, progress, and status events. Traditional terminal output models present two critical friction points:

1. **Line Pollution & Context Scroll-Off**: Emitting standard newline-terminated log lines (`\n`) for every intermediate tick or percentage delta floods the terminal scrollback buffer, obscuring operator visibility and context for interactive commands.
2. **Polling Latency & TUI Drift**: TUI dashboards or terminal harnesses relying on polling loops or periodic queries introduce artificial latency (e.g. 100ms–1000ms delay), lagging behind real-time execution states and creating inconsistent visual state.

The Knowledge Kernel addresses this by introducing the **`TerminalProgressSubscriber`** within the reactive callback mesh (`cmd/zqk/callback`). As an implementation of `CallbackSubscriber`, it receives event-driven callback entries without polling delay, synchronizes writes under a strict thread-safe mutex, and performs zero-latency in-place stream invalidation using standard ANSI carriage return escape sequences (`\r\033[K`).

---

## 2. Component Architecture & Event Topology

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
        |                                                      |
        v                                                      v
+-----------------------+                            +-----------------------+
|  ShockwaveSubscriber  |                            | TerminalProgressSub   |
|  (Invalidation Bus)   |                            | (TUI Stream Redraw)   |
+-----------------------+                            +-----------------------+
                                                               |
                                             +-----------------+-----------------+
                                             |                                   |
                                    [Sync Inline Write]                 [Async Channel Buffer]
                                             |                                   |
                                             v                                   v
                                    +-----------------+                 +-----------------+
                                    |  Mutex Lock     |                 |  Worker Loop    |
                                    +-----------------+                 +-----------------+
                                             \                                   /
                                              \                                 /
                                               v                               v
                                          +-----------------------------------------+
                                          | Atomic Line Invalidation (\r\033[K)     |
                                          | Output Target (os.Stderr / io.Writer)   |
                                          +-----------------------------------------+
```

---

## 3. ANSI Carriage Return Redraw Protocols

### 3.1 Line Invalidation Sequence (`\r\033[K`)

ANSI TUI redraws leverage two atomic terminal control characters:
- **`\r` (Carriage Return)**: Moves the cursor to the first column (column 0) of the current line without advancing down.
- **`\033[K` (Clear to End of Line)**: Erases all existing characters from the current cursor position to the terminal margin.

When combined (`\r\033[K`), subsequent writes completely overwrite previous status or progress lines without leaving ghost remnants from longer previous text lines.

### 3.2 Live Progress vs. Terminal Completion States

- **In-Flight Progress Events**:
  Events with type `progress` or in-progress status omit a trailing newline (`\n`). The cursor remains at the end of the updated line, ready for immediate in-place overwriting upon the next reactive callback.
- **Terminal Completion & Error Events**:
  Terminal events (`completion`, `error`, `failed`, `completed`) append a trailing newline (`\n`). This seals the final state onto the terminal display and positions subsequent stdout/stderr logs or prompts on a clean new row.
- **Cursor Line Finalization on Close**:
  When `Close()` is invoked on `TerminalProgressSubscriber`, if the last emitted line did not terminate with `\n`, a closing newline is emitted to guarantee cursor restoration.

### 3.3 Plain Text & Non-TTY Fallback

When `AnsiEnabled` is configured to `false` (e.g., non-interactive CI environments, redirected log pipes, or dumb terminals), the subscriber disables carriage return control sequences. Every event is written as a clean, newline-delimited log entry (`\n`), preventing corrupt control characters in file logs.

---

## 4. Writer Buffering & Thread-Safe Synchronization

### 4.1 Synchronous vs. Asynchronous Dispatch

`TerminalSubscriberConfig` provides flexible dispatch semantics:

| Mode | Configuration | Behavior |
| :--- | :--- | :--- |
| **Synchronous** | `BufferSize == 0` | Writes occur directly within the calling goroutine's `Notify()` under a `sync.Mutex` lock. Zero goroutine overhead, suitable for low-to-medium event volumes. |
| **Asynchronous Buffered** | `BufferSize > 0` | `Notify()` dispatches to a buffered channel (`chan *CallbackEntry`), consumed by a dedicated worker goroutine registered with `goroutinelabels`. |

### 4.2 Non-Blocking Delivery & Buffer Saturation Resilience

Under extreme callback bursts, slow terminal outputs (e.g. over remote SSH or blocked pipes) must not block scheduler execution or worker threads.
- If `DropOnFull: true`, the subscriber immediately drops overflowing events using non-blocking channel selects (`select ... default:`), incrementing an atomic drop counter (`DroppedCount()`).
- If `DropOnFull: false`, `Notify()` honors context cancellation (`ctx.Done()`), exiting cleanly if the parent context cancels while waiting for buffer space.

### 4.3 Clean Shutdown & Resource Hygiene

The subscriber guarantees zero goroutine or channel leaks:
- `Close()` marks the subscriber closed via atomic compare-and-swap (`closed.CompareAndSwap(false, true)`).
- Active worker contexts are cancelled, and pending events are drained to the writer.
- Worker termination is synchronized using `sync.WaitGroup`.
- `Flush()` invokes underlying writer flushers (`interface{ Flush() error }`, `interface{ Flush() }`, or `interface{ Sync() error }`).

---

## 5. Event Normalization & Status Formatting

`TerminalProgressSubscriber` formats heterogenous payload schemas into uniform status lines:

```
[PREFIX] [CATEGORY] Job: <id> | <metrics> | <message>
```

1. **Job Wakers (`[WAKER]`)**:
   Formats lifecycle and task waker signals: `[WAKER] requirement REQ-100 | Status: waking | Waking dependent tasks`.
2. **Progress Events (`[PROGRESS]`)**:
   Formats execution percentage, step counters, and status messages: `[PROGRESS] Job: job-101 | 45.5% | step 9/20 | Compiling package...`.
3. **Completion Events (`[COMPLETE]`)**:
   Formats successful terminations with elapsed duration: `[COMPLETE] Job: job-ansi | Status: completed | Duration: 3.42s`.
4. **Failure Events (`[FAILED]`)**:
   Formats execution errors and failure messages: `[FAILED] Job: job-fail | Status: failed | Error: disk quota exceeded`.
5. **Color Stripping (`StripColors`)**:
   When enabled, removes all ANSI color and style escapes (`\x1b[...m`), preserving line-invalidation escape characters while guaranteeing clean output for monochromatic terminals.

---

## 6. Verification and Traceability

| Artifact | Location |
| :--- | :--- |
| **Subscriber Implementation** | `cmd/zqk/callback/terminal_subscriber.go` |
| **Unit & Concurrent Test Suite** | `cmd/zqk/callback/terminal_subscriber_test.go` |
| **Task / Work Item** | `BLI-TERMINAL-TUI-STREAM-INVALIDATION` |
| **Test Case** | `TST-TERMINAL-TUI-STREAM-INVALIDATION` |
| **Priority Plan** | `PRI-TERMINAL-TUI-STREAM-INVALIDATION` (Cycle 20) |
| **Functional Acceptance Criterion** | `CRIT-TERMINAL-PROGRESS-DISPATCH` |
| **Boundary & Error Criterion** | `CRIT-TERMINAL-NIL-SAFE-DISPATCH` |
| **Architecture Specification Criterion** | `CRIT-TERMINAL-TUI-DOC-SPEC` |
