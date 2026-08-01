> [!WARNING]
> **ARCHIVED DOCUMENT**: The primary commands referenced in this architectural document have been pruned from the `zqk` CLI.

# Pre-Change Checklist

**Use this checklist before every code change or CLI operation.** It consolidates architectural concerns, coding best practices, and project policies. If a line item applies to your change, confirm it before proceeding. If something is unclear, prompt the human—do not guess.

**Canonical location:** `docs/architecture/PRE_CHANGE_CHECKLIST.md`  
**Cursor rule:** `.cursor/rules/pre-change-checklist.mdc` instructs agents to consult this checklist before code changes and CLI operations.

**Object count / retention / audit:** If your change touches object count report, retention, aggregation, audit cleanup, or object ID cache, also read **`docs/architecture/OBJECT_COUNT_SELF_MAINTENANCE.md`** and ensure the self-maintaining outcome is achieved (see rule `object-count-self-maintenance.mdc`).

**After behavior is correct (semantic density):** Re-scan the diff for duplication, stray literals, and clarity — **§13** post-verify. **Goal:** most lines express *domain* intent; repeated scaffolding is factored out. Prefer extracting on the **second** copy when the abstraction is obvious; **by the third** copy, extract or table-drive. Details: **`docs/best-practices/coding/DRY_PATTERN_EXTRACTION.md`**.

**Rebuild / restart (local):** After pulling **Go** changes under `cmd/` or `pkg/`, run **`go build -o bin/zqk ./cmd/zqk`** (or your install target) before expecting new CLI behavior when you type `zqk` — or invoke **`./bin/zqk`** explicitly. Regenerating **command builders** (`.zqk/cli/specs` → `pkg/cli/bldr_cli_cmd_v1`) also requires a rebuild to pick up new flags. After regenerating builders for leaf commands that own cobra `Use` (e.g. object list), run **`./scripts/check-cli-critical-empty-use.sh`** so empty `NewCommandBuilder("")` stubs cannot re-break swarm inventory (BLI-EXAMPLE). **Scheduler:** restart the scheduler **daemon** only if you changed scheduler registration, job handlers, or IPC that a long-running process caches; many flows support **`--allow-degraded`** without a daemon. Prefer **foreground** `go test ./… -timeout …` for quick checks; heavy suites use **`go test`** / **`test-runner.sh`** per **`tests-background-output-to-file.mdc`**.

---

## 1. Process and object data (POL-CODE-002, process-data-cli-only)

- [ ] **No direct edits** to instance/process YAML under `docs/architecture/` (backlog, requirements, criteria, goals, milestones, orgs, etc.). All create/update/delete for that data goes through the **zqk CLI** (e.g. `zqk object create`, `zqk object update`). Exception: `docs/architecture/_internal/` schema/config when CLI or tooling is unavailable—document why and follow-up.
- [ ] If the change would edit a YAML file under `docs/architecture/` for **instance data**, stop and use the CLI instead.
- [ ] **Documentation graph (context only):** Phased work linking markdown, `document_refs`, and ontology surfaces is tracked as **`BLI-EXAMPLE`** (**deferred** — same id when work resumes; see **`related_object_refs`**: **`GLS-EXAMPLE`** plus three **`doc_entry`** ids — assessment index, CLI alpha plan, backlog-refs design); see **[ASSESSMENT_AND_ONBOARDING_INDEX.md](./ASSESSMENT_AND_ONBOARDING_INDEX.md)**. Not a gate for every change—only when you are intentionally changing doc navigation or cross-reference strategy.

---

## 2. CLI performance and hot path (CLI_PERFORMANCE_AND_CONSISTENCY)

- [ ] **Response time:** Commands that must be fast (create, list, count, etc.) do not add work that would push response time over **1 second** in normal (e.g. warm) conditions. If the change touches `cmd/zqk/object/create.go`, list, count, or system check entry paths, confirm no new synchronous heavy work (e.g. no `LoadFields()` on every create, no `ClearCache()` on every create).
- [ ] **Blocking:** No new **blocking** work on CLI hot paths. Long-running work is **async** (background goroutine, scheduler job, or callback). If the user must wait, progress must be reported at least every **1 second** (or a documented interval).
- [ ] **Caching:** No **full cache clear** or **full cache (re)population** on single-object operations or on every CLI invocation. Caches are **incrementally maintained** (update/invalidate on create/update/delete). Full population only at scheduler init or when the user explicitly requests it.
- [ ] **Concurrency bounds:** List, count, and other paths that fan out over many items use a **bounded worker pool** (fixed number of goroutines pulling from a work channel), not one goroutine per item. See [unbounded-concurrency-fixes.md](./unbounded-concurrency-fixes.md) for the pattern and past fixes.
- [ ] **Fallback paths:** When a "main" path uses a bounded pool (e.g. budgeted goroutinelabels pool), any fallback path (e.g. when budget is nil or pool creation fails) must use the **same concurrency bound** (e.g. fixed N workers on a channel). Fallbacks must not spawn one goroutine per item or otherwise circumvent the bounded-concurrency guarantee.

