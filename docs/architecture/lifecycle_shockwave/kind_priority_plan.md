# Kind exam: `priority_plan`

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Status:** Active exam record (2026-08-30)  
**Family:** [gantt_column](./family_gantt_column.md)  
**Class catalog (planes, listeners, overlay ops):** [LIFECYCLE_SHOCKWAVE_MAP.md](../LIFECYCLE_SHOCKWAVE_MAP.md)  
**SSOT YAML:** `.zqk/specs/lifecycles/priority_plan_lifecycle.yaml`  
**TRACK:** `BLI-1785439369431933000-f0cccd6c` (check valve), `BLI-1785439367722386000-7bd43e71` (YAML matcher), `BLI-1785784867143912000-635942fb` (`on_all_dependents_status`)

## Overview

Filled five-question exam for **`priority_plan`** — occupancy lock, check valve, and last-child complete. Other compiled Plane C targets (`milestone`, `goal`, `criteria`) inherit the hub matcher; this page is PRI specialty.

Gantt partners are **trigger kinds or seal exams**, not a second copy of this machine. `applyDependencyRefEvent` returns unless `kind == priority_plan`.

---

## 1. Trigger graph (who fires whom)

```
backlog_item status save
        │
        ▼
  Plane C: ApplyDependencyRefEvents
        │  targets = outbound refs ∪ reverse-index dependents
        │
        ├─ target kind ≠ priority_plan  →  no-op
        │
        ├─ BLI → terminal (from non-terminal)
        │     + plan active|in_progress  →  Plane E (last-child complete)
        │
        ├─ BLI → exploring|validated (from terminal)
        │     → increment remaining_open_count
        │     + YAML match active|paused|blocked → grooming  →  Plane C matcher
        │
        ├─ YAML on_dependent_status match (BLI → in_progress)
        │     + ready-or-later exam (Go special case)  →  lock + clear active_order
        │     + seed remaining_open_count (cold List at lock)
        │
        └─ no YAML match  →  Plane F occupancy lock
              (all linked ready-or-later ∧ ≥1 in_progress)

agent_task / workstream / goal / milestone status save
        │
        ▼
  Plane C walks neighbors, but PRI matcher ignores non-BLI trigger_kind
  → those kinds are not PRI shockwave catalysts today
```

Membership edge is **child-owned**: `backlog_item.priority_plan_ref`. Reverse-index dependents of a BLI include the plan; outbound refs of a BLI include the plan. Either direction is enough for one-level walk.

**Not a PRI shockwave:** ATK complete, workstream pause, goal block, milestone hop. Those machines have their own edges (see their families). If a Gantt partner must refuse a PRI hop, that is a **Plane A exam** (ref exists / status role), not a new `Subscribe()`.

---

## 2. Occupancy: into-state holds (questions 2–3 on the **status**)

Transition preconditions gate **entry**. Status preconditions (and role) are **holds** while occupying. Shockwave `side_effects` are postconditions of the hop that produced this occupancy.

| Status | Role | How you get here | Hold while occupying | Postcondition of the hop that entered | Listener on **entry** |
|--------|------|------------------|----------------------|--------------------------------------|------------------------|
| `grooming` | `grooming` | Origin instantiate; shockwave unseal; `complete → grooming` | Membership **open** (new BLIs allowed) | Intake column; `active_order` may still be set until a later lock/cancel | Instantiate; C matcher (`→ grooming`); B reopen |
| `active` | `shovel_ready` | Manual seal; `complete → active` | Membership **sealed**; ≥1 planned child **should** still hold (exam not evaluated); unique numeric `active_order` | Column sealed, not locked | **A + B only.** Then F (`MaybeExecutionLockPlan`) if children already in flight |
| `in_progress` | `execution_locked` | Shockwave lock; halt resume; occupancy lock; (suspect) `archived → in_progress` | Scope expansion refused; check valve | `active_order` **unset**; `remaining_open_count` **seeded** | **C matcher + F.** CAP (I) should observe this occupancy |
| `paused` / `blocked` | `halted` | Manual from `active` \| `in_progress` | Not shovel-ready; not a resume-to-`active` parking lot | Halt; work frozen | **B only** |
| `complete` | `terminal` (`work_done`) | Last-child; promote; grooming escape | **All linked BLIs terminal** (YAML status hold) | `active_order` unset | **E + B.** A must evaluate the hold (today fail-open) |
| `cancelled` | `terminal` | Manual from live statuses | Aborted column | `active_order` unset | **B** |
| `archived` | `terminal` (`archive`) | `* → archived` | Historical blob remains | `active_order` unset; delete-worthiness recomputed | **B + CAS erase plane** |

