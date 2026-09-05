# Work envelope: completable, effort-aware, and satisfiable facets

**Last Verified:** 2026-08-31


**Status:** Design contract (2026-08-20). Implementation is the next unlocked kernel column — do **not** mint onto `PRI-CEF-R9-MEASURE-001` (execution-locked) or other sealed Gantt columns.  
**TRACK:** `BLI-KERNEL-WORK-ENVELOPE-001` / `REQ-KERNEL-WORK-ENVELOPE-001`  
**Spec plane:** [`SPEC_ORIGIN_PLANE.md`](./SPEC_ORIGIN_PLANE.md)  
**Trait DNA:** [`spec-loading-order-v1.0.md`](./spec-loading-order-v1.0.md), `KindHasTrait` (`pkg/objects/trait_lookup.go`)  
**Incident that forced the facet:** wall-clock clamp of `actual_effort: 2h` on `BLI-CEF-R10-REMEDIATE-001` (36-minute `created_at`→`updated_at` span, no `completed_at`).

## Verdict

`estimated_effort` and `actual_effort` on **`base_object`**, plus `completed_at` copied onto three leaf specs, plus a Go `map[string]struct{}` of kinds, is a **use-case leak**. Record provenance and work measurement are different intervals. Process admins should not type clocks or actuals on status hops — **the membrane autofills** them.

Do **not** put `completed_at` / `started_at` / effort on **`auditable`**. Auditable is “this is a persisted record” (`created_at`, `updated_at`, `archived_at`). Almost every kind can be archived. Almost none of them are *worked*.

## Three facets (do not collapse)

| Facet | Clocks / mark | Meaning |
|-------|---------------|---------|
| **Record** (`auditable`) | `created_at`, `updated_at`, `archived_at` | Provenance of the CAS object |
| **Work** (`completable`) | `started_at`, `completed_at` | When execution began and when work-done was reached |
| **Satisfaction** (`satisfiable`) | lifecycle `satisfied` (not a work clock) | The proposition holds. Evidence, not duration. |

`created_at`→`completed_at` is only a **conservative upper bound** (record age). Honest **actual effort** is the work interval: `started_at`→`completed_at`. Calendar span must not freeze as the ontology of “actual.”

A criterion **achieving** `validated` or `complete` is not a work interval. The kernel already speaks this: `criterion_satisfied` / `AppendCriterionSatisfied`, and `CriterionStatusMeetsMilestoneGateForMilestone` already treats **`validated` or `complete`** as the proposition holding for parent rollup. Do not stamp `started_at`/`completed_at` on that hop.

Do **not** name this facet `verifiable` (collides with VDS / IEEE verify-vs-validate) or `validated` (already a status token). Category (`acceptance`, `test`, `compliance`, …) is a **field**, not a trait — future categories must not mint new traits.

## Facets (interface segregation)

Opt-in. Not universal. Same DNA as `auditable` (field mixin via `extends`) plus `auto_status_transitionable` (behavior via `KindHasTrait`).

| Facet | Owns | Requires | Meaning |
|-------|------|----------|---------|
| **`completable`** | `started_at`, `completed_at` | — | Kind participates in a **work interval**. `completed_at` is done-of-**work**, not archive-of-record. |
| **`effort_aware`** | `estimated_effort`, `actual_effort` | `completable` | Planned vs realized cost. Actual is only lawful against the work clock. |
| **`satisfiable`** | satisfaction hop (`satisfied` on the status, evidence refs) | — | Kind is a **predicate**. Done means the proposition holds. Not a timesheet. Not a work clock. |

**`effort_aware` without `completable` is invalid** (spec validator). `satisfiable` does **not** include `completable`. Completable without effort is the Gantt/strategy/run plane. Satisfiable without completable is the criteria plane. A glossary term, policy, mission, or vision is none of these.

### Who opts in (not “the five execution kinds”)

`complete` on a lifecycle is **not** enough. Audit events, journal rows, and criteria predicates also say `complete`. The test is: **does this kind’s work-done hop mean a planning or execution body finished**, as opposed to a record closing or a standing north-star archiving?