---

## 3. Caching and expensive operations (CACHE_MANAGEMENT_STRATEGY, CLI performance)

- [ ] **No `LoadFields()` (or equivalent “load all specs”) on hot path** for create/list/count. Use cached field registry or load only what’s needed for the current kind; expensive global loads belong in pre-warm or scheduler init.
- [ ] **No `ClearCache()` (or global spec cache clear) on every create/update.** Clearing global caches on each CLI invocation violates the “incremental maintenance” and “1s response” contract.
- [ ] **Pre-warm / scheduler:** Heavy or one-time population (e.g. object ID cache, list cache, validation cache) is triggered by scheduler pre-warm or explicit user command, not by normal create/list/check.

### 3a. Spec origin plane (spec-derived caches and indexes)

When adding or changing **spec-derived** behavior (walking `docs/architecture/_internal/object_specs`, building kind lists, field snapshots, dependency graphs, or any cache that should stay consistent with loaded specs):

- [ ] Read **`docs/architecture/SPEC_ORIGIN_PLANE.md`** — one **spec plane** (revision + invalidation); **detail** layer vs **thin** derived indexes; avoid orphan disk walks without a documented contract.
- [ ] **Kind enumeration:** If you need the spec-known kind set as `map[string]struct{}` and a **`SpecIndex`** is already loaded (or you are working from materialized **`spec_index.json`**), prefer **`objects.KindNamesFromSpecIndex`** over walking **`object_specs`** with **`kindnames.LoadKindNamesFromSpecsDir`**, unless you intentionally need a custom specs directory or a stateless one-shot.
- [ ] **Spec index file:** After changing object spec YAML under **`docs/architecture/_internal/object_specs`**, either run **`zqk system generate-spec-index` (PRUNED)** or apply changes through **`zqk system update-specs`** (which refreshes **`spec_index.json`** after writes). Do not leave **`spec_index.json`** stale relative to edited specs when those edits flow through **`update-specs`**.
- [ ] **Pipeline outcome keys:** After editing **`docs/architecture/_internal/pipeline_outcome_keys.yaml`**, run **`zqk system generate-pipeline-outcome-keys` (PRUNED)** and commit **`pkg/pipeline/outcome_keys.go`**. If you change flags, help, or examples, edit **`.zqk/cli/specs/system/generate_pipeline_outcome_keys_command.yaml`** and run **`zqk system generate-command-builders --overwrite`** (see **`cli-command-spec-codegen.mdc`**).
- [ ] **Streams / data cells:** New stream summaries, segments, or cell membranes must not duplicate **spec** truth (ontology, fields). Resolve behavior through **`SpecLoader`**, validators, and the field registry. See **`docs/architecture/SPEC_ORIGIN_PLANE.md`** (streams and **Compatibility with data cells and streams**) and **`docs/architecture/STREAM_STORAGE.md`**.
- [ ] Prefer hooking new materialized views to the same invalidation path as **`SpecLoader`** (`ClearCache` / `InvalidateSpec` / `InvalidateSpecByFile`) or document why a stateless one-shot is intentional (e.g. codegen, drift CLI).
- [ ] **Tracked backlog (P1, current plan):** `zqk object get BLI-EXAMPLE` — unified spec revision + derived cache alignment.

### 3b. Glossary–spec alignment (glossary as derived index)

When adding or renaming **object specs**, **lifecycles**, or **canonical configs** under **`docs/architecture/_internal/`** that introduce a **new persisted kind**, **lifecycle**, or **operator-facing config surface**:

- [ ] Read **`docs/architecture/SPEC_ORIGIN_PLANE.md`** — section **Glossary as a derived index (target)** — run **`zqk system sync-glossary-from-specs --dry-run`** after spec/lifecycle/config edits to detect missing terms. If terms are intentionally added in this change, use **`--apply --dry-run=false`** or explicit **`zqk object create glossary_term`** with **`scripts/templates/glossary_*.yaml`** patterns (process-data-cli-only).
- [ ] Link new vocabulary to **[CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md](./CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md)** when the term concerns CLI output routing, nested convergence, or cross-component **membrane** behavior.
- [ ] Do **not** treat glossary YAML under **`docs/architecture/glossary_terms/`** as hand-editable instance data; use the **CLI** for creates/updates.
- [ ] **Navigation vocabulary graph (GAPE / strategic pillars / cross-walk):** If you add or remove **`vocabulary_scheme`**, **`glossary_term_relation`**, or navigation roots wired in **`VOCABULARY_GRAPH.md`**, run **`sh ./scripts/verify-vocabulary-gtr-counts.sh`** and update the documented **expected counts** there if totals change. Inspect schemes with **`sh ./scripts/show-seeded-vocabulary-schemes.sh`**.

