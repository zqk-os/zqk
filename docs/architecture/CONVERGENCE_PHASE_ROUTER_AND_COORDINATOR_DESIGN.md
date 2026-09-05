# Convergence phase router, tombstones, and coordinator-backed routing

**Last Verified:** 2026-08-31


**Status:** Design with **implemented** pieces in-tree (phase router, session routing context, suggested CVS fields from `zqk scheduler convergence measure`); coordinator-backed async evaluation remains **partial / optional** per profile notes. Traceability via `doc_entry` + `requirement` + `criteria` in process data.

**See also:** [CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md](./CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md) — rollup / nested-session orchestration (**[REDACTED-ID]**), **Goal 5** + **Appendix F** (multi-session coordination, shared-resource contention, object-kind strategy). This document and that one stay **in parity** until the surface converges into a **single, easy-to-follow feature** with **explicit behavior across processes** (see **Parity and convergence target** below).

**Automated verification (implementation alignment):**

| Layer | What it covers |
|-------|----------------|
| `pkg/scenario` `TestApplyScenarioBundle_ConvergenceLifecycleBundle` | Applies `convergence-lifecycle-bundle.yaml`; asserts created goal, criteria, requirement, doc_entry, CVS, backlog IDs. |
| `TestConvergenceLifecycleBundle_MeasureAndApplySuggestedUpdate` | Green health → **neutral** / **`c6_exit`**; first **`activity_log`** measure entry; full **`object_update_body`** persist. |
| `TestConvergenceLifecycleBundle_RemediateThenGreenTwoIterations` | **Fail → `trending_away` / `c4_act`**, then **pass → `trending_toward` / `c5_verify`**; **`activity_log` length 2** (append). |
| `TestConvergenceLifecycleBundle_RemediateToExitThreeIterations` | Same as two-step remediate, then **clean single-fingerprint window** → **neutral** / **`c6_exit`**; **`activity_log` length 3** (full loop to exit-ready phase). |
| `TestConvergenceLifecycleBundle_StampTombstoneAndDisparity` | **`stampTombstone`** → persisted **`before_state_snapshot`**; second measure with extra fingerprint → **`tombstone_disparity`** / **scope drift** (`scope_fingerprint_match` false). |
| `TestConvergenceLifecycleBundle_FinalizeDebriefAndDebriefNotes` | Session **`predictions`** + **`finalizeDebrief`** + operator string → **`prediction_debrief`**, **`predictions.retrospective`**, **`debrief_notes`** in **`object_update_body`** and re-read. |
| `TestConvergenceLifecycleBundle_SchedulerFastRoutingProfileViaFlag` | **`ResolveConvergenceRoutingSession`** with **`flow_variant`** = **`scheduler_fast`** → **`phase_router.routing_profile`**. |
| `TestConvergenceLifecycleBundle_SkipSessionContextDoesNotReadCVS` | **`skipSessionContext`** → no **`before_state_snapshot`**; **`tombstone_disparity.active_tombstone` false** (CLI **`--skip-session-context`**). |
| `cmd/zqk/scheduler` `TestMarshalConvergenceOutputJSON_*` | JSON shape for **`suggested_convergence_session_fields`**, **`phase_router`**, **`tombstone_disparity`**, **`object_update_body`** keys without storage (synthetic snapshots). |
| `cmd/zqk/scheduler` `TestCLI_TestFailuresConvergence_JSON_Integration` | **Subprocess:** builds **`zqk`**, isolated **`ZQK_TEST_ROOT`**, writes **`health.jsonl`**, runs **`zqk scheduler convergence measure --format json`** (no session, then **`--session-id`** after **`object create convergence_session`**); asserts **`suggested_convergence_session_fields`** / **`object_update_body`** on stdout. Skipped with **`-short`**. |
| `pkg/scheduler` `TestComputeTombstoneDisparity_*`, `TestBuildTestBundleConvergenceSnapshot_*` | Unit coverage for disparity math and health-window delta assessment. |
| `TestConvergenceSessionTickHandler_Execute_UpdatesSession`, `TestConvergenceSessionTickHandler_SkipsDuplicateWatermark` | **`convergence_session_tick` handler:** storage-backed measure → CVS update; **watermark dedup** skips second run. |
| `TestConvergenceSessionTickHandler_SkipsPersistWhenSessionTerminal`, `TestConvergenceSessionTickHandler_SpawnsFollowupDraftWhenTerminalAndEnv` | **Terminal CVS:** no measurement persist; optional **`CONVERGENCE_TICK_SPAWN_FOLLOWUP_DRAFT`** creates draft follow-up session when **`health.jsonl`** still shows work. |
| `TestWriteTestBundleHealthStreamSummary` | Pilot **data stream summary** row written to `.zqk/stream_summary/test_bundle_health.json` (see `DATA_STREAM_SUMMARY_PILOT.md`). |
| `TestEvaluateConvergencePredicateReadiness` | Optional **`thresholds.start_gate`** / **`completion_gate`** hints vs snapshot (`CONVERGENCE_PREDICATES_AND_GATES.md`). |
| `pkg/gantt` `TestMinimalInteractionSVG_HasContractHooks` | Minimal **SVG interaction contract** for Gantt (BLI-214); export PNG/PDF via **rsvg** when installed (`GANTT_EXPORT_DEFERRED.md`, BLI-213). |
| `pkg/objects` `TestMaybeStripRedundantTopLevelTraits_*` | **BLI-210:** top-level `traits` omitted on YAML persist when expanded set matches spec (`MaybeStripRedundantTopLevelTraits` + `FileObjectStorage.yamlMarshalForPersistence`). |

