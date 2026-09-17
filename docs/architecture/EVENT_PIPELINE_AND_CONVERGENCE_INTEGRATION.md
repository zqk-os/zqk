# Event pipelines, convergence, and integration choices

**Last Verified:** 2026-08-31


**Purpose:** Preserve design wisdom: **post-processing** over raw events can drive **behavior** (including `convergence_session` updates), but **how** we integrate should follow the **characteristics of the data flow**—especially **rate** and **payload shape**—not a single mandated pattern.

**Status:** Guidance (architecture). Implementation pieces live in `pkg/scheduler`, `cmd/zqk/scheduler`, and related docs.

**Contract parity:** **[CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md](./CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md)** (**§ Parity and convergence target**) and **[CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md](./CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md)** define **shared vocabulary** (measure, persist, rollup, multi-session) so **hook**, **tick**, **CLI**, **orchestrate**, and future **coordinator** workers do not drift into incompatible meanings for **ready** / **next** / **complete**.

---

## Mental model

- **Pipeline:** event or completion signal → interpret → optional **governed** side effects (CVS fields, `activity_log`, enqueue work, notify).
- **Same idea, many surfaces:** the repo already supports **several** integration styles in different places; **do not** box the project into one story (e.g. “always HTTP callback” or “always synchronous hook”).
- **Choose by flow:** match the mechanism to **volume/burst**, **latency tolerance**, **idempotency**, and whether the payload is **structured for automation** vs **narrative / human-facing**.

### Causality over clocks (events as blood supply)

Think of **data in motion** as what keeps the system aligned with reality: structured signals are the **circulatory** layer—work completes, the **next** step is entitled to react, and the graph of **cause → effect** stays **inspectable** (for example bundle finished → **`health.jsonl`** → measure → **`convergence_session`** update; append-only JSONL such as **`context_events`** ([`pkg/contextevents`](../../pkg/contextevents)), **`criteria_verification_evidence`** lines on test-bundle completion, or other **FieldKey-aligned** streams for **sparse** handoffs). Chains like that keep the organism **in sync** because each link has a **named trigger**; debugging is **which edge did not fire** or **which payload was not machine-parseable**, not “which of seventeen schedules ran.”

**Minimize arbitrary time-triggered sprawl.** Cron and maintenance ticks are easy to duplicate until no one remembers why each exists. Prefer **event-driven edges** where the business meaning is “something happened, therefore…”. Reserve **scheduled** jobs for cases where **time itself** is the contract (retention, compaction, **operational envelope** housekeeping) or **batching** is required under load—see **`data_cell_envelope_tick`** (`SCH-dce-tick`) and the envelope table in [DATA_CELL_MODEL.md](./DATA_CELL_MODEL.md). Do **not** use a new timer to paper over a **missing listener** on an existing causal path.

**Intervention** when something breaks should usually **extend or fix the graph** (listener, validator, envelope token, policy gate)—not add another orphan schedule. When a **new scenario** appears (missed requirement, unexpected failure mode), adjust the system so the **same event vocabulary** covers it; only then does additional **cadence** belong in the inventory (see **Scheduler automation discipline** below).

---

## Two axes (quick lens)

| Axis | Question | Tendency |
|------|----------|----------|
| **Rate / burst** | How often do signals arrive? Can they pile up? | High rate → batching, dedup, **watermarks**, **max ticks per hour**, async handoff. Sparse → direct post-hooks or a single tick may suffice. |
| **Data shape** | Is the signal **stable and machine-parseable** (IDs, exit codes, fingerprints, JSON lines) or **rich text** (logs, “history report” prose)? | Structured → deterministic convergence fields (`after_state_snapshot`, phase router inputs). Narrative → **`activity_log`**, separate artifacts, or best-effort append—often not the same path as automated measure fields. |

---

## Integration menu (existing building blocks)

Use the **smallest** fit; combine only when requirements say so.

