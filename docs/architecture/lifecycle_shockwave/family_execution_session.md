# Family: execution_session

**Last Verified:** 2026-08-31

**Version:** 1.0.0  
**Status:** Active  
**Hub:** [LIFECYCLE_SHOCKWAVE_MAP.md](../LIFECYCLE_SHOCKWAVE_MAP.md)

## Overview

Sessions and jobs whose live token is **`execution_locked`**: convergence_session, zqk_session, mcp_session, scheduler_job, scheduler_health_metric.

`active` (or `running`) **is** in-flight work. `paused → active` is halt → **lock**, which is the correct resume. Copying the PRI check valve here would forbid the resume the session needs.

## Occupancy (CVS template)

| Value | Role | Notes |
|-------|------|--------|
| `draft` | `realign` | Contract being authored |
| `active` | `execution_locked` | Measuring / executing (CVS phases C1–C5) |
| `paused` | `halted` | Resume → `active` (lock) |
| `completed` / `abandoned` / `archived` | `terminal` | Desired end state vs discontinue |
| `escalated` / `error` | `halted` or terminal per YAML | Specialty |

Scheduler job uses `running`-class occupancy mapped to `execution_locked`; queued/ready maps to `shovel_ready`. Same family: live means locked.

## Class hops

| Hop | Legal? |
|-----|--------|
| `halted → execution_locked` | **Yes** (CVS `paused → active`) |
| `halted → shovel_ready` | Only if the kind has a shovel-ready live token (scheduler queued). Not CVS. |
| PRI-style `in_progress → active` unseal | **No** — there is no shovel-ready `active` on CVS |

Planes: **A + B** for promote/pause/complete. **H** for metrics. Not a compiled Plane C target. Convergence measure / `health.jsonl` is session evidence, not lifecycle shockwave.

## Kind specialty (Q5 only)

| Kind | Specialty |
|------|-----------|
| `convergence_session` | Hypothesis, phases C1–C6, `desired_end_state`. Do not treat bundle green as session complete. |
| `zqk_session` / `mcp_session` | Seat / MCP connection lifetime; terminal on disconnect. |
| `scheduler_job` | Runtime delta, not `stream_current`. Occupancy is execution state; Plane I is not CAP-on-PRI. |
| `scheduler_health_metric` | `active` = locked observation window; metric builder statuses only. |
