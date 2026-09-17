# Family: predicate

**Last Verified:** 2026-08-31

**Version:** 1.0.0  
**Status:** Active  
**Hub:** [LIFECYCLE_SHOCKWAVE_MAP.md](../LIFECYCLE_SHOCKWAVE_MAP.md)  
**WAL design:** [LIFECYCLE_EVENT_LISTENER_AND_CRITERIA.md](../LIFECYCLE_EVENT_LISTENER_AND_CRITERIA.md)

## Overview

Satisfiable predicates, not Gantt columns: **criteria**, **verification_matrix**, **question**. `satisfied: true` on a status is the hold that other kinds’ complete exams consult. Work clocks (`started_at`) do not belong here unless the kind mixes in `completable`.

## Occupancy (criteria)

| Value | Role | Notes |
|-------|------|--------|
| `awaiting_verification` | `shovel_ready` | Defined; waiting for evidence |
| `in_progress` | `execution_locked` | Evaluation in flight |
| `validated` | `realign` + `satisfied` | Evidence accepted; not always terminal |
| `blocked` | `halted` | Resume is the criteria exam (not PRI valve) |
| `complete` / `rejected` / `archived` | `terminal` | `complete` is satisfied terminal |

## Shockwave

**Compiled (Plane C):** `criteria` is `status_reactive`. The class hop `awaiting_verification` (`shovel_ready`) → `in_progress` (`execution_locked`) listens to parent `backlog_item` occupying `in_progress`. Dual `manual` + `auto`. Destination is **`in_progress`**, not `validated` (`validated` is `realign`). Composition is parent-owned (`criteria_refs`).

**Designed (Plane J):** criterion satisfied → enqueue auto hop on a parent. **Not compiled** as `on_dependent_status` onto PRI. Do not invent a List-scan complete path beside J and E.

Criteria auto hops (YAML `auto: true` without trigger DSL) other than the parent-lock edge are the same overlay hole as other kinds: bind tokens or they fail-open.

## Kind specialty (Q5 only)

| Kind | Specialty |
|------|-----------|
| `criteria` | Linked from BLI/REQ complete exams; parent BLI lock shockwaves shovel-ready CRITs to `in_progress`. `blocked → …` is evidence/blocker, not a column unseal. |
| `verification_matrix` | Aggregate of criteria; `active` = shovel-ready matrix, not enforced policy. |
| `question` | Inquiry probe; linear enough that it also resembles [record](./family_record.md) — kept here because the product job is satisfiable Q&A, not an audit blob. |
