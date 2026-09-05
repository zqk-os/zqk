# Convergence orchestration, outcome aggregation, and nested CVS

**Last Verified:** 2026-08-31


**Status:** MVP — **`pkg/convergerollup`**, **`rollup_status_core`** in **`zqk scheduler convergence measure`**, JSONL **`rollup_status_core.jsonl`**; **`scripts/cvs_outcome_rollup.py`** (full matrix/drift), **`cvs_convergence_orchestrate.sh`** (persist + rollup).  
**Related:** [CONVERGENCE_PREDICATES_AND_GATES.md](./CONVERGENCE_PREDICATES_AND_GATES.md), [CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md](./CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md) (**Parity and convergence target** + single-session phase router / tick / hook), [EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md](./EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md), [CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md](./CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md) (membrane / cell / organelle vocabulary; glossary `GLS-*` for nested collaboration). **Glossary:** **`GLS-1776410364614477000-e6d4c940`** (*Blackboard handoff*) — agent–system loop, stdout + process objects, exit-code contract. **Appendix F:** multi-session coordination, shared-resource contention, evaluate existing object kinds before a new coordinator type. **Doc parity:** keep this file aligned with the phase-router doc when changing **shared contracts** or **cross-process** behavior until convergence surfaces unify.

**Operator scripts:** **`scripts/check_convergence_promotion_readiness.sh`** (optional promotion gate — **`--help`**), **`scripts/record_convergence_overseer_run.sh`** (append **`overseer_run_v1`** audit line — **`--help`**). Defaults and naming live in **[EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md](./EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md)** (Operating rhythm; **Declarative validators (concept)**). **Index (env tables, examples):** [scripts/README.md — Convergence promotion and overseer helpers](../scripts/README.md#convergence-promotion-and-overseer-helpers).

### Entrypoints (shell, CLI, jobs)

**Which runner?** Use the **smallest** surface that matches the contract you need (persist vs rollup vs gate vs audit). Full **integration menu** (hooks, ticks, bundles, wrappers): **[EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md](./EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md)** (**Integration menu** table).

| Entrypoint | Typical use | Pointer |
|------------|-------------|---------|
| **`zqk scheduler convergence measure`** | Single-shot bundle snapshot → JSON / **`--persist-session`** update | [CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md](./CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md), [CONVERGENCE_PREDICATES_AND_GATES.md](./CONVERGENCE_PREDICATES_AND_GATES.md) |
| **`scripts/cvs_convergence_orchestrate.sh`** / optional timer **`SCH-convergence-orchestrate`** | Governed **persist + `cvs_outcome_rollup.py --apply`** (full Path A) | **Appendix E**; **`scripts/scheduler_jobs/convergence_orchestrate.yaml`** |
| **`convergence_session_tick`** (**`SCH-cvs-pipeline-tick`**) | Cron measure from **`health.jsonl`**; optional **`CONVERGENCE_TICK_ROLLUP`** | [EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md](./EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md) (**Integration menu**) |
| **`scripts/check_convergence_promotion_readiness.sh`** | Optional **promotion gate** (measure JSON + rollup + test-failures; **`PROMOTION_REQUIRE_*`**) | [scripts/README.md — helpers](../scripts/README.md#convergence-promotion-and-overseer-helpers) |
| **`scripts/record_convergence_overseer_run.sh`** / **`SCH-convergence-overseer-record`** | Read-only **`convergence overseer`** → **`overseer_run_v1`** append | Same; **`.zqk/logs/scheduler/cvs/overseer_runs.jsonl`** |
| **`zqk scheduler convergence promotion-readiness`** / **`record-overseer-run`** | Thin CLI wrappers around the same two scripts (**`bash`**); sets **`ZQK_PROJECT_ROOT`** / **`ZQK_BIN`** | [scripts/README.md — helpers](../scripts/README.md#convergence-promotion-and-overseer-helpers) |

## Problem

Operators must manually chain **scheduler test bundles**, **`zqk scheduler convergence measure`**, **gate scripts**, **vetting matrix** automation, and **`zqk object update`**. There is no single **autonomous** process that runs “the convergence loop” to completion.

Separately, **`ready_for_session_completion`** and **`measurement_implied_phase`** (e.g. **c6_exit**) are derived **only** from the **test-bundle health snapshot** (`pkg/scheduler` `applySessionCompletionGates`). That is correct for **bundle health** but **must not** be treated as “the whole session contract is satisfied.” Today, confusing those signals risks **premature** phase or status updates unless humans enforce **desired_end_state** and **process rules** outside the measurement path.

**Canonical measurement conclusions:** Every measurer should classify results using the **four primary outcomes** in [MEASUREMENT_OUTCOME_TAXONOMY.md](./MEASUREMENT_OUTCOME_TAXONOMY.md) (`measurement_yields_convergence` \| `measurement_yields_divergence` \| `measurement_yields_halt_or_error` \| `measurement_yields_ambiguous_outcome`). Legacy fields such as **`delta_assessment`** and **`rollup_v1.rollup_status`** remain until emitters are unified; see the taxonomy doc for interim mapping and **`[REDACTED-ID]`**.

## Goals

1. **CVS-wide recommendation** — One structured view that aggregates **multiple evaluation surfaces** (bundles, literal gates, drift baselines, matrix rows, optional child sessions) and answers: *what is blocked, what is green, and what is the recommended next operator or automation step* — without implying completion until **configured** gates agree.

2. **Self-driving loop (orchestration)** — A **governed** runner (scheduler job, tick, or script) that **invokes** the right measurements in order, **persists** evidence to the right objects, and **stops or escalates** on failure — reducing “which command next?” to a **single entrypoint** (or a small set of profiles).

3. **Nested convergence sessions** — **Parent** `convergence_session` objects coordinate **child** sessions (or linked work), each with **narrow, explicit exit criteria**. Depth should stay **small** (e.g. 2–3 levels) to avoid unbounded recursion and audit pain.

4. **Alignment with existing semantics** — Keep **bundle snapshot** logic as **one input** to the aggregator; do **not** overload **`ready_for_session_completion`** to mean global completion unless **thresholds** on the CVS explicitly redefine it (see predicates doc).

5. **Multi-session coordination** — When several active **`CVS-*`** (and agents or jobs) touch **shared resources** whose state changes independently (**`health.jsonl`**, bundle fingerprints, scheduler capacity, **git**), an **overseeing interpretation** can **merge or sequence** guidance so **next actions** **coalesce** toward mutual benefit instead of **destructive contention**. See **Appendix F**.

## Non-goals

- Replacing **human** judgment for escalation, suspend, or abandon.
- Unbounded nesting or arbitrary graphs of CVS objects without **documented** linking convention and **automation** support.
- Direct edits to instance YAML under **`docs/process/`** — all persisted updates remain **`zqk object update`** (and bulk/batch where appropriate).

## Architecture (conceptual)

### Blackboard handoff pattern (agent–system loop)

**Canonical glossary:** **`GLS-1776410364614477000-e6d4c940`** — full definition, agent prompts, and machine hints live in CAS.

**Intent:** One **logical** orchestration shape for multi-step CVS work, regardless of surface (single CVS, nested sessions, matrix rows, backlog item + criteria + test refs, priority plan slice). **Durable state** stays in **process objects** (and their journals / links); a thin entrypoint **projects** each tick to **stdout** (next unit of work + acceptance hints) and uses a **small exit-code contract**. The agent completes work and **re-invokes** with structured completion (`zqk object update`, handoff file, ids) — not long-lived interactive stdin and not chat as source of truth.

**Toy reference:** `scripts/cursor/math-volley-blackboard.sh` (`--file`, `VOLLEY_BB_V1`) demonstrates the **shape**; production blackboards **join** the same objects this doc already aggregates (bundles, rollup, child CVS, gates).

**Visibility:** Chat may carry **occasional** human-facing status; **efficiency** comes from driving the loop from **stdout + CLI + objects**. **Inferencing** applies **inside** the corridor the system narrows (metrics, health, validators) — not instead of persisted rules.

### Outcome aggregator

**Role:** Combine signals into a **rollup** attached to a **parent** convergence session (or a dedicated reporting object), e.g. under **`after_state_snapshot`** or a versioned **`rollup_v1`** key documented by convention.

**Inputs (examples):**

| Surface | Source |
|--------|--------|
| Bundle health | `health.jsonl` window → `BuildTestBundleConvergenceSnapshot` / existing snapshot fields |
| Field-key / ZQK-env gates | Exit codes + optional parse of script output |
| Drift / hardcoded literals | Baseline files under `.zqk/logs/drift/` or script JSON |
| Vetting matrix | Row counts / profile completion from automation |
| Child CVS | **`status`**, **`current_phase`**, **`delta_assessment`** of linked sessions |

**Outputs:**

- **`rollup_status`**: e.g. `blocked | partial | ready_for_review | satisfied` (exact enum TBD at implementation).
- **`blockers`**: list of machine-readable codes + human strings.
- **`recommended_next_action`**: single string suitable for **`next_action`** or operator chat.
- **`ready_for_parent_completion`**: bool **only** if **all** configured parent gates pass (distinct from bundle-only **`ready_for_session_completion`**).

The aggregator is the **only** component that should drive **parent-level** “safe to complete” **when** the session’s **`thresholds`** declare that composite gates are in scope.

### Orchestrator (single entrypoint)

**Role:** Run a **defined** sequence: refresh bundles → run convergence measure → run gate scripts → update matrix step → call aggregator → optionally **`object update`** on CVS with **`--persist-session`**-style payloads or a dedicated **`convergence_session_rollup_tick`** job.

**Properties:** idempotent where possible, **log paths** for every step, **non-zero exit** on any **required** failure, **respect** `tests-background-output-to-file` for long work.

### Nested CVS

**Linking:** Prefer existing **`base_object.related_object_refs`** (or a future typed **`child_convergence_session_refs`**) to point from **parent** to **child** `convergence_session` ids. Document **direction** (parent holds children) and **max depth**.

**Prefer nest veneer (trivial path):** `zqk scheduler convergence nest-spawn` / `nest-link` / `nest-status` (see [CAP_LOOP_CONTRACT.md](./CAP_LOOP_CONTRACT.md)). Avoid hand-editing `related_object_refs` when the veneer is available.

**Child session:** Small **hypothesis** / **desired_end_state** (e.g. “pkg/foo bundles green”). Exit: **`status`** = completed when **child** aggregator (or child-only gates) says so.

**Parent session:** **desired_end_state** requires **all children complete** + **global** gates. Parent **`current_phase`** advances only when **parent rollup** allows — not when a single child or bundle sub-signal flips green.

**Recursion:** Cap depth in **automation**; optional **cycle detection** when resolving refs.

### Multi-session coordination (summary)

Concurrent active sessions are **allowed** by the lifecycle; **shared surfaces** are not automatically partitioned per **`CVS-*`**. A **coordination layer** (parent rollup, optional arbitration step, or future dedicated record) should resolve **who goes first** when **recommended_next_action**-style guidance **collides** on the same bottleneck. **Appendix F** defines the intent, evaluates **existing object kinds** before inventing a new one, and states the **anti-pattern** (overloading a single-purpose kind into an ambiguous grab bag).

## Relation to today’s CLI and prompts

- **`zqk scheduler convergence measure`** remains the **measurement + suggested fields** path for **test-bundle** evidence; **`--format agent-prompt`** includes **`rollup_status_core`** as **Next measured action (rollup)** so chat prompts follow the same disambiguated **`recommended_next_action`** as JSON (plus literal gates + direct child CVS rows).
- **`zqk scheduler convergence overseer`** (coordinator **`CVS-*`**) adds a **nested tree** and **arbitrated** parent line; use when **`related_object_refs`** links children.
- Agent prompt **Scope** sections document narrow vs contract-wide semantics.
- The **aggregator** **adds** a layer **above** that snapshot; it does **not** remove the need for **process rules** documented in **CONVERGENCE_PREDICATES_AND_GATES.md**.
- **Integration mechanics** (hook vs tick vs callback, rate/shape): **EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md**. **Phase router + parity** across those entrypoints: **CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md** (**§ Parity and convergence target**).

## Phased delivery (suggested)

1. **Design lock** — This doc + backlog item; **rollup_v1** shape and linking convention are sketched in **Appendices A–C** (refine during implementation).
2. **MVP aggregator** — **`python3 scripts/cvs_outcome_rollup.py`** reads convergence JSON, gate scripts, vetting matrix, drift search-baseline symlink, child **`CVS-*`** refs; optional **`--apply`** merges **`rollup_v1`** into **`after_state_snapshot`** (no automatic **`status`** change).
3. **Orchestrator wrapper** — **`./scripts/cvs_convergence_orchestrate.sh <CVS-id>`** runs **`convergence measure --persist-session`** then the aggregator with **`--apply`**; optional **`CVS_ORCH_SCAN_TESTS=1`** for a preceding **`scan-tests`** (requires **`CVS_ORCH_SCAN_TESTS_ARGS`**).
4. **Nested CVS pilot** — One parent + two children with explicit acceptance tests.
5. **Tighten automation** — Optional **`object update`** proposals when rollup **`satisfied`** and lifecycle allows.
6. **Coordination (when needed)** — Implement arbitration **per Appendix F**: start from **parent `CVS-*` + `rollup_v1`** and **plan/backlog** tie-breakers; add **leases** or a **new kind** only if the matrix there is insufficient.

## Implementation reference (MVP — locked)

Operator-facing decisions for **`[REDACTED-ID]`** so wiring does not drift. More detail: **`scripts/README.md`** (rollup / orchestrate examples).

| Topic | Decision |
|--------|----------|
| **Rollup merge key** | **`after_state_snapshot.rollup_v1`**. The Python aggregator sets **`rollup["schema_version"] = "rollup_v1"`** and merges into the parent session’s **`after_state_snapshot`** (preserving other keys). Implementation: **`_merge_after_state_snapshot`** in **`scripts/cvs_outcome_rollup.py`**. **`--apply`** runs **`zqk object update`** on the parent **`CVS-*`** id with **`after_state_snapshot`** only (no automatic **`status`** change). |
| **Orchestrator entrypoint** | **`./scripts/cvs_convergence_orchestrate.sh`** — argument: parent **`CVS-*`** id. Fixed order: optional **`scan-tests`** (env-gated) → **`zqk scheduler convergence measure --format json --session-id … --persist-session`** → **`python3 scripts/cvs_outcome_rollup.py --session-id … --apply`**. Exits non-zero if persist or rollup fails (**fail closed** on required steps). Extra args after the CVS id are forwarded to **`cvs_outcome_rollup.py`**. |
| **Orchestrator environment** | **`ZQK_PROJECT_ROOT`** (default cwd), **`CVS_ORCH_LOG_DIR`**, **`CVS_ORCH_PERSIST_JSON`**, **`CVS_ORCH_ROLLUP_OUT`**, **`CVS_ORCH_SKIP_PERSIST`**, **`CVS_ORCH_SCAN_TESTS`** + **`CVS_ORCH_SCAN_TESTS_ARGS`**, **`CVS_ORCH_RUNS_JSONL`** (append-only run log; default **`.zqk/logs/scheduler/cvs_orchestrate_runs.jsonl`**, **`schema_version`: `cvs_orchestrate_run_v1`**), **`CVS_ORCH_DISABLE_JSONL`**. Each run appends via **`zqk scheduler record-cvs-orchestrate-run`** (stdin JSON → **`pkg/scheduler.AppendCVSOrchestrateRunV1`**) — see header comment in **`scripts/cvs_convergence_orchestrate.sh`**. |
| **Tick job rollup (`convergence_session_tick`)** | Optional **`CONVERGENCE_TICK_ROLLUP`** runs the same orchestrate script with **`CVS_ORCH_SKIP_PERSIST=1`** after a successful tick, then reads the rollup JSON for **`WriteJobOutcome`**. Set **`CVS_ORCH_ROLLUP_OUT`** on the **`scheduler_job`** when you override the default rollup path (absolute or repo-relative); the scheduler forwards it into the subprocess so the shell writes where **`ResolveCVSRollupLatestJSONPath`** reads (**`pkg/paths`** default layout under project data **`logs/`** + drift segment). Test-bundle jobs inherit the same keys when spawned from bundles (forwarded env). |
| **Go vs script split** | **`pkg/convergerollup`** owns **`ComputeRollupStatus`** / **`rollup_status`** enum parity with **`rollup_status_core`** on **`zqk scheduler convergence measure --format json`**. **Python** owns vetting-matrix row counts, drift search-baseline stats, and assembling full **`rollup_v1`** JSON for **`--apply`** (see **Appendix D**). |
| **Bundle completion vs parent completion** | **No rename** of **`ready_for_session_completion`** in convergence JSON for this MVP. Parent-level readiness is **`rollup_v1.ready_for_parent_completion`** (distinct from bundle-only signals). |
| **Nested CVS linking (pilot)** | Parent lists children on **`related_object_refs`** (parent → **`CVS-*`**). Example parent tied to this backlog: **`[REDACTED-ID]`** (see backlog **`related_object_refs`**). Automation should respect **max depth** (Appendix C). BFS tree for **`convergence-overseer`**: **`pkg/convergerollup.CollectCVSTreeBFS`** with **`DefaultOverseerCVSTreeMaxDepth`** (3); unit tests in **`cvs_tree_bfs_test.go`**. |

## Open questions

- **Coordinator** integration: queue heavy gate runs vs inline CLI (see coordinator_async profile notes).
- **Coordination:** Where should **soft leases** or **fingerprint ownership** live (parent **`rollup_v1`** vs small dedicated fields vs new kind) — see **Appendix F**.

**Resolved (MVP):**

- ~~Should **`ready_for_session_completion`** be renamed~~ — **Supplement, not rename**: use **`rollup_v1.ready_for_parent_completion`** for composite parent gates.
- ~~Where should the aggregator live first~~ — **Hybrid**: **`pkg/convergerollup`** for core status math + **Python** for full multi-surface **`rollup_v1`** payload and **`--apply`** (see **Appendix D** and table above).

## Traceability

- **Backlog:** `[REDACTED-ID]` — *Convergence orchestration: outcome aggregator + nested CVS + self-driving loop* (`priority_plan_ref` = `[REDACTED-ID]`, related CVS `[REDACTED-ID]`). Created via **`zqk object create backlog_item`**.

---

## Appendix A: `rollup_v1` shape (convention, MVP)

Store under **`after_state_snapshot`** (or a dedicated key agreed at implementation time). Version in **`schema_version`** inside the rollup so tooling can evolve.

```json
{
  "schema_version": "rollup_v1",
  "generated_at_rfc3339": "2026-04-07T21:00:00Z",
  "parent_convergence_session_id": "CVS-…",
  "rollup_status": "partial",
  "surfaces": {
    "test_bundles": {
      "ready_for_session_completion": true,
      "failing_fingerprints_now": [],
      "delta_assessment": "neutral"
    },
    "field_key_literals_gate": { "exit_code": 0, "script": "scripts/check-field-key-literals-repo.sh" },
    "zqk_env_literals_gate": { "exit_code": 0, "script": "scripts/check-zqk-env-literals-repo.sh" },
    "drift_baseline": { "path": ".zqk/logs/drift/search-baseline/…", "pattern8_total_bytes": 0, "pattern8_total_lines": 0 },
    "vetting_matrix_go_rows": { "pending_rows": 12, "profile_id": "codebase_vetting_v1" },
    "child_sessions": [
      { "id": "CVS-child-1", "status": "completed", "blockers": [] }
    ]
  },
  "blockers": [
    { "code": "matrix_rows_pending", "detail": "12 human-scope .go row(s) not fully done (gate columns)" }
  ],
  "ready_for_parent_completion": false,
  "recommended_next_action": "Run automated vetting batch or narrow scan-tests for remaining packages; re-run gates after edits."
}
```

**Field notes:**

- **`rollup_status`**: `blocked` | `partial` | `ready_for_review` | `satisfied` — **`ready_for_review`** = bundle + literal gates green but matrix rows and/or child sessions still need attention (see `scripts/cvs_outcome_rollup.py`).
- **`ready_for_parent_completion`**: true only when **all** configured parent gates pass; **independent** of **`surfaces.test_bundles.ready_for_session_completion`**.
- **`surfaces`**: Extensible map; omit keys when a surface is not in scope for this session.

## Appendix B: Orchestrator step order (reference)

Single process (script or scheduler job) should execute **in order**, logging each step:

1. **Optional:** `zqk scheduler scan-tests` (targeted) or rely on existing **health.jsonl** — policy: **tests-background-output-to-file**.
2. **`zqk scheduler convergence measure --session-id <CVS> --persist-session`** (or measure-only if persist is separate).
3. **Gate scripts** — e.g. `sh ./scripts/check-field-key-literals-repo.sh`, `sh ./scripts/check-zqk-env-literals-repo.sh`; capture exit codes and tail paths.
4. **Drift / matrix helpers** (as configured on the session) — non-zero fails the run if required.
5. **Aggregator** — merge JSON from steps 2–4 + **`zqk object get`** for **child** CVS ids listed on parent **`related_object_refs`**; write **`rollup_v1`** payload for **`object update`**.
6. **No automatic `status: completed`** until operator or policy accepts **`ready_for_parent_completion`** and lifecycle rules.

Failures at any **required** step: exit non-zero; do not claim rollup **`satisfied`**.

## Appendix C: Parent / child linking (convention until spec field exists)

- **Parent** `convergence_session` holds **child** ids in **`related_object_refs`** (children are **`CVS-*`**). Direction: **parent → child** only for the MVP; document reverse navigation via **`zqk object list`** / indexes if needed.
- **Max depth (nested tree):** **`zqk scheduler convergence overseer`** walks a depth-capped BFS over child **`CVS-*`** links; cap **`pkg/convergerollup.DefaultOverseerCVSTreeMaxDepth`** (3), with cycle/dedup — see **`CollectCVSTreeBFS`**. **`scripts/cvs_outcome_rollup.py`** (MVP) does **not** recurse the tree: it loads **immediate** child sessions from the parent’s **`related_object_refs`** only, bounded by **`--max-children`** (default 16).
- **Child** sessions use **narrow** **desired_end_state** text so exit is auditable.

## Appendix D: Product evolution — scripts, core assimilation, hooks, and metrics

**Intent:** Python (or other) scripts are appropriate for **early prototyping and one-off tasks**; the durable product is a **small set of stable contracts** implemented in **Go** where the rest of the system already lives (scheduler, storage, metrics, reports), with **explicit extension points** so external automation never becomes a parallel operating system.

### Design principles

1. **Parity with phase-router doc** — **[CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md](./CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md)** (**§ Parity and convergence target**): same **contracts** across entrypoints until the UX is **one** clear feature; no divergent meanings for **ready** / **next** / **complete** without a documented bridge.

2. **Scripts prove semantics; the core owns execution.** Rollup rules (`rollup_status`, blockers) should eventually live in **`pkg/`** (callable from CLI and scheduler) so behavior is tested, versioned with the binary, and not forked across repos.
3. **Custom scripts remain valid** for experiments, research, or vendor-specific glue. They should integrate through **documented hooks** (CLI flags, JSON shapes, env) — not by scraping private file layouts.
4. **Everything worth steering the product should be observable.** Each “exchange” between automation and core (measure, gate, rollup, object update) should be able to emit **structured signals** (metrics + optional JSONL / object fields) that **rollup into reports** the same way test bundles and scheduler jobs already do.

### Hooks that already exist (reuse before inventing)

| Hook | Role |
|------|------|
| **`zqk scheduler convergence measure`** | Stable JSON surface + optional **`--persist-session`** — primary measurement → CVS fields. |
| **`zqk object get` / `zqk object update`** | Authoritative read/write for **`after_state_snapshot`**, **`activity_log`**, thresholds — audit trail on CAS. |
| **`health.jsonl` / test-bundle events** | Append-only streams for **outcomes** and job attribution (`AppendTestBundleHealthEvent`, `AppendTestBundleEvent` in **`pkg/scheduler`**). |
| **`convergence_session_tick`** | Cron-style **governed** measure with watermark / rate limits — same *contract* as convergence CLI. |
| **Test-bundle run-wrapper hook** | In-process reaction after bundles when **`CONVERGENCE_SESSION_ID`** is set — low-latency path from jobs to convergence logic. |
| **Scheduler metrics + `observability.Recorder`** | Job lifecycle and **named metric** records — pattern for optional backends; **durable** orchestrate observability today is **`cvs_orchestrate_runs.jsonl`** (below), not a separate Recorder sink. |
| **`cvs_orchestrate_runs.jsonl`** | Append-only: one line per **`scripts/cvs_convergence_orchestrate.sh`** run (**`schema_version`**: **`cvs_orchestrate_run_v1`**). **Implementation:** shell builds JSON → **`zqk scheduler record-cvs-orchestrate-run`** (**`pkg/scheduler.AppendCVSOrchestrateRunV1`**) — same pattern family as **`rollup_status_core.jsonl`**. Default **`.zqk/logs/scheduler/cvs_orchestrate_runs.jsonl`**; **`CVS_ORCH_RUNS_JSONL`**, **`CVS_ORCH_DISABLE_JSONL`**. |

See **[EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md](./EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md)** for rate/shape tradeoffs (when to tick, when to JSONL, when to object update).

### Target “knobs and buttons” (evolving surface)

These are the **intended** control and observation points — implemented incrementally:

1. **Versioned payloads** — **`rollup_v1`** (and convergence JSON) carry **`schema_version`** so CLI, scheduler jobs, and reports migrate together.
2. **A first-class rollup operation in core** — e.g. **`zqk scheduler …`** or **`zqk system convergence rollup`** that runs the same logic as today’s script **inside** `pkg/`, optionally as a **`scheduler_job`** with **stdout/log + exit code + metrics**, not only a shell wrapper.
3. **Metrics** — For each rollup (or orchestrate) run: **duration**, **`rollup_status`**, **blocker counts by code**, gate exit codes — may later feed **`observability.Recorder`** / reports; **today** orchestrate runs also persist **`cvs_orchestrate_run_v1`** lines (structured fields in JSONL) for the same signals without a parallel pipeline.
4. **Event stream (convergence path)** — **`rollup_status_core.jsonl`** under **`.zqk/logs/scheduler/`** — one line per **`convergence measure`** run (timestamp, optional session id, full **`rollup_status_core`** map). Broader “every rollup attempt” metrics remain future work.
5. **Orchestrate run log** — **`cvs_orchestrate_runs.jsonl`** (same directory) — one line per **`cvs_convergence_orchestrate.sh`** run; **`schema_version`** **`cvs_orchestrate_run_v1`**. **Why not append from shell alone:** validation and append live in **`pkg/scheduler`** ( **`AppendCVSOrchestrateRunV1`** ) via **`zqk scheduler record-cvs-orchestrate-run`** so CI and tests can exercise the same code path. Fail-closed shell exits still produce a line with non-zero **`exit_code`** (record step is best-effort: **`|| true`** in the trap so orchestrate’s exit code stays fail-closed on persist/rollup).

### Phasing (aligned with § Phased delivery)

- **Now:** Scripts call **only** public **`zqk`** commands and documented paths; no reliance on undiscovered internals.
- **Next (started):** **`ComputeRollupStatus`** lives in **`pkg/convergerollup`** (Go tests mirror **`scripts/cvs_outcome_rollup_test.py`**). **`zqk scheduler convergence measure --format json|yaml`** embeds **`rollup_status_core`** (bundle + **`scripts/check-field-key-literals-repo.sh`** / **`scripts/check-zqk-env-literals-repo.sh`** exit codes + optional child CVS from **`related_object_refs`**; **`--skip-rollup-gates`** skips gate subprocesses). Vetting matrix and drift baselines remain **`scripts/cvs_outcome_rollup.py`**. Each convergence json/yaml/table run **appends** one line to **`.zqk/logs/scheduler/rollup_status_core.jsonl`**. Keep Python **`_compute_rollup_status`** aligned with **`convergerollup.ComputeRollupStatus`**.
- **Then (started):** **`scripts/scheduler_jobs/convergence_orchestrate.yaml`** ( **`SCH-convergence-orchestrate`** ) — **`zqk object create scheduler_job --file … --keep-file`**; update **`CONVERGENCE_SESSION_ID`** when the active CVS changes. **`scripts/templates/convergence-orchestrate-job.yaml`** remains a copy/edit starter. Further: extension of **`convergence_session_tick`** or dedicated job type; **metrics** feeding **reports** and **policy** (e.g. “no promote if rollup blocked in last N hours”).

This keeps **external scripts** as **clients** of a **stable API**, while **core** retains **policy, storage, and observability** — matching the vision of **knobs** (CLI + object fields + thresholds) and **metrics** that **bubble up** into **system behavior**.

## Appendix E: Employing the loop (operator runbook)

**Goal:** Run measure → persist → full rollup on a **parent** `convergence_session` without ad-hoc command chaining.

### Prerequisites

- **`zqk`** on **`PATH`** (rebuilt after code changes); scheduler **daemon** running if you use timer jobs.
- **Test-bundle evidence:** `.zqk/logs/scheduler/test-bundles/health.jsonl` advancing (from **`zqk scheduler scan-tests`** / bundles). No health window ⇒ convergence has little to measure.
- **Session id:** a **`convergence_session`** (**`CVS-*`**) you own; optional **`related_object_refs`** to child **`CVS-*`** for rollup.

### Path A — Local shell (fastest to try)

From repo root:

```bash
./scripts/cvs_convergence_orchestrate.sh CVS-your-id
```

Optional: **`CVS_ORCH_SCAN_TESTS=1`** + **`CVS_ORCH_SCAN_TESTS_ARGS='--package ./pkg/foo'`** first (long-running — see **tests-background-output-to-file** policy).  
Artifacts: **`cvs_orch_last_persist.json`**, **`cvs_rollup_latest.json`** under **`.zqk/logs/drift/`** (or **`CVS_ORCH_LOG_DIR`**).

### Path B — `zqk` only (gates + `rollup_status_core`, no matrix/drift)

```bash
zqk scheduler convergence measure --format json --session-id CVS-your-id --persist-session
# json includes rollup_status_core; appends .zqk/logs/scheduler/rollup_status_core.jsonl
```

Use **`--skip-rollup-gates`** for quick probes. Full matrix + drift still require **Path A**’s Python step or **`python3 scripts/cvs_outcome_rollup.py`**.

### Path C — Scheduled automation

**Preferred:** maintained job file **`scripts/scheduler_jobs/convergence_orchestrate.yaml`** (id **`SCH-convergence-orchestrate`**).

1. **`zqk object create scheduler_job --file scripts/scheduler_jobs/convergence_orchestrate.yaml --keep-file`** (process data — CLI only; do not hand-edit CAS YAML under **`docs/process/`** except via **`zqk`** per project rules). If the id already exists: add **`--force`** to refresh from the file.
2. Set **`environment_variables.CONVERGENCE_SESSION_ID`** to your active **`CVS-*`** ( **`zqk object update SCH-convergence-orchestrate …`** when the session changes). Optionally set **`ZQK_PROJECT_ROOT`** on the job if the scheduler working directory is not the repo root.
3. Set **`enabled: true`** when ready ( **`status`** stays **`active`**); watch job logs under **`.zqk/logs/scheduler/`** and tail **`rollup_status_core.jsonl`**.

**Alternate:** copy **`scripts/templates/convergence-orchestrate-job.yaml`**, set **`id`**, **`CONVERGENCE_SESSION_ID`**, **`ZQK_PROJECT_ROOT`** (absolute path), **`schedule_expression`**, **`max_runtime_seconds`**, then create as above.

The job runs orchestrate with **`--no-fail-on-gates`** on the Python rollup so **matrix-only** blockers do not fail the scheduler job exit code; tighten flags in a **fork** of the YAML for stricter CI.

### Autonomous loop hardening (`convergence_session_tick` + `cvs_convergence_orchestrate.sh`)

- **Interruptibility:** Scheduler handlers run under **`execCtx`** from **`max_runtime_seconds`** on the **`scheduler_job`** (dispatcher cancellation does not shorten execution — see **`job_execution`**). Keep **`max_runtime_seconds`** set on **`SCH-cvs-pipeline-tick`** (and similar) so stuck ticks terminate cleanly.
- **Session targeting:** **`CONVERGENCE_SESSION_ID`** must match the canonical **`CVS-*`** object id pattern **before** storage reads (rejects malformed env).
- **Concurrency:** Overlapping ticks for the **same** CVS are **serialized** so two runners cannot interleave **`storage.Update`** on one session.
- **Large `health.jsonl`:** Tail reads periodically honor **cancellation** so oversized logs do not ignore job timeouts.
- **Orchestrator script:** **`SIGINT`/`SIGTERM`** append **`…_interrupted`** to **`ORCH_STAGE`**, record exit **130** in **`cvs_orchestrate_runs.jsonl`**, and exit without double-running the **`EXIT`** trap.

### What is not automated yet

- **`SCH-convergence-orchestrate`** is a **maintained YAML** under **`scripts/scheduler_jobs/`** (not auto-created by **`zqk system init`**). It runs the **full** orchestrator path (measure persist, Python rollup **`--apply`**, optional **`scan-tests`**, etc.—see script header). By contrast, **`convergence_session_tick`** is **primarily** bundle-health measure → CVS (**same contract** as **`zqk scheduler convergence measure --session-id`**); it can optionally run **rollup-only** (**`CONVERGENCE_TICK_ROLLUP`**) via **`cvs_convergence_orchestrate.sh`** with **`CVS_ORCH_SKIP_PERSIST`** and record outcomes on the **tick job** events JSONL—not a substitute for the full **`SCH-convergence-orchestrate`** job when you need the entire Path A automation.
- **Policy gates** (“do not promote if rollup blocked”) — design in Appendix D; wire when metrics/recorder work lands.
- **Cross-session arbitration** — overseer semantics in **Appendix F**; a **read-only CLI** entrypoint exists: **`zqk scheduler convergence overseer --coordinator-session-id <CVS-*>`** (rollup + depth-capped **`related_object_refs`** tree + cycle detection + **`arbitrated_next_action_markdown`**). It does **not** persist process objects or replace a scheduled arbitration loop.

---

## Appendix F: Multi-session coordination, shared resources, and object-kind strategy

### Problem

Multiple **active** **`convergence_session`** objects, **agents**, and **scheduler jobs** can run **in parallel**. Each session has its own **CAS-backed** record and **`--session-id`**-scoped measure/rollup, but **evaluation surfaces** such as **test-bundle `health.jsonl`**, **shared bundle fingerprints**, and **CI/scheduler capacity** are **repo-wide**. Without coordination, **next_action**-style guidance can **collide** on the same bottleneck: duplicate reruns, thrashing, or contradictory operator directives — **destructive contention** instead of **mutually beneficial coalescence**.

### Design intent: an overseeing interpreter (not a silent mutex)

We want a layer that can:

1. **Detect** overlap on **shared state-changing resources** (e.g. same failing fingerprint claimed by two campaigns; same package under two active sessions).
2. **Interpret** priorities using **declared** rules and **linked** process objects (plans, backlog ownership), not ad hoc chat.
3. **Emit** a **single ranked or merged** recommendation — **arbitrated next step** — so humans and automation **converge** rather than fight.

This may be implemented as a **scheduled job**, an **orchestrator extension**, or **in-process** logic once **`rollup_v1`** and metrics exist; the **contract** matters more than the binary shape in this appendix.

### Mechanisms (building blocks)

| Mechanism | Role |
|-----------|------|
| **Parent `convergence_session` + `rollup_v1`** | Already the right **aggregate** place for **child** refs and **composite** blockers; can be extended to hold **cross-session** arbitration output **if** the parent is explicitly the **coordinator** session (see below). |
| **`priority_plan` / `backlog_item`** | **Priority and ownership** when two sessions conflict — tie-breakers (“prefer PRI-x”, “owner BLI-y”) without duplicating full session state. |
| **`thresholds` / `activity_log` on `convergence_session`** | Rate limits and **audit** of coordination decisions per session. |
| **Soft leases (future)** | Optional **time-bounded claims** on a fingerprint or scope (e.g. “session A owns remediation of F until T or green”) — stored only where semantics stay **clear** (see **Object-kind strategy**). |
| **Policy objects (`POL-*`)** | **Declarative** rules (“if rollup blocked, do not promote”) — complement, not replace, **stateful** arbitration for **simultaneous** active sessions. |

### Object-kind strategy: evaluate existing kinds before adding a new one

**Rule:** Before introducing a **new** system object kind for “coordination,” **explicitly evaluate** whether **extending** existing kinds — with **narrow, well-named fields** and **documented intent** — achieves the same outcome **more cohesively**.

**Anti-pattern (reject):** **Morphing** a **single-purpose** kind into a **conglomerate** where **different properties** apply in **different contexts** and readers cannot tell **which role** an instance plays. Example: every **`convergence_session`** carrying optional “global traffic cop” fields that are **ignored** on leaf sessions but **overloaded** on others without a clear **declared role**.

**Evaluation matrix (starting point):**

| Kind | Fit | Caveat |
|------|-----|--------|
| **`convergence_session`** | **Strong** for **parent** sessions that **already** anchor **nested** work via **`related_object_refs`**. Coordination **logically belongs** with a **declared parent** whose **`desired_end_state`** includes **resolving child conflicts** or **sequencing**. | Do **not** push **cross-repo arbitration** fields onto **every** leaf session. Prefer a **dedicated parent** (or explicit session **subtype / role** in documentation until spec supports it). |
| **`rollup_v1` / `after_state_snapshot`** | **Strong** for **versioned**, **auditable** outputs: **arbitrated_next_action**, **conflict_set**, **resolved_priority_ref**. | Keep **schema_version** so tooling evolves safely. |
| **`priority_plan` / `backlog_item`** | **Strong** for **who wins** when priorities differ; **weak** alone for **live** technical merge of measures. | Use as **inputs** to the interpreter, not the only store of truth for **health** state. |
| **`policy`** | **Strong** for **gates** and **non-promotion** rules; **weak** for **dynamic** “session A vs B” without external state. | Pair with **session- or rollup-held** conflict payloads. |
| **`scheduler_job`** | **Strong** for **when** coordination runs (cron, **run_wrapper**). | **No** persistent **interpretation** state in the job object itself; job **invokes** logic that **writes** process objects. |
| **`workflow`** (if used) | Possible **orchestration** of steps across kinds; **evaluate per project** whether a workflow instance is already the **spine** for multi-step maintenance. | Avoid **two** parallel definitions of the same sequence (workflow vs CVS parent) without **one** being canonical. |

**When a new kind might still be justified**

Introduce a **new** kind **only if**:

- Parent **`convergence_session` + rollup** cannot hold arbitration **without** blurring **parent vs leaf** semantics, **and**
- **`priority_plan` / backlog** cannot express **durability** needs for **coordination state** (leases, queues) **without** abusing those kinds, **and**
- The new kind has a **single, clear responsibility** (e.g. “coordination board for repo R over window W”) — **not** a second copy of **`convergence_session`**.

Until then, **prefer** a **coordinator-shaped parent `CVS-*`** with **explicit** **`related_object_refs`** to children and **rollup-held** arbitration fields — **one overseer object** with a **clear title and desired_end_state**, not implicit global behavior.

### Relation to other docs

- **[CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md](./CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md)** — phase router and **coordinator-backed** routing; **§ Parity and convergence target** states the **split of responsibility** with this doc and the **direction** toward one **uniform** convergence feature (**same versioned payloads** across CLI, tick, hook, orchestrate, coordinator). Multi-session **conflict** (here) **complements** single-session **phase** (there); **maintain both** when **vocabulary** or **process guarantees** change.
- **Scheduler concurrency** — `docs/process/architecture/scheduler-concurrency-summary.md`: process-local **job** concurrency; **not** a substitute for **cross-session** semantic arbitration.