| Mechanism | Typical use | Notes / pointers |
|-----------|-------------|------------------|
| **Test-bundle run-wrapper hook** | After a bundle finishes, react **in-process** with minimal latency. | `maybeRunConvergenceTickAfterTestBundleHealth` — when **`CONVERGENCE_SESSION_ID`** is on the job env; forwards **`HEALTH_*`**, phase/flow flags, **`SKIP_SESSION_CONTEXT`**, **`CONVERGENCE_TICK_SPAWN_FOLLOWUP_DRAFT`**, **`CVS_MEASUREMENT_EVENTS`**, **`CONVERGENCE_TICK_ROLLUP`**, **`CVS_ORCH_ROLLUP_OUT`** when set. See [CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md](./CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md) (test-bundle hook). |
| **`convergence_session_tick` job** | Same measure/update contract as **`zqk scheduler convergence measure --session-id`**; cron or enqueue. | `pkg/scheduler/handlers_convergence_session_tick.go`; respects **watermark dedup** and **`thresholds.max_ticks_per_hour`**. On successful persist, appends **`cvs/cvs_measurement_events.jsonl`** with **`event_type`** **`cvs_measure_applied`** (disable with **`CVS_MEASUREMENT_EVENTS=0`**). Optional **`CONVERGENCE_TICK_ROLLUP`**: rollup-only orchestrate subprocess + rollup summary on **`…/SCH-cvs-pipeline-tick.events.jsonl`** (not full Path A **`SCH-convergence-orchestrate`**). Suspend cron tick: **`zqk object update SCH-cvs-pipeline-tick --field enabled=false`**. |
| **`NotificationContext`** | In-process subscribers to job lifecycle notifications (terminal/desktop/event channel, optional **`OnNotification`**). | `pkg/scheduler/notification_context.go` — for **daemon-internal** reactions without HTTP. |
| **`callback_listener` job** | HTTP ingress for **external** systems posting completion or commands. | Pipeline: `pkg/scheduler/callback_listener_pipeline.go`; use when the producer is **not** the scheduler process. |
| **Scheduler bundles + logs** | Async verification; inspect failures **after** completion from log paths. | [tests-background-output-to-file.mdc](../../.cursor/rules/tests-background-output-to-file.mdc), [PRE_CHANGE_CHECKLIST.md](./PRE_CHANGE_CHECKLIST.md) §6. |
| **Context events JSONL** | Sparse, structured **evidence** (matrix/CSV outcomes, criteria hints, correlation ids) without a scheduler job — same “append JSONL, interpret later” spirit as metrics lines. | **`pkg/contextevents`** → **`.zqk/metrics/context_events.jsonl`** (POL-OBS-001); **`zqk system emit-context-event` (PRUNED)** writes one line using **`objects.FieldKey*`-aligned** payloads. **`quality.RunTestBundleMatrixPipeline`** appends **`test_bundle_matrix_verify_ok`** when **`verify-test-bundle-matrix`** succeeds (skipped verify → no line). **Evidence only** — does not CAS-update criteria or CVS; operators use **`zqk object update`** when policy allows authoritative state. |
| **Shell wrappers (promotion / overseer)** | Optional **promotion gate** (measure JSON + **`rollup_status_core`** + **`test-failures list`**, each tunable) or **read-only overseer audit** appended as **`overseer_run_v1`** JSONL — **no** `convergence_session` persist from these entrypoints alone. | **`scripts/check_convergence_promotion_readiness.sh`**, **`scripts/record_convergence_overseer_run.sh`** (**`--help`**); index [scripts/README.md](../scripts/README.md#convergence-promotion-and-overseer-helpers); timer **`SCH-convergence-overseer-record`** (`scripts/scheduler_jobs/convergence_overseer_record_daily.yaml`). |

### Operating rhythm (defaults)

1. **Verify tests** without blocking the terminal for huge trees: **`zqk scheduler scan-tests --package ./path/to/pkg`** (or saved bundles); inspect logs under **`.zqk/logs/scheduler/cvs/test-bundles/`** and **`zqk scheduler test-failures`**.
2. **Bundle health** drives **`health.jsonl`**; **`convergence_session_tick`** (or **`convergence measure --persist-session`**) advances the CVS when the watermark moves.
3. **Deep automation** (matrix rollup apply, optional repo **`scan-tests`** inside orchestrate): **`SCH-convergence-orchestrate`** / **`scripts/cvs_convergence_orchestrate.sh`** — separate from the hourly tick’s optional rollup-only pass.
4. **Multi-session read model:** **`zqk scheduler convergence overseer --coordinator-session-id CVS-*`** until a scheduled arbitration worker exists.
5. **Promotion gate (optional, script-level):** **`scripts/check_convergence_promotion_readiness.sh`** combines **`convergence measure --format json`**, **`rollup_status_core`**, and **`test-failures list`** (each piece optional via **`PROMOTION_REQUIRE_*`** env — **`./scripts/check_convergence_promotion_readiness.sh --help`**). **Native CLI:** **`zqk scheduler convergence promotion-readiness`** delegates to the same script. **Overseer audit trail:** **`scripts/record_convergence_overseer_run.sh`** or **`zqk scheduler convergence record-overseer-run`** or timer **`SCH-convergence-overseer-record`** → **`.zqk/logs/scheduler/cvs/overseer_runs.jsonl`**. **Env tables:** [scripts/README.md — Convergence promotion and overseer helpers](../scripts/README.md#convergence-promotion-and-overseer-helpers).

### Scheduler automation discipline (efficiency / DRY)

The same instincts as **DRY** and **clear boundaries** in code apply to **persisted `scheduler_job` objects** and **timer sprawl**:

- **Parameterize, don’t clone.** Prefer **one job** whose **`environment_variables`** carry **`CONVERGENCE_SESSION_ID`**, **`COORDINATOR_SESSION_ID`**, roots, and flags over **many scheduler jobs** that differ only by id or path. Retarget with **`zqk object update`** when the active session or coordinator changes (same pattern as lifecycle sync for **`SCH-cvs-pipeline-tick`**).
- **One script, many schedules (only when needed).** Implement shared logic once under **`scripts/`**; jobs invoke the same entrypoint with different env. Avoid duplicating long **`run_wrapper`** command blocks across YAML templates.
- **Consolidate “same contract,” not “same binary.”** Jobs that share **side-effect semantics** (e.g. five timers each running **`convergence measure --persist-session`** for different CVS ids) are candidates to merge into **one parameterized job** or a **single coordinator** that loops configured ids — **after** product/CLI support for batch or list env (don’t fake it with fragile shell loops in five places). Jobs that differ by **contract** (tick vs full orchestrate vs overseer read-only) stay **separate job types or separate templates** because failure modes, runtime, and concurrency class differ.
- **Inventory and discovery.** Keep **`.zqk/specs/configs/scheduler_maintenance_config.yaml`** (`required_jobs`) aligned with jobs you rely on so **`ensure-retention-jobs`** and operators see **one** list — ad-hoc creates without updating the config drift the system’s picture of “what runs.”
- **When multiple jobs are justified.** Different **cadence**, **category**, **conflict rules**, **max_runtime**, or **blast-radius isolation** — not “this CVS got its own copy.”

### Declarative validators (concept)

**Status:** Vocabulary and direction only — no committed schema, object kind, or scheduler hook yet. The aim is to give the idea **bones**: **declarative configurations** that act like **custom automated test cases** over process reality (objects, CAS paths, log streams, external snapshots), producing **named pass/fail or scored indicators** that **measure**, **rollup**, and operator prompts can treat as **first-class signals** alongside test-bundle health.

**Analogy.** A **validator** is to a long-running operational workflow what a **focused automated test** is to code: a **repeatable predicate** with a clear outcome, not a prose-only **`desired_end_state`** bullet. Example: a flow generates thank-you letters for everyone who attended an event — a validator checks **1:1 correspondence** between letters and individuals (no duplicates, no omissions). The logic can be simple; the win is that it is **machine-checkable**, **diffable**, and **schedulable** like any other gate.

**CVS loop.** Indicators from validators should eventually feed the **same measurement culture** as bundles: visible in **`rollup_status_core`** / **`rollup_v1`** (or successor fields), comparable across **CLI vs tick vs hook**, and honest about **what is proven** — bundles attest **test health**; validators attest **domain invariants** you chose to automate.

**Recursion (kernel / CLI / scheduler).** The stack is already **recursive**: objects drive jobs, jobs mutate state, state feeds the next measure. Adding validators amplifies that unless boundaries are strict: prefer **read-only evaluation** paths that emit indicators; separate **detection** from **remediation**; cap **frequency** and **fan-out** (watermarks, categories, idempotent writes); avoid validators that blindly create objects and enqueue work that retriggers the same validator without a **bounded** contract. With that discipline, declarative validators scale from **small correspondence checks** to richer **cross-session** rules without turning the kernel into an unmaintainable meta-interpreter.

**Implementation deferral.** Concrete **per-job validation profiles** or refs on **`scheduler_job`** (dispatch-time enforcement) stays **deferred** until **parameterized jobs + shared scripts** are the steady posture — **Scheduler automation discipline** above — so we do not bake the wrong abstraction early.

---

## Non-goals

- **One true pattern** for all convergence-adjacent automation.
- **Replacing** human judgment or process objects with a single automated channel.

---

## Related docs

- [scripts/README.md — Convergence promotion and overseer helpers](../../scripts/README.md#convergence-promotion-and-overseer-helpers) — **`check_convergence_promotion_readiness.sh`**, **`record_convergence_overseer_run.sh`**, env tables; pairs **Operating rhythm** above.
- [SCHEDULER_OVERLOAD_AND_TIMEOUT.md](./SCHEDULER_OVERLOAD_AND_TIMEOUT.md) — overload, goroutine ceiling, timer-maintenance priority; pairs with **Scheduler automation discipline** above when scaling jobs.
- [CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md](./CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md) — tick handler, test-bundle hook, tombstones, phase router; **§ Parity and convergence target** (joint maintenance with orchestration doc).
- [CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md](./CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md) — rollup, orchestrator, nested **`CVS-*`**, **Appendix F** (multi-session / shared-resource coordination); [**Entrypoints (shell, CLI, jobs)**](./CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md#entrypoints-shell-cli-jobs) table (orchestrate vs tick vs promotion vs overseer).
- [CONVERGENCE_PREDICATES_AND_GATES.md](./CONVERGENCE_PREDICATES_AND_GATES.md) — what **bundle** measurement does and does **not** imply vs **`desired_end_state`** (see also **Declarative validators (concept)** above).
- [LIFECYCLE_EVENT_LISTENER_AND_CRITERIA.md](./LIFECYCLE_EVENT_LISTENER_AND_CRITERIA.md) — different concern (lifecycle WAL, criteria, transitions); complementary, not duplicate.
- [tests-background-output-to-file.mdc](../../.cursor/rules/tests-background-output-to-file.mdc) — background test verification and bundle workflow.
- [CAS_LIST_GET_CONSISTENCY.md](../process/architecture/CAS_LIST_GET_CONSISTENCY.md) — CAS index vs disk; **operational hazards** for `recover-cas` / `fix-hash-mismatches` and scheduler contention (misuse can make **`convergence_session` disappear from `object list`** while YAML remains on disk).