| Plane | Kind | `completable` | `effort_aware` | `satisfiable` | Why |
|-------|------|:-------------:|:--------------:|:-------------:|-----|
| Gantt frame | `roadmap` | yes | no | no | Auto-completes when linked milestones complete. Timeline is `timeline_*`, not effort. |
| Gantt row | `workstream` | yes | no | no | Lane of work; same auto-complete. |
| Gantt column | `priority_plan` | yes | no | no | Column seals and completes; last-child shockwave. Estimate lives on member BLIs. Wall-clock of a 3-week PRI is **not** `actual_effort`. |
| Gantt anchor | `milestone` | yes | **yes** | no | Lifecycle gates `estimated_effort` on complete; `actual_effort` is membrane-autofilled. |
| Outcome | `goal` | yes | no | no | `complete` is work-done. `achieved_at` stays the **metric** clock; do not collapse it into `completed_at`. |
| Strategy | `strategic_plan` | yes | no | no | Multi-year body with `complete` ≠ `archived`. |
| Execution | `backlog_item`, `technical_debt`, `agent_task` | via `effort_aware` | **yes** | no | Lifecycle `estimated_effort` on in_progress / work-done. `actual_effort` is autofilled, not a typed hold. |
| Commitment | `requirement` | yes | no | no | Body of work. Satisfaction lives on child `criteria_refs`, not on the requirement timesheet. |
| Verification body | `test_case`, `convergence_session` | yes | no | no | `complete` / `completed` is the **run** finishing. Evidence may satisfy a criterion; the run itself is work. |
| Standing intent | `mission`, `vision` | **no** | no | no | `draft` → `active` → `archived` only. No work-done hop. |
| Predicate | `criteria` | **no** | no | **yes** | Category (`acceptance`, `test`, `compliance`, …) is a **field**. `validated` or `complete` already counts as the proposition holding for parent gates (`CriterionStatusMeetsMilestoneGateForMilestone`). Not a work clock. |
| Lexicon / standing rule | `policy`, `glossary_term` | **no** | no | no | Standing definitions, not predicates-in-flight. |
| Telemetry | `audit_event`, `change_journal_entry`, metrics | **no** | no | no | `completed` is ingest, not work. |

**Do not** put `effort_aware` on Gantt containers. After effort lifts off `base_object`, those kinds **lose** inherited timesheet columns — that is the leak closing, not a gap. Capacity of a column is the sum of `effort_aware` children.

### Spec shape (target)

Single inheritance, insert mixins — do not diamond-inherit:

```
auditable
  └─ base_object             # identity, refs — NO clocks, NO effort
      └─ work_interval       # started_at, completed_at  (completable)
          ├─ occupancy       # + claimed_by, claimed_at (occupiable) — inherit for Gantt
          ├─ roadmap, workstream, priority_plan, goal, strategic_plan
          ├─ requirement, test_case, convergence_session
          └─ work_unit       # + estimated_effort, actual_effort (effort_aware)
              ├─ backlog_item
              ├─ milestone
              ├─ technical_debt
              └─ agent_task  # composes occupancy (timesheet + occupancy)
```

`work_interval` / `work_unit` / `occupancy` YAML mixins exist (`skip_specs`, not CLI kinds). Leaf kinds still list `completable` / `effort_aware` / `occupiable` on the object spec so `KindHasTrait` stays explicit. Occupancy is **not** a `work_unit` child: putting it there split Gantt columns (PRI) from timesheets (ATK). Occupancy extends `work_interval`. Timesheet kinds that also need occupancy **compose** it (`composes: [occupancy]`) and keep `extends: work_unit`. Gantt bodies inherit by switching `extends` to occupancy.

Object-level traits (names = YAML stems; no Go const table):

- `completable` — list on kinds that finish a work interval without a timesheet.
- `effort_aware` — `includes: [completable]`; listing `effort_aware` is enough.
- `occupiable` — occupancy slot (`claimed_by`, `claimed_at`). The slot is open or held; empty `claimed_by` is unoccupied. Exclusive **claim** fills the slot; **assignment** (`persona_refs`) does not. Fields live on the `occupancy` mixin (extends `work_interval`). Timesheets compose occupancy; Gantt columns inherit it.
- `satisfiable` — list on kinds whose done-state is “the proposition holds” (`criteria`). Does not compose `completable`.

**Forbidden:** `if kind == backlog_item { … } else if kind == milestone`. Storage, validation, list projections, and autofix ask the trait/spec.

### Lifecycle: work-done vs archive-of-record

Status tokens differ (`complete` vs `resolved`). Do not special-case the string `"complete"`.

Mark the **status** (lifecycle YAML) with a flag, e.g. `work_done: true`, on:

- `backlog_item.complete`, `milestone.complete`, `agent_task.implemented`
- `technical_debt.resolved` (not `complete` — TDE has no such status)
- Gantt/strategy work-done: `priority_plan.complete`, `workstream.complete`, `roadmap.complete`, `goal.complete`, `strategic_plan.complete`
- Verification: `test_case.complete`, `convergence_session.completed`
- Commitment: `requirement.complete`

`archived` stays an **auditable** hop. Autofill of `completed_at` must **not** fire on archive.

Execution-locked statuses (`in_progress`, TDE `in_progress`, CVS `active`, test_case `metrics_captured`) stamp `started_at` when the kind is `completable`. Criteria `in_progress` is execution-locked but **not** completable — no work clock.

## Autofill (process-admin overhead → 0)

Admins promote/complete. Clocks and actuals fill themselves. Humans may **override down** (honest understatement). They must not persist actual **above** the work (or record-age fallback) span.

