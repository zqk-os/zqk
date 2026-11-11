# Show HN: ZQK — a microkernel OS for autonomous agents (local-first, capability-secured)

**Tagline:** Stop building agents on prompt wrappers. ZQK is a host-level state machine that treats the LLM as an unprivileged inference device.

---

## The problem

Every production AI agent hits the same wall:

- **Non-determinism:** The LLM decides what state to enter, what tool to call, and where to route next. When it hallucinates, your agent enters undefined behavior.
- **Memory bloat:** Agent "state" lives in the prompt. After 20+ turns, you're spending thousands of tokens reconstructing context that should be in a database.
- **No real security:** "Capability" means a system prompt saying "don't delete things." The LLM can be jailbroken. There's no kernel enforcing anything.
- **No multi-agent coordination:** Two agents sharing a task means coordinating via prompt injection. Race conditions and overwrite conflicts are invisible.

---

## What ZQK does differently

```
Traditional:   LLM → decides next tool → decides next state → hopes for the best
ZQK:           Host FSM → calls LLM as a plugin → receives structured output → drives next state
```

**The kernel, not the model, is in control.**

### Deterministic host FSM
State transitions live in Go code (`pkg/pipeline/orchestrator.go`). The LLM returns structured data — the FSM decides what happens next. No `next_state` from model strings. Ever.

### Capability-secured operations
Every storage operation goes through a role enforcement gate. Admin operations require an explicit key. Agents can't escalate privilege via a clever prompt.

### Kernel-managed state (not prompt state)
Task state lives in a structured graph store (Neo4j/MemGraph) — not in the conversation. `zqk workflow whats-next` always gives the agent accurate current state in ~50 tokens, regardless of session length.

### Native MCP (JSON-RPC 2.0)
The community binary exposes `tools/list` and `tools/call` over stdio and TCP. Works with Cursor, Claude, VS Code, and any MCP client out of the box.

---

## Apache / Enterprise delineation ACK

| Tier | License | What you get |
|------|---------|--------------|
| **Community kernel** | **Apache License 2.0** (`LICENSE` / `NOTICE`) | Single-node cell: CLI, MCP, scheduler, local graph/CAS |
| **Enterprise** | **Separately licensed** (commercial) | Multi-node fleet mesh, RBAC, cross-cell compliance, sealed fleet audit |

Community Apache ≠ Enterprise entitlement. Evaluating the open-core tree does not grant Enterprise modules.

## Quick start (< 2 min)

Aligned with [Community First-Run](../onboarding/COMMUNITY_FIRST_RUN.md) and [60-second capability demo](./SIXTY_SECOND_CAPABILITY.md):

```sh
# Install (public binary → go install → source; no GITHUB_TOKEN required)
curl -sSL https://raw.githubusercontent.com/zqk-os/zqk/main/scripts/install.sh | sh

# First agent:
mkdir my-project && cd my-project
zqk init
zqk workflow whats-next          # kernel tells agent current mission
zqk mcp ensure                   # wire Cursor / Claude Code
```

Buyer smoke (~60s): see [docs/demos/SIXTY_SECOND_CAPABILITY.md](./SIXTY_SECOND_CAPABILITY.md).

---

## Comparison

| | LangChain / CrewAI | AutoGen | **ZQK** |
|--|-------------------|---------|---------|
| State lives in | Prompt text | Prompt text | **Kernel (graph store)** |
| State machine | LLM decides | LLM decides | **Host FSM (Go code)** |
| Capability enforcement | System prompt | System prompt | **Kernel gate** |
| Multi-agent coordination | Prompt injection | Message passing | **Shared kernel (atomic CAS)** |
| Init time | 38–52s | 52s+ | **~180ms** |
| Memory (idle agent) | 95–120MB | 120MB+ | **~18MB** |
| MCP support | Plugin | Plugin | **Native (first-class)** |
| Language | Python | Python | **Go (precompiled binary)** |

---

## Who this is for

- **Infra/backend engineers** burned by non-deterministic agent loops in production
- **Local-first / privacy teams** running Ollama, Apple Silicon, local GPU (no cloud required)
- **Platform teams** building multi-agent systems that need real coordination, not prompt negotiation

---

## What's in the box

- `zqk system init` — scaffold structured project (goals, plans, backlog, milestones)
- `zqk workflow whats-next` — kernel-driven mission discovery (no prompt reconstruction)
- `zqk scheduler` — background task runner with retry, concurrency limits, test bundling
- `zqk mcp serve --stdio` — MCP JSON-RPC 2.0 server (Cursor/Claude/VS Code)
- `zqk object` — full CRUD for all kernel object types
- `pkg/pipeline/orchestrator.go` — embeddable host FSM for your own agent workflows

---

## Open-core model

| Tier | License | Contents |
|------|---------|----------|
| Community kernel | **Apache License 2.0** | Single-node kernel, MCP, scheduler, local graph/CAS |
| Enterprise | Separately licensed (commercial) | Multi-node fleet mesh, enterprise IdP/RBAC, sealed fleet audit |

See the ACK table above. No MIT community SKU remains on this trust surface.

---

## Links

- [Architecture](../architecture/README.md) — host FSM · capability flow · paging
- [Guides & Recipes](../guides/README.md) — caged worker · MCP DB agent · long-running research
- [60-second capability demo](./SIXTY_SECOND_CAPABILITY.md)
- [Quickstart](../../README.md#quickstart--2-min)
- [Community First-Run](../onboarding/COMMUNITY_FIRST_RUN.md)
- [Security Primitives & Sanitation](../guides/SECURITY_PRIMITIVES_AND_SANITATION_GUIDE.md)

---

*Built by the ZQK team. Local-first, deterministic, and production-ready for AI + human hybrid teams.*
