# Measurement outcome taxonomy (normative)

**Last Verified:** 2026-08-31


**Status:** Adopted; **`primary_measurement_outcome`** / **`primary_measurement_outcome_detail`** / **`measurement_outcome_schema_version`** are emitted on **`rollup_v1`** and **`rollup_status_core`** (see § below). Tracking: **`[REDACTED-ID]`** (complete).  
**Priority plan:** `[REDACTED-ID]` — CLI alpha launch readiness  
**Related:** [CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md](./CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md), [CLI_ALPHA_LAUNCH_PLAN.md](./CLI_ALPHA_LAUNCH_PLAN.md), [DECLARATIVE_LAUNCH_AND_MATRIX_LOOP.md](./DECLARATIVE_LAUNCH_AND_MATRIX_LOOP.md) (GTM launch stages route on these outcomes)

## Purpose

Every anticipated or expected outcome from a **measurement** (validator, vettor, health snapshot, gate script, matrix rollup, or composite orchestrator) must be **classifiable** so work does not stall in a partially complete state. Classifications must be **measurable**: stored, logged, or derivable from persisted artifacts.

This document defines the **four primary responses** a measurer concludes with. Names are stable identifiers (use exactly these strings in code and JSON when emitting the primary outcome).

---

## Primary outcomes (exactly one per measurement conclusion)

| Identifier | Meaning | Operator / automation response |
|------------|---------|--------------------------------|
| **`measurement_yields_convergence`** | Desirable momentum toward the stated hypothesis or **`desired_end_state`**; signals align with intent. | **Continue** — keep going; only minimal adjustments if any. |
| **`measurement_yields_divergence`** | Undesirable momentum or drift vs intent (e.g. regressions, growing failure set, matrix rows sliding backward) without a hard infra fault. | **Investigate** — find cause; **adjustments likely required** (code, config, or scope). |
| **`measurement_yields_halt_or_error`** | Measurement could not complete cleanly: tool crash, non-zero exit where success was required, missing prerequisite, I/O failure, unrecoverable parse error. | **Handle programmatically** where safe (retry, backoff, fix data); **otherwise extend handlers** so the condition is classified and surfaced; avoid silent partial success. |
| **`measurement_yields_ambiguous_outcome`** | Evidence is insufficient, contradictory, or needs judgment (e.g. stale window, mixed pass/fail without a clear trend, conflicting surfaces). | **Escalation path** — review existing knowledge base and prior **`activity_log`**; attempt automated disambiguation if defined; if still unclear, **sequester human context** (explicit **`next_action`**, backlog item, or session note). |

### Notes

- **Convergence** is not the same as “green everywhere”; it means **intent-aligned progress** on the **scoped** evaluation surface for that measurement.
- **Divergence** assumes the measurement **finished** but the **signal** is wrong directionally.
- **Halt/error** means the **measurement pipeline** failed, not merely “tests failed” (test failures under a working harness are often **divergence** unless policy says otherwise).
- **Ambiguous** is a **first-class** outcome: forcing a binary pass/fail when evidence is incomplete creates the “partially complete effort” failure mode this taxonomy prevents.

---

## Direct emission (implemented) and legacy signals

**Implemented:** `rollup_v1`, scheduler **`rollup_status_core`** (Go: `pkg/convergerollup`), **CLI** `zqk scheduler convergence measure --format json`, and **`after_state_snapshot`** / **`BuildTestBundleConvergenceSnapshot`** emit **`primary_measurement_outcome`**, **`primary_measurement_outcome_detail`**, and **`measurement_outcome_schema_version`** together. Python **`scripts/cvs_outcome_rollup.py`** stays aligned with Go via shared tests (`pkg/convergerollup/rollup_parity_json_test.go`, `scripts/cvs_outcome_rollup_test.py`).

**Legacy:** `convergence_session.delta_assessment` (and related enums) may still be **updated** alongside measurements but is **not** the canonical taxonomy field. Use **`primary_measurement_outcome`** on persisted rollup / snapshot JSON when automating.

**Approximate mapping** for older dashboards or objects that only store **`delta_assessment`** / **`rollup_status`** without primary fields:

| Existing signal | Typical mapping to primary outcome |
|-----------------|-------------------------------------|
| `delta_assessment`: `trending_toward` | `measurement_yields_convergence` |
| `delta_assessment`: `trending_away` | `measurement_yields_divergence` |
| `delta_assessment`: `neutral` (bundles healthy, gates green, no open matrix blockers) | `measurement_yields_convergence` (at rest / maintained) |
| `delta_assessment`: `neutral` with conflicting surfaces | `measurement_yields_ambiguous_outcome` |
| `delta_assessment`: `unknown` | `measurement_yields_ambiguous_outcome` |
| `rollup_status`: `satisfied` (all configured gates pass) | `measurement_yields_convergence` |
| `rollup_status`: `partial` / `blocked` / `ready_for_review` with actionable blockers | `measurement_yields_divergence` or `measurement_yields_ambiguous_outcome` (use ambiguity when rollup cannot choose between scope vs infra) |
| Gate script / tool: required failure, measurement did not run | `measurement_yields_halt_or_error` |
| Orchestrate / persist / JSON parse failure | `measurement_yields_halt_or_error` |

### Test-bundle snapshot (`after_state_snapshot` from `zqk scheduler convergence measure`)

