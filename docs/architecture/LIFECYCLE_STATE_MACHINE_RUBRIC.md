# Lifecycle state-machine rubric

**Last Verified:** 2026-08-31


**Status:** Active  
**TRACK:** `REDACTED` (priority_plan check valve)  
**Complements:** [LIFECYCLE_STATUS_ROLES.md](./LIFECYCLE_STATUS_ROLES.md), [lifecycle_taxonomy.md](../process/architecture/lifecycle_taxonomy.md), [POLICY_LIFECYCLE.md](./POLICY_LIFECYCLE.md), [LIFECYCLE_DEFINITIONS_EXPLAINED.md](./LIFECYCLE_DEFINITIONS_EXPLAINED.md), `docs/process/_internal/lifecycles/README.md`

This is the design exam for **every** `*_lifecycle.yaml`. Status names are local. **Roles** (`status.role`) are the class plane. Edges that look identical (`paused → active`) are legal on one kind and a membrane leak on another.

## Discrete method: N! then prune

For a kind with status set \(S\) of size \(n\):

1. **Enumerate** the complete directed graph without self-loops: \(n(n-1)\) candidate edges, plus any `* → archived` (or `* → error`) wildcards.
2. **Project onto class roles** (the six canonical roles in `.zqk/cli/specs/schemas/lifecycle_spec.schema.json`): `grooming`, `shovel_ready`, `execution_locked`, `realign`, `halted`, `terminal`. The class graph is smaller and is the invariant you actually want.
3. **Prune** any edge that fails the five questions below, or that **launders** a class invariant (same destination reachable by skipping a qualifying exam).
4. **Keep** only edges that have a named **catalyst** (origin, manual, auto, shockwave, qualifying exam) and a **postcondition** that is not already implied by the destination status occupying the object.

Do not add an edge because a UI “Resume” button historically targeted `active`. Resume is a class hop: halt → whatever role that kind uses for *live work*, which is **not always** `shovel_ready`.

## Class state vs object state

| Plane | What it is | Example |
|-------|------------|---------|
| **Class (role)** | Cross-kind semantic occupancy | `halted`, `execution_locked` |
| **Object (value)** | Kind-local token | priority_plan `paused`; convergence_session `paused`; policy `active` |

Homonyms are expected. `active` on a **priority_plan** is shovel-ready (sealed, not yet locked). `active` on a **policy** or **role** is membrane-live (`enforced`). `active` on a **convergence_session** is execution-locked. Do not let Gantt ranking or PRI check-valve copy rules treat POL/ROL `active` as a column seal. See [POLICY_LIFECYCLE.md](./POLICY_LIFECYCLE.md).

**Start every campaign with policies.** They have almost no execution state, yet they are the membrane that other objects’ hops must satisfy. A thin policy machine is correct; a missing *exam* for “is this policy live?” is not.

## Five questions (status and every remaining edge)

Answer all five **in writing** on the lifecycle YAML (`description`, `preconditions`, `postconditions`, `on_dependent_status`, `side_effects`) or the hop is not designed.

### 1. Catalyst — what caused occupancy?

| Catalyst | Typical for | Notes |
|----------|-------------|--------|
| **Origin / instantiate** | `origin: true` statuses | Object creation. Easy exam. |
| **Qualifying exam** | Seal, lock, complete | Preconditions that are *evaluated*, not assumed. The exam itself is the trigger. |
| **Manual** | Operator promote / pause / cancel | First-class when `manual: true`. |
| **Auto / shockwave** | Child status, version change, effective date | Must declare `on_dependent_status` (or equivalent DSL) on strict kinds. |
| **Re-entry** | Terminal → live | A **new** closed system, not a resume of the previous one. Stronger exam than halt resume. |

Draft/preliminary occupancy is answered by origination. Later statuses are **worthiness** exams: configuration and graph must already be true, then the hop certifies them.

### 2. Preconditions — what must be true to *attempt* the hop?

Transition `preconditions` are the qualifying exam. Status `preconditions` (e.g. priority_plan `complete`) are **holds**: they must remain true while the object occupies that status, not only at the instant of entry.

### 3. Postconditions — what must be true *after* a successful hop?

Schema field: `transitions[].postconditions` (declarative strings; evaluator not required yet). Typical claims: destination `role`, `active_order` unset, membership membrane (no new BLIs), ledger seeded. If a `side_effects.clear` exists, the matching postcondition should name the cleared field.

Do not duplicate the destination status’s entire hold list unless the hop can leave the object in that status while violating the hold (that is a bug).

### 4. Shockwave — which system events may fire this hop?

Record `on_dependent_status`, WAL/`executeLifecycleHook`, version/effective-date rules, or “manual only.” An auto-only edge without a trigger is a hidden door (`PromoteTransitionTargets` will not see it; break-glass historically skipped validation). Dual `manual: true` + `auto: true` when operators must be able to take the same hop. See [LIFECYCLE_STATUS_ROLES.md](./LIFECYCLE_STATUS_ROLES.md) § First-class promote.