| Hop | Facet | Autofill (only if empty, unless noted) |
|-----|-------|----------------------------------------|
| → execution-locked (`in_progress`, …) | `completable` | `started_at` = `updated_at` / now (`LayoutObjectDateTimeZ`) |
| → work-done | `completable` | `completed_at` = `updated_at` / now |
| → work-done | `effort_aware` | If `actual_effort` empty: format(`completed_at` − `started_at`); if `started_at` missing, fall back to `created_at` (record-age bound) and still stamp `started_at` from `created_at` when that is the only clock |
| → work-done | `effort_aware` | If `actual_effort` already set **and** exceeds the span: **clamp** (same rounding as `ClampActualEffortToWallClock`) |
| any | `effort_aware` | **Never** copy `estimated_effort` onto `actual_effort` |
| any | `effort_aware` | **Never** autofill `estimated_effort` from calendar |

Idempotent: already-set `started_at` / `completed_at` are not overwritten on later field edits of a complete object (do not re-stamp from a bumped `updated_at`).

Wall-clock clamp during **validate** is a **warning**, not a blocking field error. Persist is the transition membrane. Lifecycle **break-glass / `--override`** skips clamp the same way it skips auto-only status edges (an override may persist an overstated actual). Process admins do not “fix” `actual_effort` on the hop.

### What process admins still type

- `estimated_effort` at shovel-ready / in_progress **precondition** (planner intent).
- Optional honest actual **less than** the span (pair programming, wait time).
- Nothing for clocks on happy-path promote/complete.

## Today’s snowflakes (remove)

| Site | Status |
|------|--------|
| `base_object` effort fields | Lifted onto `work_unit` (`CRIT-KERNEL-WORK-ENVELOPE-BASE-LIFT-001`) |
| Leaf `completed_at` copies | Inherited from `work_interval` (including TDE / ATK) |
| `requirement` effort redeclares | Removed; requirement is completable without a timesheet |
| `kindsWithActualEffortOnComplete` | Replaced by `KindHasTrait` + `work_done` (prior cut) |
| Hybrid projection extra slots for BLI effort only | Trait-gated: `effort_aware` slots `estimated_effort`/`actual_effort`; `completable` slots `started_at` (`completed_at` stays global) |

## Storage role

`started_at`, `completed_at`, `actual_effort`, and `status` are **runtime_delta** candidates (high churn, not identity). `estimated_effort` is structural-enough for planning reports; default structural is acceptable until a delta pass.

## Implementation order

1. Architecture + kernel objects (this doc, `REQ-*` / `CRIT-*` / `BLI-KERNEL-WORK-ENVELOPE-001`).
2. **Trait DNA (landed):** `docs/process/_internal/traits/{completable,effort_aware,satisfiable,occupiable}.yaml`; kinds opt in via object-spec `traits:`. `effort_aware.includes` composes `completable`. `satisfiable` does **not**. Occupancy fields live on the `occupancy` mixin (`extends: work_interval`); timesheets **compose** it.
3. **Work-envelope defaults (landed):** `applyCompleteTransitionDefaults` keys off `KindHasTrait(completable|effort_aware)` + lifecycle `work_done` / `execution_locked`. TDE `resolved` and ATK `implemented` stamp clocks. Criteria `satisfied: true` on `validated`/`complete` drives `CriterionStatusMeetsMilestoneGateForMilestone` — not a `completed_at` stamp.
4. **BASE-LIFT (this cut):** `work_interval` / `work_unit` mixins; effort off `base_object`; leaf `completed_at` / requirement effort redeclares deleted; instance builders regenerated (`CRIT-KERNEL-WORK-ENVELOPE-BASE-LIFT-001`).
5. **Projection / clamp / autofix (this cut):** trait-gated, not `KindBacklogItem`. Hybrid extra slots follow `KindHasTrait`; wall-clock clamp/leftover/autofix skip non-`effort_aware` kinds.
6. **Backfill (this cut):** complete/resolved effort-aware objects missing `completed_at` get it from historical `updated_at` **once** via `zqk system kernel-integrity backfill-work-envelope` (dry-run default; `--apply` to write). Not silent mutate on `object get`.

## Related

- [`KERNEL_COHERENCE_AND_REF_GRAPH.md`](./KERNEL_COHERENCE_AND_REF_GRAPH.md) — graph honesty; this contract is **field/lifecycle** honesty.
- [`CAS_MUTATION_SHOCKWAVE.md`](./CAS_MUTATION_SHOCKWAVE.md) — BLI → `complete` shockwave; envelope autofill runs **inside** the same status hop, before validation.
- `pkg/storage/lifecycle_complete_defaults.go` — work-envelope autofill keyed by trait + `work_done`.
- `pkg/validation/effort_wallclock.go` — span math; keep, retarget start clock to `started_at`.
