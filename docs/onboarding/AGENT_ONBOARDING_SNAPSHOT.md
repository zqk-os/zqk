# Agent onboarding snapshot — current focus

**Doc entry:** `DOC-1775197432882643000-89463915` — `zqk object get DOC-1775197432882643000-89463915` (or `zqk object list doc_entry --filter group=onboarding`).

**As of:** 2026-08-11
**Replaces:** Rolling “latest dated summary” in `docs/onboarding/` (see [`AGENT_ONBOARDING_SUMMARIES_DIGEST.md`](./AGENT_ONBOARDING_SUMMARIES_DIGEST.md) and [`archive/`](./archive/)).

**Purpose:** Short, **re-verify**-friendly view of plan + convergence + what to run before acting. Numbers below are **examples from the last explicit review** — always confirm with **`zqk object …`**.

---

## Norms (unchanged)

CLI-first, object-first, **PRE_CHANGE_CHECKLIST** before substantive code, **AGENT_GUIDELINES** for output/logging, **`AGENT_CONTEXT_REFRESH.md`** for generated reminders (**glossary** / `glossary_term` + **semantic density / section 13** post-verify after logic works; runbook **`scripts/README.md`**), scheduler-backed long tests, convergence evidence-first. Full detail: [`AI_AGENT_ONBOARDING.md`](./AI_AGENT_ONBOARDING.md).

**Cross-vendor mesh (Go-only ship):** `zqk feed steer` / `zqk feed emit-status` / MCP **`chat_send`** → `.zqk/logs/cursor-hooks/agent_chat_channel.jsonl` (policy: `.zqk/config/agent_chat_channel.json`, materialize via `zqk system materialize-agent-chat-channel`). Cursor MCP entrypoint: **`zqk mcp cursor-adapter`** → private session to daemon TCP (legacy: `mcp proxy`). Shell wake/emit scripts are non-ship legacy.

---

## Live pointers (re-verified 2026-08-11)

| Item | Live ID / note |
|------|----------------|
| **Mission (archived legacy)** | `MIS-1775446507801844000-42ba7fc8` — archived; keep for history only |
| **Mission (active)** | `MIS-1786213402170185000-8d36a3c8` — *Reliable kernel control plane (MMORCH + CAP)* |
| **Priority plan (whats-next primary)** | `PRI-CAS-MEMBRANE-ENFORCE-001` — *CAS membrane enforce — PrivilegedWriter fail-closed + path deny* — **`status=active`** |
| **Priority plan (parallel)** | `PRI-AGENT-IDLENESS-ACCUMULATOR-001` — *Agent idleness accumulator — scoreboard + reduction CVS* — `status=active` |
| **Active convergence** | `REDACTED`, `REDACTED`, `REDACTED`, `REDACTED`, `CVS-AGENT-IDLENESS-REDUCTION-001` |
| **Onboarding prep (this Cursor session)** | `AGENT-PREP-1786432697035375000-1f95dc6a` — complete |
| **Agent chat feed (lite)** | Materialize from live `agent_feed` (`zqk system materialize-agent-chat-channel --feed-id AGF-…`); expect `delivery_mode: notify`. Ghost AGF ids in lite file are a defect — rematerialize. |
| **Test health JSONL** | `.zqk/logs/scheduler/cvs/test-bundles/health.jsonl` (not the older `test-bundles/` path without `cvs/`) |
| **generate-agent-configs** | **`zqk-admin` only**: `ZQK_ADMIN_API_KEY=<ACC-*> ./bin/zqk-admin system generate-agent-configs` (legacy `account:*` keys rejected — POL-AGENT-ACCOUNT-LOGIN-001) |

**When a CVS is active:** Honor **`desired_end_state`**, **`next_action`**, and **`iteration_process`**. **When none:** Drive **active** priority-plan backlog via CLI.

---

## Commands to re-verify before acting

```bash
zqk workflow whats-next --format json
zqk object list priority_plan --filter 'status=active' --format table
zqk object list backlog_item --filter 'priority_plan_ref=[REDACTED-ID]' --format table
zqk object list convergence_session --filter status=active --format json
zqk system check --fast
zqk scheduler test-failures health
zqk system policy-interrupts list
# Cross-vendor chat
tail -20 .zqk/logs/cursor-hooks/agent_chat_channel.jsonl
```

---

## Next steps (standing)

1. Re-run the block above so IDs and phases match the moment of execution.
2. Align work with **active PRIs** (and any **active CVS** if opened); keep process updates on the **CLI** path.
3. Before substantive code changes: [`PRE_CHANGE_CHECKLIST.md`](../architecture/PRE_CHANGE_CHECKLIST.md) + [`AGENT_GUIDELINES.md`](../process/enforcement/AGENT_GUIDELINES.md).
4. For Agy↔Cursor: use **`zqk_chat_send`** / channel JSONL; track MCP proxy via ATKs on the proxy plan.

---

## Related

- **Historical dated files:** [`archive/`](./archive/)
- **Compressed timeline:** [`AGENT_ONBOARDING_SUMMARIES_DIGEST.md`](./AGENT_ONBOARDING_SUMMARIES_DIGEST.md)
- **Assessment (one-off, 2026-03-21):** [`AGENT_ONBOARDING_ASSESSMENT_2026-03-21.md`](./AGENT_ONBOARDING_ASSESSMENT_2026-03-21.md)
