# Lifecycle status roles

**Last Verified:** 2026-08-31


**TRACK:** `[REDACTED-ID]`

Cross-kind semantic roles live on lifecycle YAML as `status.role` (schema:
`.zqk/cli/specs/schemas/lifecycle_spec.schema.json`). Membership, airtight lock,
and ranking should consult roles via `objects.StatusChecker.Role` / helpers in
`pkg/objects/lifecycle_roles.go` — not ad-hoc per-kind status string switches.

## Roles

| Role | Meaning |
|------|---------|
| `grooming` | Plan/backlog reshaping; not execution-facing |
| `shovel_ready` | Ready to execute / on the roadmap |
| `execution_locked` | In flight; scope locked |
| `enforced` | Membrane live (policy/role `active`) — not a Gantt column |
| `realign` | Not ready for execution-facing membership (exploring/roadmap/…) |
| `halted` | In-place repair / pause without scope-creep ambiguity (replaces BLI-only `error` allowlist) |
| `terminal` | Done / archived / cancelled |

## Coverage

Every `*_lifecycle.yaml` under `docs/process/_internal/lifecycles/` annotates
each status with `role` **and** a non-empty `description` (why the state exists —
so next-month-us is not guessing). Gate: `TestLifecycleRoles_AllStatusesAnnotated`.

Bulk helpers (`--dry-run` first):

- `python3 scripts/annotate_lifecycle_roles.py` — roles (skips hand-tuned PRI/BLI)
- `python3 scripts/annotate_lifecycle_status_descriptions.py` — missing descriptions
  (kind+value overrides for strategic/orchestration; role-based template otherwise)

Role heuristics map `terminal`/`archive` → `terminal`, `system`/paused/blocked →
`halted`, preliminary → `realign`/`grooming`, with kind overrides for strategic
parents (`active` → `shovel_ready`) vs sessions/jobs (`active` → `execution_locked`).

## Priority plan ↔ backlog_item (hand-tuned)

| Kind | Status | Role |
|------|--------|------|
| `priority_plan` | `active` | `shovel_ready` |
| `priority_plan` | `in_progress` | `execution_locked` |
| `priority_plan` | `grooming` / `planning` / `prioritizing` | `grooming` |
| `priority_plan` | `paused` / `blocked` | `halted` |
| `backlog_item` | `planned` | `shovel_ready` |
| `backlog_item` | `in_progress` | `execution_locked` |
| `backlog_item` | `exploring` / `validated` / `roadmap` / `deferred` | `realign` |
| `backlog_item` | `error` | `halted` |
| `policy` | `active` | `enforced` |
| `role` | `active` | `enforced` |

## `in_progress` ranking vs `active` (not a status collapse)

For priority plans, **ranking** treats `in_progress` as top-of-stack (≡ `active_order` 0) so other shovel-ready plans occupy unique 1+ slots. Role for `in_progress` is `execution_locked`; role for `active` is `shovel_ready`.

**Check valve:** `active` means ready to act (not parked). Once the plan is `in_progress`, the only exits are `paused`, `blocked` (halt), `complete`, or `cancelled`. There is **no** demotion to `active` or `grooming`. Halt resume is `paused|blocked → in_progress` (re-lock), **not** `→ active` (`in_progress → paused → active` is the same leak). `status_mapping` must not map `in_progress` → `active` (sibling statuses). Design exam: [LIFECYCLE_STATE_MACHINE_RUBRIC.md](./LIFECYCLE_STATE_MACHINE_RUBRIC.md). TRACK: `REDACTED`.

Complete children on an `active` plan mean **closeout failed** (promote to `complete`), not a valid parked state.

## Membership vs lock

- **Membership** (may link to execution-facing plan): `shovel_ready`,
  `execution_locked`, `terminal`, **`halted`**.
- **Airtight lock** (all linked BLIs ready for `→ in_progress`): same except
  **`halted` does not count as shovel-ready**. Resume from halt re-enters
  **`execution_locked`**, not `shovel_ready`.

## Conversational synonyms (do not invent kernel statuses)