Together, **`pkg/scenario`** exercises **storage-backed** persistence and **`pkg/scheduler` + `cmd/zqk/scheduler`** cover **pure logic + CLI JSON marshaling** for the same field contracts.

## Scheduler: `convergence_session_tick` and test-bundle hook

- **Job type:** `convergence_session_tick` — same measure/update contract as `zqk scheduler convergence measure --session-id` (`pkg/scheduler/handlers_convergence_session_tick.go`). Environment: **`CONVERGENCE_SESSION_ID`** (required), optional **`HEALTH_LIMIT`**, **`CURRENT_PHASE`**, **`FLOW_VARIANT`**, **`SKIP_SESSION_CONTEXT`**. Honors **`thresholds.max_ticks_per_hour`** and skips when **`last_measurement_at`** matches the current health watermark. Measurement fields persist only when status is **`active`**. For **`paused` / `escalated` / `error` / lifecycle-terminal** statuses the handler **does not** persist measurement fields (Option A: escalated is halted park-but-CAP-bound, **not** terminal — see **`pkg/convergence` session status matrix** and `REDACTED`); it logs follow-up signals from `health.jsonl`. Optional **`CONVERGENCE_TICK_SPAWN_FOLLOWUP_DRAFT=true`**: when follow-up is indicated after a **lifecycle-terminal** session (`completed` / `abandoned` / …), create a **draft** follow-up `convergence_session` (copied hypothesis and links) with **`related_object_refs`** pointing at the terminal session; idempotent per `(session, health watermark)` via **`.zqk/scheduler/terminal_followup_spawn/`** (`pkg/scheduler/convergence_terminal_tick.go`).
- **Optional post-tick rollup:** **`CONVERGENCE_TICK_ROLLUP`** (and optional **`CVS_ORCH_ROLLUP_OUT`**) runs **`cvs_convergence_orchestrate.sh`** **rollup-only** (`CVS_ORCH_SKIP_PERSIST`) after a successful tick and records rollup timing/summary on the **tick job** events JSONL. Use **`scripts/scheduler_jobs/convergence_orchestrate.yaml`** (**`SCH-convergence-orchestrate`**) for the **full** orchestrator loop (measure persist + **`cvs_outcome_rollup.py --apply`**, etc.). Suspend the hourly pipeline tick: **`zqk object update SCH-cvs-pipeline-tick --field enabled=false`**.
- **Run-wrapper hook:** When a **test-bundle** job’s **`environment_variables`** include **`CONVERGENCE_SESSION_ID`**, the handler runs **`maybeRunConvergenceTickAfterTestBundleHealth`** after appending **`health.jsonl`** (success, timeout, or failure terminal lines). Forwards **`CONVERGENCE_TICK_ROLLUP`** / **`CVS_ORCH_ROLLUP_OUT`** when set on the bundle job (see **`convergenceTickEnvKeysForwardedFromTestBundleJob`**). This is an **event-shaped** path: new evidence from the bundle run can trigger one governed tick without waiting for a separate cron (subject to the same watermark and rate limits).

