## Scenario Bundles and End-to-End Traceability

**Context:** The rationale for the CLI split and the decision to start with scenario bundles (including the persistence-bundle as the first objective) is in [CMDV2_AND_CLI_SPLIT_RATIONALE.md](./CMDV2_AND_CLI_SPLIT_RATIONALE.md).

**Spec origin (system model):** Traceability objects (`requirement`, `criteria`, `backlog_item`, `test_case`, `goal`, …) are **instances** of kinds defined under `docs/architecture/_internal/object_specs`. Bundle apply ordering comes from the **spec index** (see `pkg/scenario/apply.go`). How spec loading, derived indexes, caches, and invalidation fit together — and how that relates to streams/data-cell direction — is summarized in [SPEC_ORIGIN_PLANE.md](./SPEC_ORIGIN_PLANE.md).

This document defines the **accepted pattern** for:

- Modeling functional requirements and acceptance criteria,
- Tracking implementation work,
- Linking automated tests to criteria, and
- Seeding projects with consistent data via **scenario bundles** (including test-scenarios and traceability bundles).

It is the reference for any new feature, persistence mechanism, or test harness.

---

### 1. Requirement → Criteria → Backlog → Test Case → Code

Every “must-have” behavior (functional requirement, persistence invariant, policy) must be represented as:

- **Requirement** (`requirement` kind): describes *what must be true*.
- **Criteria** (`criteria` kind): atomic, testable acceptance conditions for that requirement.
- **Backlog item** (`backlog_item` kind): per-domain work item (per kind / feature / subsystem) that implements specific criteria.
- **Test case** (`test_case` kind): maps criteria to concrete automated tests (e.g. Go test functions).
- **Code harness**: shared test helpers (e.g. persistence harness) that actually exercise the system and assert the criteria.

For example, persistence/lifecycle verification might have:

- `REQ-PERSISTENCE-001` – *Object Lifecycle Persistence Verification*.
- `CRIT-PERSIST-ORIGIN-001` – Create writes CAS + audit_event.
- `CRIT-PERSIST-UPDATE-001` – Updates reflected in CAS + audit_event.
- `CRIT-PERSIST-RUNTIME-001` – Runtime deltas recorded in change journal.
- `CRIT-PERSIST-DELETE-001` – Deletes recorded in deleted-stream + audit_event + change journal.
- `CRIT-PERSIST-AGG-001` – Aggregations reflect underlying CRUD history.

And a per-kind backlog/test-case pair:

- `BLI-PERSIST-scheduler_job` – backlog for scheduler_job persistence.
- `TC-PERSIST-scheduler_job` – test case object pointing at:
  - `criteria_refs: [CRIT-PERSIST-*]`
  - `test_functions: ["pkg/storage.TestSchedulerJob_CRUDAuditInvariants"]`

---

### 2. Scenario Bundles (Generic Engine)

We standardize on a **generic scenario/bundle schema** that can be used for:

- **Traceability bundles** (requirements + criteria + backlog + test_case + fixtures),
- **Execution scenarios** (what today lives under `test-scenarios/`),
- Any future “seed this project with X” flows.

Conceptual YAML shape:

```yaml
api_version: zqk/v1
kind: scenario_bundle

metadata:
  name: "scheduler_persistence"
  description: "End-to-end persistence and traceability for scheduler_job."

objects:
  requirements:
    - id_hint: REQ-PERSISTENCE
      title: Object Lifecycle Persistence Verification
      body: |
        ...

  criteria:
    - id_hint: CRIT-PERSIST-ORIGIN
      requirement_ref: REQ-PERSISTENCE
      title: Creation writes CAS and audit_event
      description: |
        ...

  backlog_items:
    - id_hint: BLI-PERSIST-scheduler_job
      title: Persistence lifecycle for scheduler_job
      requirement_refs: [REQ-PERSISTENCE]
      criteria_refs:
        - CRIT-PERSIST-ORIGIN
        - CRIT-PERSIST-UPDATE
        - CRIT-PERSIST-RUNTIME
        - CRIT-PERSIST-DELETE

  test_cases:
    - id_hint: TC-PERSIST-scheduler_job
      title: Scheduler job persistence lifecycle harness
      kind_under_test: scheduler_job
      requirement_refs: [REQ-PERSISTENCE]
      criteria_refs:
        - CRIT-PERSIST-ORIGIN
        - CRIT-PERSIST-UPDATE
        - CRIT-PERSIST-RUNTIME
        - CRIT-PERSIST-DELETE
      test_functions:
        - pkg/storage.TestSchedulerJob_CRUDAuditInvariants

  fixtures:
    scheduler_jobs:
      - id_hint: SCH-PERSIST-TEST
        template:
          job_type: cache_prewarm
          trigger_type: timer
          schedule_expression: "*/10 * * * *"
          category: maintenance
          execution_mode: reusable
          enabled: true

steps:
  # Optional: used by executable scenarios (test-scenarios); not required for pure bundles.
  - id: "step-1"
    description: "Run cache_prewarm and retention once."
    commands:
      - "zqk scheduler trigger SCH-007"
      - "zqk scheduler status"
```