`applyDependencyRefEvent` **skips terminal plans**. Child reopen cannot un-complete a plan via shockwave; that is manual re-entry (B) with a stronger exam than it has today.

---

## 3. Remaining edges: five questions → listener

`n=8` → 56 directed edges + `* → archived`. YAML keeps the set below. Forbidden check-valve edges (`in_progress → active|grooming`, `paused|blocked → active`) are **absent**; no listener should synthesize them.

Legend for **Q4 listener:** plane letter from the [hub](../LIFECYCLE_SHOCKWAVE_MAP.md).

### 3.1 Qualifying exam (manual) — Plane A + B

These hops are **not** shockwave. Overlay DSL must compile the prose into ops. `OpLifecyclePreconditions` is the named slot; it must stop being a no-op.

| Edge | Q1 Catalyst | Q2 Preconditions (YAML) | Q3 Postconditions | Q4 Shockwave | Q5 Class vs specialty | Listener |
|------|-------------|-------------------------|-------------------|--------------|----------------------|----------|
| `grooming → active` | Qualifying exam (seal) | ≥1 `planned` BLI via `priority_plan_ref`; team_configuration_ref **or** persona_refs; plan validated; workflow_ref constraints if set | Role `shovel_ready`; membership sealed; `active_order` assignable | Manual only | **Class:** `grooming → shovel_ready`. **Specialty:** child-owned membership + dispatch identity | **A** fail-closed planned-child + team exam. **B** promote. Then **F** if occupancy already locked in children |
| `grooming → complete` | Qualifying exam (escape hatch) | All linked BLIs terminal | Role terminal; `active_order` unset | Manual only | Specialty: intake with no fuel left | **A + B** |
| `active → paused` / `active → blocked` | Manual halt | (none declared) | Role `halted`; not shovel-ready | Manual only | **Class:** `shovel_ready → halted` | **B** |
| `in_progress → paused` / `in_progress → blocked` | Manual halt | (none declared) | Role `halted`; check valve preserved | Manual only | **Class:** `execution_locked → halted` | **B** |
| `* → cancelled` (from grooming/active/in_progress/paused/blocked) | Manual abort | (none declared) | Role terminal; `active_order` unset | Manual only | **Class:** live → terminal abort | **B** |
| `* → archived` | Manual archive | (none declared) | Archive hold; `active_order` unset; delete-worthiness fact | Manual only | **Class:** any → archive | **B + CAS linger/GC** |
| `complete → grooming` | Re-entry (new closed system) | (none declared) | Membership re-opens | Manual only | Specialty: scope must grow; stronger than halt resume | **B**; **A** should require an explicit reason / remaining open work |
| `complete → active` | Re-entry | team/persona only | Role `shovel_ready` | Manual only | **Suspect** (rubric): exam should require remaining **open** children, not only dispatch identity | **A + B** — do not reuse seal-from-grooming without the open-child exam |
| `archived → in_progress` | Re-entry | (none declared) | Role `execution_locked` | Manual only | **Suspect:** skips shovel-ready | **B**; prune or force `archived → grooming` then seal |

**Seal hole (closed 2026-08-30):** `checkReadyBacklogReferencesPlan` now runs from `validateLifecycleState` via restored `dispatchPrecondition`. YAML `grooming → active` binds when lookups are present. `PrecondWorkflowConstraintsIfSet` is recognized: unset `workflow_ref` is vacuous true; a set ref fails closed unless the workflow exists, is kind `workflow`, and `enabled`. `PrecondPriorityPlanValidated` is recognized: inherited `title` **or** specialized `description`, plus `workstream_refs` **or** leftover singular `workstream_ref`. Persona/team and planned-child stay separate tokens. Remaining fail-open is overlay-unrecognized English on other edges, not this seal sentence.

