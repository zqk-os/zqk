# Lifecycle shockwave map (kernel catalog)

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Status:** Active (2026-08-30)  
**Rubric:** [LIFECYCLE_STATE_MACHINE_RUBRIC.md](./LIFECYCLE_STATE_MACHINE_RUBRIC.md)  
**Roles:** [LIFECYCLE_STATUS_ROLES.md](./LIFECYCLE_STATUS_ROLES.md)  
**Kind index (all 65):** [lifecycle_shockwave/KIND_INDEX.md](./lifecycle_shockwave/KIND_INDEX.md)  
**CUD shockwave:** [CAS_MUTATION_SHOCKWAVE.md](./CAS_MUTATION_SHOCKWAVE.md)  
**WAL / criteria listener:** [LIFECYCLE_EVENT_LISTENER_AND_CRITERIA.md](./LIFECYCLE_EVENT_LISTENER_AND_CRITERIA.md)  
**TRACK:** `BLI-CEF-R26-REMAINING-KINDS-001`, `REDACTED`, `REDACTED`

## Overview

This is the **class catalog**. Planes, class hops, and listener assignment live here once. Every kernel object inherits them through a **family**. Kind specialty is a delta, not a second 200-line exam.

**Filled specialty today:** [`priority_plan`](./lifecycle_shockwave/kind_priority_plan.md) (check valve + occupancy lock). Compiled Plane C **targets** (`status_reactive` + YAML `on_dependent_status`): `priority_plan`, `milestone`, `goal`, `criteria` (parent-lock). Every other kind is in the [kind index](./lifecycle_shockwave/KIND_INDEX.md) pointing at a family.

Do **not** clone the PRI exam onto policy, CVS, goal, or BLI. Homonyms: PRI `active` = shovel-ready; POL/ROL `active` = enforced; CVS `active` = execution-locked.

---

## How to read (and how not to duplicate)

Three layers. Walk down; stop when the question is answered.

| Layer | What it owns | Where |
|-------|----------------|-------|
| **1. Class** | Roles, planes A–J, hop-class → listener | This file |
| **2. Family** | What `active` means; which class hops are legal; default planes | [`lifecycle_shockwave/`](./lifecycle_shockwave/README.md) |
| **3. Kind** | Field DNA, membership owner, YAML tokens, suspect edges | Index row; a `kind_*.md` only when the delta exceeds the family |

**To fill the next kind:** add/adjust one row in the kind index and, if needed, a short **Q5 specialty** bullet on the family page. Create `kind_<ontology>.md` only when the remaining edges need a table (PRI did). YAML + contract tests remain SSOT for what is legal; these pages assign listeners.

Spec inheritance (`auditable` → `base_object` → `work_interval` / `work_unit`) is **clocks and fields**. Shockwave family is **occupancy physics**. They often coincide; they are not the same axis.

---

## 1. Planes (do not collapse into one subscriber)

A hop has a catalyst. The catalyst picks the plane. Mixing planes is how prose preconditions fail-open while Go special-cases one of them.

| Plane | When it runs | Job | Code today |
|-------|----------------|-----|------------|
| **A. DECIDE / overlay** | Before the mutation commits | Qualifying exam: refuse the hop | `GoValidator.dispatchPrecondition` on the storage save path. `OpLifecyclePreconditions` remains attach-only until MutationInput carries from/to. Unrecognized English still fail-open. |
| **B. Promote CLI** | Operator `zqk object promote` / demote | Manual catalyst; probe order; first-class dual edges | `cmd/zqk/object/promote.go` |
| **C. Sync shockwave** | Inside `kernel.cas_object_transition` after status save | One hop up (outbound refs) and one hop down (reverse-index) | `storage.SetLifecycleHookHandler` → WAL append + `lifecycle.ApplyDependencyRefEvents`. Integrity-pure path. |
| **D. Coordinator subscriber** | Operational event `lifecycle.dependency_ref` | Same rules as C, for processes that emit instead of applying inline | `RegisterDependencyPropagationWithCoordinator` → `DependencyPropagationSubscriber`. Fallback, not a second policy. |
| **E. Last-child remaining-open** | Child enters **terminal** while parent occupies live work | Atomic decrement of mixin field `remaining_open_count`; zero → YAML auto complete | Trait `open_countable` + mixin `remaining_open`. Interpreter CAS-decrements that field (`applyOpenCountableLastChild`). Unset field fail-closes — do not List members. Cold seed at execution lock Lists members once and writes the field (`SeedRemainingOpenCountFromMembers`). TRACK: `BLI-CEF-CONTAINER-REMAINING-OPEN-001`. |
| **F. Occupancy lock** | Child hop did not match a lock trigger, but work is already in flight | Still lock | Compiled today only for `priority_plan` (`applyOccupancyExecutionLock` / `MaybeExecutionLockPlan`). |
| **G. Hold auditor** | `system check`, instance validation | Status **holds** while occupying | `pkg/validation` — dispatch on save and holds. Unrecognized English still fail-open. |
| **H. Cache / UX** | Any mutation | Invalidate caches, check progress | `CacheEventSubscriber`. **Not** occupancy policy. |
| **I. CAP / dispatch** | After an object occupies `execution_locked` | Team cell / persona identity | YAML may claim it; **no listener evaluates it** on shockwave. TRACK: `REDACTED`. |
| **J. WAL criteria updater** | CriterionSatisfied → enqueue auto hop | Designed listener for “all children done” without a List | `pkg/lifecycle/listener.go` — parallel to E; do not invent a third complete path. |