Key points:

- `objects` is **shared** across traceability bundles and test-scenarios.
- `steps` is only used by executable scenarios; bundles that only seed objects can omit it.
- `id_hint` allows human-friendly IDs while letting the loader assign final IDs and rewrite refs.

#### Repository traceability bundles (examples)

| Bundle | YAML path | Primary IDs |
|--------|-----------|---------------|
| **object-stream-steward-bundle** | `test-scenarios/object-stream-steward-bundle/object-stream-steward-bundle.yaml` | `REQ-STREAM-001`, `CRIT-STREAM-001`–`008`, `TEST-STREAM-001`–`002` (runtime-delta + maintenance/stream compaction harnesses), `BLI-STREAM-001` |
| persistence-bundle | `test-scenarios/persistence-bundle/persistence-bundle.yaml` | See bundle / `.zqk/scenarios/persistence-traceability-bundle/scenario-summary.json` |
| **process-hygiene-config-traceability-bundle** | `pkg/scenario/testdata/process-hygiene-config-traceability-bundle/process-hygiene-config-traceability-bundle.yaml` | `GOAL-PHC-001`, `REQ-PHC-001`, `CRIT-PHC-001`–`005`, `TEST-PHC-001`, `BLI-PHC-001`–`002`, `DOC-PHC-001`–`002` (persisted hygiene rules + `object-hygiene-scan` traceability) |
| **agent-orchestration-traceability-bundle** | `test-scenarios/agent-orchestration-traceability-bundle/agent-orchestration-traceability-bundle.yaml` | `GOAL-AO-001`, `REQ-AO-001`, `CRIT-AO-001`–`006`, `BLI-AO-001`–`002`, `DOC-AO-001`–`003` (multi-agent prompt delivery, audit, convergence, policy/knowledge self-assessment) |
| **cap-loop-honesty-traceability-bundle** | `pkg/scenario/testdata/cap-loop-honesty-traceability-bundle/cap-loop-honesty-traceability-bundle.yaml` | `GOAL-CAPH-001`, `REQ-CAPH-001`–`004`, `CRIT-CAPH-001`–`012`, `TEST-CAPH-001`–`003`, `BLI-CAPH-001`–`004`, `DOC-CAPH-001`–`003` (CAP CVS-bound advances, nest veneer, fail-closed review, agent wake contract) |

**Object stream stewardship** (synonym: *object stream maintainer*): declarative policies, single orchestrator, bounded workers for compaction / retention / aggregation / overlay GC — see `REQ-STREAM-001` body and glossary **GLS-EXAMPLE** (`zqk object get GLS-EXAMPLE`). Template: `scripts/templates/glossary_object_stream_stewardship.yaml`.

Apply (from repo root; `test-scenarios/` is gitignored for *new* paths until force-added—use `git add -f` for new bundle files):

```bash
go build -o bin/zqk-scenario ./cmdv2/zqk-scenario/
./bin/zqk-scenario bundle apply -f test-scenarios/object-stream-steward-bundle/object-stream-steward-bundle.yaml --project-root .
```

`test_case` IDs must use the `TEST-` prefix per `id_prefixes_config` (not `TC-`).

#### Agent orchestration, prompt delivery, and organizational memory (reconciliation)

Earlier product notes described agent chat, convergence prompts, and knowledge storage in overlapping terms. The **persistent bundle** `agent-orchestration-traceability-bundle` reconciles that with established traceability:

