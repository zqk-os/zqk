# Dispatch Loop: Outline and MCP Use

**Last Verified:** 2026-08-31


**Goal:** Define a single **dispatch loop** that accepts work (CLI command or MCP tool), runs it with timeout and progress, and emits status/outcome on a regular interval. Both **CLI** and **MCP** use this loop so progress and cancellation behave the same everywhere, and MCP can stream progress to clients instead of only returning a final result.

## What the dispatch loop is

A **dispatch loop** is the single place that:

1. **Accepts** a work item (e.g. “run this command with these args” or “run this tool with these params”).
2. **Runs** the work with:
   - A **timeout context** (so execution is bounded).
   - A **progress callback** on context (so any layer can report stage/message).
3. **Emits** status and outcome to a **bus** (the existing Coordinator):
   - On start, progress updates (including heartbeat at a regular interval), and on completion or error.

Optional extension: a **queue + worker pool** so multiple items can be in flight with bounded concurrency and optional prioritization. The outline below assumes a single loop that runs one job at a time; the same contract applies when we add a pool.

## What it looks like (single-loop version)

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         DISPATCH LOOP                                    │
│                                                                         │
│  1. Accept work item: { op: "cli" | "tool", id, args, secCtx, ... }    │
│  2. Create execCtx with timeout; attach progress callback → Coordinator │
│  3. Run:                                                                │
│       - If op == "cli"  → rootCmd.ExecuteContext(execCtx) [in-process]  │
│       - If op == "tool" → same or delegate to CLI path (in-process)     │
│  4. Progress flows: context callback → ProgressHelper → Coordinator     │
│     (heartbeat every 5s; same as RunWithAsyncProgress today)             │
│  5. On done: emit complete/error; return result to caller               │
└─────────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────────┐
│  Coordinator (existing)                                                  │
│  - Logging, audit, metrics                                               │
│  - OperationalEventSubscriber(s): e.g. MCP forwards as notifications     │
└─────────────────────────────────────────────────────────────────────────┘
```

- **Input:** One work item per “command” or “tool call” (with identity, args, security context, optional request context for cancellation).
- **Output:** Progress and outcome on the bus; final result (or error) returned to the caller that submitted the item.
- **No subprocess for MCP:** Today MCP runs CLI tools by spawning a subprocess (`exec` of the zqk binary). With the dispatch loop, MCP would **submit the same work to the loop** and the loop would run the command **in-process** (same `rootCmd.ExecuteContext` path as the CLI). So one code path, one place for timeout and progress, and MCP gets real-time progress by subscribing to the Coordinator.

## CLI use

- **Today:** `main` → timeout hook → `rootCmd.ExecuteContext(execCtx)` → exit. One shot, no loop.
- **With dispatch loop (inline mode):** The “loop” can be a single iteration: parse argv, build one work item “run root with these args”, run it through the dispatcher (same timeout + progress as now), then exit. So the CLI binary stays one-shot; the “dispatcher” is just the shared function that runs one job with the same contract (timeout, progress to Coordinator). No new process or daemon required.
- **With dispatch loop (server mode):** A long-running process (e.g. “zqk serve” or the MCP server) runs the loop: accept requests (from stdin, socket, or MCP JSON-RPC), enqueue work, run via the same “run one job” path, stream progress to subscribers, return result. CLI could then be a thin client that submits to this server. Optional; not required for the first version.

So **minimal change for CLI:** Extract the “run one command with timeout + progress” into a **dispatcher.Run(ctx, workItem)** (or similar). `main` calls it with one item built from `os.Args`. Behavior stays the same; we just have a single, reusable entry point for “run a command.”

## MCP use

- **Today:** MCP tool call → `executeCLICommandWithContextForServer` → **subprocess** `exec(zqk, args...)` → wait for exit → capture stdout/stderr → parse JSON → return result. Progress from the subprocess is not visible to the MCP client; only the final result is.
- **With dispatch loop:** MCP tool call → **submit work item** to the same dispatcher (in-process). Dispatcher runs **in-process** `rootCmd.ExecuteContext(execCtx)` (or equivalent). Progress flows to the Coordinator; MCP server is (or registers) an **OperationalEventSubscriber** and forwards progress as MCP notifications (e.g. `notifications/event` with progress payload). When the job completes, return the same result to the tool caller. Benefits:
  - **Same code path** as CLI: one place for timeout, progress, and cancellation.
  - **Real-time progress** for MCP clients (e.g. “Waiting for list/count slot…”, “Operation in progress: object_list — storage: …”).
  - **No subprocess:** Fewer processes, no stdout/stderr parsing for result, and cancellation (e.g. client disconnect or timeout) cancels the same `execCtx` that the command uses.
  - **Easier testing:** In-process execution can be exercised without spawning the binary.

**Implementation note:** In-process execution for MCP requires that the MCP server have access to the same `rootCmd` and context setup (profile, project root, etc.) that the CLI uses. The dispatch loop would receive a “run CLI command” work item with command path + args and invoke the cobra tree in-process, with the same `BindAsyncProgress` / timeout hook semantics as the CLI entrypoint.

## Summary table

| Aspect        | CLI today              | CLI with loop (inline)     | MCP today              | MCP with loop                |
|---------------|------------------------|----------------------------|------------------------|------------------------------|
| Entry         | main → ExecuteContext  | main → dispatch.Run(1 item)| Tool → subprocess zqk  | Tool → dispatch.Run(1 item)  |
| Execution     | In-process             | In-process (same)          | Subprocess             | In-process (same as CLI)     |
| Progress      | Coordinator + heartbeat| Same                       | Lost (stderr buffered) | Coordinator → MCP notifs     |
| Timeout/cancel| execCtx                | Same                       | Kill subprocess        | Same execCtx                 |
| Result        | Exit code + output     | Same                       | Parse stdout JSON      | Return from Run()            |

## Optional: queue and worker pool

- **Queue:** Work items can be enqueued (e.g. by MCP or by a future “zqk serve” API); the loop pulls one item at a time (or N workers pull from the queue).
- **Rate limiting / priority:** The dispatcher can apply policy (e.g. max concurrent list/count already exists; we could cap “max concurrent CLI runs” per client or globally).
- **Observability:** One place to log “job started / progress / completed” and to attach metrics (latency, success/failure per op type).

This can be added later without changing the contract: the **work item** and **run with timeout + progress → Coordinator** stay the same.

## Implementation (current)

- **pkg/dispatch:** `WorkItem`, `Run(execCtx, item, runner)` and the progress+heartbeat logic live here so that both the CLI entrypoint and MCP in-process path use the same contract without creating an import cycle (pkg/cli → coordination → storage → pkg/cli).
- **cmd/zqk/root.go:** Builds one `WorkItem`, passes a runner that sets `rootCmd.SetArgs`/`SetContext` and calls `ExecuteContext`; `hook.WrapCommand` provides the timeout, then `dispatch.Run` runs the job with progress and heartbeat.
- **MCP in-process:** When the MCP server is started from `zqk mcp serve`, it sets an in-process CLI runner via `SetInProcessCLIRunner` and starts the dispatch pool. The runner is implemented in `cmd/zqk/mcp` (to avoid pkg/mcp importing pkg/dispatch): it serializes access to the shared `rootCmd` with a mutex, captures stdout/stderr, runs `dispatch.RunViaPool` (or `dispatch.Run` if the pool is not started) with `rootCmd.ExecuteContext`, then uses `mcp.BuildCLICommandResultFromOutput` to produce the same result shape as the subprocess path. Progress flows to the Coordinator so MCP clients can receive real-time progress (e.g. via event subscription).
- **internal/cli:** `RunWithAsyncProgress` keeps its own progress+heartbeat implementation for cobra commands (same behavior); it cannot call pkg/dispatch or pkg/cli for this without introducing a cycle.

### Dispatch pool (concurrent tool execution)

- **pkg/dispatch/pool.go:** A bounded worker pool (default 24 workers, queue 64) allows multiple tool runs to be in flight. The pool uses `goroutinelabels` with a dedicated budget so it works when the scheduler is not running (e.g. `zqk mcp serve` only).
- **Lifecycle:** MCP server calls `dispatch.StartGlobalPool(ctx)` before `Serve()` and `defer dispatch.StopGlobalPool()` so the pool runs for the lifetime of the server.
- **RunViaPool:** When the global pool is started, the in-process CLI runner uses `dispatch.RunViaPool` instead of `dispatch.Run`. Callers block until the job completes (result channel); the pool only bounds concurrency and queue depth.
- **CLI serialization:** The shared `rootCmd` is not safe for concurrent use. The in-process runner therefore holds a mutex for the duration of each in-process CLI run (buffer setup, `RunViaPool`, result build). So at most one in-process CLI execution runs at a time; the pool still provides a bounded queue and worker set for when the serve loop is extended to handle multiple concurrent requests by id.
- **Deadlock avoidance:** No locks are held while waiting for a pool slot or for the result; the result is received on a buffered channel after submitting the work. Lock ordering: `cliExecMu` is only taken inside the in-process runner; the pool’s internal locks are not held when calling the runner.

### Instance bootstrap (one storage/config per process)

When the process that runs the dispatcher starts (e.g. `zqk mcp serve`), we run a **single bootstrap** through the dispatcher so that storage, scheduler config, and logging config are created/loaded once and reused for all tool calls and in-process CLI runs. That way caches and config serve all data requests without re-creating per request.

- **What to bootstrap:** (1) **Storage** – one `ObjectStorageProvider` per project root, via `storage.NewStorageFactory` (file or graph backend). (2) **Scheduler config** – load once (e.g. same loader as scheduler daemon) and keep in memory so retention/observability code can read it without re-opening the file. (3) **Logging config** – ensure profile and router are set once (e.g. MCP profile); often already done at entry.
- **How:** Run bootstrap as a **single dispatch work item** at startup (e.g. `dispatch.Run(bootstrapCtx, item, runner)` before starting the pool). The runner creates storage, wires `OnStorageCreated` if set, and **registers the provider** in the process-wide cache used by `GetObjectStorageForCommand` (e.g. `cli.RegisterStorageForProjectRoot`). The MCP server then uses that same instance for `SetStorageProvider` and coordinator integration. In-process CLI commands that run later will get storage from the cache, so one instance serves both MCP handlers and CLI.
- **Scheduler config:** The bootstrap runner can load scheduler config (enabled, project_type) once and store it on the server or in a small bootstrap result struct so code that needs it (e.g. retention, future features) does not re-read the file. Optional in the first version.
- **Ordering:** Bootstrap runs before the pool starts (inline `Run`); then start the pool and serve. No concurrent bootstrap; all later work uses the bootstrapped instances.

### Instance context (application container)

The **instance context** (`dispatch.InstanceContext`) is the one-stop-shop for access to expensive, process-scoped components. It is created when a **long-running process** starts (scheduler or MCP serve) and cleared when that process exits. **One-shot CLI** (single command, no server) does not set instance context; it stays nil so existing paths (context, cache, globals) apply.

- **When it is set:** (1) **MCP serve** – bootstrap creates storage and registries, then `SetInstanceContext(ic)`; `ClearInstanceContext()` on defer when the server exits. (2) **Scheduler daemon** – when the scheduler starts (foreground or background), it creates storage, wires `OnStorageCreated`, registers storage in the CLI cache, builds the same `InstanceContext` with storage and registries, and calls `SetInstanceContext(ic)`; `defer ClearInstanceContext()` runs when `startScheduler` returns (interrupt or error). Single-agent, one-shot invocations (e.g. `zqk object list` from the shell with no scheduler and no MCP) never set it.
- **Contents:** Storage (`GetStorage()`), project root, profile, and a **registry** of loaders/registries keyed by constants (`KeySpecLoader`, `KeyLifecycleLoader`, `KeySynonymResolver`, `KeyKindMapper`, `KeyValidatorRegistry`, `KeyFieldRegistry`). Callers use `dispatch.GetInstanceContext().GetRegistry(dispatch.KeySpecLoader)` and type-assert to the concrete type (e.g. `*objects.SpecLoader`).
- **Use:** When running under scheduler or MCP, handlers and in-process CLI can call `dispatch.GetInstanceContext()` and use `GetStorage()` or `GetRegistry(key)`. When nil (one-shot CLI), code continues to use context, cache, and globals as today.
- **Rationale:** One application container per long-running process; one-shot stays lightweight without it.

### Future: zqk_session

- The concept of **mcp_session** (retention, observability) could be generalized to **zqk_session** so a single session represents a client using both MCP and the CLI (e.g. one identity, one set of metrics and retention). No implementation change in this step; consider when unifying session-scoped config and observability.

## References

- **Async progress and heartbeat:** `docs/architecture/CLI_ASYNC_AND_PROGRESS.md`
- **Coordinator and message bus:** `docs/architecture/STABILITY_FOR_MULTI_AGENT_AND_CLI_MESSAGE_BUS.md`
- **MCP event subscription:** `docs/process/architecture/MCP_EVENT_SUBSCRIPTION.md`, `pkg/mcp/mcp_event_subscriber.go`
- **CLI timeout and context:** `pkg/cli/timeout_hook_execution.go`, `cmd/zqk/root.go`
- **Coordinator:** `pkg/coordination/coordinator.go`