---

## 4. Field storage roles: structural vs runtime delta

- [ ] **When adding or changing spec fields** (especially in shared bases like `auditable`), explicitly decide each field’s storage role:
  - `structural`: part of the canonical object definition (affects identity/invariants); changes are stored via normal CAS/YAML updates.
  - `runtime_delta`: high-churn or ephemeral fields (statuses, timestamps, counters, runtime metrics) that may be stored as deltas (change journal / stream) instead of full CAS rewrites.
- [ ] **For kinds listed in `docs/architecture/_internal/configs/runtime_delta_kinds.yaml`**, ensure write paths:
  - Route **only runtime_delta fields** through the delta mechanism (change journal / stream) when they change.
  - **Avoid CAS/YAML rewrites** when no structural fields changed.
- [ ] **Before merging spec changes**, confirm new or modified fields either:
  - Have an explicit `storage_role` (structural vs runtime_delta), or
  - Intentionally rely on the default structural role, with a brief rationale (e.g. field participates in identity or validation invariants).

---

## 5. Instance builders and generated code (instance-builders-and-metrics)

- [ ] **Do not edit** `pkg/specbuilder/bldr_instance_v1/*_instance_builder.go` by hand; they are generated. To change a builder, update the object spec and run `zqk system generate-instance-builders --overwrite`.
- [ ] **Creating spec-backed objects in code:** Use the **instance builder** for that kind (e.g. `NewSchedulerHealthMetricInstanceBuilder`), set lifecycle-valid status, call `Build()`, and handle errors. Do not build a literal `map[string]any` for objects that have a generated builder.
- [ ] **Config builder codegen (`zqk system generate-config-builders`):** Canonical YAML is under **`docs/architecture/_internal/configs/`**. The repo may also have `*_config.yaml` under **`docs/architecture/_internal/`** root with **different** content. The generator reads **one** flat directory (no recursion). **`make build-all`** runs `generate-config-builders --overwrite` with **no** `--configs-dir`; the CLI default **must** remain `.../configs` or `pkg/specbuilder/bldr_config_v1` will regenerate from the wrong inputs. See **`cmd/zqk/system/generate_config_builders.go`** (`NewGenerateConfigBuildersCmd` doc comment) before changing defaults or flags.

---

## 6. Tests (tests-background-output-to-file)

