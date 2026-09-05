# Agent Correspondence Feed Standard

**Last Verified:** 2026-08-31


**Status:** initiative (human steering, 2026-07-28); **Phase A Go-only ship path landed 2026-07-29**  
**Kernel plan:** `[REDACTED-ID]` (*Standardize agent correspondence feed*)  

**Related:** `docs/architecture/DATA_CELL_RUNTIME_ORGANISM.md` (`agent_feed` / delivery narrative), `scripts/mesh/README.md`, `agent_feed` object spec, MCP `tools_chat_responder.go`, `docs/process/architecture/mcp_proxy_daemon.md`

## Community default seating (out of the gate)

Greenfield `zqk system init` (and community edition) seeds a **default agent seating pack** so
`emit-status --persona-ref` works without Studio-only personas:

| Stable ID | Kind | Role field | Purpose |
|-----------|------|------------|---------|
| `PER-DEFAULT-OPERATOR` | persona | `operator` | Human / operator seat |
| `PER-DEFAULT-AGENT` | persona | `agent` | Peer agent seat |
| `ASK-DEFAULT-FEED-CORRESPONDENCE` | agent_skill | — | How to use feed steer / emit-status |

Templates live under `scripts/default_personas/` and `scripts/default_agent_skills/` (copied into
bootstrap archive; seeded by `SeedDefaultAgentSeatingPack`). Do not invent parallel role enums.

**If `feed ack` says persona not found:** the pack is missing (scrub / incomplete init / draft-only mint).
Repair: `zqk system seed-default-agent-seating`. `zqk feed doctor` reports `missing_default_seating`
when either `PER-DEFAULT-*` id is absent.

Example:

```bash
zqk feed emit-status --persona-ref PER-DEFAULT-OPERATOR --agent-id my-laptop --summary "ready"
zqk feed emit-status --persona-ref PER-DEFAULT-AGENT --agent-id worker-01 --summary "ACK"
```

## Ship path (native Go only)

**Launch / Open-Core acceptance does not require any shell script.** Operators use:

| Surface | Command |
|---------|---------|
| Cursor MCP entrypoint | `./bin/zqk mcp cursor-adapter --tcp 127.0.0.1:8443` (daemon: `zqk mcp daemon --tcp …`; legacy: `mcp proxy`) |
| Materialize lite policy | `zqk system materialize-agent-chat-channel --feed-id AGF-…` |
| Human / coordinator steer | `zqk feed steer --message "…"` (when `delivery_mode=notify\|paste`, also best-effort wakes the peer via the project wake script; `--no-wake` stamps JSONL only) |
| Mesh status stamp | `zqk feed emit-status --persona-ref PER-DEFAULT-OPERATOR\|PER-DEFAULT-AGENT --agent-id <unique-swarm-seat> --summary "…"` |



| IDE tool | MCP `chat_send` (shared writer: `pkg/agentfeed`) |

Legacy peer-wake / emit shell helpers and AppleScript paste are **non-ship** (optional diagnostics only).

Live Studio feed (example): rematerialize after creating/updating `agent_feed`; lite file `.zqk/config/agent_chat_channel.json` must show matching `feed_id` and `delivery_mode: notify`.

## Problem

TPM↔AGY (and future multi-agent) correspondence today is a **brittle stack of side channels**:

| Layer | What we have | Failure mode seen in mesh |
|-------|----------------|---------------------------|
| Process membership | Dual `PRI.backlog_item_refs` + `BLI.priority_plan_ref` (now migrating to child-owned + `zqk pplan add`) | Dangling refs; missing PRI CAS; false “bound” claims |
| Status bus | `.zqk/logs/mesh/tpm_agy_status.jsonl` + watchdog | Quiet bus; cooldown starvation (fixed per-direction) |
| Wake | AppleScript paste + agentapi notify (`wake-agy.sh` / `wake-cursor-tpm.sh`) | Accessibility deny (1002); focus steal; paste cutoff / confused ATTN lines |
| Chat channel | `.zqk/logs/cursor-hooks/agent_chat_channel.jsonl` + optional `agent_feed` materialize | Spurious / per-agent files; pull-only does not wake idle clients |
| Kernel contract | `WFL-TPM-AGY-MESH-001` (doc’d) | Object missing → blocked PRI updates |

Human steering via **IDE chat paste** is unreliable under load (cutoff, interleaved ATTN lines, no durable ack).

## Direction (enterprise-ready)

