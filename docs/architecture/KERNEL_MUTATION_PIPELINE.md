# Kernel Mutation Pipeline

**Last Verified:** 2026-08-31


**Status:** Active (v1 membrane complete; v2 composition in progress)  
**Program (v1 membrane):** `PRI-REDACTED` — complete  
**Program (v2 composition):** `PRI-REDACTED` (Kind-composed mutation pipelines)  
**CVS (v2):** `CVS-REDACTED`  
**Glossary:** `GLS-1785787009880656000-b83ceca2` (Kind-Composed Mutation Pipeline); v1 `GLS-1785785028277485000-b6abe94b`  
**Strategic context:** `SC-1785787009159086000-39210507`  
**TRACK:** KMP2 BLIs under v2 PRI (`BLI-REDACTED` … `BLI-REDACTED`)

## Purpose

Dictate **all** logical mutations of process objects through the existing [`pkg/pipeline`](../../pkg/pipeline) API. Determinism comes from a closed set of **pipeline kinds** (intents) and a fixed stage order — not from combinatorial per-path guards.

**v2:** DECIDE / FINALIZE behavior for each kind is **composed configuration**, not a growing Go `switch`. At initialization, a compiler walks object spec (completeness), lifecycle ready-state policy, and policy / verification overlays into **`pipeline_definition`** instances. Runtime selects the definition by kind × intent.

Canonical stage contract: [`data-pipeline-lifecycle.md`](./data-pipeline-lifecycle.md). Create membrane sketch: [`CAS_CREATE_MEMBRANE_FLOW.md`](./CAS_CREATE_MEMBRANE_FLOW.md). Core erase policy: [`CRUD_BUCKETING_ARCHIVING_REQUIREMENTS.md`](./CRUD_BUCKETING_ARCHIVING_REQUIREMENTS.md). **Delete/update shockwave ≡ promote/demote:** [`CAS_MUTATION_SHOCKWAVE.md`](./CAS_MUTATION_SHOCKWAVE.md). Verification DSL sketch: [`../process/architecture/verification_dsl_proposal.md`](../process/architecture/verification_dsl_proposal.md).

## Non-negotiable

- Use `pipeline.NewBuilder(kind, logger).AddStage(...).Run` — **do not** invent a parallel orchestrator.
- **COMMIT** is the only stage that may write process YAML / CAS index / draft plane for logical mutate.
- **DECIDE** owns lifecycle evaluation and erase/archive policy (including critical-kind fail-closed).
- **DECIDE predicates** for a covered kind come from the **composed `pipeline_definition`**, not from parallel Go hooks.
- `ReconcileIndex` may reindex; it must **never** sole-delete a hash-shaped CAS object.
- **No backward-compat shims** when retiring pre-composition paths: delete legacy enforcement in the same tranche that covers a kind.

## Pipeline kinds (v1 closed intents)

| Kind constant | Intent |
|---------------|--------|
| `kernel.cas_object_create` | Create (draft plane or CAS) |
| `kernel.cas_object_update` | Field update (non-status or guarded status) |
| `kernel.cas_object_transition` | Promote / demote / auto status transition |
| `kernel.cas_object_erase` | Logical erase; DECIDE plans unlink dependents |
| `kernel.cas_object_restore_merge` | state-restore write merge |
| `kernel.cas_object_reconcile_index` | Orphan hash → reindex only |
| `kernel.cas_blob_gc` | Superseded hash GC after successful commit |

Go registry: [`pkg/kernelcas/kinds.go`](../../pkg/kernelcas/kinds.go). These **intent skeletons** stay closed; only DECIDE/FINALIZE bodies become kind-composed.

## Stage map

| Stage | Responsibility |
|-------|----------------|
| **INGEST** | Resolve kind/id/op; load current object (read-only) |
| **NORMALIZE** | Classify intent; attach paths / defaults |
| **DECIDE** | Evaluate composed definition (spec completeness + lifecycle ready-state + policies); set `Outcome["plan"]` |
| **COMMIT** | Authoritative writes only |
| **FINALIZE** | Visibility proof, metrics, unlink / verification completion checks |

`Outcome` keys for plan/lifecycle/erase are registered in [`.zqk/specs/pipeline_outcome_keys.yaml`](../process/_internal/pipeline_outcome_keys.yaml).

## Composition stack (v2)

| Layer | Source | Role in DECIDE |
|-------|--------|----------------|
| Intent skeleton | `kernel.cas_object_*` | Stage order + COMMIT membrane |
| Completeness / shape | Object spec (+ checklist / field profiles) | Required fields, ref cardinality |
| Ready-state machine | Lifecycle YAML (`shovel_ready`, `execution_locked`, …) | Transitions + preconditions |
| Nuanced policy | `policy` (+ domain / enterprise / architectural) | Extra gates (membership, integrity, …) |
| Verification DSL | Spec / transition-bound gates | Promote-time proofs |

**Init-time:** compile → `pipeline_definition` per (kind × intent).  
**Runtime:** select definition; fail closed if missing for a covered kind.

