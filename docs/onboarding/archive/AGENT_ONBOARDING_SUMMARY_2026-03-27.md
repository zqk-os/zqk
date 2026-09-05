# Agent onboarding summary — 2026-03-27

**Generated:** 2026-03-27  
**Category:** onboarding · agent-summary · workflow  
**Purpose:** Consolidated review of the onboarding guide set: what each layer covers, how the pieces fit together, gaps called out elsewhere, a **live** CLI snapshot, and **anticipated next steps** after finishing this read-through.

**Related snapshots:** [AGENT_ONBOARDING_SUMMARY_2026-03-25](./AGENT_ONBOARDING_SUMMARY_2026-03-25.md), [AGENT_ONBOARDING_SUMMARY_2026-03-22](./AGENT_ONBOARDING_SUMMARY_2026-03-22.md).

---

## 1. Corpus reviewed

| Document | Role |
|----------|------|
| [README.md](../README.md) | Entry point: points to `AI_AGENT_ONBOARDING.md`, lists supplementary docs, and gives a minimal “new agent” checklist (`priority_plan`, `convergence_session`, `STRAT-PLAN-001`, `zqk system check --fast`). |
| [AI_AGENT_ONBOARDING.md](../AI_AGENT_ONBOARDING.md) | Canonical guide: mission/context, **essential routines** (timestamps, `command-timings`, prefer `zqk query` over jq/yq, maintenance jobs, **convergence_session** usage), **CLI/MCP-only** object mutation, git/PR workflow, backlog-driven progress, **policy** and **architecture** discovery via objects, **OHTV**, lessons learned, pre-implementation checklist items. |
| [QUICK_CONTEXT_HELPERS.md](./QUICK_CONTEXT_HELPERS.md) | Copy-paste commands for priorities, strategic plan, policies, system check; still uses `jq` in a few examples (onboarding elsewhere prefers `zqk query`). |
| [EFFICIENT_DATA_PROCESSING.md](./EFFICIENT_DATA_PROCESSING.md) | Pipeline thinking: state transitions, sequential vs parallel work, complexity targets; ties autofix/convergence-style work to explicit state machines. |
| [FIELD_STATE_TRACKING_PRINCIPLES.md](./FIELD_STATE_TRACKING_PRINCIPLES.md) | Validation-oriented: fields that encode state need finite value sets or explicit conventions. |
| [IMPORT_CYCLE_RESOLUTION.md](./IMPORT_CYCLE_RESOLUTION.md) | Required: treat cycles as architecture signals; abstraction and dependency direction over quick hacks. |
| [STABLE_BINARY_MANAGEMENT.md](./STABLE_BINARY_MANAGEMENT.md) | When/how to refresh `bin/zqk-stable`, `.zqk/mcp/config.yaml`, and smoke expectations for MCP. |
| [MCP_CONFIGURATION.md](./MCP_CONFIGURATION.md), [CURSOR_MCP_SETUP.md](./CURSOR_MCP_SETUP.md) | MCP wiring for editors and CLI bridge usage. |
| [TEST_MCP_ECHO.md](./TEST_MCP_ECHO.md), [TEST_ECHO_TOOL.md](./TEST_ECHO_TOOL.md) | Narrow test/diagnostic helpers for MCP. |
| Prior assessments | [AGENT_ONBOARDING_ASSESSMENT_2026-03-21](./AGENT_ONBOARDING_ASSESSMENT_2026-03-21.md) records improvement ideas (e.g. replace “recreate this prompt” closing with live CLI pointers; clarify glossary vs convergence_session vs scheduler test-failures). |

**Cross-links already implied by onboarding:** [OBJECT_FIRST_ALIGNMENT.md](../../process/enforcement/OBJECT_FIRST_ALIGNMENT.md), [PRE_CHANGE_CHECKLIST.md](../../architecture/PRE_CHANGE_CHECKLIST.md), [AGENT_GUIDELINES.md](../../process/enforcement/AGENT_GUIDELINES.md), [PROJECT_CONTEXT_SUMMARY.md](../../process/analysis/PROJECT_CONTEXT_SUMMARY.md), [LESSONS_LEARNED.md](../../process/architecture/LESSONS_LEARNED.md).

