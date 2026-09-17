# Convergence predicates and gates

**Last Verified:** 2026-08-31


**Purpose:** Machine-readable hints that align with `convergence_session` fields `start_condition` (text, CVS-018) and `desired_end_state` (text, CVS-016) and with `thresholds` (CVS-020).

**See also:**

- [CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md](./CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md) — **§ Parity and convergence target**: single-session **phase router**, **tick**, **hook**, **`next_action`** / **`measurement_implied_phase`** from bundle measurement; **same contracts** across CLI vs async paths as orchestration evolves.
- [CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md](./CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md) — **outcome aggregator** (`scripts/cvs_outcome_rollup.py`), **orchestrator** (`scripts/cvs_convergence_orchestrate.sh`), **nested CVS**; **Goal 5** + **Appendix F** (multi-session coordination on **shared** surfaces such as **`health.jsonl`** / fingerprints). [**Entrypoints (shell, CLI, jobs)**](./CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md#entrypoints-shell-cli-jobs) — which runner (measure vs orchestrate vs tick vs promotion vs overseer).
- [EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md](./EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md) — choosing hook vs tick vs callback by **rate** and **payload shape** (complements this file’s **what** the bundle snapshot does *not* imply). **Declarative validators (concept)** — domain-shaped, test-like indicators beyond bundle gates; future alignment with rollup/measure surfaces. **Operator scripts / env tables:** [scripts/README.md — Convergence promotion and overseer helpers](../scripts/README.md#convergence-promotion-and-overseer-helpers).

**Disconnect guard:** Predicates here govern **threshold hints** vs **bundle snapshot** only. **Parent-level** “all gates satisfied” and **cross-session** arbitration live in **orchestration** / **Appendix F**, not in **`ready_for_session_completion`** alone.

### `rollup_v1` vs bundle-only signals

- **`ready_for_session_completion`** (from **`zqk scheduler convergence measure`**) still means **only** “bundle health snapshot passes **completion_gate**” — unchanged above.
- **`after_state_snapshot.rollup_v1`** (from **`scripts/cvs_outcome_rollup.py`**, optional **`--apply`**) adds **composite** surfaces: literal repo gates, vetting matrix rows, drift baselines, **child `CVS-*`** rows. Use **`rollup_v1.ready_for_parent_completion`** when asking whether **parent** exit criteria are met — **not** **`ready_for_session_completion`** alone.
- **Child rows in `rollup_v1` (MVP):** **`cvs_outcome_rollup.py`** loads **immediate** children from the parent’s **`related_object_refs`** only (bounded by **`--max-children`**). It does **not** walk a nested tree. For a **depth-capped BFS** over child links (cycle-safe), use **`zqk scheduler convergence overseer`** — read-only; it does **not** merge into **`rollup_v1`** (see **[CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md](./CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md)** Appendix C).
- **`rollup_v1.rollup_status`** and **`recommended_next_action`** are the right operator/automation hints for **campaign-level** next steps; threshold **`completion_gate`** on the session does **not** subsume rollup.
- Implementation table and script names: **[CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md](./CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md)** — **Implementation reference (MVP — locked)**.

## Structured keys (thresholds object)

Optional nested objects:

| Key | Role |
|-----|------|
| `start_gate` | When `status` is `draft`, `EvaluateConvergencePredicateReadiness` can require `min_lines_in_window` (int, default 1) against `TestBundleConvergenceSnapshot.LinesInWindow`. |
| `completion_gate` | Optional `require_ready_for_session_completion` (bool, default true). When set to `false`, the completion predicate is treated as satisfied for planning signals only — **lifecycle transitions** (`draft`→`active`, `active`→`completed`) remain governed by `convergence_session_lifecycle.yaml` and operator/`zqk object update` unless extended elsewhere. |

**Admission vs start_gate:** `thresholds.start_gate` is a **measure readiness hint**, not the membrane admission gate for leaving `draft`. Draft→active requires non-empty `hypothesis`, `desired_end_state`, and `current_phase` (`convergence_session_lifecycle.yaml` + lifecycle builder). TRACK: `BLI-1786686769839541000-f5a3260f`.

## Session status behavior matrix (Option A)

Canonical helpers: `pkg/convergence` (`SessionStatusEligibleForCAP`, `SessionStatusPersistsMeasurement`, `SessionStatusListedForWhatsNextMeasure`, `SessionStatusEligibleForAutoStaleEscalate`). Decision: `DEC-1786686986572580000-54124993`. Semantics BLI: `BLI-1786686768606200000-31133cc3`.

| Status | CAP bind | Measure persist | whats-next measure | Auto-stale→escalated |
|--------|----------|-----------------|--------------------|----------------------|
| `draft` | no | no | no | no |
| `active` | yes | yes | yes | yes |
| `paused` | yes | no | yes | yes |
| `escalated` | yes (park-but-CAP-bound) | no | no | n/a |
| `error` | no | no | no | no |
| `completed` / `abandoned` / `archived` | no | no | no | no |

**Operator exit from `escalated`:** resume→`active` when this session owns the loop again; `completed` when end state was accepted outside the loop; `abandoned` when dropping without resume.

## Human-readable fields

- **`start_condition`:** Operator/automation narrative for what triggers applicability; not evaluated by `EvaluateConvergencePredicateReadiness`.
- **`desired_end_state`:** Target narrative; completion readiness still comes from measurement gates (`ReadyForSessionCompletion` in health snapshot) plus process rules.

## Agent prompt and `zqk scheduler convergence measure`

- **`ready_for_session_completion`** is computed only from the **test-bundle health snapshot** (`pkg/scheduler` `applySessionCompletionGates`: fingerprints, window history, heartbeat, trigger queue). It does **not** read **desired_end_state**, vetting matrix CSV, FieldKey/ZQK-env gates, or drift baselines.
- **`measurement_implied_phase`** (e.g. **`c6_exit`**) uses the **same** snapshot. Treat **`c6_exit`** as “bundle health allows *considering* exit,” not automatic proof that every **desired_end_state** bullet is done.
- The **agent-prompt** markdown (`--format agent-prompt`) includes a **Scope** section that states this explicitly (including **rollup_v1** vs **rollup_status_core** and **convergence-overseer**); keep it aligned with this file so humans and automation do not conflate **bundle green** with **session contract complete**.

## Code

- `pkg/scheduler/convergence_predicates.go` — `EvaluateConvergencePredicateReadiness(status, thresholds, snap)`.
- `pkg/scheduler/convergence_testbundle.go` — `applySessionCompletionGates`, `BuildTestBundleConvergenceSnapshot`.
