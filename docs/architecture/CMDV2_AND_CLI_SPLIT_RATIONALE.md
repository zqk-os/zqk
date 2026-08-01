# CMDv2 and CLI Split: Rationale and Plan

**Purpose:** Document why the CLI was split into multiple binaries (cmdv2) and why scenario bundles are the first objective. This provides context for the parallel-build strategy and the persistence/traceability work.

**Related:** [SCENARIO_BUNDLES_AND_TRACEABILITY.md](./SCENARIO_BUNDLES_AND_TRACEABILITY.md), [SPEC_ORIGIN_PLANE.md](./SPEC_ORIGIN_PLANE.md) (specs as canonical model, derived indexes, bundle ordering), [CLI_ARCHITECTURE.md](../process/architecture/CLI_ARCHITECTURE.md), `cmdv2/` (zqk-v2, zqkdev, zqk-scenario, zqk-svc).

---

## 1. Trigger: Data Loss and Untraceable Removal

A **critical scheduler job** (cache prewarm) **disappeared** from the system and was removed with **no way to trace what happened**. That incident drove the decision to:

- Treat the **persistence layer** as a first-class concern: it must be **transactionally and lineage-secure** so that object lifecycle (create/update/delete) and scheduler job state are auditable and recoverable.
- Avoid a single sprawling CLI where behavior is hard to reason about and failures are hard to isolate.

---

## 2. Goals That Shaped the Refactor

| Goal | Intent |
|------|--------|
| **Transactional and lineage-secure persistence** | Ensure we can trace and recover object and job state; no silent disappearance of critical data. |
| **Divided CLI** | The main CLI was sprawling and confusing. Split responsibilities so each surface has a clear purpose and is easier to maintain and reason about. |
| **Deep traceability** | Extend scenario builders so we have **requirements → criteria → backlog items → test cases** (and links to implementation/testing). |
| **Stronger e2e** | We had a lightweight e2e path for CRUD on any object; we need a **stronger, more reliable** path to validate behavior end-to-end. |
| **Better information architecture** | Improve taxonomy and organization so the system is more **intuitive and discoverable**. |

---

## 3. Strategy: Parallel Build (No Big-Bang Breakage)

Rather than refactor the existing CLI in place and risk losing functionality or introducing major breakage, we chose to **build the new structure in parallel** to the previous one:

