# Family: work_unit

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Status:** Active  
**Hub:** [LIFECYCLE_SHOCKWAVE_MAP.md](../LIFECYCLE_SHOCKWAVE_MAP.md)

## Overview

Executable work items and the `base_object` five-role template they extend: backlog_item, agent_task, technical_debt, milestone, test_case, agent_instruction, agent_skill, agent_onboarding_preparation, persona, risk_blocker, base_object.

Most of these kinds **have no `active`**. Shovel-ready is `planned` (BLI) or an equivalent ready token. Live work is `in_progress` (`execution_locked`). Halt resume returns to **lock**, not to planned.

## Occupancy (BLI template; others alias)

| Value | Role | Notes |
|-------|------|--------|
| `exploring` / `validated` / `roadmap` / `deferred` | `realign` | Draft or parked; not execution-facing membership |
| `planned` | `shovel_ready` | Committed on a plan (BLI specialty: milestone linkage) |
| `in_progress` | `execution_locked` | Scope locked until halt or terminal |
| `paused` / `blocked` / `error` | `halted` | Resume → `in_progress` |
| `complete` / `archived` | `terminal` | Dual complete on BLI (manual + auto); ATK complete does **not** complete a BLI via Plane C |

## Shockwave

| Kind | Role in compiled graph |
|------|------------------------|
| `backlog_item` | **PRI, milestone/goal, and criteria shockwave trigger.** Hops `→ in_progress` lock the plan column, any linked milestone (`not_started` → `in_progress`), proposed goals (`→ active`), and shovel-ready criteria (`awaiting_verification` → `in_progress`). Membership edges are child-owned (`priority_plan_ref`, `milestone_refs`, `goal_refs`); criteria composition is parent-owned (`criteria_refs`). |
| `agent_task` | Composition under BLI. Claim/`in_progress` is the catalyst that should already have locked the BLI. If the BLI is already `in_progress`, Plane C fans out the BLI's hierarchy refs so a late ATK claim still locks the milestone. Hourglass / `agent next` remains **feed** policy (`POL-AGENT-ORCH-HOURGLASS-001`), not a PRI lock from ATK complete. |
| `milestone` | **Compiled target.** Linked BLI `in_progress` occupies `execution_locked` (`in_progress`). |
| Everyone else in this family | Exam-only. Status may appear in a Plane A token on another kind’s seal. |

Do not add a subscriber so that ATK **complete** locks a plan.

## Class hops

- `realign → shovel_ready`: qualifying exam (BLI: plan + milestone). **A + B**.
- `shovel_ready → execution_locked`: start work. Manual (B). Not a PRI column valve.
- `execution_locked → halted` / `halted → execution_locked`: class pause/resume.
- **Do not** copy PRI `halted ↛ shovel_ready` onto BLI as a global rule without a BLI-specific contract — parked statuses (`deferred`/`roadmap`) are park verbs, not halt resume.

## Kind specialty (Q5 only)

| Kind | Specialty |
|------|-----------|
| `backlog_item` | PRI membership owner; park vs promote; CRIT refs on complete. |
| `agent_task` | Hourglass callback; worktree isolation; not a PRI trigger. |
| `technical_debt` | Debt work unit; same roles as BLI-like machines. |
| `milestone` | Gantt anchor under a lane; `blocked → …` is the milestone’s exam, not PRI’s. |
| `test_case` | Extends `work_interval`; `active` if present is shovel-ready for the case, not a column seal. |
| `persona` | Identity that CAP (Plane I) consults; execution_locked when a seat is in flight is **session-like** — keep in this family because the YAML is the five-role work template, not `enforced`. |
| `base_object` | Template machine other kinds map onto. Not instantiated as a product object. |

TRACK: `BLI-CEF-R26-GANTT-PARTNERS-001`.
