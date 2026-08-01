# Agent onboarding snapshot — current focus

**Doc entry:** `DOC-EXAMPLE` — `zqk object get DOC-EXAMPLE` (or `zqk object list doc_entry --filter group=onboarding`).

**As of:** 2026-07-27
**Replaces:** Rolling “latest dated summary” in `docs/onboarding/` (see [`AGENT_ONBOARDING_SUMMARIES_DIGEST.md`](./AGENT_ONBOARDING_SUMMARIES_DIGEST.md) and [`archive/`](./archive/)).

**Purpose:** Short, **re-verify**-friendly view of plan + convergence + what to run before acting. Numbers below are **examples from the last explicit review** — always confirm with **`zqk object …`**.

---

## Norms (unchanged)

CLI-first, object-first, **PRE_CHANGE_CHECKLIST** before substantive code, **AGENT_GUIDELINES** for output/logging, **`AGENT_CONTEXT_REFRESH.md`** for generated reminders (**glossary** / `glossary_term` + **semantic density / section 13** post-verify after logic works; runbook **`scripts/README.md`**), scheduler-backed long tests, convergence evidence-first. Full detail: [`AI_AGENT_ONBOARDING.md`](./AI_AGENT_ONBOARDING.md).

**Cross-vendor mesh:** Agents (Cursor, Antigravity/Agy, etc.) coordinate via kernel objects + **`zqk_chat_send`** → `.zqk/logs/cursor-hooks/agent_chat_channel.jsonl` (policy: `.zqk/config/agent_chat_channel.json`). MCP stdio↔TCP disparity: **`zqk mcp proxy`** / daemon (see `docs/architecture/README.md`). Prefer Content-Length framing for MCP stdio clients.

---

## Live pointers (re-verified 2026-07-27)

| Item | Live ID / note |
|------|----------------|
| **Mission (archived legacy)** | `MIS-1775446507801844000-42ba7fc8` — archived; keep for history only |
| **Mission (active commercial)** | `MIS-1784740125390806000-d6300420` — *ZQK Commercial Mission* |
| **Priority plan (whats-next primary)** | `PRI-EXAMPLE` — *STRAT-PLAN-006 Feedback Remediation* — **`status=active`** (filter `in_progress` returns empty) |
| **Priority plan (parallel)** | `PRI-EXAMPLE` — *Phase 1: The Autonomy Inbox* — `status=active` |
| **MCP proxy sprint plan** | `PRI-EXAMPLE` — object `status=complete`; related ATKs may still show `active` — re-query |
| **Active convergence** | **None** (no `status=active` or `paused` `convergence_session` as of this snapshot) |
| **Onboarding prep (this Cursor session)** | `AGENT-PREP-1785173927163123000-5ccad89d` — complete |
| **Agent chat feed (lite)** | `.zqk/config/agent_chat_channel.json` references `AGF-1776329682817339000-a1b03fc7` — **CAS `agent_feed` list empty**; rematerialize or recreate feed when touching delivery policy |
| **Test health JSONL** | `.zqk/logs/scheduler/cvs/test-bundles/health.jsonl` (not the older `test-bundles/` path without `cvs/`) |
| **generate-agent-configs** | **`zqk-admin` only**: `ZQK_ADMIN_API_KEY=account:system ./bin/zqk-admin system generate-agent-configs` |

**When a CVS is active:** Honor **`desired_end_state`**, **`next_action`**, and **`iteration_process`**. **When none:** Drive **active** priority-plan backlog via CLI.

---

## Commands to re-verify before acting

```bash
zqk workflow whats-next --format json
zqk object list priority_plan --filter 'status=active' --format table
zqk object list backlog_item --filter 'priority_plan_ref=PRI-EXAMPLE' --format table
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
