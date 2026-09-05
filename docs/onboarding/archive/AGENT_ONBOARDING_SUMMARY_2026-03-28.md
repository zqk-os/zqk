# Agent onboarding summary — 2026-03-28

**Generated:** 2026-03-28  
**Category:** onboarding · agent-summary · workflow  
**Purpose:** Consolidated review after reading the onboarding corpus: expectations, how pieces fit, **live** CLI snapshot at generation time, and **anticipated next steps**.

**Related snapshots:** [AGENT_ONBOARDING_SUMMARY_2026-03-27](./AGENT_ONBOARDING_SUMMARY_2026-03-27.md), [AGENT_ONBOARDING_SUMMARY_2026-03-25](./AGENT_ONBOARDING_SUMMARY_2026-03-25.md), [AGENT_ONBOARDING_SUMMARY_2026-03-22](./AGENT_ONBOARDING_SUMMARY_2026-03-22.md).

---

## 1. Corpus reviewed (this session)

| Document | Role |
|----------|------|
| [README.md](../../../README.md) (repo root) | AI agents → [AI_AGENT_ONBOARDING.md](../AI_AGENT_ONBOARDING.md); links architecture, best practices, codebase summary. |
| [docs/onboarding/README.md](../README.md) | Index: main guide, dated summaries, supplementary docs (efficient processing, field state, import cycles). |
| [AI_AGENT_ONBOARDING.md](../AI_AGENT_ONBOARDING.md) | Canonical guide: STRAT-PLAN-001, routines (`command-timings`, prefer structured CLI over ad hoc jq/yq where documented), maintenance jobs, **convergence_session**, **CLI/MCP-only** mutation of process objects, git/PR flow, policies/architecture via objects, OHTV, live discovery commands at end. |
| [AGENT_GUIDELINES.md](../../process/enforcement/AGENT_GUIDELINES.md) | Policies (POL-CODE-007 logging, POL-CODE-005 integrity), command surfaces (user vs privileged), templates + `zqk object create`, pattern grep examples, compliance scripts. |
| Cursor / workspace rules | Process YAML via **zqk CLI only**; PRE_CHANGE_CHECKLIST before changes; long tests via scheduler / `test-runner.sh`; `go test` with `-timeout`; priority-plan order; convergence evidence-first; instance builders not hand-edited. |

**Cross-links:** [OBJECT_FIRST_ALIGNMENT.md](../../process/enforcement/OBJECT_FIRST_ALIGNMENT.md), [PRE_CHANGE_CHECKLIST.md](../../architecture/PRE_CHANGE_CHECKLIST.md), [LESSONS_LEARNED.md](../../process/architecture/LESSONS_LEARNED.md).

---

## 2. Thematic synthesis (expectations)

1. **Normative path:** Process and object data live as **system objects**; mutate via **`zqk`** (or MCP wrapping it), not by editing instance YAML under `docs/process/` (except documented `_internal` schema workflows).  
2. **Session start:** Active **`priority_plan`** (especially `in_progress`), active **`convergence_session`** objects if any, and **`zqk system check --fast`**; align to **STRAT-PLAN-001**.  
3. **Quality:** TDD where appropriate; discover policies before large features; **OHTV** for investigations; use **instance builders** for spec-backed creates; logging via framework / `cli.WriteOutput`, not raw `fmt.Print*` in command paths.  
4. **Work ordering:** Workspace rule **priority-plan-order** — complete the current plan’s active backlog thread before treating other plans as the main execution line.  
5. **Tests:** Short package tests with explicit **`-timeout`**; large suites via **scheduler** or **`scripts/test-runner.sh`** with logs under `.zqk/logs/`.

---

## 3. Recent vs current work (from live objects + branch context)

- **Strategic alignment:** Three-phase plan through 2028 remains **STRAT-PLAN-001** (see onboarding guide).  
- **Current priority plan (in progress):** **`[REDACTED-ID]`** — *Report-driven improvement & multi-binary CLI* (`status=in_progress`, `active_order=1`, `previous_plan_id=PRI-223`). Plan note (2026-03-28): grooming complete; five items with acceptance criteria; P0–P2 themes.  
- **Backlog on that plan (2026-03-28):**  
  - **Complete:** retention/congruence + maintenance jobs; CLI surface boundaries; multi-binary shared runtime (`zqk-admin`) / cmd layout.  
  - **Planned:** audit streams and report-driven metrics feedback loop; multi-agent execution (scheduler test bundles / convergence reliability).  
- **Convergence:** No **`convergence_session`** with `status=active` at snapshot time.  
- **System check (`--fast`):** Public/internal object tiers clean; lifecycle reminder: **retention_drift** (internal object volume above target — see `zqk system retention-status` (PRUNED) / maintenance jobs as needed).  
- **Git:** Current branch naming aligns with this theme (`feature/pri-report-driven-improvement`). If the working tree has bulk `docs/process/` deletions or edits, reconcile with **CLI-only** process-data rules before committing.

---

## 4. Live state snapshot (re-verify before acting)

```bash
zqk object list priority_plan --filter 'status=in_progress' --format table
zqk object list priority_plan --filter 'status=active' --sort-by active_order --format table
zqk object list backlog_item --filter 'priority_plan_ref=[REDACTED-ID]' --format table
zqk object list convergence_session --filter status=active --format json
zqk system check --fast
```

---

## 5. Anticipated next steps

1. **Re-run §4** so priority/backlog/convergence match the moment you execute work.  
2. **On [REDACTED-ID]:** Pick up the two **`planned`** items in priority tier order (audit streams / metrics feedback; then multi-agent execution / scheduler–convergence quality), or groom them into **`in_progress`** with clear exit criteria per plan note.  
3. **When a `convergence_session` is active again:** Drive **`desired_end_state`** / **`next_action`**; use the session’s snapshot command (often `zqk scheduler convergence measure`) as the feedback loop.  
4. **Retention reminder:** If `retention_drift` persists, follow **`zqk system retention-status` (PRUNED)** and the ensure/aggregate/tolerance path documented in system check output.  
5. **Before substantive code or CLI UX changes:** [PRE_CHANGE_CHECKLIST.md](../../architecture/PRE_CHANGE_CHECKLIST.md) + [AGENT_GUIDELINES.md](../../process/enforcement/AGENT_GUIDELINES.md).  

---

*Last updated: 2026-03-28*