**CLI one-step measure + agent directive:** `zqk scheduler convergence measure --session-id CVS-* --persist-session` applies `object_update_body` to the CVS (same payload as `convergence_session_tick`), then emits `--format json|yaml|agent-prompt` output so the persisted measurement and the chat-facing prompt stay aligned. Skips the write when the health watermark matches `last_measurement_at` (duplicate-watermark rule). For an **agent chat handoff**, add **`--copy`** (macOS clipboard) or **`--paste-cursor`** (paste + submit in Cursor); **`-o` alone does not inject into the chat.**

**See also:** [EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md](./EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md) — how to choose among hooks, ticks, notifications, and HTTP callbacks without mandating a single integration pattern (rate and data shape drive the choice). **Shell wrappers (promotion gate, overseer audit, env):** [scripts/README.md — Convergence promotion and overseer helpers](../scripts/README.md#convergence-promotion-and-overseer-helpers). **Orchestration entrypoints table:** [CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md — Entrypoints](./CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md#entrypoints-shell-cli-jobs).

## Registered process objects

| Kind | ID |
|------|-----|
| Requirement | `[REDACTED-ID]` |
| Doc entry | `DOC-1774218769623646000-1953e8de` |
| Criteria (tombstones / disparity) | `[REDACTED-ID]` |
| Criteria (phase router configurability) | `[REDACTED-ID]` |
| Criteria (coordinator pattern) | `[REDACTED-ID]` |
| Criteria (traceability compliance) | `[REDACTED-ID]` |
| Backlog item (E2E convergence work, PRI-221) | `[REDACTED-ID]` |
| Convergence session (draft execution thread) | `[REDACTED-ID]` |
| Doc entry (semantic kernel / CLI façade sidebar) | `DOC-1774227494115025000-fc0404a6` |

## Purpose

Extend the **convergence lifecycle** (glossary: convergence lifecycle GLS; object kind `convergence_session` CVS-*) with:

1. **Tombstones** — explicit iteration baselines so “disparity” is measured against a **named, reproducible** state, not ad hoc narrative.
2. **Phase router** — a **bounded** state machine that proposes **legal** `current_phase` (C1–C6) transitions and **decisive `next_action`** text from **measurement + gates**, with **per-session configurability** so different flows (test-bundle health, autofix/system check, mixed) can share the same lifecycle shell.
3. **Coordinator integration** — optional **asynchronous, distributed** evaluation and routing (work queued to the existing coordinator/scheduler ecosystem) so routing rules and heavy measurements do not block the CLI hot path.

This design complements **[REDACTED-ID]** (evaluation surface and deviation response): that requirement locks **vocabulary**; this document locks **mechanism** for **iteration control** and **next-step generation**.

## Parity and convergence target (orchestration doc + this doc)

**Today (explicit split):**

| Concern | Primary home |
|---------|----------------|
| **Single-session** phase (C1–C6), **tombstones**, **disparity**, **`next_action`** from **measurement + bundle gates** | **This document** + `zqk scheduler convergence measure`, **`convergence_session_tick`**, test-bundle **hook** |
| **Composite rollup** (`rollup_v1`), **nested parent/child `CVS-*`**, **orchestrator** / **scheduler jobs**, **multi-session** arbitration on **shared resources** | **[CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md](./CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md)** — **Appendix F** |

**Uniform feature (direction):** One operator- and agent-facing story: **the same versioned payloads and field names** whether the writer is **CLI**, **tick**, **hook**, **run_wrapper orchestrate**, or a **coordinator worker**. No duplicate semantics (e.g. two different definitions of “ready” or “next”) without a **documented** bridge. **Process boundaries** stay explicit:

- **Per `CVS-*`:** **`--session-id`** (or env **`CONVERGENCE_SESSION_ID`**) scopes measure/persist; **duplicate-watermark** skip is shared (**CLI** and **tick**).
- **Cross-process writers:** **Idempotency** and **single-writer** expectations in **Coordinator ecosystem** apply; **multi-session** contention is **not** solved by the phase router alone — use **parent session + rollup arbitration** (or future dedicated kind per **Appendix F**), not ad hoc **`next_action`** overrides on leaf sessions.
- **Gates:** Bundle-only **`ready_for_session_completion`** remains **one input**; **parent-level** “safe to complete” follows **orchestration** + **CONVERGENCE_PREDICATES_AND_GATES.md**.

**Doc maintenance:** When behavior or contracts change in **Go** (`pkg/scheduler` convergence paths, tick, hook) or in **rollup/orchestrate**, update **both** this file and **CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md** in the same effort when the change affects **shared vocabulary** or **cross-process guarantees**.

## Non-goals

- Replacing human judgment for escalation or suspend.
- Unbounded automation (finite action set remains mandatory per glossary).
- Direct edits to instance YAML under `docs/process/` — all persisted session updates remain **`zqk object update`**.

## Tombstones and disparity

**Tombstone (iteration anchor):** A structured snapshot bound to an iteration boundary, minimally including:

- Evaluation surface identifier (e.g. `scheduler_test_failures_convergence_v1`, `system_check_tier1`).
- **Watermark** (e.g. health.jsonl last line timestamp or hash registry generation).
- **Scope fingerprint** — sorted set hash of bundle_command_fingerprint (or equivalent keys for non-bundle surfaces).

Stored in **`before_state_snapshot`** at iteration start (or end of prior iteration), and copied forward in **`activity_log`** for audit.

**Disparity:** Computed diff between **current measurement** and **active tombstone** (and optionally **`predictions`** on the session). Feeds existing fields: **`delta_assessment`**, **`outcome_character`**, and **`next_action`** candidates.

## Phase router

**Measurement-only inputs (implemented today):** The router’s **`measurement_implied_phase`** is derived from **`TestBundleConvergenceSnapshot`** only (`pkg/scheduler/convergence_phase_router.go` — e.g. neutral + **`ready_for_session_completion`** → **`c6_exit`**). It does **not** ingest **desired_end_state**, matrix rows, or repo script gates. Unknown **`flow_variant`** values fall back to default rules and a note in JSON output.

**Operator alignment:** Persisted **`current_phase`** may lag (**`phase_alignment: session_behind`**). Do not move to **`c6_exit`** / completed **status** on green bundles alone if the **convergence_session** contract still lists unfinished gates; see **CONVERGENCE_PREDICATES_AND_GATES.md** (measurement + process rules).

**Inputs (illustrative, broader design):** `current_phase`, `delta_assessment`, heartbeat/staleness, trigger queue depth, `flow_variant`, session thresholds, optional autofix persistence flags.

**Outputs:** Allowed transition(s), primary **`next_action`** string, optional structured “recommended commands”, and **blockers** mirroring `SessionCompletionBlockedReasons`-style gates.

**Instance-level configurability:**

- **Routing profile** referenced from the session (e.g. `flow_variant` or a dedicated field once specified): selects rule tables, ordering of gates, and which coordinator jobs may run.
- **Extension points:** rules as data (versioned JSON/YAML in CAS or dedicated object kind in a later phase), not hard-coded only in Go.

## Coordinator ecosystem

Heavy work (large health windows, cross-referencing autofix batches, aggregating metrics) may **enqueue** evaluation to **coordinator-backed** workers (same family as existing scheduler/coordinator patterns), returning **handles** or **event callbacks** that **merge** results into **`after_state_snapshot`** / **`activity_log`**.

**Multi-session work** (arbitration across several active **`CVS-*`**, shared fingerprints) belongs in the **orchestration** path (**Appendix F**): coordinator workers should **read the same health/rollup contracts** and **write** only to **declared** targets (e.g. **parent** session **`rollup_v1`**, not silent updates to unrelated leaf sessions).

Constraints:

- **Idempotent** writers updating the same `convergence_session` (or use single-writer lease / operation id).
- **Evidence-first:** persistence and convergence checks remain aligned with **autofix-convergence-evidence-first** policy.

## Relation to `convergence_session` fields

| Field | Role in this design |
|--------|---------------------|
| `before_state_snapshot` / `after_state_snapshot` | Tombstone + latest measure |
| `current_phase` | Router output |
| `next_action` | Primary human/CI next step |
| `iteration_process` | Describes loop body + references routing profile |
| `activity_log` | Append-only proof of iteration boundaries |
| `automation_hooks` | Optional subscriber labels for coordinator |
| `predictions` / `predictions.retrospective` | Up-front estimates (`expected_signal`, `hypothesis_confidence`); at **finalize**, `zqk scheduler convergence measure --session-id CVS-* --finalize-debrief` adds a **retrospective** block comparing predictions to the latest health measure (see `prediction_debrief` in JSON output) for `zqk object update` |
| `debrief_notes` | Optional operator analysis and handoff for the **next** session: same command with `--debrief-notes` or `--debrief-notes-file` (requires `--session-id`) populates `object_update_body.debrief_notes` on update; read prior CVS with `zqk object get <id>` when seeding a new `convergence_session` |

## Manual ledger (operator): measurement JSON → `zqk object update`

This is the **evidence-first** handoff from **`zqk scheduler convergence measure`** to persisted **`convergence_session`** (no narrative edits to YAML under `docs/process/`).

### Commands

1. Ensure **`.zqk/logs/scheduler/cvs/test-bundles/health.jsonl`** exists (run scheduler test bundles or place a synthetic window for development).
2. Run (from project root):

   `zqk scheduler convergence measure --format json --session-id <CVS-id> [flags]`

   Useful flags: **`--stamp-tombstone`**, **`--finalize-debrief`**, **`--flow-variant <name>`**, **`--skip-session-context`**, **`--debrief-notes`** / **`--debrief-notes-file`** (see command help).

3. The file you pass to **`zqk object update <CVS-id> --file`** must contain the **flat** map **`suggested_convergence_session_fields.object_update_body`** — the same keys as top-level `convergence_session` fields for this update, **not** wrapped in an `object_update_body` property.

### Keys typically present in `object_update_body`

| Field | Role |
|-------|------|
| `delta_assessment` | `trending_toward` / `trending_away` / `neutral` / `unknown` from the health window |
| `last_measurement_at` | Health watermark (RFC3339) |
| `after_state_snapshot` | Structured latest measure (includes evaluation surface id, scope fingerprint, failing fingerprints, gates) |
| `before_state_snapshot` | Iteration tombstone — only when **`--stamp-tombstone`** was used for this run |
| `next_action` | Human-readable next step |
| `current_phase` | Suggested C1–C6 phase from the **phase router** (with session / flags) |
| `activity_log` | **One** new entry per measure; storage **appends** to existing `activity_log` on `convergence_session` (does not replace the full list) |
| `predictions` | Present when **`--finalize-debrief`** merges retrospective into session predictions |
| `debrief_notes` | Present when **`--debrief-notes`** / **`--debrief-notes-file`** set |

### Top-level JSON (review; selective persist)

`suggested_convergence_session_fields` may also include **`phase_router`**, **`tombstone_disparity`**, **`session_routing_context`**, **`prediction_debrief`**. Use these for alignment and audits; **`object update --file`** should follow **`object_update_body`** unless you intentionally persist overlapping structured fields.

### Helper script

**`scripts/convergence-session-measure.sh`** writes **`object_update_body`** JSON for **`zqk object update --file`**. Environment:

- **`STAMP_TOMBSTONE=1`** → passes **`--stamp-tombstone`**
- **`FINALIZE_DEBRIEF=1`** → passes **`--finalize-debrief`**

## Verification

Mapped to **criteria** linked from the **requirement** that references this document: design sections present, tombstone/disparity defined, router configurability described, coordinator pattern described, traceability to glossary/convergence_session stated.

## Scenario bundle (stamp-out scaffold)

A **scenario bundle** can create the full traceability shell in one apply: goal, criteria, requirement, **doc_entry** (this path), **convergence_session** (draft CVS), and optional backlog. Tracked as:

`pkg/scenario/testdata/convergence-lifecycle-bundle/convergence-lifecycle-bundle.yaml`

Apply (from repo root, with a project that has process data):

`zqk-scenario bundle apply -f pkg/scenario/testdata/convergence-lifecycle-bundle/convergence-lifecycle-bundle.yaml -R .`

CLI measurement for test-bundle health emits `phase_router` and `session_routing_context` under `zqk scheduler convergence measure --session-id`: effective `current_phase` / `flow_variant` default from the CVS object when readable (flags override; `--skip-session-context` disables the read). The JSON field `suggested_convergence_session_fields.object_update_body` is the subset intended for `zqk object update <CVS-id> --file` (see `scripts/convergence-session-measure.sh`).

Replace the `*-CLF-*` id_hints with your own prefix before applying to a shared repo to avoid ID collisions. See bundle header comments for details.

## References

- `docs/process/_internal/object_specs/convergence_session.yaml`
- `pkg/scheduler/convergence_testbundle.go` (`TestBundleConvergenceSnapshot`, gates)
- [CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md](./CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md) — **Appendix F** (multi-session coordination; **parity** with this doc)
- `.cursor/rules/convergence-session-agent-discipline.mdc`
- `.cursor/rules/autofix-convergence-evidence-first.mdc`
- Requirement **[REDACTED-ID]** (evaluation semantics for remedies/convergence)