### 5. Similarity — is this a class hop or a kind specialty?

If the same role-to-role hop appears on many kinds, the **class** rule belongs in roles + contract tests, not copy-pasted YAML comments. Kind specialty is what remains after the class rule (PRI membership via `priority_plan_ref`, policy `related_patterns` on supersede, CVS phase C1–C6).

## Check valve (execution columns)

For kinds whose `in_progress` (or equivalent) is **`execution_locked`** and whose `active` is **`shovel_ready`** (priority_plan today):

- Lock (`shovel_ready → execution_locked`) is a **check valve**.
- Exits from lock: `halted` (pause/block), `terminal` (complete/cancel). **Not** back to `shovel_ready` or `grooming`.
- Halt resume: `halted → execution_locked` (re-lock), or auto `halted → grooming` only when children realign (`exploring`/`validated`). **Not** `halted → shovel_ready`.

`in_progress → paused → active` is the same leak as `in_progress → active`. Closing one edge without the other is not a closed system.

This valve does **not** apply to kinds where `active` *is* live work (policy enforced, goal commitment, CVS `active` = `execution_locked`). Copying PRI’s valve onto those kinds is the opposite error.

## Worked example: `priority_plan`

**Statuses (\(n=8\)):** `grooming`, `active`, `in_progress`, `paused`, `blocked`, `complete`, `cancelled`, `archived`.

**Complete graph:** \(8\times7=56\) directed edges (plus `* → archived`).

**Class occupancy:**

| Value | Role | Catalyst (summary) |
|-------|------|--------------------|
| `grooming` | `grooming` | Origin: instantiate intake column; membership open |
| `active` | `shovel_ready` | Exam: ≥1 planned child + team/persona; seal |
| `in_progress` | `execution_locked` | Shockwave: first child `in_progress`; check valve |
| `paused` / `blocked` | `halted` | Manual halt; not shovel-ready |
| `complete` / `cancelled` / `archived` | `terminal` | Work done, abort, or history |

**Pruned happy path:** `grooming → active → in_progress → complete`.

**Halt:** `active|in_progress → paused|blocked`. Resume: `paused|blocked → in_progress` only (re-lock). Child reopen: auto `→ grooming`. Cancel from halt allowed.

**Forbidden (launder / skip):** `in_progress → active`, `paused → active`, `blocked → active`. Contract: `TestLifecycleContract_PriorityPlanExecutionCheckValve`, `TestLifecycleContract_PriorityPlanHaltDoesNotResumeToShovelReady`.

**Class catalog (planes, families, all 65 kinds):** [LIFECYCLE_SHOCKWAVE_MAP.md](./LIFECYCLE_SHOCKWAVE_MAP.md). **PRI filled exam:** [lifecycle_shockwave/kind_priority_plan.md](./lifecycle_shockwave/kind_priority_plan.md). **Kind index:** [lifecycle_shockwave/KIND_INDEX.md](./lifecycle_shockwave/KIND_INDEX.md).

**Remaining prune candidates (not closed this pass):**

| Edge | Why it is suspect |
|------|-------------------|
| `grooming → in_progress` | Skip-seal shockwave repair |
| `archived → in_progress` | Skip shovel-ready on restore |
| `complete → active` | Terminal re-entry; exam should require remaining open children, not only team/persona |
| `active → grooming` | Unseal after seal; child-realign only (auto) — keep as shockwave, not manual resume |

## Applying this to every lifecycle

Do **not** clone the PRI hop tables. Inherit [the catalog](./LIFECYCLE_SHOCKWAVE_MAP.md) → a [family](./lifecycle_shockwave/README.md) → one [kind-index](./lifecycle_shockwave/KIND_INDEX.md) row. Write `kind_*.md` only when remaining edges need their own table.

1. List \(S\), compute \(n(n-1)\), list current YAML edges, subtract: that is the undocumented remainder (usually correctly absent) **and** the extra edges that should not exist.
2. Assign the **family** from occupancy of `active` / `in_progress` (hub §3), not from spec `extends` alone. Fill five questions only for **Q5 specialty** plus any edge the family does not already classify.
3. Add a **kind-specific** contract test for any class valve you claim (do not globalize PRI’s `halted ↛ shovel_ready` — workstream `paused → active` and goal `blocked → active` need their own exams; CVS `paused → active` is halt → `execution_locked` and is already the correct resume).
4. Policies first: even a linear `draft → under_review → active → {deprecated,superseded,archived}` must say what “active” *means* (enforced membrane) and which kernel hops consult it. Family: [membrane](./lifecycle_shockwave/family_membrane.md).

Schema: `preconditions` / `postconditions` / `on_dependent_status` / `side_effects` on `.zqk/cli/specs/schemas/lifecycle_spec.schema.json`. Go: `pkg/objects.Transition`.