---

## 2. Thematic synthesis (what a new agent should internalize)

1. **Normative path:** `zqk` (and MCP tools that invoke it) for object and process data; no direct edits to instance YAML under `docs/process/` except documented `_internal` schema work flows.  
2. **Operating rhythm:** Start sessions with **active priority plan(s)**, **active convergence sessions** (if any), and **`zqk system check --fast`**; align work to **STRAT-PLAN-001**.  
3. **Safety and ergonomics:** CLI-backed timeouts for CLI work; non-CLI shell wrapped with **`command-timings`** where onboarding requires it; prefer **`zqk query`** over ad hoc JSON/YAML tooling for structured extraction.  
4. **Quality bar:** TDD expectations, policy discovery before large features, architecture pattern reuse, OHTV for investigations, **Lessons Learned** before risky changes.  
5. **Supplementary guides** deepen specific areas (import cycles, field semantics, pipelines, MCP binary lifecycle) without repeating the full main guide.

---

## 3. Notable gaps / follow-ups (documentation)

- **`QUICK_CONTEXT_HELPERS.md`** mixes onboarding-preferred **`zqk query`** with **`jq`** examples—harmless but slightly inconsistent with the main guide.  
- **Discoverability:** README already indexes dated summaries; keeping one **latest** summary linked (see below) remains the lightweight index strategy.

**Resolved 2026-03-27:** The **“Next steps (live context)”** section in `AI_AGENT_ONBOARDING.md` now replaces the old “recreate this prompt” closing with CLI discovery commands (per 2026-03-21 assessment).

---

## 4. Live state snapshot (re-verify before acting)

Commands:

```bash
zqk object list priority_plan --filter 'status=in_progress' --format table
zqk object list priority_plan --filter 'status=active' --sort-by active_order --format table
zqk object list backlog_item --filter 'priority_plan_ref=PRI-221' --format table
zqk object list convergence_session --filter status=active --format json
zqk system check --fast
```

**At summary generation time (2026-03-27):**

- **`priority_plan` in progress:** **PRI-221** — *Product & Performance (1 week)*.  
- **Other `active` plans (by policy ordering):** **PRI-222** (*Documentation & Observability*), **PRI-223** (*Phase 1 Core – Backlog*).  
- **Active `convergence_session`:** none (`returned_count: 0`).  
- **PRI-221** still has non-complete items (e.g. convergence lifecycle E2E **in progress**, several convergence-themed items **exploring**, Gantt/data-stream items **planned**; many others **complete** or **archived**).

Workspace rule **priority-plan-order** applies: finish the current plan’s active work before treating other active plans as the main execution thread.

---

## 5. Anticipated next steps (after completing this review)

1. **Run the live commands in §4** (or your equivalent dashboard) so priority and convergence state match **now**, not this file’s timestamp.  
2. **If PRI-221 remains `in_progress`:** pick the next backlog item in tier/order per project rules (e.g. advance **BLI-17742285** convergence lifecycle E2E, or groom **exploring** convergence items toward exit criteria).  
3. **When PRI-221 is fully complete or superseded:** shift primary attention to **PRI-222** then **PRI-223** per `active_order` and backlog filters.  
4. **When a `convergence_session` becomes active again:** re-read `desired_end_state` / `next_action` on the CVS object and use **`zqk scheduler convergence measure`** (or the session’s configured snapshot command) as the urgent feedback loop.  
5. **Before substantial code or CLI output work:** apply [PRE_CHANGE_CHECKLIST.md](../../architecture/PRE_CHANGE_CHECKLIST.md) and [AGENT_GUIDELINES.md](../../process/enforcement/AGENT_GUIDELINES.md).  
6. **Doc hygiene (done 2026-03-27):** `AI_AGENT_ONBOARDING.md` ends with live CLI pointers; `CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md` documents automated test coverage for the router and suggested-fields path.

---

*Last updated: 2026-03-27*