1. **One correspondence plane owned by ZQK**  
   - Canonical: **`agent_feed` (`AGF-*`)** in CAS + materialized lite policy (`.zqk/config/agent_chat_channel.json`) + **one** append-only events JSONL per feed (not per-agent ad-hoc files).  
   - Mesh status bus becomes a **producer** into the same feed (or a typed event stream), not a parallel private protocol.

2. **Delivery is policy, wake is transport**  
   - `delivery_mode`: `off | log | clipboard | paste | notify` (extend toward `mcp_notify`, later `webhook` / `messaging_bridge`).  
   - **End state:** live MCP notify wakes the peer; **IDE chat paste is not part of the control plane** and should be off when MCP is healthy.  
   - AppleScript / Terminal paste remains a **local fallback** for offline MCP only (today: short wake stubs; substance on feed).  
   - MCP must **wake / notify subscribed clients**; if the session is dead, **surface a human-visible fault** (do not silently stamp JSONL).

3. **Human steering ingress (no chat paste required)**  
   - CLI: `zqk feed steer --message "…"` writing a typed `steering` event onto the feed (optional `--feed-id` check).  
   - MCP `chat_send` shares `pkg/agentfeed.AppendEvent`.  
   - Enterprise: same event shape via Signal / Telegram / Teams / Slack / Messenger / VoIP bridges into the node.

4. **IDE-optional → API-primary (public or private)**  
   - **End state:** operators and swarm peers **do not need IDE agents**; they use **HTTP/API / MCP / CLI** as the primary surface (steer, wake, ack, correspondence).  
   - IDE chat paste is interim wake only; retire when MCP notify is healthy.  
   - IDE remains an **optional client** of the same `agent_feed` contract — not the control plane.  
   - Same event shape for enterprise messaging bridges (Signal / Telegram / Teams / Slack / …).
   - Private API: node-local or VPC; public API: authenticated multi-tenant edge in front of cloud nodes.

5. **Inter-agent correspondence rules live in the kernel**  
   - Workflow + glossary (mesh WFL/GLS) define roles, stages, ack semantics.  
   - Agents do not invent new feed files; they publish through `zqk` / MCP / HTTP APIs only.

## Architecture debt observed (document → fix)

| ID | Debt | Proposed fix |
|----|------|--------------|
| D1 | Phantom `object create` success with no CAS readback | Create→immediate `get` gate in mesh recipes; fail closed |
| D2 | Custom-ID / status=`ready` create flakiness | Prefer exploring→promote; investigate create abort after JSON print |
| D3 | Bidirectional PRI↔BLI membership | Done directionally: `pplan add` → child `priority_plan_ref` only |
| D4 | Focus-stealing paste as sole wake | **Done (default):** MCP/`delivery_mode=notify` + `--notify-only` ship path; paste opt-in only (`delivery_mode=paste` / `--chat`). Human alert on unrepaired disconnect remains Phase B. |
| D5 | Spurious chat/mesh log files per agent | Single `agent_feed` + materialize; retire parallel JSONL inventiveness |
| D6 | Missing `WFL-TPM-AGY-MESH-001` broke PRI updates | Recreate/restore workflow or drop required `workflow_ref` until restored |
| D7 | ATTN lines truncated / interleaved in chat; no peer receipt visibility | **Partial fix:** stable `event_id` + `peer_ack` / `delivery_receipt`; seat inbox/outbox on `workflow whats-next --agent-id`; `zqk feed ack`; **required** **`--await-peer-ack`** callback ringer on directed steers (`POL-AGENT-ORCH-HOURGLASS-001`); **chat paste minimized** (substance on feed; `MESH_WAKE_PASTE_FULL=1` escape). |
| D12 | “Comms check” conflated with transport (toast / `delivery_receipt` / `peer_wake_live`) | **Policy:** `POL-AGENT-COMMS-CHECK-001` — PASS = seat-authored life (nonce ack) ∧ work (kernel probe); transport alone is FAIL. Wired on `WFL-TPM-AGY-MESH-001` + mesh README / `tpm-agy-mesh-wake.mdc`. |
| D8 | “Shockwave” named for local policy checks | Rename or wire real `pkg/shockwave` for association cascades |
| D9 | Mega/plan BLIs with empty `category` (86/86 on PRI-bdc4dd36) | **Mesh readiness requires non-empty `category`** before VERIFIED; backfill existing; keep schema `required: false` until backfill done (avoid mass validation failure) |
| D10 | List index shows BLI (`BLI-MEGA-CHILD-010`) but `object get` / no CAS YAML | Same as D1 — VERIFIED needs `get` success; triage index ghosts separately |
| D11 | AGY chat says `status=complete` but kernel shows `exploring` / `error` | See **Status persistence** below — not a list lag; lifecycle + autofix |