**Rule:** qualifying-exam hops (seal, complete, terminal re-entry, membrane activate) belong on **A + B**. Child-status hops belong on **C + D** (same YAML matcher). Rollup complete belongs on **E** (or J once DSL exists). Do not put seal exams on the shockwave subscriber.

**Compiled vs exam-only:** Plane C publishes one event per **outbound ref** on the catalyst (listener stubs). Each `status_reactive` target interprets the event from its own lifecycle. A no-op does not walk the graph; a self-update is a new catalyst. Reverse-index dependents are not part of this hop.

`strictAutoTriggerKinds` is only `priority_plan`. Auto-only edges on other kinds without `on_dependent_status` are completion-rollup allowlist / fail-open English until they bind.

---

## 2. Class hops (role → role)

Status **values** are local. These **role** hops are the invariant. Families say which of them are legal; kinds say the exam tokens.

| Class hop | Typical catalyst | Planes | Copy onto every kind? |
|-----------|------------------|--------|------------------------|
| `realign` / `grooming` → `shovel_ready` | Qualifying exam (seal / activate / plan) | **A + B** | Exam tokens are specialty. Membrane uses `enforced` instead of `shovel_ready`. |
| `shovel_ready` → `execution_locked` | Start work / first child in flight | **C + F** or **B** | **Check valve** only on [gantt_column](./lifecycle_shockwave/family_gantt_column.md). Work units start work; they are not PRI columns. |
| `execution_locked` → `halted` | Manual pause / block | **B** | Class. |
| `halted` → `execution_locked` | Resume into lock | **B** (and **C** on PRI) | Class for locked families (PRI, work_unit, session, predicate). |
| `halted` → `shovel_ready` | Resume to pickup | **B** | Legal on [gantt_lane](./lifecycle_shockwave/family_gantt_lane.md) (`paused → active`, `blocked → active`). **Forbidden** on gantt_column (PRI check valve). |
| `shovel_ready` → `grooming` | Child realign below shovel-ready | **C** | PRI compiled specialty. Do not invent on policy/CVS. |
| live → `terminal` | Complete / abort / archive | **A + B**, plus **E** when last-child | Complete exam is specialty. Archive also hits [CAS linger](./CAS_MUTATION_SHOCKWAVE.md). |
| `terminal` → live | Re-entry (new closed system) | **A + B** | Stronger exam than halt resume. Never skip shovel-ready on gantt_column. |

**Do not add** a coordinator subscriber per kind. Partner **status** is a Plane A exam when the hop is seal/complete. Partner **status change** is a compiled listener when the kind has `status_reactive` and YAML declares `on_dependent_status`. Admission is the trait; the reaction is YAML (plus a local ledger when the object owns one).

Organism specialization (`pkg/specialization`) is a different bus. Do not route occupancy policy through it.

---

## 3. Families

| Family | Occupancy of `active` (or equivalent) | Compiled shockwave | Legal `halted → shovel_ready`? | Page |
|--------|----------------------------------------|--------------------|--------------------------------|------|
| **gantt_column** | `shovel_ready` (sealed, not locked) | **Target** (`priority_plan` only) | **No** (check valve) | [family_gantt_column.md](./lifecycle_shockwave/family_gantt_column.md) |
| **gantt_lane** | `shovel_ready` (live row / commitment) | Exam-only (refs on seal) | **Yes** (resume the lane) | [family_gantt_lane.md](./lifecycle_shockwave/family_gantt_lane.md) |
| **work_unit** | Usually no `active`; `planned` = shovel-ready, `in_progress` = locked | **Trigger** = `backlog_item` only | Resume → `in_progress`, not → `planned` | [family_work_unit.md](./lifecycle_shockwave/family_work_unit.md) |
| **membrane** | `enforced` | Exam-only (other hops consult) | N/A (`active` is not shovel-ready) | [family_membrane.md](./lifecycle_shockwave/family_membrane.md) |
| **execution_session** | `execution_locked` | Exam-only | N/A (`paused → active` is halt → lock) | [family_execution_session.md](./lifecycle_shockwave/family_execution_session.md) |
| **predicate** | shovel-ready *awaiting evidence*; `in_progress` locked | Plane **C** parent-lock (`criteria`); Plane **J** designed | Specialty | [family_predicate.md](./lifecycle_shockwave/family_predicate.md) |
| **record** | Often no `active`; linear pending → done | None | N/A | [family_record.md](./lifecycle_shockwave/family_record.md) |
| **default_shovel** | `shovel_ready` | Exam-only | Usually yes (thin machines) | [family_default_shovel.md](./lifecycle_shockwave/family_default_shovel.md) |

