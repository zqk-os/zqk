# CLI Async and Progress: Never Hang Without Feedback

**Last Verified:** 2026-08-31


**Goal:** Every CLI command is async-aware and provides status/progress so the user is **never** left wondering what’s going on. If the process is waiting (e.g. for a lock or semaphore), the user sees a clear message—not a silent hang.

## Principles

1. **Every CLI command runs with async progress**  
   Commands use `BindAsyncProgress` so they get:
   - An **execution context** with timeout (so blocking is bounded).
   - A **progress callback** on context (so any layer can report what it’s doing).

2. **Never block without telling the user**  
   Any code path that can block (lock, semaphore, I/O wait) on a CLI request **must** emit a progress/status message **before** blocking. Examples:
   - Waiting to acquire the list/count semaphore → emit e.g. “Waiting for list/count slot (up to 8 concurrent operations)...”
   - Waiting for a lock → emit “Waiting for … lock...”
   - Long I/O phase → emit “Loading …” or “Scanning …”

3. **One path for progress**  
   Progress is carried on the request context and flows through the stack:
   - **CLI:** `RunWithAsyncProgress` attaches `progressFn` to context via `pkgctx.WithValidationProgress`.
   - **Storage / other layers:** Call `pkgctx.GetValidationProgress(ctx)`; if non-nil, call it with `(stage, message)` before blocking or before a long phase.
   - **Coordinator:** The callback triggers `EmitStatusChange` so the message is logged and visible (e.g. human profile sees it in logs/stderr).

4. **Regular interval (heartbeat)**  
   So the user is never left without feedback for long, `RunWithAsyncProgress` starts a **progress heartbeat** goroutine that emits a status message every **5 seconds** (e.g. “Operation in progress: object_list — storage: Waiting for list/count slot...”). The heartbeat uses the latest stage/message from the progress callback. It stops when the command returns. This matches the pattern used in system check (validation heartbeat) and pattern-cli (maxSilence 5–7s).

## Implementation

- **Progress heartbeat:** `internal/cli/async_progress.go`: `runProgressHeartbeat` runs until the command returns and emits status every `progressHeartbeatInterval` (5s). The message includes operation type and the latest stage/message so the user always sees context.
- **Timeout context:** The timeout hook passes the execution context (with timeout) into the command so that when the timeout fires, blocking calls (e.g. `AcquireListCountSlot`) see `ctx.Done()` and return. See `cmd/zqk/root.go` and `pkg/cli/timeout_hook_execution.go`.
- **Progress before blocking:** For list/count, storage calls `EmitListCountWaitProgress(ctx)` immediately before `AcquireListCountSlot(ctx)`. That uses `GetValidationProgress(ctx)` to emit a status message when the CLI is waiting for a slot. Other blocking points (locks, etc.) should follow the same pattern: get progress from context, emit a short message, then block.
- **Visibility:** The coordinator’s logging router uses the `message` field from progress/status events as the log line so the user sees text like “Waiting for list/count slot...” instead of only the operation type.

## Checklist (for new or changed code)

- [ ] **CLI entrypoint:** Command uses `BindAsyncProgress` (or equivalent) so it runs with timeout context and progress callback on context.
- [ ] **Blocking calls:** Before any call that can block (semaphore, lock, long I/O), emit progress via `pkgctx.GetValidationProgress(ctx)` when available (e.g. `EmitListCountWaitProgress` for list/count slot).
- [ ] **No silent waits:** Avoid adding new blocking points without a prior progress message; if in doubt, emit a short “Waiting for …” or “Loading …” message before the wait.

## Optional future: dispatcher pool and dispatch loop

A **dispatch loop** is the single place that accepts work (CLI command or MCP tool), runs it with timeout and progress, and emits status/outcome to the Coordinator. Both CLI and MCP can use it so progress and cancellation behave the same everywhere; MCP can then stream progress to clients instead of only returning a final result. See **`docs/architecture/DISPATCH_LOOP_AND_MCP.md`** for the full outline.

A **dispatcher pool** would be a more centralized design:

- A long-running **dispatcher** process (or reuse the scheduler) **listens** for CLI requests (e.g. over a socket, stdio, or job queue).
- Requests are **routed** to a **pool of workers** that run the actual command logic via the same dispatch loop contract.
- The dispatcher (or worker) **emits status/progress/outcome on a regular interval** (e.g. every 5s) and on every state change, so the client never goes too long without a message.
- The CLI binary could become a thin **client** that submits the request and streams progress/result back.

**When it might be worth it:** Multiple concurrent CLI users, need for central rate limiting or prioritization, or running CLI in a server/API context where you want a single process to own all command execution. For the current “single user, local CLI” model, the **in-process heartbeat** (above) already gives “regular interval that doesn’t leave the user wondering” without a separate dispatcher. If we later add a “zqk serve” or multi-tenant CLI gateway, a dispatcher pool would fit naturally.

## References

- **Pre-change checklist:** `docs/architecture/PRE_CHANGE_CHECKLIST.md` (section 12).
- **Progress heartbeat:** `internal/cli/async_progress.go` (`runProgressHeartbeat`, `progressHeartbeatInterval`).
- **Progress on context:** `pkg/context/validation_progress_context.go` (`WithValidationProgress`, `GetValidationProgress`).
- **List/count progress:** `pkg/storage/list_count_concurrency.go` (`EmitListCountWaitProgress`, `AcquireListCountSlot`).
- **Timeout and context:** `pkg/cli/timeout_hook_execution.go`, `cmd/zqk/root.go` (execution context passed into command).
- **Dispatch loop and MCP:** `docs/architecture/DISPATCH_LOOP_AND_MCP.md`.