**Promote probe (Plane B, 2026-08-30):** `promoteForwardProbeOrder` drops `complete` only when an **execution** hop (`in_progress`) is also on the probe list, so a planned BLI cannot skip to complete. After a seal hop (`active`) with no execution hop, `complete` stays as the `grooming → complete` escape so a dead column can still promote. TRACK: `BLI-CEF-R26-REMAINING-KINDS-001`.

### 3.2 Check-valve lock (auto + dual) — Plane C matcher + F

YAML matcher: auto edges with `on_dependent_status.kind: backlog_item` and `to: in_progress`. Go additionally refuses the lock unless **all linked children are ready-or-later** (`planLinkedBacklogReadyOrLater`). Team/persona on these edges is **not** evaluated here.

| Edge | Q1 Catalyst | Q2 Preconditions | Q3 Postconditions | Q4 Shockwave | Q5 Class vs specialty | Listener |
|------|-------------|------------------|-------------------|--------------|----------------------|----------|
| `active → in_progress` | Auto: first child `in_progress` (or F if siblings already in flight) | All linked BLIs ready-or-later (**evaluated in C**); team/persona (**not evaluated in C**) | Role `execution_locked`; `active_order` unset (`side_effects`); ledger seeded | `on_dependent_status` BLI→`in_progress` | **Class:** `shovel_ready → execution_locked` (check valve) | **C matcher.** **F** occupancy. **I** CAP after commit. Move ready-or-later + team into **A** so C stays a thin executor |
| `grooming → in_progress` | Auto: child started during intake | Same as lock | Same; **skips seal** | Same trigger | **Suspect** (rubric skip-seal repair). Keep until seal exam is fail-closed, then prune | Same as lock |
| `paused → in_progress` | Dual: manual resume **or** child `in_progress` | Same as lock | YAML postconditions: role `execution_locked`; `active_order` unset | Manual **and** `on_dependent_status` | **Class:** `halted → execution_locked` (PRI valve: not → `shovel_ready`) | **B** (manual) **and** **C** (child restart) |
| `blocked → in_progress` | Dual: same | Same as lock | Same YAML postconditions | Same | Same class hop | Same |

**No** `in_progress → grooming` edge. Child reopen while locked does **not** unseal. That is the valve. YAML realign (`→ grooming`) is only from `active` \| `paused` \| `blocked`.

### 3.3 Child realign (auto unseal) — Plane C matcher

Trigger: BLI `to: exploring` **or** `validated` (typically from terminal — Go also notes open-set re-entry).

| Edge | Q1 Catalyst | Q2 Preconditions | Q3 Postconditions | Q4 Shockwave | Q5 Class vs specialty | Listener |
|------|-------------|------------------|-------------------|--------------|----------------------|----------|
| `active → grooming` | Auto: linked work reopened below shovel-ready | (none declared) | Role `grooming`; membership re-opens | `on_dependent_status` BLI→`exploring`\|`validated` | **Class:** `shovel_ready → grooming` unseal. Keep auto-only (not a Resume button) | **C matcher** |
| `paused → grooming` | Same | (none) | Halt abandoned for intake | Same | `halted → grooming` because children realigned, not because operator resumed | **C matcher** |
| `blocked → grooming` | Same | (none) | Same | Same | Same | **C matcher** |

If a **complete** plan’s child is reopened, C **does not** fire (terminal skip). Operator must take `complete → grooming` / `complete → active` (B) with a real exam.

### 3.4 Last-child complete (dual) — Plane E, not the YAML matcher

These edges are `manual: true` + `auto: true` **without** `on_dependent_status`. The matcher never sees them. Auto path is ledger shrink.

| Edge | Q1 Catalyst | Q2 Preconditions | Q3 Postconditions | Q4 Shockwave | Q5 Class vs specialty | Listener |
|------|-------------|------------------|-------------------|--------------|----------------------|----------|
| `in_progress → complete` | Last linked BLI → terminal (ledger empty) **or** promote when holds pass | All linked BLIs terminal; `branch_ref` is ancestor of trunk | Role terminal; `active_order` unset | Dual: E auto + B promote. Not `on_dependent_status` | **Class:** `execution_locked → terminal` (work_done). **Specialty:** child-owned membership + branch provenance | **E** (auto). **B** (promote). **A** must evaluate both preconditions (branch_ref currently prose). Compile as `on_all_dependents_status` |
| `active → complete` | Same when closeout runs on a still-shovel-ready column (children all terminal, lock never happened) | Same | Same | Same | Closeout failed to lock first; still legal | Same |

