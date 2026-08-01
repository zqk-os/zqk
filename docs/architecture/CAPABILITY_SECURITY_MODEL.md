# ZQK Capability Security Model

**BLI:** `BLI-EXAMPLE` | **Plan:** `PRI-EXAMPLE`
**Source:** Open-Core roadmap invariants — `docs/strategy/open-core/OPEN_CORE_ROADMAP.md`

---

## Invariants (from Open-Core roadmap)

1. **LLM is an unprivileged inference device** — the host FSM drives state; LLM output is data, never a routing key.
2. **Capability handles are kernel-managed** — not raw API keys or prompt strings in agent memory.
3. **Deny-by-default** — agents operate within explicitly granted capability surfaces.
4. **Timeouts enforced at host level** — no unbounded LLM calls.

---

## What ZQK enforces today

### 1. Host FSM — deterministic, not model-driven

**Location:** `pkg/pipeline/orchestrator.go`

Stage transitions are code-driven `switch` statements on `current_stage`:

```
pre_processing → agent_dispatch → agent_execution → handoff_verification → post_processing → success/failure
```

LLM output is **never** used as a `nextStage` routing key. The orchestrator calls the LLM as a plugin (`PipelinePlugin`) and receives structured output — the FSM decides what happens next based on exit code and kernel state.

**Red-flag check:** ✅ No `next_state` read from model strings found in `pkg/pipeline/` or `pkg/scheduler/`.

### 2. LLM timeouts — enforced at client boundary

**Location:** `pkg/llm/client.go`

| Setting | Value |
|---------|-------|
| `LLM_TIMEOUT` env var | Default 300s (configurable) |
| `ResilientClient` hard cap | 15 minutes |
| Retry backoff | 2s exponential |

All LLM calls are wrapped in `ResilientClient` which enforces the timeout. Unbounded inference loops are **not possible** — the host terminates them.

### 3. Agent capability surface — role enforcement

**Location:** `pkg/storage/role_enforcement.go`, `pkg/zqkenv/agent_guard.go`

- **Role enforcement** (CHILD-104): agents are bound to declared roles; storage operations check role before executing.
- **Agent guard** (`ZQK_ADMIN_API_KEY`): privileged operations (e.g., `generate-agent-configs`) require explicit admin key — not available to unprivileged agent persona.
- **Context profiles**: `--context ai-agent` vs `--context human` surfaces different capability tiers in CLI output and operation gates.

### 4. Agent rules manifest — allowlist

**Location:** `pkg/agentrules/manifest.go`

Tracks allowlisted agent rule files (`.cursor/rules/*.mdc`). Any rule file not in the manifest is flagged as unauthorized. This prevents unapproved agent behavior injection via rule files.

### 5. Scheduler capacity limits — no resource starvation

**Location:** `.zqk/config/package_concurrency_limits.json`

Package-level test concurrency is bounded. Agents cannot schedule unbounded parallel work that starves the host.

---

## Gaps (v0.1 scope — not yet implemented)

| Gap | Description | Target |
|-----|-------------|--------|
| Unforgeable kernel handles | Capability tokens are currently role/context strings, not cryptographic handles | v0.2 |
| Public capability API | No external-facing `zqk capability grant/revoke` command | v0.2 |
| Per-agent resource quotas | CPU/memory quotas not enforced at kernel level | v0.2 |

---

## For OSS consumers — what you get today

```sh
# Agents run under declared role contexts:
zqk object create backlog_item --context ai-agent   # ai-agent tier
ZQK_ADMIN_API_KEY=account:system ./bin/zqk-admin    # system/admin tier only

# Role enforcement on every storage operation:
# pkg/storage/role_enforcement.go enforces role before any write

# LLM calls are always timeout-bounded:
# export LLM_TIMEOUT=120   # override default 300s cap

# Agent rule manifests prevent unauthorized rule injection:
# .zqk/config/internal/agent_rules_manifest.yaml
```

---

## Architecture diagram

```
┌─────────────────────────────────────────────────────┐
│                   ZQK Host FSM                       │
│  ┌──────────┐  ┌──────────┐  ┌───────────────────┐  │
│  │ pre_proc │→ │  agent   │→ │ handoff_verify    │  │
│  └──────────┘  │ dispatch │  └───────────────────┘  │
│                └─────┬────┘                          │
│                      │ structured output only        │
│                      ↓                               │
│              ┌───────────────┐                       │
│              │  LLM Plugin   │  ← unprivileged       │
│              │ (timeout=300s)│    inference device   │
│              └───────────────┘                       │
│  Role enforcement on every storage op               │
│  Agent guard gate for admin operations              │
└─────────────────────────────────────────────────────┘
```