## DECIDE plans (closed)

- `refuse` — deterministic error; no COMMIT writes  
- `break_glass` — requires reason; audit in FINALIZE/TRIGGER  
- `draft_plane` — preliminary create/update on draft membrane  
- `cas_sync` — CAS materialize / update  
- `erase_unlink` — erase after unlink plan  
- `reindex_only` — CAS index repair; never delete sole file  

## Break-glass

`WithLifecycleBreakGlass` / DECIDE `plan=break_glass` with audited reason. Silent lifecycle skip for critical kinds is refused. Composed integrity overlays skip only when `pkgctx.IsLifecycleBreakGlass` (force **and** non-empty reason)—bare `WithForceLifecycleOverride` is not a policy bypass.

**Hold invariant (fail-closed):** break-glass and trusted shockwave may skip auto-only *edge admission* (`IsValidTransition`), but storage always runs `ValidateLifecycle`, and the validator **still enforces** complete-status / transition **preconditions** for `backlog_item` and `priority_plan` (linked CRITs validated/complete; plan children terminal). Break-glass is not a second way to ignore the shockwave. TRACK: `BLI-REDACTED`.

**When break-glass is still needed:** critical-kind DECIDE elevation (lifecycle updater auto-complete), audited CLI `--force`/`--override`, promote recover of undefined statuses. It is **not** required merely because an edge was formerly auto-only — dual `manual: true` + `auto: true` makes promote first-class (see `LIFECYCLE_STATUS_ROLES.md` § First-class promote).

### Critical kinds (spec-driven)

`kernelcas.IsCriticalKind` / `objects.IsKernelCriticalKind` read **`kernel_critical`** from object specs (materialized in `spec_index.json`):

| Resolution | Result |
|------------|--------|
| Explicit `kernel_critical: true/false` on **this** kind YAML | That value (not inherited — avoids stream children picking up cas parent true) |
| Unset + `storage_profile: stream` or `light_file` | **false** (ephemeral) |
| Unset + `storage_profile: cas_entity` | **true** (fail-closed) |
| No spec / unknown kind | **false** |

Opt out ephemeral CAS kinds with `kernel_critical: false` (e.g. `scheduler_job`, `agent_task`, `agent_feed`). Do **not** maintain a Go switch of kind names.

## Legacy cleanup (required — no compat)

When a kind is covered by composition, **delete** in the same tranche:

- `pkg/validation` `customRuleValidators` / `validate*CustomRules` for that kind  
- Hardcoded `decideAllowCasSync` / kind-special stubs that duplicate the composed definition  
- Call sites that force-skip lifecycle without composed break_glass policy  
- Dead feature flags / dual-run “old path vs new path” switches  

Do **not** leave shims, wrappers, or “until migration” branches. Prefer breaking tests that asserted the old path; rewrite them to the composed path.

## Migration order

**v1 (done):** objectify membrane → pipeline kinds → storage CUD → check/restore/retention → break_glass → dangling heal.

**v2 (active PRI):**

1. Objectify + this doc (KMP2-0)  
2. Init-time composition compiler → `pipeline_definition` (KMP2-1)  
3. Pilot `backlog_item` × `transition` (KMP2-2)  
4. Wire `kernelcas` DECIDE to composed definitions (KMP2-3)  
5. Validation DSL + policy overlays as compiler inputs (KMP2-4)  
6. **DELETE** legacy pre-composition paths (KMP2-5)  
7. Expand critical kinds + integrity composition coverage (KMP2-6)  

## Verification

- Allowlist test: mutators reference `kernel.cas_*` / membrane guards (`pkg/kernelcas/coverage_test.go`)  
- `zqk system kernel-integrity report` — splits health signals:
  - **`membrane_healthy`**: pipeline kinds, composition registry size, dangling refs, legacy custom-rules absence, membrane file scan (structural KMP)
  - **`object_compliance`**: last compact `system check` summary (`blocking_issues`, warnings, info, pending autofix batches) plus **trend** vs `.zqk/state/kernel_health/object_compliance.jsonl`
  - **`kernel_healthy`**: `membrane_healthy` **and** `object_compliance_ok` when a check cache exists; if no cache, membrane-only with `kernel_healthy_scope=membrane_only_no_check_cache` (do **not** read “green” as instance-validation clean)  

- `zqk system kernel-integrity compose` — rematerialize `pipeline_definition` (YAML under `.zqk/process/pipeline_definitions` is gitignored; compose is source of truth)  
- `zqk system kernel-integrity heal-dangling --apply` — unlink missing refs  

**Membrane note:** File CUD, Graph CUD, and tx critical deletes enter `kernelcas` or `denyCoreKernelHardDelete`. Write-behind WAL for non-critical leaf kinds remains an intentional fast path. Critical-kind `--force` requires `--reason-code` (break_glass).
- After KMP2-5: no `customRuleValidators` registrations for covered kinds  
- CVS measure vs `desired_end_state` on `CVS-REDACTED`  