| Topic | Where it lives |
|-------|----------------|
| **Priority audible** (elevate orchestration; organism / society metaphor → streams & cells) | [AGENT_ORCHESTRATION_AUDIBLE.md](./AGENT_ORCHESTRATION_AUDIBLE.md) |
| Strategic ↔ tactical chain (mission → goal → milestone → REQ → CRIT → BLI → TEST) | [requirements-traceability-system-v1.0.md](../process/architecture/requirements-traceability-system-v1.0.md) — unchanged |
| **REQ-AO-001** | Multi-agent orchestration: pluggable **delivery** (file, clipboard, future HTTP/MCP/cloud), **audit** artifacts, **CLI-only** process edits, **convergence** as quality measure, **policy/knowledge** cross-reference for self-assessment, **documented** script handoffs |
| Prompt **content** vs **transport** | Content: `zqk scheduler convergence measure --format agent-prompt`. Transport: configurable (see criteria **CRIT-AO-001**–**CRIT-AO-002**); Cursor is one consumer |
| Organizational memory | Same object store (requirements, policies, glossary, lessons learned); semantic/graph workstreams stay authoritative—no parallel schema |
| Apply bundle to CAS | `zqk-scenario bundle apply -f test-scenarios/agent-orchestration-traceability-bundle/agent-orchestration-traceability-bundle.yaml --project-root .` (then assign `priority_plan_ref` / milestones via `zqk object update` as needed) |

---

### 3. Scenario/Bundle Loader Responsibilities

We will centralize bundle/scenario application in a `pkg/scenario` package with an API like:

```go
type ApplyMode int

const (
	ApplyObjectsOnly ApplyMode = iota
	ApplyObjectsAndSteps
)

func ApplyScenarioBundle(ctx context.Context, projectRoot string, r io.Reader, mode ApplyMode) (*BundleSummary, error)
```

The loader is responsible for:

1. **Parsing** the bundle (YAML/JSON) into typed structs.
2. **Resolving IDs**:
   - For each object with `id_hint`, assign a valid ID per kind rules.
   - Rewrite internal refs (`requirement_ref`, `criteria_refs`, `test_case_refs`, `kind_under_test`, fixture links) from hints to actual IDs.
3. **Creating objects**:
   - Use the same object APIs the CLI uses (storage/object layer), not direct YAML writes under `docs/architecture/` (preserves CAS, validation, audit trails).
4. **Executing steps** (when `mode == ApplyObjectsAndSteps`):
   - Run listed commands / triggers in order, using the same environment as `test-scenarios`.
5. **Emitting a summary**:
   - Write a `scenario-summary.json` into the project (e.g. under `.zqk/scenarios/<name>/`) with:
     - `hint → id` mappings,
     - requirement → criteria → backlog_item → test_case → test_functions links.

Existing `test-scenarios` flows will be refactored to call `ApplyScenarioBundle` instead of bespoke loaders.

---

### 4. Persistence Verification Harness (Example Consumer)

The persistence harness uses the scenario bundle to drive tests:

- Apply the **persistence traceability bundle** for the test project.
- For each kind `K`, a Go test like `Test<K>_PersistenceLifecycle` calls a shared helper:

```go
type PersistenceProfile struct {
	Kind            string
	ID              string
	BuildObject     func(id string) map[string]any
	HasDeletedStream  bool
	HasChangeJournal  bool
	HasKindStream     bool
	UsesRuntimeDeltas bool
	HasAggregations   bool
	RunAggregations   func(t *testing.T, env *testconfig.TestEnvironment, id string)
	ExtraAssertions   func(t *testing.T, env *testconfig.TestEnvironment, id string)
}

func VerifyPersistenceLifecycle(t *testing.T, profile PersistenceProfile)
```

The helper:

- Uses `SetupCompleteTestEnvironment` for an isolated project root.
- Performs `Create → Read → structural Update → runtime_delta Update → Archive/Delete → Aggregation`.
- Asserts invariants for:
  - CAS / primary file(s) for the object,
  - `.zqk/streams/<kind>/...` (if applicable),
  - `.zqk/state/stream_deleted_<kind>.jsonl` (for delete),
  - `.zqk/streams/change_journal_entry/...` (for deltas),
  - `.zqk/streams/audit_event/...` (creation/update/deletion),
  - Metric/aggregation objects and streams.

