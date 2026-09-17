# Stability for Multi-Agent Workflow and CLI Message Bus

**Last Verified:** 2026-08-31


**Goal:** Get the kernel stable so we can run a **multi-agent workflow** where agents trigger operations according to **privileges and role**, and agents **communicate with each other via a CLI message bus**.

## Target Model

- **Multi-agent workflow:** Multiple agents (human, AI, or automated) perform work; they may run in parallel or in sequence, and need to coordinate.
- **Privileges and role:** Every operation is executed under a **security context** (account, roles, permissions). Agents get a context and can only trigger operations they’re allowed to perform (enforced at MCP and storage).
- **CLI message bus:** A single, consistent channel for **status, progress, and outcome** of CLI-triggered work. Agents (and humans) can **subscribe** to events so they see what’s happening and can react (e.g. “agent B waits for agent A’s list to finish” or “orchestrator sees completion and triggers the next step”).

## What “CLI message bus” is today

The **message bus** is the existing **event/coordinator** path, used by every CLI command that goes through the async progress pattern:

1. **CLI commands** (via `BindAsyncProgress`) run with a **timeout context** and a **progress callback** on context. They emit **started**, **progress/status** (including heartbeat every 5s), and **complete/error** through the **Coordinator** (`pkg/coordination`).
2. **Coordinator** routes every event to:
   - **Logging** (user-visible messages, e.g. “Waiting for list/count slot…”)
   - **Audit** (if configured)
   - **Metrics**
   - **Operational subscribers** (any component that subscribed for operational events)

So today:

- **Producers:** CLI (and anything that uses the same coordinator) emits status/progress/outcome.
- **Bus:** Coordinator + routers.
- **Consumers:** Logging (and thus terminal/log files), audit, metrics, and any **OperationalEventSubscriber** (e.g. an MCP layer that forwards to agents, or a future agent relay).

There is no separate “CLI message bus” process; the bus is **in-process** and used by every async CLI command. For multi-agent, agents can:

- **Trigger operations** via MCP (CLI bridge) or by invoking the CLI, with **role/privs** enforced by SecurityContext.
- **Observe** what’s happening via the same events (today: logging/metrics; future: explicit subscriptions or agent-to-agent relay).

## What we’ve done for stability (no silent hangs, regular feedback)

So that multi-agent workflows don’t get stuck or leave agents (or humans) without context:

| Concern | Mechanism |
|--------|-----------|
| **No indefinite hangs** | Timeout context is passed into every CLI command; blocking calls (e.g. list/count semaphore) use it and return on timeout. |
| **Context before blocking** | Before any blocking point (e.g. `AcquireListCountSlot`), we emit a progress message (e.g. “Waiting for list/count slot…”). |
| **Regular interval feedback** | Progress heartbeat in `RunWithAsyncProgress`: every 5s we emit “Operation in progress: &lt;op&gt; — &lt;stage&gt;: &lt;message&gt;” so the bus always has recent status. |
| **Single path for progress** | All progress flows through the context callback → ProgressHelper → Coordinator, so logging and any subscriber see the same events. |

This gives a **stable, predictable** basis: every CLI-triggered operation feeds the same bus on a regular interval and at meaningful state changes, with bounded run time.

## Role and privileges (existing)

- **MCP:** Tools are exposed with **security context**; the bridge filters by role/privilege so each agent only sees and can call what it’s allowed to.
- **Storage / object operations:** Enforce **SecurityContext** (account, roles, permissions) so create/read/update/delete respect privileges.
- **Scheduler:** Jobs run with a context; permissions and cancellation are defined per job.

So “agents trigger operations based on privs and role” is already supported at MCP and storage; stability work (timeout, progress, heartbeat) makes those operations observable and non-hanging on the bus.

## Gaps and next steps for multi-agent + message bus

Documented elsewhere; summarized here so this doc is a single entry point:

1. **Agent-to-agent messaging**  
   Today agents don’t have a direct “send message to agent B” channel. They coordinate only via **server events** (e.g. what the coordinator emits). **Future:** e.g. `agents/message` + `agent.message.received` (see `MCP_MULTI_AGENT_ORCHESTRATION.md`).

2. **Agent registration / capability negotiation**  
   Agents are identified by security context; there’s no explicit **agent registration** or capability advertisement. **Future:** e.g. `agents/register`, `agents/list`, `agents/capabilities` so the bus (or orchestrator) knows who can do what.

3. **Explicit “CLI message bus” API for agents**  
   Agents could subscribe to **operational events** (e.g. “object_list completed”, “object create started”) so they can react in workflows. Today they’d do that via MCP event subscription or by observing the same coordinator-backed events; a thin “message bus” API (subscribe to topics, get status/progress/outcome) could formalize that.

4. **Dispatch loop and dispatcher / worker pool (optional)**  
   A **dispatch loop** is the single place that accepts work (CLI or MCP), runs it with timeout and progress, and emits to the same coordinator; **MCP** can use it for in-process execution and real-time progress. For many concurrent agents or central rate limiting, a **dispatcher** (queue + worker pool) would sit in front of that loop; the bus would still be the same. See **`docs/architecture/DISPATCH_LOOP_AND_MCP.md`** for the outline and `CLI_ASYNC_AND_PROGRESS.md` (dispatcher pool section).

## References

- **Dispatch loop and MCP:** `docs/architecture/DISPATCH_LOOP_AND_MCP.md`
- **Async progress and heartbeat:** `docs/architecture/CLI_ASYNC_AND_PROGRESS.md`
- **Pre-change checklist (CLI stability):** `docs/architecture/PRE_CHANGE_CHECKLIST.md` (section 12)
- **AI agent communication channels:** `docs/architecture/ai-agent-communication-channels-v1.0.md`
- **MCP multi-agent orchestration:** `docs/architecture/MCP_MULTI_AGENT_ORCHESTRATION.md`
- **Coordinator (event routing):** `pkg/coordination/coordinator.go`
- **MCP security / role:** `pkg/mcp/config_security.go`, `pkg/mcp/role_guidance.go`