- [ ] **Scope (default narrow):** Prefer **`zqk scheduler go test`** with **`--package`**, **`--test` / `--tests`**, or **saved / load-bundle** coverage for packages touched by the change. Use **`go test --all`** (or other broad scans) when the affected surface is unknown, spans many packages, or tracking what to re-run is harder than a full baseline—see **`.cursor/rules/tests-background-output-to-file.mdc`** (“Scope: default to what changed”).
- [ ] **Convergence / cleanup cadence:** During a session, quick **`go test`** on changed packages (with **`-timeout`**, per `tests-go-test-timeout`) is fine for immediate feedback; schedule **targeted** bundles at **iteration boundaries** and triage via **`zqk scheduler test-failures`** / test-bundle health logs—avoid re-running the full matrix after every small edit unless sprawl demands it.
- [ ] **Convergence refactor — one package to completion:** Finish **all** intended changes in **one** package (multiple tranches in that package are OK); **rebuild** `zqk` if the scheduler invokes it; **restart** the scheduler daemon when you need a clean reload (`zqk scheduler stop` / `start`, see `scripts/scheduler/README.md`); then **`zqk scheduler go test --package ./that/pkg`**. **Stay in the package** until bundle logs show green—remedy failures, re-run, verify—**then** move to the next package. Prefer **not** editing that package again until the broader refactor is done (localizes failures; **no need to re-test a package you stop touching**). This is a **package gate** (verification scope + no unexplained failures), **not** “every CVS criterion satisfied on every `.go` file”—see **Terminology** and **Rationale** in **`.cursor/rules/tests-background-output-to-file.mdc`** (“Convergence refactor: one package to completion”).
- [ ] **Long-running or full test suites** (e.g. `go test ./cmd/zqk/system/...`, `go test ./...`, or runs that may take > ~1 minute): use **`go test ./...`** or **`zqk scheduler go test`** so tests run in the background. **Do not** run them in the foreground and wait in the terminal.
- [ ] **Output to file:** Test runs must write results to a log file (e.g. `--log-file .zqk/logs/tests/...`, scheduler bundle logs under `.zqk/logs/scheduler/cvs/test-bundles/`) so failures can be inspected. Do not rely only on exit code.
- [ ] **Convergence automation helpers:** Before promoting or closing a loop, optionally run **`scripts/check_convergence_promotion_readiness.sh`** (bundles + **`rollup_status_core`** + **`test-failures`**; tune with **`PROMOTION_REQUIRE_*`** / **`PROMOTION_REQUIRE_CLEAN_TEST_FAILURES=0`** when you need measure+rollup only). Optional daily overseer audit trail: **`scripts/record_convergence_overseer_run.sh`** / timer **`SCH-convergence-overseer-record`** → **`.zqk/logs/scheduler/cvs/overseer_runs.jsonl`**. Defaults and rhythm: **`docs/architecture/EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md`** (Operating rhythm). **Env tables and index:** [scripts/README.md — Convergence promotion and overseer helpers](../scripts/README.md#convergence-promotion-and-overseer-helpers). **Which runner (measure vs orchestrate vs tick vs shells):** [CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md — Entrypoints](./CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md#entrypoints-shell-cli-jobs).
- [ ] **New `scheduler_job` objects:** Prefer **parameterizing** an existing job (**`environment_variables`**, **`zqk object update`**) or a **shared script** under **`scripts/`** over adding another persisted job that differs only by **`CVS-*`** id or path; register net-new maintenance jobs in **`docs/architecture/_internal/configs/scheduler_maintenance_config.yaml`** when they belong in **`ensure-retention-jobs`**. Discipline summary: **`EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md`** (Scheduler automation discipline). **Future (intentionally deferred):** declarative **per-job validation** profiles — same doc (**Declarative validators (concept)** + implementation deferral there); **`scheduler_maintenance_config.yaml`** header comment.
- [ ] **Mechanical refactors across many `*.go` files:** Prefer **`scripts/go-safe-replace.sh`** (dry-run first). For test helpers around **`wrapCLIContextForTest`**, see **`scripts/refactor-wrap-cli-context-tests.py`** (default **`cmd/zqk/system`**; optional directory args) in **`scripts/README.md`**. See **`scripts/README.md`** for literal vs regex, **prefix-collision** pitfalls, and the **CLI context quick reference** (when to use `internal/cli` vs `internal/cli/context`; **`pkg/storage` must not import `internal/cli`**). For CLI context test/handler patterns, see **`docs/architecture/CLI_CONTEXT_TEST_PATTERNS.md`**.
- [ ] **Async readiness (no “hope sleep”):** Do not use a fixed **`time.Sleep`** as the main way to wait for storage/CAS/write-behind visibility. Prefer a **bounded loop** with an explicit condition (e.g. `storage.Read` succeeds, file present) and a **short sleep only as backoff between attempts** (tens of ms). Fixed long sleeps are flaky under scheduler bundles and load; see `waitUntilStorageReadable` / `createWithRetryUntilSuccess` in `cmd/zqk/system/check_impl_cache_refresh_test.go`. When **`Read` succeeds but reference validation or object-ID cache build still fails**, use bounded **retry on `Create`** or **repeat `BuildCache` + `Get`** with flush between attempts—not a single long sleep.

---

## 7. Observe–Hypothesize–Test–Verify (observe-hypothesize-test-verify)

- [ ] **Observe:** Have I checked current state (logs, status, metrics) and relevant policies/patterns for this area?
- [ ] **Hypothesize:** Does this approach comply with project policies? In particular: **no direct edits to process/object YAML** under `docs/architecture/`; use the CLI.
- [ ] **Test:** Is there a clear success/fail criterion and a way to capture output for analysis?
- [ ] **Verify:** After testing, confirm success criteria and note any side effects or follow-ups.
- [ ] **Convergence work:** If this turn is part of a **convergence lifecycle** (failures, autofix, reliability), verify progress against the **session contract** — `convergence_session` (`CVS-*`) **`hypothesis`**, **`predictions`**, and **`desired_end_state`** (or the same commitments from the start of the session if not yet persisted). See `.cursor/rules/convergence-session-agent-discipline.mdc`. **Do not** treat **bundle-only** green / **`ready_for_session_completion`** as full contract completion — `docs/architecture/CONVERGENCE_PREDICATES_AND_GATES.md`. For **rollup / orchestrate / multi-session** behavior and **doc parity** across entrypoints: [CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md](./CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md#entrypoints-shell-cli-jobs) (**Entrypoints**) + `docs/architecture/CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md` (**§ Parity and convergence target**). Optional **promotion / overseer** shells (§6): [scripts/README.md — Convergence promotion and overseer helpers](../scripts/README.md#convergence-promotion-and-overseer-helpers).

---

## 8. CLI output, logging, and agent guidelines (agent-guidelines-and-patterns, POL-CODE-007)

- [ ] **Before adding or changing** user-facing output, progress, or logging: consult `docs/architecture/README.md` and existing patterns in the codebase (e.g. `GetLoggerFromProfile`, `cli.WriteOutput`). For multi-field structured logs, prefer pooled fluent builders per **`docs/best-practices/coding/FLUENT_LOGGING.md`** (`logging.Fluent`, `scheduler.SLog`, `storage.StorageLog`, `logging.FluentEvent`). Do not use `fmt.Print*` or raw `os.Stdout`/`os.Stderr` for user-facing output; use logger from context or `cli.WriteOutput(cmd, data)`.
- [ ] **Command output writer:** Do not duplicate `outCtx := cmd.Context(); if outCtx == nil { … }; logging.GetCommandOutputWriter(outCtx)` across commands. Use **`cli.CommandContextOr(cmd, fallback)`** or **`cli.CommandOutputWriter(cmd, fallback)`** (see AGENT_GUIDELINES “Mistake 2a” and `internal/cli/helpers.go`).
- [ ] **Lint mapping:** Production code must not use `fmt.Print` / `Printf` / `Println` to implicit stdout — enforced by **`forbidigo`** in `.golangci.yml` (tests exempt). See **`docs/architecture/LINT_RULE_INVENTORY.md`** for how policies map to golangci vs scripts vs process-only rules.
- [ ] **Signals:** For Ctrl+C / SIGTERM–style shutdown, prefer **`signal.NotifyContext`** over ad-hoc **`signal.Notify` + channel + `context.WithCancel` + goroutine** unless you need signal-specific behavior **`NotifyContext` cannot express** (document why). See **`docs/architecture/README.md`**.
- [ ] **`*cli.Context` wrapper methods:** Code that delegates to the embedded **`*context.Context`** must go through **`withInnerContext`** in **`internal/cli/context.go`** (single nil guard). Do not duplicate **`(c == nil || c.Context == nil)`** on each method. **`scripts/check-cli-inner-guard.sh`** enforces exactly one occurrence. See **`docs/architecture/WRAPPER_INNER_CONTEXT_GUARD_PATTERN.md`** — glossary **`GLS-EXAMPLE`**.

---

## 9. Definition of “works”

- [ ] **Logic is correct** and **coding best practices** are followed.
- [ ] **Architecture policies** reflected in this checklist and in `docs/architecture/CLI_PERFORMANCE_AND_CONSISTENCY.md` are considered and followed.
- [ ] **Functional and non-functional** requirements are met. If non-functional requirements (e.g. latency, throughput) are not clear, **prompt the human** to establish them before marking the work complete.

---

## 10. Long-running or terminal commands (POL-WORKFLOW-004, CLI performance)

- [ ] **Long-running commands** (e.g. full test suite, system check with validation, large retention/aggregation): run via **scheduler** or **background job** with output to file; do not block the CLI for minutes. If the CLI must trigger long work, it should return quickly and use a callback or job ID for results.
- [ ] When suggesting or running terminal commands that may take > ~1 minute or that are run-wrapper candidates, prefer `go test ./...`, `zqk scheduler go test`, or the documented run-wrapper pattern.

---

## 11. Documentation and ambiguity

- [ ] **New or updated behavior** that affects CLI, caches, or policies: ensure it is reflected in the appropriate doc under `docs/` (architecture, process, or policies) and that this checklist is still accurate. If the checklist is missing a concern, add it here (one place) rather than creating a new doc that duplicates it.
- [ ] **If anything is unclear** (policy, requirement, or how to satisfy an item), **prompt the human** to establish it. Do not guess.
- [ ] **Process data location (future direction):** Long-term work aims to move **canonical process/instance state** toward **`.zqk/`** (with optional git snapshots) so routine CLI updates do not churn `docs/architecture/`; commits become **opt-in** when a team chooses to publish. Until then, **`docs/architecture/`** stays canonical and **CLI-only** for instance updates. See **`docs/architecture/FILESYSTEM_DATA_LAYOUT.md`** (CAS, runtime delta, scheduler alignment).

---

## 12. CLI async and progress (never hang without feedback)

- [ ] **CLI commands** that can block or run long use the async pattern (e.g. `BindAsyncProgress`) so they run with a **timeout context** and a **progress callback** on context.
- [ ] **Before any new blocking call** (lock, semaphore, I/O wait) on a CLI path: emit a progress/status message so the user sees context (e.g. “Waiting for list/count slot…”). Use `pkgctx.GetValidationProgress(ctx)` when available; see `docs/architecture/CLI_ASYNC_AND_PROGRESS.md` and existing use of `EmitListCountWaitProgress`.

---

## 13. Constants, DRY, and lean code (no magic strings or values)

- [ ] **Prefer constants over magic strings/numbers:** Operation names, logger profiles, versions, file suffixes, and configuration-like literals belong in a **constants file** or **externalized config** (e.g. `pkg/paths/constants.go`, `pkg/validation/constants.go`, `pkg/context` for profiles). This makes updates and discovery one-place and avoids imprecise find-and-replace.
- [ ] **Don't Repeat Yourself (DRY):** Reuse common patterns—functional-style builders, shared error handling, callbacks, emitters—instead of duplicating if/else chains or ad-hoc logic. Prefer small, well-named helpers and shared abstractions so the codebase stays lean and maintainable.
- [ ] **Purpose and organization:** Every literal has a clear purpose; related constants are grouped and documented. Prefer tidy, well-constructed code over one-off strings or values scattered across files.
- [ ] **Object field-name keys in Go:** For map keys, **`SetField("…")`**, and **`["…"]`** reads that denote **persisted object field names**, use **`objects.FieldKey*`** from **`pkg/objects`**. Where **`pkg/objects` must not be imported** (import cycle), use **package-local string constants** with the **same wire values** (e.g. **`pkg/logging/object_field_keys.go`**). Full-repo gate: **`sh ./scripts/check-field-key-literals-repo.sh`**. Pre-commit diff check: **`scripts/check-field-key-literals.sh`**. See **`.cursor/rules/field-key-literal-scan.mdc`** and **`scripts/README.md`** (`check-field-key-literals-repo.sh` / `check-field-key-literals.sh`).
- [ ] **Persisted / operator-visible timestamps (Go):** use **`pkg/zqktime`** (`NowRFC3339UTC`, `FormatRFC3339UTC`, `FormatLayoutUTC`, layout constants) instead of ad hoc `time.Now().Format(time.RFC3339)`. See **`docs/architecture/README.md`** (Timestamps UTC).
- [ ] **Periodic drift sweeps (optional):** When doing DRY / `maps.Copy` / style consistency work, use the copy-paste **`git grep` recipes** in **`docs/architecture/GIT_DRIFT_SEARCH_PATTERNS.md`** to spot regressions or remaining hotspots (complements `golangci-lint` and `zqk system analyze-drift-hotspots` (PRUNED)). **Pattern 9** there (`._*` / AppleDouble sidecars): prefer **`pkg/appledouble`** over ad hoc **`HasPrefix(..., "._")`** when skipping macOS metadata files in YAML/spec walks—run a pre-release sweep to migrate stragglers.
- [ ] **Drift analyzer changes:** If you edit **`pkg/drifthotspots`** (or drift-related CLI wiring), rebuild **`bin/zqk`** before trusting **`zqk system analyze-drift-hotspots` (PRUNED)** from PATH—otherwise you may still be running a stale analyzer (see **§15** for normal `make bin/zqk` / `make build-all` practice).
- [ ] **Go `for` loops vs iterator / seq APIs (Go 1.23+):** If you only walk a collection once, prefer **`for x := range …Seq` / `slices.All` / `maps.Keys` / `maps.Values`** (and **`strings.SplitSeq`**, **`strings.Lines`**, **`bytes.SplitSeq`**) over **`strings.Split`** / **`bytes.Split`** / hand-rolled buffers that materialize a full slice first. Same for **`slices.Clip`**-style patterns when trimming capacity matters. This cuts allocations and keeps loops succinct when the stdlib already exposes a lazy iterator.
- [ ] **Post-verify pass (after logic is correct):** Re-read the change for **DRYability**, **literals**, **readability**, and **anti-patterns** (**`docs/architecture/README.md`**). **Goal: semantic density** — domain intent is obvious; scaffolding is not duplicated. **Rule of two:** extract on the second copy when the abstraction is clear; **by the third** copy, extract or table-drive. Not “minimize LOC.” See **`docs/best-practices/coding/DRY_PATTERN_EXTRACTION.md`**.
- [ ] **Emerging patterns:** Same structure in multiple places → **one helper + named constants**, comment → **CLI spec YAML** or doc when usage must stay aligned. **Do not** chase a “% unique code” metric. See **`docs/best-practices/coding/DRY_PATTERN_EXTRACTION.md`**.

---

## 14. No exponential feedback loops (LESSONS_LEARNED Lesson 11)

- [ ] **Create/update/delete triggers:** If this change adds or changes logic that runs **on object create/update/delete** (e.g. change notification, handler, callback) and that logic **creates or updates objects**, or triggers jobs that do: confirm it **cannot** form a feedback loop. A loop exists if the created/updated object (or its kind) can trigger the same path again (e.g. creating a scheduler_job on every change, and scheduler_job is an object kind → each new job triggers another create). Prefer **trigger-with-args** (one reusable job, event data) over **create-per-event**.
- [ ] **Audit and metrics:** If the path writes audit events or metrics in response to object changes, ensure that processing those events does not create more objects (or events) that re-enter the same path and double each cycle.
- [ ] **Early warning:** "On create of K, create X" where X is or leads to K (or to a kind that triggers the same handler) is a recursion risk. Break the loop (exclude kind, single reusable job, or bounded queue).

---

## 15. Use built binary for CLI commands (no go run for operational use)

- [ ] **When running zqk CLI commands** (e.g. `zqk system retention-tolerance` (PRUNED), `zqk system aggregate-audit` (PRUNED), `zqk system retention-status` (PRUNED), scheduler start/stop): use the **built binary**, not `go run ./cmd/zqk`. Build with `make clean && make build-all` (or `make build-all` when a full clean isn’t needed), then invoke `./bin/zqk system <command>` (or `./bin/zqk-stable` for the scheduler/daemon). This matches the project’s build system, avoids repeated compile cost, and ensures the same binary is used for long-running and operational commands.

---

## 16. Fast path and cache fallback (FAST_PATH_AND_CACHE_FALLBACK_CHECKLIST)

- [ ] **When adding or changing a cache-based fast path** (e.g. List/Count that uses high-volume event cache, or a partial index): consult **`docs/architecture/FAST_PATH_AND_CACHE_FALLBACK_CHECKLIST.md`**.
- [ ] **Fallback when empty:** If the fast path returns 0 results, does execution **fall through** to the full path (e.g. full CAS list, index + filter) so the result reflects actual storage? Do not return empty solely because the cache returned 0.
- [ ] **Same contract for all variants:** For the same logical operation (e.g. audit_event list with created_at filter), do all variants (time-window, older-than, etc.) use the same fallback rule? If one falls through when cache returns 0, the others should too.

---

## 17. Filesystem data layout (populated directories)

- [ ] **Top-level budget:** Directories the system fills with generated data should have **≤100 top-level entries** (files or subdirs). If usage can exceed that, use **bucketing** or segmentation so listings stay manageable. See **`docs/architecture/FILESYSTEM_DATA_LAYOUT.md`**.
- [ ] **`scheduler_job`:** Not stream-backed; storage is **CAS** (`docs/architecture/scheduler_jobs/`) plus **`runtime_delta_current`** for eligible hot-field updates. Do not treat **`stream_current/scheduler_job/`** as live storage (legacy/stale if present).

---

## 18. CAS index repair commands (operators)

- [ ] **Avoid blanket repair:** Do **not** run **`zqk system fix-hash-mismatches --kind (PRUNED) <kind>`** without explicit object IDs unless you have a written runbook. **`--strategy force-reindex`** currently **removes stale index rows** (same path as remove-stale-index)—it is **not** a safe “reindex everything” for whole kinds like **`convergence_session`**.
- [ ] **`recover-cas` and moved blobs:** After updates, objects can move to a **new** hash-named file. If **`recover-cas`** runs when the index still points at an **old** hash, it may **remove the id from the index** even though a **new** hash file exists—**`object list` then omits** the object until the index is reconciled. Prefer **`zqk system check`** / disparity-driven refresh; use **`recover-cas`** only for confirmed missing blobs or per ops docs.
- [ ] **Scheduler + CLI:** Repair commands contend on the same CAS **index lock files** as the **scheduler**. Expect **timeouts or apparent stalls** if both hammer the same kind; **restart the scheduler** after a bad repair episode once indexes are consistent.

See **`docs/architecture/CAS_LIST_GET_CONSISTENCY.md`** (end of file): *Operational hazards: `recover-cas`, `fix-hash-mismatches`, and the scheduler*.

---

## 19. Tracked debt and follow-ups (no “slop without a receipt”)

Workarounds and **temporary** behavior are acceptable under pressure; **silent** workarounds are not.

- [ ] **If this change introduces** a compatibility shim, dual code path, narrow regression fix you expect to replace, “until we migrate” logic, or a fallback that duplicates truth elsewhere: you must attach a **tracking artifact** in the **same change** — not a promise in chat.
- [ ] **Artifact (pick one):** (1) **`TRACK:` comment** at the nearest logical anchor: `// TRACK: <url-or-existing-backlog-id> — remove when: <objective condition>` (same pattern in YAML `# TRACK:` if comments are allowed there), **and/or** (2) PR / commit description linking the **same** issue or **`BLI-*` / internal id**.
- [ ] **Do not** use placeholder ids. If no ticket exists, **open or ask the human for** one before treating the work as complete.
- [ ] **Before marking the task done:** state what tracks follow-up work, or state **why** the design is permanent (no deferred cleanup).
- [ ] **Periodic review:** run **`./scripts/list-track-contracts.sh`** (or `git grep -n 'TRACK:'`) to audit open contracts.
- [ ] **IDE agent rules:** new or renamed rule files under the configured agent rules directory (default **`.cursor/rules/*.mdc`**) must update **`docs/architecture/_internal/configs/agent_rules_manifest.yaml`**. From repo root this is normally two commands with no other flags: **`zqk system validate-agent-rules --write-manifest` (PRUNED)** then **`zqk system validate-agent-rules` (PRUNED)**. Use **`--rules-dir`** or **`ZQK_AGENT_RULES_DIR`** only when rules live outside `.cursor/rules`. Pre-commit fails if the manifest drifts.

---

## Glossary (semantic alignment)

Terms used in this checklist—**process data**, **hot path**, **instance builder**, **object spec**, **lifecycle**, **pre-change checklist**, **object kind**, **new system object**—are defined as **glossary_term** objects so everyone stays aligned on intent. Definitions are updated as the project evolves.

- **List terms:** `zqk object list glossary_term`
- **Get one:** `zqk object get <GLS-id>`

See also: [AGENT_GUIDELINES.md](../process/enforcement/AGENT_GUIDELINES.md) § Glossary.

---

## References (source of checklist items)

- **IDE agent rules:** `.cursor/rules/` default; registry **`agent_rules_manifest.yaml`**; **`zqk system validate-agent-rules` (PRUNED)** (process-data-cli-only, tests-background-output-to-file, instance-builders-and-metrics, filesystem-data-layout, observe-hypothesize-test-verify, agent-guidelines-and-patterns, **follow-up-tracked-debt**).
- **Architecture:** `docs/architecture/CLI_PERFORMANCE_AND_CONSISTENCY.md`, `docs/architecture/CLI_ASYNC_AND_PROGRESS.md`, `docs/architecture/PROJECT_ROOT_ORIENTATION.md`, `docs/architecture/PROJECT_ROOT_USE_AND_SCHEDULER_ALIGNMENT.md`, `docs/architecture/FILESYSTEM_DATA_LAYOUT.md`, `docs/architecture/CACHE_MANAGEMENT_STRATEGY.md`, `docs/architecture/system-check-cache-first-and-async.md`, **Fast path fallback:** `docs/architecture/FAST_PATH_AND_CACHE_FALLBACK_CHECKLIST.md`, **Spec plane / derived indexes:** `docs/architecture/SPEC_ORIGIN_PLANE.md`, **Periodic `git grep` drift recipes:** `docs/architecture/GIT_DRIFT_SEARCH_PATTERNS.md`, **Event pipelines / convergence integration choices:** `docs/architecture/EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md`, **Convergence contracts (bundle vs rollup vs multi-session):** `docs/architecture/CONVERGENCE_PREDICATES_AND_GATES.md`, `docs/architecture/CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md` (**§ Parity and convergence target**), `docs/architecture/CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md` (**Appendix F**). **Object field-key literals (gate + fixer):** `scripts/check-field-key-literals-repo.sh`, `go run ./scripts/fix_field_key_literals`, `.cursor/rules/field-key-literal-scan.mdc`. **CAS list/get consistency and repair hazards:** `docs/architecture/CAS_LIST_GET_CONSISTENCY.md`. **Feedback loops:** `docs/architecture/LESSONS_LEARNED.md` Lesson 11.
- **Scheduler degraded mode:** `docs/architecture/SCHEDULER_DEGRADED_MODE_GUARDRAILS.md` (required/optional command behavior when daemon is down).
- **Policies:** `docs/architecture/README.md`.
- **Enforcement / agent:** `docs/architecture/README.md`, `docs/architecture/README.md`.
- **DRY / pattern extraction (§13 narrative):** `docs/best-practices/coding/DRY_PATTERN_EXTRACTION.md`.
