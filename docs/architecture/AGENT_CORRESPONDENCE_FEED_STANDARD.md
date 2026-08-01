# Agent Correspondence Feed Standard

**Status:** initiative (human steering, 2026-07-28)  
**Kernel plan:** `PRI-EXAMPLE` (*Standardize agent correspondence feed*)  

**Related:** `docs/architecture/DATA_CELL_RUNTIME_ORGANISM.md` (`agent_feed` / delivery narrative), `scripts/mesh/README.md`, `agent_feed` object spec, MCP `tools_chat_responder.go`

## Problem

TPM↔AGY (and future multi-agent) correspondence today is a **brittle stack of side channels**:

| Layer | What we have | Failure mode seen in mesh |
|-------|----------------|---------------------------|
| Process membership | Dual `PRI.backlog_item_refs` + `BLI.priority_plan_ref` (now migrating to child-owned + `zqk pplan add`) | Dangling refs; missing PRI CAS; false “bound” claims |
| Status bus | `.zqk/logs/mesh/community_status.jsonl` + watchdog | Quiet bus; cooldown starvation (fixed per-direction) |
| Wake | AppleScript paste + agentapi notify (`wake-agy.sh` / `zqk-community.sh`) | Accessibility deny (1002); focus steal; paste cutoff / confused ATTN lines |
| Chat channel | `.zqk/logs/cursor-hooks/agent_chat_channel.jsonl` + optional `agent_feed` materialize | Spurious / per-agent files; pull-only does not wake idle clients |
| Kernel contract | `WFL-TPM-AGY-MESH-001` (doc’d) | Object missing → blocked PRI updates |

Human steering via **IDE chat paste** is unreliable under load (cutoff, interleaved ATTN lines, no durable ack).

## Direction (enterprise-ready)

1. **One correspondence plane owned by ZQK**  
   - Canonical: **`agent_feed` (`AGF-*`)** in CAS + materialized lite policy (`.zqk/config/agent_chat_channel.json`) + **one** append-only events JSONL per feed (not per-agent ad-hoc files).  
   - Mesh status bus becomes a **producer** into the same feed (or a typed event stream), not a parallel private protocol.

2. **Delivery is policy, wake is transport**  
   - `delivery_mode`: `off | log | clipboard | paste | notify` (extend toward `mcp_notify`, later `webhook` / `messaging_bridge`).  
   - AppleScript paste remains a **local fallback**, not the primary contract.  
   - MCP must **wake / notify subscribed clients**; if the session is dead, **surface a human-visible fault** (do not silently stamp JSONL).

3. **Human steering ingress (no heavy paste)**  
   - CLI: e.g. `zqk feed steer --feed-id AGF-… --message "…"` (or `zqk agent steer`) writing a typed `steering` event onto the feed.  
   - Optional: short Cursor command / MCP tool that only appends steering (not free-form chat floods).  
   - Enterprise: same event shape via Signal / Telegram / Teams / Slack / Messenger / VoIP bridges into the node.

4. **IDE-optional primary path (public or private API)**  
   - Operators may **forego an IDE** and use a **public or private HTTP/API** as the primary surface for agent interactions (steer, wake, ack, correspondence).  
   - IDE / AppleScript paste / local MCP remain **optional clients** of the same `agent_feed` contract — not the control plane.  
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
| D4 | Focus-stealing paste as sole wake | MCP `ActionRequired` / notify path first; paste fallback; human alert on unrepaired disconnect |
| D5 | Spurious chat/mesh log files per agent | Single `agent_feed` + materialize; retire parallel JSONL inventiveness |
| D6 | Missing `WFL-TPM-AGY-MESH-001` broke PRI updates | Recreate/restore workflow or drop required `workflow_ref` until restored |
| D7 | ATTN lines truncated / interleaved in chat | Steering events with ids + ack; mesh summary ≠ full payload |
| D8 | “Shockwave” named for local policy checks | Rename or wire real `pkg/shockwave` for association cascades |
| D9 | Mega/plan BLIs with empty `category` (86/86 on PRI-bdc4dd36) | **Mesh readiness requires non-empty `category`** before VERIFIED; backfill existing; keep schema `required: false` until backfill done (avoid mass validation failure) |
| D10 | List index shows BLI (`BLI-MEGA-CHILD-010`) but `object get` / no CAS YAML | Same as D1 — VERIFIED needs `get` success; triage index ghosts separately |
| D11 | AGY chat says `status=complete` but kernel shows `exploring` / `error` | See **Status persistence** below — not a list lag; lifecycle + autofix |

## Status persistence (why VERIFIED ≠ kernel status)

Observed (2026-07-28 TPM investigation; mega children / CHILD-079 class):

1. **`zqk object update --field status=complete` is blocked**  
   Manual status mutations are restricted (`cmd/zqk/object/update.go`). Agents must use **`zqk object promote`** (or human-approved `--override --reason-code=…`). A failed update leaves prior status; chat can still claim COMPLETED.

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

### Phase A — Standardize local feed (near-term)

- Inventory all writers of mesh/chat JSONL; route through `agent_feed` materialize + one events path.  
- TPM/AGY recipes: emit status → feed event → MCP notify → paste only if `delivery_mode=paste`.  
- Human: `zqk feed steer` (or equivalent) for durable steering without IDE paste.

### Phase B — MCP client health

- Heartbeat / session lease for Cursor & AGY terminals.  
- On unrepaired break: notify human (log + optional OS notify); stop pretending wake succeeded.

### Phase C — Enterprise messaging bridges

- Pluggable ingress adapters (Slack/Teams/Telegram/Signal/…): map to the same steering event schema.  
- Cloud node: feed + bridges run as services; IDE paste optional for operators on the box.

### Phase D — IDE-optional API primary interactions

- First-class **HTTP/API** for primary agent I/O (steer, wake, health, feed tail/ack) without Cursor or any IDE.  
- Same event schema as CLI/`agent_feed`; reference non-IDE client.  
- Document public vs private deployment (auth, TLS, tenancy). Kernel: `BLI-EXAMPLE`.

### Phase E — Multi-agent parallelism (scale AGY)

- **One coordinator, N workers**: do not run N independent TPM↔AGY paste loops.  
- Fan-out via `zqk agent orchestrate` / swarm+shockwave workers bound to **personas** (coder, reviewer, AST, architect).  
- Partition work by BLI / package / gate; coalesce accepts on the **single** `agent_feed` for TPM.  
- Kernel: `BLI-EXAMPLE` (depends on Phase A feed).

## Non-goals

- Replacing CVS / scheduler measurement with chat.  
- Using `agent_feed` alone as the full orchestration loop (see DATA_CELL narrative).

## References

- `DATA_CELL_RUNTIME_ORGANISM.md` — delivery vs orchestration vs observability  
- `scripts/mesh/README.md` — current TPM↔AGY bus  
- `pkg/mcp/tools_chat_responder.go` — JSONL + MCP `ActionRequired` wake seed  
- Object spec: `docs/architecture/_internal/object_specs/agent_feed.yaml`