- **cmdv2/** holds the new binaries: **zqk-v2** (core runtime), **zqkdev** (developer/tooling), **zqk-scenario** (scenarios and traceability bundles), **zqk-svc** (long-lived services).
- The **existing** `cmd/zqk` (single `zqk` binary) remains the production CLI until the new surfaces are ready and we migrate.
- This lets us **arrive at the desired end state without risking major breakage** during the transition.

---

## 4. First Objective: Scenario Bundle for Full Traceability

The **first objective** was to establish a **scenario bundle** that can be used to create:

- **Requirement** objects
- **Criteria** objects (linked to requirements)
- **Backlog items** (linked to requirements/criteria)
- **Test cases** (linked to requirements/criteria and to test functions)

so that we have **full traceability** from requirements through implementation and testing.

We **opted to start with scenario bundles first** (before moving large parts of the main CLI into zqk-v2/zqkdev). Work **left off** with the **test-scenarios/persistence-bundle** direction: a bundle that seeds the persistence verification story (requirements, criteria, backlog items, test cases, and fixtures such as scheduler_job) so that:

- The same bundle format drives both **traceability** (object graph) and **execution** (test-scenarios, steps).
- The persistence layer and scheduler job lifecycle can be verified end-to-end with a well-defined, auditable object graph.

See [SCENARIO_BUNDLES_AND_TRACEABILITY.md](./SCENARIO_BUNDLES_AND_TRACEABILITY.md) for the accepted pattern (requirement → criteria → backlog → test case → code harness) and the generic bundle schema.

---

## 5. Current Implementation State (Summary)

- **Bundle schema** (`pkg/scenario/bundle.go`): Defines `Bundle` with `objects.goals`, `objects.requirements`, `objects.criteria`, `objects.backlog_items`, `objects.test_cases`, and `objects.fixtures` (including `scheduler_jobs`). Templates use `id_hint` and refs (e.g. `goal_refs`, `requirement_ref`, `criteria_refs`) for traceability.
- **Apply engine** (`pkg/scenario/apply.go`): `ApplyScenarioBundle` applies scheduler_job fixtures, then traceability objects in **create order derived from the spec index** (goal, criteria, requirement, test_case, backlog_item; cycles broken with deferred ref updates). Goals, requirements, criteria, backlog_items, and test_cases are created via instance builders and storage; scenario summary is written to `.zqk/scenarios/<bundle-name>/scenario-summary.json`.
- **CLI entrypoint**: `zqk-scenario bundle apply --file <bundle> --project-root <root>` applies a bundle in objects-only mode and prints created counts and the summary path.
- **Persistence-bundle**: **test-scenarios/persistence-bundle/persistence-bundle.yaml** defines the persistence verification graph (goal, requirement, criteria, backlog_items, test_cases) and is wired into the test suite via `TestApplyScenarioBundle_PersistenceBundleLoad`; see test-scenarios/persistence-bundle/README.md for apply instructions.

---

## 6. Next Steps (When Resuming)

1. **Apply persistence-bundle from CLI:** After `ApplyScenarioBundle`, the CLI calls `storage.FlushAllCASIndexesForProjectRootWithTimeout(projectRoot, 15s)` so List/Read by ref see new objects. Apply against a project root where path-cache and specs are available.
2. **Persistence harness and scenario-summary (done):** `scenario.LoadScenarioSummary(projectRoot, bundleName)` reads `.zqk/scenarios/<name>/scenario-summary.json`. `TestSchedulerJob_CRUDAuditInvariants_FromBundle` (pkg/storage, external test) creates a job, writes a summary, loads it, and asserts each `CreatedSchedulerJobIDs` entry is readable—validating that tests can be driven by the summary. Full delete+deleted-stream invariant remains in `TestSchedulerJob_CRUDAuditInvariants`; use scenario-summary to resolve test_case_refs/criteria_refs for reporting when needed (SCENARIO_BUNDLES_AND_TRACEABILITY.md §4).
3. **Extend scenario builders** for deeper traceability and for executable steps (ApplyObjectsAndSteps) as needed for test-scenarios.

---

## 7. Data cell operator discovery (`zqk system data-cells` (PRUNED) vs cmdv2)

Operator-facing **cell discovery** (kinds, `storage_profile`, profile contracts, primary paths, operational envelope, optional `--json` / `--json-envelope` blocks including test-bundle health summary and stream stewardship drift) ships on the main CLI as **`zqk system data-cells` (PRUNED)** — see [DATA_CELL_MODEL.md](./DATA_CELL_MODEL.md) and backlog **`BLI-EXAMPLE`**. Output is structured via **`cli.WriteOutput`** (AGENT_GUIDELINES / POL-CODE-007). **`cmdv2/`** binaries do **not** replace this surface yet; avoid duplicating the same discovery story there without an explicit migration note and spec alignment.

---

## 8. References

- **Scenario bundle pattern and schema:** [SCENARIO_BUNDLES_AND_TRACEABILITY.md](./SCENARIO_BUNDLES_AND_TRACEABILITY.md)
- **CLI architecture and patterns:** [CLI_ARCHITECTURE.md](../process/architecture/CLI_ARCHITECTURE.md)
- **cmdv2 binaries:** `cmdv2/zqk`, `cmdv2/zqkdev`, `cmdv2/zqk-scenario`, `cmdv2/zqk-svc`
- **Scenario package:** `pkg/scenario/` (bundle.go, apply.go)
