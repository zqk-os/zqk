# ZQK Architecture — Host FSM for Autonomous Agents

> **The microkernel OS for autonomous agents. Local-first, capability-secured, deterministic.**

---

## Core model: LLM as unprivileged device

Most agent frameworks put the LLM in control — it decides what tool to call, what state to enter, and where to route next. ZQK inverts this:

```
Traditional frameworks:        ZQK:
──────────────────────         ──────────────────────────────────
LLM decides → Tools run        Host FSM decides → LLM is called
LLM decides → Next state       Code drives state → LLM is data
Prompt = security boundary     Kernel enforces capability handles
```

The LLM is an **inference device** — like a GPU. The host runs the state machine.

---

## Host FSM — deterministic event loop

```
                   ┌─────────────────────────────────────────────┐
                   │              ZQK Host FSM                    │
                   │                                              │
  trigger ────────▶│  pre_processing                              │
                   │       │                                      │
                   │       ▼                                      │
                   │  agent_dispatch ──── capability check        │
                   │       │              (role_enforcement.go)   │
                   │       ▼                                      │
                   │  agent_execution ─── LLM call               │
                   │       │              (timeout: 300s)         │
                   │       │              structured output only  │
                   │       ▼                                      │
                   │  handoff_verification                        │
                   │       │                                      │
                   │       ▼                                      │
                   │  post_processing ──▶  success / failure      │
                   │                                              │
                   └─────────────────────────────────────────────┘
```

**Key invariants:**
- Stage transitions driven by code (`pkg/pipeline/orchestrator.go`), never by LLM string output
- LLM output enters as structured data — the FSM decides what happens next
- Every stage is timeout-bounded; no unbounded inference loops

---

## Capability flow

```
Agent identity
     │
     ▼
Context profile (--context ai-agent | human | system)
     │
     ▼
Role enforcement gate ─── pkg/storage/role_enforcement.go
     │
     ├── PASS ──▶ storage operation proceeds
     │
     └── FAIL ──▶ error returned (no side effects)

Admin operations:
     ZQK_ADMIN_API_KEY=account:system  ──▶  privileged tier only
```

---

## Context paging — hot vs cold

```
Agent working memory (hot context):
  ┌─────────────────────────────────┐
  │  Active task window             │
  │  Current sprint + BLI refs      │  ← bounded, kernel-managed
  │  Live object IDs                │
  └─────────────────────────────────┘

Cold episodic store (graph-backed):
  ┌─────────────────────────────────┐
  │  Completed task history         │
  │  Prior session decisions        │  ← Neo4j / MemGraph (roadmap)
  │  Long-running research results  │
  └─────────────────────────────────┘

Eviction: kernel promotes hot → cold before token window blowup
```

---

## MCP integration

```
AI Tool (Cursor / Claude / VS Code)
         │
         │  JSON-RPC 2.0 (stdio or TCP)
         ▼
  zqk-community mcp serve --stdio
         │
         │  tools/list → enumerate kernel tools
         │  tools/call → kernel executes, returns structured result
         ▼
  ZQK Knowledge Kernel
  (object CRUD, workflow orchestration, scheduler, graph store)
```

---

## Key packages

| Package | Role |
|---------|------|
| `pkg/pipeline/orchestrator.go` | Host FSM — deterministic stage machine |
| `pkg/storage/role_enforcement.go` | Capability gate on all storage ops |
| `pkg/llm/client.go` | LLM client — unprivileged, timeout-enforced |
| `pkg/mcp/handler.go` | MCP JSON-RPC 2.0 server |
| `pkg/scheduler/` | Background task scheduler + test runner |
| `pkg/graph/` | Graph store provider (Neo4j / MemGraph) |
| `pkg/zqkenv/` | Environment config + admin guard |