---

## 4. Gantt partners (what this exam does **not** duplicate)

| Kind | Relation to PRI | Listener implication |
|------|-----------------|----------------------|
| `backlog_item` | **Only** compiled PRI shockwave **trigger** | Family [work_unit](./family_work_unit.md). ATK complete does **not** complete a BLI via Plane C |
| `agent_task` | Composition under BLI, not PRI | Hourglass / `agent next` is feed policy, not this shockwave |
| `workstream` / `goal` / `milestone` / `roadmap` | Gantt ranking / refs on the plan | Seal **preconditions** (Plane A). `paused → active` / `blocked → active` on those kinds are **their** class hops ([gantt_lane](./family_gantt_lane.md)) |
| `policy` / `role` | Membrane | Start-of-campaign exams; `active` means enforced ([membrane](./family_membrane.md)) |

Partner YAML postconditions (R26 gantt-partner exam) answer questions 3 and 5 **on those kinds**. They do not add PRI subscribers.

---

## 5. Implementation order (PRI)

Same listener order as the hub, with PRI-specific prune list:

1. Plane A fail-closed for `grooming → active` planned-child exam (done). Same op family for complete holds.  
2. Keep Plane C as YAML matcher; move ready-or-later into the overlay.  
3. Name Plane E as `on_all_dependents_status`.  
4. CAP (I) on `execution_locked` entry.  
5. Prune suspect edges (`grooming → in_progress`, `archived → in_progress`, weak `complete → active`) only after remaining overlay English is fail-closed.

Contract tests: `TestLifecycleContract_PriorityPlanExecutionCheckValve`, `TestLifecycleContract_PriorityPlanHaltDoesNotResumeToShovelReady`, `strictAutoTriggerKinds`.

---

## 6. Field DNA (specialized vs inherited) — keep lean

`priority_plan` extends `work_interval` → `base_object` → `auditable`. CLI `zqk object priority_plan fields` splits **29 common** vs **20 specialized**. Seal and GROOM-AHEAD must drive **inherited clocks and identity first**, then specialized Gantt/dispatch fields. Off-spec extras (`goal_refs`, `roadmap_ref` on instances) are not ontology — do not store them; use `related_object_refs` or add a spec field.

| Field | Plane | Logic today | Verdict |
|-------|--------|-------------|---------|
| `title` | inherited | UI, search, whats-next | **Keep. Seal identity.** Required at create. |
| `description` | specialized (duplicates title/context) | optional prose | **Keep optional.** Do not require it when title is set. |
| `workstream_refs` | specialized | Gantt lane, autofix, seal | **Keep.** Canonical lane. |
| `workstream_ref` | specialized singular | alias in some paths | **Collapse** into plural; keep as read alias until instances migrate. |
| `persona_refs` / `team_configuration_ref` | specialized | CAP dispatch, shaped, seal | **Keep.** Separate seal token. |
| `workflow_ref` | specialized | activation constraints | **Keep.** Vacuous if unset. |
| `active_order` | specialized | whats-next rank; cleared on lock | **Keep.** Shockwave postcondition. |
| `branch_ref` / `base_sha` | specialized | complete provenance | **Keep.** Complete exam (still partly prose). |
| `started_at` / `completed_at` | work_interval (`completable`) | wall-clock membrane | **Keep.** Inherited work clock — do not copy onto the leaf spec. |
| `related_object_refs` | inherited | graph extras | **Use** for goal/milestone/roadmap until those are spec fields. |
| `next_plan_id` / `previous_plan_id` / `plan_version` / `rationale` / `release_ref` | specialized | **no Go const usage** outside builders | **Remove or wire.** Dead storage. TRACK: `BLI-CEF-R26-REMAINING-KINDS-001` |
| `note` / `plan_date` / `source_file` / `source_format` | specialized | import/dashboard, not seal | **Keep off the seal exam.** Candidates to drop if import retires. |
| `id` redeclared on PRI spec | overlay pattern | identity | **Keep** as pattern overlay, not a second id. |

Do not mint replacement fields onto sealed R26. Spec deletes go through `.zqk/specs/objects/priority_plan.yaml` + builder regen, then instance migrate via CLI.
