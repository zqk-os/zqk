# Family: gantt_column

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Status:** Active  
**Hub:** [LIFECYCLE_SHOCKWAVE_MAP.md](../LIFECYCLE_SHOCKWAVE_MAP.md)  
**Filled exam:** [kind_priority_plan.md](./kind_priority_plan.md)

## Overview

One kind today: **`priority_plan`**. This is a Gantt **column**: intake (`grooming`) → sealed pickup (`active` = `shovel_ready`) → locked flight (`in_progress` = `execution_locked`) → terminal.

`active` here is **not** live work and **not** an enforced membrane. Copying this valve onto policy, CVS, or goal is the opposite error.

## Occupancy

| Value | Role | Hold while occupying |
|-------|------|----------------------|
| `grooming` | `grooming` | Membership open |
| `active` | `shovel_ready` | Membership sealed |
| `in_progress` | `execution_locked` | Scope expansion refused (check valve) |
| `paused` / `blocked` | `halted` | Not shovel-ready; resume is re-lock, not unseal |
| `complete` / `cancelled` / `archived` | `terminal` | Complete ⇒ all linked members terminal |

## Class hops (this family only)

- **Check valve:** `shovel_ready → execution_locked` is one-way. No `in_progress → active`, no `paused|blocked → active`.
- **Halt resume:** `halted → execution_locked` (or auto `→ grooming` when children realign).
- **Compiled shockwave target:** Plane C matcher + E last-child + F occupancy lock. `strictAutoTriggerKinds` = this kind.
- **Trigger kind:** `backlog_item` only (`on_dependent_status`). ATK / workstream / goal / milestone hops are **not** PRI catalysts.

## Planes

**A + B** for seal, complete, cancel, archive, terminal re-entry. **C + D + E + F** for child-status occupancy. **I** after lock. Do not put the seal exam on the subscriber.

## Specialty (do not copy)

Membership is **child-owned** (`backlog_item.priority_plan_ref`). Seal tokens, field DNA, remaining suspect edges: [kind_priority_plan.md](./kind_priority_plan.md).

YAML: `docs/process/_internal/lifecycles/priority_plan_lifecycle.yaml`.