Go **`BuildTestBundleConvergenceSnapshot`** and **`BuildAfterStateSnapshotMap`** include the same three fields as **`rollup_v1`**, using the bundle-only slice with literal rollup gates skipped (aligned with `rollup_status_core` when gates are not evaluated for that measure).

### `rollup_v1` fields (implemented)

Full Python rollup (`scripts/cvs_outcome_rollup.py`) and scheduler **`rollup_status_core`** (Go) now include:

| Field | Description |
|-------|-------------|
| **`primary_measurement_outcome`** | One of the four identifiers above |
| **`primary_measurement_outcome_detail`** | Short machine-oriented reason (aligned across Go/Python) |
| **`measurement_outcome_schema_version`** | Currently **`"1"`** — bump if mapping rules change |

---

## Vettors and validators

| Measurer | Primary outcomes it can emit (via rollup merge) | Where to read |
|----------|--------------------------------------------------|---------------|
| Test-bundle **`health.jsonl`** window | Convergence, divergence, ambiguous, halt (when bundled with gate/matrix failures in rollup) | `rollup_status_core`, `after_state_snapshot`, `BuildTestBundleConvergenceSnapshot` |
| **`scripts/cvs_outcome_rollup.py`** / **`rollup_v1`** | All four | `rollup_v1` JSON; `--apply` merges into convergence snapshot |
| **`rollup_status_core`** (Go) | All four | `zqk scheduler convergence measure --format json`, `.zqk/logs/scheduler/rollup_status_core.jsonl` |
| **`CODEBASE_VETTING_MATRIX.csv`** + profile | Often **halt** (unreadable) or contributes **ambiguous** / **partial** via blockers | Rollup blockers (`vetting_matrix_*`) |
| Repo gate scripts (field-key literals, ZQK-env literals) | **Halt** when a required gate exits non-zero | Rollup / `rollup_status_core` gate exit fields |
| Optional drift scans | Contributes to **divergence** or **ambiguous** via rollup | `rollup_v1` drift stats |

Unit coverage: **`pkg/convergerollup`** (e.g. **`TestComputePrimaryMeasurementOutcome_*`**) distinguishes **halt** vs **divergence**; CLI integration tests in **`cmd/zqk/scheduler`** assert **convergence** and **divergence** on real `zqk` binary output.

---

## Non-goals

- Replacing human escalation for policy or ethics decisions.
- Collapsing the four outcomes into pass/fail for user-facing CLI without retaining the primary outcome in structured output.

---

## Supplementary guidance (user outcomes)

These recommendations bias toward **clarity, trust, and next steps** for humans and agents using the CLI, scheduler, and convergence objects—not only internal correctness.

### 1. Pair every primary outcome with **evidence** and **scope**

Users should always see **what was measured** (evaluation surface, time window, commit or job id when relevant) alongside the classification. Without scope, “convergence” is easy to misread as “everything in the repo is fine.” Implementation should prefer explicit **`evaluation_surface`** (or equivalent) in the same payload as the primary outcome.

### 2. Optional **confidence** or **staleness** (secondary fields)

- **Staleness** (e.g. health watermark age) explains why a green signal might still feel wrong; surfacing it reduces false confidence.
- **Confidence** (low \| medium \| high) is useful when heuristics map legacy enums to the four outcomes—so users know when to trust automation vs re-run measurement.

### 3. **Default next step** per outcome (actionability)

| Primary outcome | What users need most |
|-----------------|----------------------|
| Convergence | Confirmation that **continuing current work** is appropriate; link to the **scoped** success criteria. |
| Divergence | **One** prioritized investigation path (which surface failed first, suggested command or doc). |
| Halt / error | **Non-jargony** error, whether **retry** is safe, and **where** logs live—never an empty success exit. |
| Ambiguous | **Why** it is ambiguous (contradiction, stale window, mixed surfaces) and **one** disambiguation step before human escalation. |

Align with existing **`recommended_next_action`** / **`next_action`** on `convergence_session` where applicable.

### 4. **False convergence** is a UX bug

Treat “convergence” as invalid if prerequisites failed (e.g. measurement never ran, or window is stale). Prefer **halt_or_error** or **ambiguous** over convergence when the pipeline did not complete or evidence is too old—users should not see green when the system did not actually observe the intended state.

### 5. **Divergence** vs **ambiguous** — decision hint for operators

- If the **measurement completed** and the **signal is clearly bad** for the stated intent → **divergence**.
- If the **measurement completed** but **multiple surfaces disagree** or **time window does not cover the claimed change** → **ambiguous** until disambiguated.

This reduces thrash from mislabeled “failure” when the real issue is unclear evidence.

### 6. Composite / parent rollups need an explicit **combine rule**

When several measurers feed one parent (e.g. bundles + matrix + gates), document whether the parent takes **worst-of**, **all-must-pass**, or **tiered** semantics. Users should not have to infer policy from absence of blockers.

### 7. **Stable, machine-readable** identifiers

Use the **same string values** in JSON (`--format json`), agent prompts, and logs so automation and humans never diverge on vocabulary.

### 8. **Escalation budget** for ambiguous

Repeated **ambiguous** on the same scope should surface a **backlog** or **process** nudge (e.g. widen bundles, fix staleness, split CVS)—so users are not stuck in “ambiguous forever” without a structural fix.