## Event-driven gaps (2026-07-31) — unencumbered & reliable comms

**Kernel plan:** `[REDACTED-ID]` · **Req:** `REQ-COMMS-RELIABLE-001` · **Criteria:** `CRIT-COMMS-001`…`007`

Polling a TPM inbox on a timer (Cursor `/loop`, `tpm-correspondence-tick.sh`) is a **degraded fallback**, not the product model. Target shape matches scheduler job completion: **assign → register await on a named status event → callback notifies a live subscriber (or surfaces unrepaired fault)**.

| ID | Weakness | Fix direction (BLIs on PRI) |
|----|----------|-----------------------------|
| W1 | Poll-primary TPM duty cycles / tick shells as coordination | Retire poll loops; event awaits default (`CRIT-COMMS-001`) |
| W2 | Awaits mostly `peer_ack` only — not checkpoint / ATK done / job complete | Status-event await registry (`CRIT-COMMS-002`) |
| W3 | Callback often JSONL stamp / wake script — no guaranteed live TPM turn | **Partial (2026-08-06):** `ApplyCoordinatorMCPInterrupt` — MCP ActionRequired + ≥1 IDE subscriber ⇒ `peer_wake_live` / transport `mcp_action_required` (`CRIT-COMMS-003`, `BLI-COMMS-TPM-LIVE-WAKE-001`). Zero subscribers ⇒ `mcp_no_subscriber` unrepaired. Remaining: Cursor turn auto-start from ActionRequired (IDE waiter). |
| W4 | `CallbackJobID` / scheduler timeout on awaits not wired | Same shape as `agent next --on-validation-failure wake` (`CRIT-COMMS-004`) |
| W5 | Seat aliases (`antigravity-*` vs `peer-open-core-*`) split inbox/outbox | One correspondence identity (`CRIT-COMMS-005`) |
| W6 | No `feed doctor` for awaits / ready verdict | `zqk feed doctor` JSON (`CRIT-COMMS-006`) |
| W7 | Wakes/evidence leak to vendor paths (e.g. `.gemini`) | Stay on `.zqk` contracts (`CRIT-COMMS-007`) |

**Related:** Phase A feed standard `[REDACTED-ID]` (complete); MCP process ops `[REDACTED-ID]` (planning, `active_order=20`); this PRI `active_order=15` (planning — activate after CAS P0 or human reprioritize).

## Status persistence (why VERIFIED ≠ kernel status)

Observed (2026-07-28 TPM investigation; mega children / CHILD-079 class):

1. **`zqk object update --field status=complete` is blocked**  
   Manual status mutations are restricted (`cmd/zqk/object/update.go`). Agents must use **`object promote` / `demote`** only (studio policy: Native Orchestration Protocol). Lifecycle `--override` is a human interactive TTY snap-remedy and is **blocked for non-TTY/agent shells**. A failed update leaves prior status; chat can still claim COMPLETED.

2. **`promote` cannot jump `exploring` → `complete`**  
   Backlog lifecycle only allows `in_progress` → `complete` (needs `commit_refs`, etc.). From `exploring`, promote stops when preconditions fail (`actual_effort`, `priority_plan_ref`, `milestone_refs`, …).

3. **Create-with-`status=complete` can succeed without hold preconditions**  
   Probe: create as `complete` without `actual_effort`/`estimated_effort` works; later field updates then **fail** Tier-2 complete preconditions (effort must be **strings**). Easy to think “it’s complete” while follow-up updates silently fail.

4. **`system check` autofix demotes to `error`**  
   `cmd/zqk/system/check_impl_autofix.go`: any non-auto-fixable issue (except `integrity`) sets `status=error` and may persist. That is how accepted mega children with TRACE still show **`error`** in plan lists.

5. **`error` is a trap without recovery fields**  
   Lifecycle allows `error` → `planned` only (not → `complete`). Promote from `error` needs **`milestone_refs`** (and related planned preconditions). Demote from `error` also stuck. Recovery proof: CHILD-052 after `actual_effort`/`estimated_effort` + `milestone_refs` → promote → `planned`.

**AGY recipe before claiming VERIFIED status=complete:**

```bash
# Prefer honest create at exploring, then:
zqk object update <BLI> --field 'actual_effort="0.5"' --field 'estimated_effort="0.5"'
zqk object update <BLI> --add-ref 'milestone_refs=MIL-…'   # if missing
zqk object update <BLI> --field 'commit_refs=["<sha>"]'    # or equivalent
zqk pplan add <PRI> <BLI>
zqk object promote <BLI>   # repeat until complete, or report stuck status
zqk object get <BLI> --format json   # MUST show status=complete before VERIFIED
```