| People say | Canonical `value` | Role | Notes |
|------------|-------------------|------|-------|
| ready / shovel-ready (BLI) | `planned` | `shovel_ready` | Display: `Planned (shovel-ready)` |
| ready / shovel-ready (PRI) | `active` | `shovel_ready` | Display: `Active (shovel-ready)` |
| locked / execution-locked (PRI) | `in_progress` | `execution_locked` | Check valve; ranking uses active_order 0 |
| halted / needs repair (BLI) | `error` | `halted` | Display: `Halted (error)`; recover → `planned` |

Whats-next plan ranking consults **roles** via `pkg/objects` helpers
(`PlanWhatsNextStatusBonus`, `PlanStatusExecutionFacing`, …), not ad-hoc
`switch status` matrices. TRACK: `[REDACTED-ID]`.

## Contract tests (YAML properties)

`pkg/objects/lifecycle_contract_test.go` (TRACK: `REDACTED`):

1. **Auto-only ≠ promote** — every `auto: true` / `manual: false` edge is excluded
   from `PromoteTransitionTargets` (no combinatorial status matrix).
2. **Strict DSL kinds** (`priority_plan`) — every auto-only non-system edge has
   `on_dependent_status`, except allowlisted completion rollups until an
   all-dependents trigger exists.
3. **Role invariants** — terminal/archive ⇒ `terminal`; system ⇒ `halted`;
   `shovel_ready` is never preliminary; descriptions required.
4. **PRI check valve** — `in_progress` does not target `active`/`grooming`;
   halted (`paused`/`blocked`) does not target `shovel_ready` (`active`).
   Rubric: [LIFECYCLE_STATE_MACHINE_RUBRIC.md](./LIFECYCLE_STATE_MACHINE_RUBRIC.md).

## First-class promote (dual edges)

**Problem:** An edge with `auto: true` and `manual: false` is **auto-only**.
`PromoteTransitionTargets` / `zqk object promote` never probe it. Completion then
runs only through lifecycle updater / shockwave, which historically armed
break-glass and (until fixed) skipped lifecycle validation — a false-complete hole.

**Filled hole (storage + validator):** break-glass / trusted shockwave no longer
disable `ValidateLifecycle`. Complete **holds** (CRITs; plan children terminal)
always run. TRACK: `REDACTED`. Multi-ID / bulk
`--field status=` uses the same promote-or-override door as single-ID update
(`BLI-CEF-CLI-MULTI-ID-UPDATE`).

**Recipe — make an auto-only completion edge promote-first-class:**

1. **Lifecycle YAML** (`docs/process/_internal/lifecycles/<kind>_lifecycle.yaml`):
   set **`manual: true`** and keep **`auto: true`** on the edge (dual). Example:
   `priority_plan` `in_progress|active → complete`. Preconditions stay the source
   of truth (e.g. all linked backlog items terminal).
2. **Contract allowlist:** if the edge was in `completionRollupAllowlist` only
   because it lacked `on_dependent_status`, remove it once it is no longer
   auto-only (`manual: true`). Prefer adding `on_all_dependents_status` DSL later
   for remaining true auto-only rollups.
3. **Promote path:** `PromoteTransitionTargets` will include `to`. Agents/humans
   run `zqk object promote <PRI-id>`; probe validates preconditions with
   `ValidateLifecycle: true` — no `--override`, no break-glass for the hop.
4. **Keep auto:** shockwave / lifecycle updater may still complete on last-child.
   Updater may still arm break-glass for **critical-kind DECIDE elevation**, not
   to skip holds.
5. **Do not** set `manual: true` without the same preconditions the auto path
   used — otherwise promote becomes a false-complete door.
6. **Verify:** `go test ./pkg/objects -run LifecycleContract -timeout 60s`;
   promote a plan whose children are all terminal and assert `status=complete`;
   attempt promote with an open child and assert refuse.

**BLI pattern already dual:** `backlog_item` `in_progress → complete` is
`manual: true` + `auto: true`. PRI completion now matches that pattern.

## PRI≈PR packaging cue (wrap time)

When an `active` / `in_progress` (or just-promoted `complete`) priority_plan has
finished children and **no open** backlog work (`BacklogCountsAsOpenWork`),
`zqk workflow whats-next` emits `packaging_cue` and `zqk object promote` on the
plan prints the same hint: one feature branch + one PR for that plan
(POL-WORKFLOW-002). Mid-execution plans with open children stay silent.
TRACK: `[REDACTED-ID]`.
