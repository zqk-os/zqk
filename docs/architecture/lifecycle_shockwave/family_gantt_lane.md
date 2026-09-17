# Family: gantt_lane

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Status:** Active  
**Hub:** [LIFECYCLE_SHOCKWAVE_MAP.md](../LIFECYCLE_SHOCKWAVE_MAP.md)

## Overview

Gantt **rows and commitments**: workstream, roadmap, strategic_plan, requirement, goal, evolution_management. `active` means **live target / live lane**, role `shovel_ready`. There is no PRI check valve.

`paused → active` and `blocked → active` are **correct** class hops here (halt → pickup). They are membrane leaks only on [gantt_column](./family_gantt_column.md).

## Occupancy (typical)

| Value | Role | Notes |
|-------|------|--------|
| `planned` / `proposed` / `grooming` | `realign` or `grooming` | Origin / reshape |
| `active` | `shovel_ready` | Live row or live north-star target |
| `paused` / `blocked` | `halted` | Resume returns to `active` |
| `complete` / `archived` | `terminal` | Often auto-only rollup (no `on_dependent_status` yet) |

Workstream maps `in_progress → active` (no separate lock status). Goal maps `in_progress → active` the same way. Do not invent an `execution_locked` column on these kinds to “match PRI.”

## Class hops

| Hop | Legal? | Planes |
|-----|--------|--------|
| `halted → shovel_ready` | **Yes** | **B** (workstream `paused → active`); **auto** (goal `blocked → active` when blockers clear) |
| `shovel_ready → execution_locked` | No (no lock status) | — |
| `shovel_ready → terminal` | Yes (often auto-only rollup) | **A** should bind; today often fail-open English. TRACK: `BLI-REDACTED` |

## Shockwave vs PRI

**Goal is a compiled target** for `proposed → active` when a linked backlog_item occupies `in_progress`. `active` remains `shovel_ready` (executing occupancy for this family — do not invent an `execution_locked` column). Other lane kinds stay exam-only.

Plane C walks neighbors on status save. PRI occupancy is unchanged (BLI trigger only). Relation to a plan is still a **Plane A exam** on PRI seal/complete: refs present and live (`workstream_refs`, `related_object_refs` for roadmap until spec’d).

Do **not** add `on_dependent_status.kind: workstream` on PRI to pause a column when a lane pauses. If that coupling is needed later, it is a new YAML trigger + contract test, not a subscriber per kind.

## Kind specialty (Q5 only)

| Kind | Specialty |
|------|-----------|
| `workstream` | Research/execution lane; `paused → active` is documented as not a PRI leak. Auto-complete when linked milestones complete (rollup allowlist). |
| `roadmap` / `strategic_plan` | Horizon / campaign frame. `grooming` on strategic_plan is intake, not PRI membership. |
| `requirement` | Spec commitment; `active` = accepted REQ, not a sealed column. |
| `goal` | Program target. `blocked → active` resumes the commitment. Auto `active → complete` when metric/workstreams done (unbound DSL). |
| `evolution_management` | Has a `grooming` role; still a lane, not a compiled column. |

YAML under `.zqk/specs/lifecycles/<kind>_lifecycle.yaml`. Contract tests must stay **kind-specific** — do not globalize PRI’s `halted ↛ shovel_ready`.
