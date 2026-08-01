# Agent onboarding summary — 2026-03-29

**Generated:** 2026-03-29  
**Category:** onboarding · agent-summary · workflow  
**Purpose:** Consolidated review after reading the onboarding corpus: expectations, how pieces fit, **live** CLI snapshot at generation time, and **anticipated next steps**.

**Related snapshots:** [AGENT_ONBOARDING_SUMMARY_2026-03-28](./AGENT_ONBOARDING_SUMMARY_2026-03-28.md), [AGENT_ONBOARDING_SUMMARY_2026-03-27](./AGENT_ONBOARDING_SUMMARY_2026-03-27.md), [AGENT_ONBOARDING_SUMMARY_2026-03-25](./AGENT_ONBOARDING_SUMMARY_2026-03-25.md), [AGENT_ONBOARDING_SUMMARY_2026-03-22](./AGENT_ONBOARDING_SUMMARY_2026-03-22.md).

---

## 1. Corpus reviewed (this session)

| Document | Role |
|----------|------|
| [README.md](../../../README.md) (repo root) | AI agents → [AI_AGENT_ONBOARDING.md](../AI_AGENT_ONBOARDING.md); links architecture, best practices, codebase summary. |
| [docs/onboarding/README.md](../README.md) | Index: main guide, dated summaries, supplementary docs (efficient processing, field state, import cycles). |
| [AI_AGENT_ONBOARDING.md](../AI_AGENT_ONBOARDING.md) | Canonical guide: STRAT-PLAN-001, routines (`command-timings`, prefer `zqk query` over ad hoc jq/yq), maintenance jobs, **convergence_session**, **CLI/MCP-only** mutation of process objects, git/PR flow, policies/architecture via objects, OHTV, live discovery commands at end. |
| [AGENT_GUIDELINES.md](../../process/enforcement/AGENT_GUIDELINES.md) | Policies (POL-CODE-007 logging, POL-CODE-005 integrity), command surfaces, templates, compliance scripts. |
| Cursor / workspace rules | Process YAML via **zqk CLI only**; PRE_CHANGE_CHECKLIST before changes; long tests via scheduler / `test-runner.sh`; `go test` with `-timeout`; priority-plan order; convergence discipline; instance builders not hand-edited. |

**Cross-links:** [OBJECT_FIRST_ALIGNMENT.md](../../process/enforcement/OBJECT_FIRST_ALIGNMENT.md), [PRE_CHANGE_CHECKLIST.md](../../architecture/PRE_CHANGE_CHECKLIST.md), [LESSONS_LEARNED.md](../../process/architecture/LESSONS_LEARNED.md).

---

## 2. Thematic synthesis (expectations)

1. **Normative path:** Process and object data live as **system objects**; mutate via **`zqk`** (or MCP wrapping it), not by editing instance YAML under `docs/architecture/` (except documented `_internal` schema workflows).  
2. **Session start:** Active **`priority_plan`** (especially `in_progress`), active **`convergence_session`** objects if any, and **`zqk system check --fast`**; align to **STRAT-PLAN-001**.  
3. **Quality:** TDD where appropriate; discover policies before large features; **OHTV** for investigations; use **instance builders** for spec-backed creates; logging via framework / `cli.WriteOutput`, not raw `fmt.Print*` in command paths.  
4. **Work ordering:** Workspace rule **priority-plan-order** — complete the current plan’s active backlog thread before treating other plans as the main execution line.  
5. **Tests:** Short package tests with explicit **`-timeout`**; large suites via **scheduler** or **`go test ./...`** with logs under `.zqk/logs/`.

---

## 3. Recent vs current work (from live objects + branch context)

- **Strategic alignment:** Three-phase plan through 2028 remains **STRAT-PLAN-001** (see onboarding guide).  
- **Current priority plan (in progress):** **`PRI-EXAMPLE`** — *Object maintenance redesign* (`status=in_progress`, `active_order=1`). Focus: retention, aggregation, catch-up, and related design docs (`OBJECT_MAINTENANCE_REDESIGN.md`, performance and retention-visibility notes). Umbrella backlog reference in plan notes: **`BLI-EXAMPLE`**.  
- **Backlog on that plan (2026-03-29):** Three items — delivery thread **in progress**; two **exploring** hygiene/spec items (`BLI-PHC-001`, `BLI-PHC-002`).  
- **No `status=active` priority plan** at snapshot time (queue shows 0 active; work is driven by the in-progress plan above).  
- **Convergence:** One active **`convergence_session`**: **`CVS-EXAMPLE`** — *Code quality convergence: DRY, pipelines, spec builders, constants, lint* (`status=active`, `current_phase=c5_verify`). Contract: six-part **desired_end_state** (shared helpers, pipelining, spec builders, constants, golangci clean, defect tracking). **next_action** (snapshot): lint/health satisfied for recent window; continue (1)–(4) and (6) or follow up storage/cache flakes if they recur.  
- **System check (`--fast`):** Public/internal object tiers clean on validation counts; lifecycle reminders: Tier 3 informational (system_check), **retention_drift** (internal object volume above target — use `zqk system retention-status` (PRUNED) and maintenance/aggregate/tolerance path as needed).  
- **Git:** Typical branch naming for this theme: `feature/pri-object-maintenance`. Reconcile any bulk `docs/architecture/` tree changes with **CLI-only** process-data rules before committing.

---

## 4. Live state snapshot (re-verify before acting)

```bash
zqk object list priority_plan --filter 'status=in_progress' --format table
zqk object list priority_plan --filter 'status=active' --sort-by active_order --format table
zqk object list backlog_item --filter 'priority_plan_ref=PRI-EXAMPLE' --format table
zqk object list convergence_session --filter status=active --format json
zqk object get CVS-EXAMPLE
zqk system check --fast
zqk scheduler test-failures health
zqk scheduler convergence measure
```

---

## 5. Anticipated next steps

1. **Re-run §4** so priority/backlog/convergence match the moment you execute work.  
2. **On PRI-EXAMPLE:** Advance the *Object maintenance redesign* delivery backlog item and related hygiene explorations per tier and plan notes; keep process mutations on the **CLI** path.  
3. **While CVS-EXAMPLE is active:** Treat **`desired_end_state`**, **`next_action`**, and **`iteration_process`** as the contract; use **`zqk scheduler test-failures`** / health JSONL when verifying test-bundle outcomes tied to the session.  
4. **Retention reminder:** If `retention_drift` persists, follow **`zqk system retention-status` (PRUNED)** and the ensure/aggregate/tolerance path documented in system check output.  
5. **Before substantive code or CLI UX changes:** [PRE_CHANGE_CHECKLIST.md](../../architecture/PRE_CHANGE_CHECKLIST.md) + [AGENT_GUIDELINES.md](../../process/enforcement/AGENT_GUIDELINES.md).  

---

*Last updated: 2026-03-29*