Every kind appears once in [KIND_INDEX.md](./lifecycle_shockwave/KIND_INDEX.md).

---

## 4. Who the listeners **should** be (class assignment)

One owner per catalyst class. Kind pages do not restate this table.

| Listener | Subscribes to | Applies hops | Must evaluate | Must not do |
|----------|----------------|--------------|---------------|-------------|
| **Overlay compiler → `OpLifecyclePreconditions`** (A) | Pipeline intent `cas_object_transition` / promote | Seal, complete, cancel, archive, terminal re-entry, membrane activate | Transition `preconditions` as **recognized tokens** (fail-closed if unrecognized) | Fire child-status hops; fail-open English |
| **Promote CLI** (B) | Operator | Every `manual: true` edge | Probe order that does not skip the qualifying exam | Invent forbidden class hops (PRI `paused → active`) |
| **`ApplyDependencyRefEvents` thin executor** (C) | Status save of **any** kind; filter **target** by compiled kinds | YAML `on_dependent_status` + `side_effects` | Only what YAML names **and** overlay already allowed | Last-child complete; seal; CAP dispatch |
| **`DependencyPropagationSubscriber`** (D) | Coordinator `lifecycle.dependency_ref` | Same as C | Same as C | A second rule table |
| **Last-child remaining-open** (E) | Child non-terminal → terminal, parent live | `→ complete` | Mixin `remaining_open_count` hits 0 (CAS; YAML auto complete hop) | List members on the hop; bolt the field only onto `priority_plan`; keep a `plan_open_children` sidecar |
| **Occupancy lock** (F) | After unmatched child hop, or after B seals | `→ execution_locked` via the same YAML lock edges | Ready-or-later ∧ ≥1 child in flight | Unseal |
| **CAP dispatch** (I) | Object **entered** `execution_locked` | None (downstream) | Dispatch identity YAML already claims | Relock or unseal |
| **CAS delete-worthiness** | Object **entered** `archived` | Linger / GC | No live inbound refs | Immediate Exists-false |
| **`system check` (G)** | Periodic / CLI | None | Status **holds** | Act as a substitute for A |
| **CacheEventSubscriber (H)** | Mutations | None | Cache keys | Lifecycle policy |
| **WAL criteria (J)** | CriterionSatisfied | Auto hop named by rule | Scope + criterion id | A parallel complete path beside E |

---

## 5. Overlay / DSL ops (by hop class, not by kind)

Compiler already emits `OpLifecyclePreconditions` with **status** hold strings. Transition exams must enter the overlay. Attach by **hop class**:

| Hop class | Overlay op (name) | Fail closed when |
|-----------|-------------------|------------------|
| Seal / activate (`* → shovel_ready` or `* → enforced`) | `lifecycle_preconditions` **transition** bind | Exam tokens false; unrecognized English |
| Lock (`* → execution_locked`) | Ready-or-later / in-flight child (when membership exists) | Exploring siblings remain; missing dispatch identity where claimed |
| Complete (`* → terminal` work_done) | `on_all_dependents_status` + kind specialty (branch provenance, CRIT refs) | Open child; specialty exam false |
| Realign (`* → grooming`) | Catalyst is the child hop | — |
| Halt / cancel | Optional reason / blocker_ref | — |
| Re-entry (`terminal → live`) | Open child **or** explicit grow-scope | Vacuous reopen |

Unrecognized precondition strings must **refuse**. Validation executes compiled ops, not English.

---

## 6. Implementation order (listeners first)

1. **Plane A fail-closed** for recognized tokens (PRI seal is the prototype). Same op family for complete holds and membrane activate.  
2. **Keep Plane C** as YAML `on_dependent_status` + `side_effects` executor; do not special-case English in Go per kind.  
3. **Name Plane E** as `on_all_dependents_status` so last-child complete is not a second hidden door.  
4. **CAP (I)** subscribe to `execution_locked` entry.  
5. Bind remaining overlay English; then prune suspect edges. Do not prune skip-seal repairs while A still fail-opens.

Contract tests: `TestLifecycleContract_*` in `pkg/objects/lifecycle_contract_test.go`. Add a **kind-specific** test for any class valve you claim — do not globalize PRI’s `halted ↛ shovel_ready`.