Test case objects (`test_case`) then point to these Go tests so requirements and criteria are traceable to concrete code.

---

### 5. CLI Commands and Scenario Tools

Two user-facing flows wrap the scenario engine:

- **Core CLI (planned)**  
  - `zqk system scenario apply --file <bundle>`  
    Apply a bundle in **objects-only** mode (e.g. traceability bundles, fixtures).

  - `zqk system scenario run --file <bundle>`  
    Apply a bundle and then execute its `steps` (test-scenarios and other executable flows).

- **Scenario/bundle CLI (implemented today)**  
  - `zqk-scenario bundle apply --file <bundle> --project-root <root>`  
    Uses `ApplyScenarioBundle` with `ApplyObjectsOnly` to create goals, requirements, criteria, backlog_items, test_cases, and fixtures (e.g. scheduler_jobs), then flushes CAS indexes so ref lookups see new objects. Writes `.zqk/scenarios/<bundle-name>/scenario-summary.json` with `hint_to_id` and created IDs.
  - `zqk-scenario bundle run --file <bundle> --project-root <root>`  
    Uses `ApplyScenarioBundle` with `ApplyObjectsAndSteps` to apply the same objects/fixtures **and** execute bundle `steps` in order. Each step’s `commands` run with `projectRoot` as CWD and `ZQK_TEST_ROOT` set, and their stdout/stderr are streamed to the CLI. This is the preferred tool for executable test-scenarios today.

Both paths:

- Use the same loader (`ApplyScenarioBundle`),
- Respect the current project root / `ZQK_TEST_ROOT`,
- Do not write directly under `docs/architecture/` except via object APIs.

This pattern (spec + scenario engine + CLI wrapper) is the **standard** for:

- Seeding projects with requirements/criteria/backlog/test_case/fixtures,
- Creating and running rich test-scenarios (via `zqk-scenario bundle run` and steps),
- Enforcing end-to-end, auditable lifecycles in tests.

When adding new traceability or test flows, prefer:

- Describing objects and steps in a **scenario bundle** under `test-scenarios/`,
- Applying them via `zqk-scenario` (for experiments and test-scenarios) or `zqk system scenario *` (when promoted to core CLI),
- Consuming `.zqk/scenarios/<bundle-name>/scenario-summary.json` in tests and harnesses instead of hand-wiring IDs.

---

### 6. Bundle apply and main CLI visibility

**Same storage, same project root.** `zqk-scenario bundle apply` writes objects through the same storage layer as the main `zqk` CLI: it uses `storage.NewFileObjectStorage(projectRoot)`, which writes under `<projectRoot>/docs/architecture/` (CAS and kind directories). The main CLI uses the same `docs/process` tree, resolving project root via `ResolveProjectRoot(".")` (env vars, `.zqk/current_root`, or walking up to find `.zqk`).

**Why applied objects might not appear in main zqk:**

1. **Different project root**  
   `zqk-scenario` does *not* fall back to `.zqk/current_root` or walking up from CWD. It only uses `--project-root`, `ZQK_PROJECT_ROOT`, or `ZQK_TEST_ROOT`. If you run `zqk-scenario bundle apply -f bundle.yaml -R .` from a **subdirectory** (e.g. `test-scenarios/persistence-bundle`), then `projectRoot` is that subdirectory. Objects are written to `<subdir>/docs/architecture/`, which is not where the main `zqk` (run from repo root) looks.

2. **Fix:** Use the **same** project root for both. From the **repository root** run:
   - `zqk-scenario bundle apply -f test-scenarios/persistence-bundle/persistence-bundle.yaml -R .`
   or set `ZQK_PROJECT_ROOT` (or `ZQK_TEST_ROOT`) to the repo root and omit `-R`. Then the main `zqk` (run from repo root or with the same env) will see the same `docs/process` tree and the applied objects.

3. **CAS index:** After apply, the scenario CLI flushes CAS indexes for that project root. A **new** invocation of `zqk` will load the updated indexes and see the new objects. An already-running process (e.g. MCP server) that cached storage or indexes may need a restart to see them.