Do **not** claim `status=complete` from create JSON alone without a subsequent successful `get`.

## BLI claim readiness (TPM accept / AGY VERIFIED)

Before claiming **DELIVERABLE … COMPLETED & VERIFIED** (and before TPM ACCEPT), all of:

1. **Create → `zqk object get <BLI-id>` succeeds** (fail closed on phantom create / index ghost).  
2. **`priority_plan_ref` bound** via `zqk pplan add <PRI> <BLI>` (child-owned).  
3. **Non-empty `category`** (findability). Prefer package-aligned values: `Storage`, `MCP`, `CAS`, `Mesh`, `Process`, `Platform`. Set at create or before claim:  
   `zqk object update <BLI-id> --field category=<Value>`.  
4. **`status=complete` on `object get`** (not chat alone). Use promote path + effort strings + milestone as in **Status persistence**; never rely on blocked `update --field status=…`.  
5. **`commit_refs`** TRACE on `origin/main`.  
6. Path-match of claimed deliverables in the TRACE commit/PR.

Empty `category` is a **reject / soft-fail** for new claims even if TRACE is green. Kernel `status≠complete` is a **reject** even if TRACE is green (D11).

## Phased plan

### Phase A — Standardize local feed (near-term) — **shipped (Go-only)**

- Single writer: `pkg/agentfeed.AppendEvent` used by MCP `chat_send`, `zqk feed steer`, `zqk feed emit-status`.  
- Materialize: `zqk system materialize-agent-chat-channel --feed-id AGF-…` (`delivery_mode=notify`).  
- Cursor entrypoint: `zqk mcp cursor-adapter` (stdio MCP face + private daemon session; not fat-stdio `mcp serve`, not legacy transparent `mcp proxy`).  
- Shell wake/emit scripts: **non-ship legacy** only.

### Phase B — MCP client health

- Heartbeat / session lease for Cursor & AGY terminals.  
- On unrepaired break: notify human (log + optional OS notify); stop pretending wake succeeded.

### Phase C — Enterprise messaging bridges

- Pluggable ingress adapters (Slack/Teams/Telegram/Signal/…): map to the same steering event schema.  
- Cloud node: feed + bridges run as services; IDE paste optional for operators on the box.

### Phase D — IDE-optional API primary interactions

- First-class **HTTP/API** for primary agent I/O (steer, wake, health, feed tail/ack) without Cursor or any IDE.  
- Same event schema as CLI/`agent_feed`; reference non-IDE client.  
- Document public vs private deployment (auth, TLS, tenancy). Kernel: `[REDACTED-ID]` (recreated after phantom CAS loss of `731f48bd`).

**Private API (landed):** `zqk feed serve --listen 127.0.0.1:8787 [--token …]`

| Method | Path | Notes |
|--------|------|--------|
| GET | `/v1/health` | No auth |
| POST | `/v1/feed/steer` | JSON body: `message`, optional `agent_id`, `to_agent_id`, `feed_id`, `no_ack`, `await_peer_ack` |
| POST | `/v1/feed/ack` | JSON: `in_reply_to`, `agent_id`, `persona_ref`, optional `summary` |
| GET | `/v1/feed/pending` | Query: `agent_id` (required), `persona_ref`, `limit` |

Bearer token optional via `--token`. Public/multi-tenant edge + wake-over-HTTP remain follow-ups.

### Phase E — Multi-agent parallelism (scale AGY)

- **One coordinator, N workers**: do not run N independent TPM↔AGY paste loops.  
- Fan-out via `zqk agent orchestrate` / swarm+shockwave workers bound to **personas** (coder, reviewer, AST, architect).  
- Partition work by BLI / package / gate; coalesce accepts on the **single** `agent_feed` for TPM.  
- Kernel: `[REDACTED-ID]` (depends on Phase A feed).

## Non-goals

- Replacing CVS / scheduler measurement with chat.  
- Using `agent_feed` alone as the full orchestration loop (see DATA_CELL narrative).

## References

- `DATA_CELL_RUNTIME_ORGANISM.md` — delivery vs orchestration vs observability  
- `scripts/mesh/README.md` — current TPM↔AGY bus  
- `pkg/mcp/tools_chat_responder.go` — JSONL + MCP `ActionRequired` wake seed  
- Object spec: `docs/process/_internal/object_specs/agent_feed.yaml`
