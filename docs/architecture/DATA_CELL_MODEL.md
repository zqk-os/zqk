> [!WARNING]
> **ARCHIVED DOCUMENT**: The primary commands referenced in this architectural document have been pruned from the `zqk` CLI.

# Data cell model (logical unit + storage profiles)

**Last Verified:** 2026-08-31


**Status:** Architecture — **intent and vocabulary** (product direction). **Spec requirement + construction pipeline** are **decided** in [ADR-DATA-CELL-SPEC-PIPELINE-v1.0.md](../process/decisions/ADR-DATA-CELL-SPEC-PIPELINE-v1.0.md). **Reality check:** partial **membrane** read paths and **operational envelope** wiring exist in code (see table below); remaining gaps are **full** stewardship/coordinator execution, a **unified cell CRUD** product API, **migration** tooling, and **admin** surfaces — not “nothing implemented yet.”  
**Audience:** Implementers and operators aligning storage, retention, and tooling around **cells** rather than ad hoc per-kind scripts.

## Formalization status (honest)

| Layer | State |
|-------|--------|
| **Intent** — cell = logical unit + operational envelope; profiles = light file / CAS / stream; multi-kind cells for low volume | **Captured** (this doc + cross-links). |
| **Spec + pipeline** — every cell has a spec; spec declares storage profile; CLI pipeline (new base spec, add field, hints, field vetting, per-field deploy gate, glossary) | **Accepted** — [ADR-DATA-CELL-SPEC-PIPELINE-v1.0.md](../process/decisions/ADR-DATA-CELL-SPEC-PIPELINE-v1.0.md). |
| **Identity** — stable cell id, naming, relationship to `kind` / `ontology` / spec plane | **Partial** — v1: `CellKindDescriptor` + `datacellregistry.LoadDataCellDescriptors` / CLI `zqk system data-cells` (PRUNED) (spec_index + `storage_profile`); multi-kind / alternate ids still roadmap. |
| **Profile contracts** — required behaviors per profile (write path, read path, integrity, migration) | **Implemented baseline** — `storage_profile` wire + validation (`pkg/datacell/profile.go`), explicit contract map in `pkg/datacell/profile_contracts.go`, and CLI discovery via `zqk system data-cells` (PRUNED) JSON fields (`write_mode`, `read_mode`, `integrity_mode`, `migration_mode`). **Deploy:** materialized `spec_index.json` is built with `objects.BuildSpecIndexFromSpecsDirStrict` (`zqk system generate-spec-index` (PRUNED), `RefreshMaterializedSpecIndex`) so invalid specs (including bad `storage_profile`) fail instead of being skipped from the snapshot; stream/CAS alignment is still checked via `ValidateHighVolumeStreamKindsMatchSpecIndex` against `high_volume_kinds.yaml`. Backend-specific deep behavior still lives in per-area docs ([STREAM_STORAGE.md](./STREAM_STORAGE.md), CAS, etc.). |
| **Operational envelope** — which subsystems implement cleanup / retention / scan / cache / archive for a “cell”; scheduler hooks; observability | **Implemented v1** (child BLI **`[REDACTED-ID]`** complete) — static map per storage profile in [`pkg/datacell/operational_envelope.go`](../../pkg/datacell/operational_envelope.go) and operator discovery via `zqk system data-cells` (PRUNED) (`operational_envelope` in JSON + table footer, including reserved `scheduler_category` `data_cell_envelope`). Each `operational_envelope` object also carries **`envelope_tick_scheduler_job_id`** (`SCH-dce-tick`) and **`envelope_tick_job_type`** (`data_cell_envelope_tick`) so JSON is self-describing without reading this table. JSON rows use **`OperationalEnvelopeForKind`** (v1 per-kind **augments** on top of profile defaults for any envelope category — `cleanup`, `retention`, `scan`, `cache`, `archive`, `health` — e.g. `audit_event`+`stream` adds `high_volume_event_cache` under `health`; `audit_aggregation_metric`+`stream` adds cleanup/retention/cache tokens; `base_metric` / `scheduler_health_metric`+`stream` add metric/scheduler-health tokens; other **stream** kinds from the spec index such as metric siblings, `mcp_session` / `zqk_session`, and `verification_matrix` have v1 tokens in [`operational_envelope.go`](../../pkg/datacell/operational_envelope.go)); the table footer stays **deduped by profile** for readability. **`zqk system data-cells --json` (PRUNED)** rows also include **`operational_envelope_kind_augmented`** (true when a non-empty per-kind augment exists for that kind+`storage_profile`) and **`operational_envelope_kind_override`** (true when [`kindOperationalEnvelopeOverrides`](../../pkg/datacell/operational_envelope.go) non-empty for that pair — remove/replace profile defaults before augments). Policy-engine dry-run: [`pkg/scheduler/datacell_envelope.go`](../../pkg/scheduler/datacell_envelope.go) (`DryRunDataCellEnvelopePolicy`). **Persisted** v1 timer job: `job_type` `data_cell_envelope_tick`, id `SCH-dce-tick`, template [`scripts/scheduler_jobs/data_cell_envelope_tick_six_hourly.yaml`](../../scripts/scheduler_jobs/data_cell_envelope_tick_six_hourly.yaml), listed in [`scheduler_maintenance_config.yaml`](../process/_internal/configs/scheduler_maintenance_config.yaml) (ensured by `zqk system ensure-retention-jobs` / daemon startup); handler logs stream-profile envelope summary ([`pkg/scheduler/handlers_datacell_envelope_tick.go`](../../pkg/scheduler/handlers_datacell_envelope_tick.go)). Metrics JSONL (`.zqk/metrics/data_cell_envelope_tick.jsonl` when metrics recording is on) includes the same canonical job id and job type keys. **Roadmap:** richer per-token RPC bindings; expanded per-kind override rows beyond the v1 pilot (`kindOperationalEnvelopeOverrides`), and coordinator execution beyond the v1 tick — **in scope for alpha completion** under the umbrella ([§ Program completion](#program-completion-definition-of-done-finite-scope)), not deferred as “post–v1 BLI”; the closed v1 BLI row is the baseline only. Runtime organism slice still **paths + aliases only** for `.zqk/` config. |
| **Multi-kind cells** — rules for when twins/triplets are allowed; isolation vs shared retention; failure domains | **Named**, not **specified**. |
| **API surface** — membrane, coordinator, discovery | **Partial** — **Membrane read-path v1 shipped** (child BLI **`[REDACTED-ID]`** complete): [`MembraneReadPaths`](../../pkg/datacell/membrane_read_path.go) (`RuntimeOrganismMembraneReadPaths`, `StreamMembraneReadPaths`, `CASEntityMembraneReadPaths`) + pilot routing at `zqk system data-cells` (PRUNED), `cli-hooks` / `path-cache`, stream overlay layout, organism chat paths — see [Membrane and coordinator — what exists today](#membrane-and-coordinator--what-exists-today-v1). **Strict nucleus path boundary (CRIT-DATACELL-001):** external packages must use [`CellCASPrimaryDir`](../../pkg/datacell/cell_boundary_paths.go) / [`CellStreamOverlayKindDir`](../../pkg/datacell/cell_boundary_paths.go) (or `MembraneReadPaths` methods), not raw [`CASEntityPrimaryDir`](../../pkg/datacell/paths.go) / [`StreamCurrentKindDir`](../../pkg/datacell/paths.go); enforced by [`scripts/check-crit-datacell-001-boundary.sh`](../../scripts/check-crit-datacell-001-boundary.sh) — [CRIT_DATACELL_001_STRICT_BOUNDARY.md](./CRIT_DATACELL_001_STRICT_BOUNDARY.md). [`CellMembrane`](../../pkg/datacell/membrane.go), [`CellCoordinator`](../../pkg/datacell/membrane.go), [`CellReadModel`](../../pkg/datacell/membrane.go) remain the boundary types. **Steward enqueue (v1):** [`EnqueueStewardMaintenance`](../../pkg/datacell/enqueue_maintenance.go) / [`StewardMaintenanceCoordinator`](../../pkg/datacell/enqueue_maintenance.go) and pkg/scheduler `StewardEnqueueCoordinator` persist maintenance ops to `steward_enqueue.jsonl`; hooks include cache invalidation (stream), post-retention stream stewardship (`compact_segments`), and CAS-entity retention tolerance (`retention_sweep` when non-stream kinds archive/delete). **Not** wired on every routine object write; **not** a single exported cross-profile object CRUD facade yet. |
| **Query / list contract** (traits → filter, sort, group, paginate, count) | **Implemented** today via spec loader + list pipeline; **named** as part of the cell model below. |

### Membrane and coordinator — what exists today (v1)

This is the **honest** slice for backlog item **`[REDACTED-ID]`** (**complete** at v1 read-path + pilots): contracts and **read** routing landed first; steward **enqueue** JSONL + selected production hooks exist; deeper per-write coverage and envelope-token execution remain **roadmap**.

| Piece | Implemented? | Where / notes |
|-------|----------------|---------------|
| **Interfaces** | **Yes** | [`pkg/datacell/membrane.go`](../../pkg/datacell/membrane.go) — `CellMembrane`, `CellCoordinator`, `CellReadModel`, `ReadModelIsStale`, `MaintenanceOp` names, `MinimalMembrane`, `NoopCellCoordinator`. |
| **Read-path adapter** | **Yes** | [`pkg/datacell/membrane_read_path.go`](../../pkg/datacell/membrane_read_path.go) — `MembraneReadPaths` resolves **light_file** runtime paths (flags, tray, hook profile, manifest, agent chat paths), **`StreamCurrentKindDir`**, **`CASEntityPrimaryDir`**, behind profile-specific constructors. |
| **Pilot call sites** | **Partial** | `zqk system data-cells` (PRUNED) resolves `primary_path` via membrane helpers; `cli-hooks` / `path-cache --show-paths` use membrane for **display** paths; [`pkg/storage/stream_current_state.go`](../../pkg/storage/stream_current_state.go) builds stream overlay paths with [`datacell.StreamCurrentKindDir`](../../pkg/datacell/membrane_read_path.go) so layout matches **data-cells**. |
| **Coordinator / stewardship** | **Partial — steward JSONL wired** | [`EnqueueStewardMaintenance`](../../pkg/datacell/enqueue_maintenance.go) + envelope tick drain ([`handlers_datacell_envelope_tick.go`](../../pkg/scheduler/handlers_datacell_envelope_tick.go)); stream/CAS/light-file hooks (incl. materialize agent chat channel → `refresh_summary`). [`CellHandle`](../../pkg/datacell/cell_handle.go) groups profile + coordinator + [`MembraneReadPathsForProfile`](../../pkg/datacell/cell_handle.go). Envelope tokens → scheduler `job_type` bindings: [`envelope_token_jobs.go`](../../pkg/datacell/envelope_token_jobs.go) + tick log/metrics field `operational_envelope_resolved_scheduler_job_types`. **Envelope tick follow-up dispatch:** [`envelope_tick_dispatch.go`](../../pkg/scheduler/envelope_tick_dispatch.go) — `ENVELOPE_TICK_DISPATCH_MODE` on SCH-dce-tick (`off` \| `shadow` \| `on`); **core** allowlist (`lifecycle_check`, `integrity_check`, `cache_prewarm`, `context_refresh`); **`ENVELOPE_TICK_DISPATCH_EXPAND`** adds `object_validation` and `metrics_collection`; **`ENVELOPE_TICK_DISPATCH_ALLOWLIST_EXTRA`** adds comma-separated `job_type`s (still subject to hard deny); **`ENVELOPE_TICK_DISPATCH_DENY_EXTRA`** blocks types on this job (operator tightening; wins over extra/core allowlists). Metrics: `envelope_tick_dispatch_allowlist_extra_n`, `envelope_tick_dispatch_deny_extra_n`, `envelope_tick_dispatch_skip_denied_env`; token policy: `envelope_tick_dispatch_token_*`. **Discovery-token JSON** (`ENVELOPE_TICK_DISPATCH_DENY_TOKENS_JSON`, `ENVELOPE_TICK_DISPATCH_ALLOW_TOKENS_JSON`): JSON string arrays on `SCH-dce-tick` trim which envelope tokens contribute to resolved `job_type`s **before** type-level allow/deny (invalid JSON → warning, **no token filter**). **Execution cap:** `ENVELOPE_TICK_DISPATCH_MAX_TRIGGERS` caps shadow + real follow-ups per tick after target resolution. The dispatch pass also **dedupes** duplicate `job_type` entries in the resolved list (defensive; normal resolution is already unique), and caps **one follow-up per concrete `scheduler_job` id** per tick when multiple resolved types would map to the same target. **`ENVELOPE_TICK_DISPATCH_SKIP_IF_TARGET_BUSY`** (default **on**) skips when the target job is already **running** in-process; **`ENVELOPE_TICK_DISPATCH_CHECK_TRIGGER_QUEUE`** (default off) optionally consults the on-disk on-disk trigger queue for a pending request whose `trigger_origin` is `data_cell_envelope_tick`. **Roadmap:** per-kind replace override table, unified cross-profile maintenance registration. |
| **Tests** | **Yes** | [`pkg/datacell/membrane_test.go`](../../pkg/datacell/membrane_test.go); stream organism E2E touches paths; saved bundle **`data-cell-stream-organism`** includes `TestMembraneReadPaths_*`. |

### SCH-dce-tick — follow-up dispatch rollout (suggested)

Use this sequence when turning on **`TriggerJob`** follow-ups from the envelope tick. Token JSON and `…_EXTRA` env vars apply to the **`SCH-dce-tick`** scheduler_job (`environment_variables`).

| Phase | `ENVELOPE_TICK_DISPATCH_MODE` | `ENVELOPE_TICK_DISPATCH_EXPAND` | Token / job-type tuning | Goal |
|-------|------------------------------|--------------------------------|---------------------------|------|
| **1 — observe** | `shadow` (or `off`) | `false` | none | Logs + metrics only; verify `operational_envelope_resolved_scheduler_job_types`. |
| **2 — shadow heavy** | `shadow` | `true` | optional deny token JSON or `DENY_EXTRA` | See shadow lines for expanded types (`object_validation`, `metrics_collection`). |
| **3 — narrow** | `shadow` | as needed | `DENY_TOKENS_JSON` / `ALLOW_TOKENS_JSON` (restrictive whitelist) | Drop unwanted discovery paths before enabling triggers. |
| **4 — live cheap** | `on` | `false` | keep deny lists | Allowlisted **core** job_types fire for real. |
| **5 — live expanded** | `on` | `true` | revisit **hard deny** in code if new types needed | Heavy allowlist tier on only after ops sign-off. |

References: [`envelope_tick_dispatch.go`](../../pkg/scheduler/envelope_tick_dispatch.go), [`envelope_tick_token_policy.go`](../../pkg/scheduler/envelope_tick_token_policy.go), [`handlers_datacell_envelope_tick.go`](../../pkg/scheduler/handlers_datacell_envelope_tick.go).

#### Alpha lane — three active data-cell BLIs (`[REDACTED-ID]`)

These **`in_progress`** items sharpen the umbrella program (**`[REDACTED-ID]`**) without duplicating the tables above:

| BLI | Focus | Where it lives today | Remaining scope (see each BLI `description`) |
|-----|-------|---------------------|-----------------------------------------------|
| **`[REDACTED-ID]`** | Dispatch **policy** beyond bare `MODE` / `EXPAND` | [`pkg/scheduler/envelope_tick_dispatch.go`](../../pkg/scheduler/envelope_tick_dispatch.go), [`envelope_tick_token_policy.go`](../../pkg/scheduler/envelope_tick_token_policy.go) — token allow/deny JSON, comma **`ENVELOPE_TICK_DISPATCH_DENY_EXTRA`** / **`ALLOWLIST_EXTRA`**, core vs expanded tiers, **`MAX_TRIGGERS`**, busy + optional trigger-queue skips | More **regression tests** and any extra **job_type–scoped** knobs still missing as env (normative edge behavior stays in code comments). |
| **`[REDACTED-ID]`** | **Per-kind** envelope replace/remove (not only additive augments) | [`pkg/datacell/operational_envelope.go`](../../pkg/datacell/operational_envelope.go) — `kindOperationalEnvelopeOverrides` + `OperationalEnvelopeForKind`; pilot override for **`audit_event`+stream** | Broaden **override rows** and contract tests beyond the pilot; JSON shape for **`zqk system data-cells` (PRUNED)** already exposes `operational_envelope_kind_override`. |
| **`[REDACTED-ID]`** | Execution **depth** from resolved types → real triggers | Same dispatch stack + [`datacell_envelope_tick_metrics.go`](../../pkg/scheduler/datacell_envelope_tick_metrics.go); acceptance **`CRIT-*`** on that BLI | Track **`[REDACTED-ID]`** (doc parity with this doc + normative comments); tighten metrics / idempotency as BLI **`acceptance_criteria`** still open. |

### v1 operational envelope — job contracts (implemented)

These rows satisfy the “documented job contracts (or hooks)” bar for v1: each names **where** the wiring lives (code or disk). Values under `cleanup` / `retention` / `scan` / `cache` / `archive` / `health` in [`operational_envelope.go`](../../pkg/datacell/operational_envelope.go) are **discovery tokens** — they group responsibilities for operators and cross-links; they are not guaranteed stable RPC names.

| Surface | Contract | Primary location |
|---------|----------|------------------|
| **Scheduler job** | Maintenance tick; category `data_cell_envelope`; `job_type` `data_cell_envelope_tick`; id `SCH-dce-tick` | [`scheduler_maintenance_config.yaml`](../process/_internal/configs/scheduler_maintenance_config.yaml), template [`data_cell_envelope_tick_six_hourly.yaml`](../../scripts/scheduler_jobs/data_cell_envelope_tick_six_hourly.yaml) |
| **Handler** | Executes tick; logs compact envelopes, kind augments, and kind override summaries; optional metrics line | [`pkg/scheduler/handlers_datacell_envelope_tick.go`](../../pkg/scheduler/handlers_datacell_envelope_tick.go), registered in [`pkg/scheduler/jobtype_registry.go`](../../pkg/scheduler/jobtype_registry.go) (`JobTypeDataCellEnvelopeTick`) |
| **Policy engine** | Synthetic envelope-category job evaluates to **execute** under default policy (invariant check) | [`pkg/scheduler/datacell_envelope.go`](../../pkg/scheduler/datacell_envelope.go); tests [`datacell_envelope_test.go`](../../pkg/scheduler/datacell_envelope_test.go), [`handler_factory_test.go`](../../pkg/scheduler/handler_factory_test.go) |
| **Operator JSON** | Per kind+profile envelope (+ `operational_envelope_kind_augmented`, `operational_envelope_kind_override`) | `zqk system data-cells --json` (PRUNED) — [`cmd/zqk/system/data_cells.go`](../../cmd/zqk/system/data_cells.go) |
| **Metrics JSONL** | One line per tick when metrics recording is enabled (`kind_operational_envelope_overrides`) | `.zqk/metrics/data_cell_envelope_tick.jsonl` — [`datacell_envelope_tick_metrics.go`](../../pkg/scheduler/datacell_envelope_tick_metrics.go) |

**Per-kind (v1):** [`OperationalEnvelopeForKind`](../../pkg/datacell/operational_envelope.go) applies optional removes/replaces from [`kindOperationalEnvelopeOverrides`](../../pkg/datacell/operational_envelope.go), then **additive** augments from [`kindOperationalEnvelopeAugments`](../../pkg/datacell/operational_envelope.go) on top of profile defaults.

Remaining **implementation** work: **deeper per-token** RPC-style bindings, broader **per-kind override** coverage than the pilot table, **scheduler** / storage **registration** beyond the v1 tick, and **migration** between profiles — with acceptance tests and backlog ownership. (Baseline deploy-time profile enforcement for **declared** `storage_profile` and materialized **spec_index** is in place; richer cross-field combination rules are still future work.)

**Isolated regression tests** (synthetic `spec_index.json` under a temp tree + `ZQK_TEST_ROOT`, no migration of real `docs/process` data): [DATA_CELL_RUNTIME_ORGANISM.md](./DATA_CELL_RUNTIME_ORGANISM.md) lists `go test` targets for `pkg/datacell` (`TestStreamOrganism*`) and `cmd/zqk/system` (`TestDataCells_StreamOrganism*`, in-process `zqk system data-cells` (PRUNED)).

## Definition

A **data cell** is:

1. A **logical grouping of data** — one coherent “lump” the system reasons about as a unit (often aligned with a **kind**, but not always — see *multi-kind cells* below).
2. Plus the **operational envelope** required to **maintain that grouping** in production: cleanup, scanning, caching, retention, archiving, bucketing, layout bounds, health signals, and any other **cell-environment** concerns.

Together, that is a **fully contained data-management package** for that logical unit: not only *where* bits live, but *how* the system keeps that region healthy over time.

The **stream** concept fits **inside** this picture as one **physical profile** (see below), not as a separate competing religion — streams are how some cells **move** high-volume data; CAS or light files are how others **store** it.

## Storage profiles (three or more)

The same **cell abstraction** can be realized with different **physical profiles** (and more may appear later):

| Profile | Typical use | Notes |
|---------|-------------|--------|
| **Lightweight file / JSONL-style** | Very low ceremony, operator-local or small JSON/YAML/JSONL surfaces | Example in-repo: feature-flag style artifacts under the **runtime organism** slice — see [DATA_CELL_RUNTIME_ORGANISM.md](./DATA_CELL_RUNTIME_ORGANISM.md). |
| **CAS-based entities** | Normal **spec-defined** instances: hash-addressed shards, lifecycle, validation | The usual `docs/process/` object story for entity kinds. |
| **Stream-backed** | Append-heavy, high-volume kinds | Segments, retention, stream stewardship — see [STREAM_STORAGE.md](./STREAM_STORAGE.md), [DATA_STREAM_SUMMARY_PILOT.md](./DATA_STREAM_SUMMARY_PILOT.md). |

**Wire values in `object_specs`:** optional top-level **`storage_profile`** — `cas_entity`, `light_file`, or `stream` ([`pkg/datacell`](../../pkg/datacell/profile.go)). Values **inherit** along `extends` when omitted; the default chain sets **`cas_entity`** on `auditable.yaml`, so typical process kinds are CAS-backed unless overridden. The materialized **`spec_index.json`** records **`storage_profile`** per kind for discoverability.

Choosing a profile is a **cell design** decision: volume, edit pattern, integrity, and operational cost — not “everything must be one shape.”

## Spec-driven plane: persistence, counting, and query surfaces (auto-wired)

A spec-backed cell **does not** bolt on persistence and reporting by hand. The **same spec loader and trait model** that exist today **auto-wire** instance behavior from field and kind metadata:

- **Persistence** — create, read, update, and delete paths for instances (storage implementation varies by profile: CAS shards, files, stream appenders, etc.).
- **Standard query surface** — listing with **filtering**, **sorting**, **grouping**, **pagination** (offset/limit), and **counts** (e.g. total / returned in result metadata), driven by **traits** on the spec such as `listable`, `filterable`, `sortable`, `groupable` where applicable. The CLI `list` command and storage layer honor those traits rather than reimplementing per kind (see `.zqk/cli/specs/object/list_command.yaml` and the `ListFilter` / `QueryResult` path).
- **Projection** — table and structured output use spec metadata (e.g. column defaults, display length) so operators get consistent **listing** and **slicing** of result sets without ad hoc scripts.

**Today:** this **already works** for spec-defined kinds through the loader + object commands. **Direction:** the **data cell / stream pipeline** **folds** that contract in explicitly — same behaviors, **unified** under the cell envelope and storage profiles, instead of inventing a parallel “cell API” that ignores traits.

## Multi-kind cells (twins, triplets, quadruplets, …)

For **extremely low-volume** kinds, the product may **group several kinds into one cell**: shared operational envelope, shared bucketing/retention policy, or shared scanning — **twins, triplets, quadruplets**, etc. The **logical** boundary is still “one cell”; the **physical** layout may multiplex multiple kinds when that reduces overhead without blurring integrity rules.

## Relationship to other docs

| Topic | Document |
|-------|----------|
| **ADR:** spec required; storage profile in spec; CLI pipeline + glossary | [ADR-DATA-CELL-SPEC-PIPELINE-v1.0.md](../process/decisions/ADR-DATA-CELL-SPEC-PIPELINE-v1.0.md) |
| Spec rules vs live data | [SPEC_ORIGIN_PLANE.md](./SPEC_ORIGIN_PLANE.md) |
| Pedagogy (spec / stream / cell language) | [SPEC_STREAM_CELL_PEDAGOGY.md](./SPEC_STREAM_CELL_PEDAGOGY.md) |
| **Implemented slice:** runtime organism paths + `pkg/datacell` | [DATA_CELL_RUNTIME_ORGANISM.md](./DATA_CELL_RUNTIME_ORGANISM.md) |
| Path aliases & relocation | [PATH_ALIAS_RESOLUTION.md](./PATH_ALIAS_RESOLUTION.md) |
| Vision (path cache, hardening) | [SPEC_RUNTIME_AND_PATH_CACHE_VISION.md](./SPEC_RUNTIME_AND_PATH_CACHE_VISION.md) |

## Implemented today vs roadmap

- **Today:** The repo ships a concrete **runtime organism** slice (operator-edited JSON/YAML under `.zqk/`, path helpers in **`pkg/datacell`**, aliases). **`CellMembrane` / `CellCoordinator` interfaces** and **`MembraneReadPaths`** (stream, CAS, light_file constructors) exist — **read** routing at selected CLI/storage boundaries, not a full nucleus rewrite. **Operational envelope** v1 (static map + scheduler tick + policy dry-run) is implemented as [above](#formalization-status-honest). That is **not** the exhaustive implementation of every envelope token as a live subsystem, nor a finished coordinator.
- **Roadmap:** Extend the same **cell** idea so that CAS entities, stream-backed kinds, and light files each declare their **profile** and share **cell-environment** tooling (cleanup, retention, scanning, …) **and** the **same spec-driven persistence + list/query semantics** (see *Spec-driven plane* above) under one model — without requiring a single physical format for all data. **Coordinator enqueue** wired to real jobs and **cross-profile migration** (**[runbook](./DATA_CELL_CROSS_PROFILE_MIGRATION_RUNBOOK.md)** + **`OperatorProfileMigrationChain`**) are **in scope for alpha completion** per the umbrella object—not “ship read paths now, finish operations later.” Automated physical relocators beyond the documented chain remain separate BLIs when product adds them.

**Tracking:** `[REDACTED-ID]` (complete) and the **`BLI-177608*`** program on **`[REDACTED-ID]`** (*CLI alpha launch readiness*). **Alpha does not ship on a partial data-cell slice:** the **full** umbrella program (cells as an **operational** surface—coordinator depth, migration path, bundles, envelope execution—per process objects below) must be **fully operational and evidenced** before alpha; refresh links as implementation lands.

## Program completion: definition of done (finite scope)

This program **can** finish because **done is defined in process objects**, not as “every theoretical cell feature.” Canonical umbrella: **`[REDACTED-ID]`** (`zqk object get [REDACTED-ID]`). **Acceptance criteria** (same object): satisfy **CRIT-DATACELL-001** and **CRIT-DATACELL-002**; bring each child workstream and linked follow-on BLI (envelope execution depth, coordinator pipeline, migration, bundles) to **complete** per recorded **`acceptance_criteria`**—**routine deferral of core operational scope is not an alpha exit**; only **explicit product-approved exceptions** belong on the umbrella or child BLIs. Keep this doc’s [formalization status](#formalization-status-honest) table honest (**implemented** vs **partial + follow-up**).

**Program gate before alpha:** discoverable **cell identity** (`zqk system data-cells` (PRUNED)), **profile contracts** in CLI/JSON, **operational envelope** (map + tick + policy dry-run **and** verifiable **token→job execution depth** per **`[REDACTED-ID]`**), **membrane** read paths **plus production coordinator / steward → job coverage** where the umbrella defines it, **migration** path (**`[REDACTED-ID]`** + **CRIT-DATACELL-002**), **admin/discovery** acceptance on **`zqk system data-cells` (PRUNED)**, and **automated proof** including **`data-cell-stream-organism`** **and** **`data-cell-stream-organism-pkg-storage`** bundles green under scan-tests. The **CMDv2 admin split binary** remains its own track ([CMDV2_AND_CLI_SPLIT_RATIONALE.md](./CMDV2_AND_CLI_SPLIT_RATIONALE.md)); **data-cell program completion** still requires everything the umbrella object lists—not a “read-path + tick only” subset.

**Child workstreams** (components on the umbrella object; status lives in each **`BLI-177608025*`** — refresh with `zqk object list backlog_item --filter priority_plan_ref=[REDACTED-ID]`):

| Child BLI | Focus | Exit signal (what “done” means for that row) |
|-----------|--------|-----------------------------------------------|
| `[REDACTED-ID]` | Registry / identity | **Complete:** descriptors from `spec_index` + tests. |
| `[REDACTED-ID]` | Profile contracts | **Complete:** contract map + `data-cells` fields + deploy gates (strict materialized spec index + high_volume alignment validation). Deeper cross-field “invalid combination” rules remain follow-up if product requires them. |
| `[REDACTED-ID]` | Operational envelope | **Complete for program exit:** v1 map + per-kind augments + `SCH-dce-tick` + policy dry-run + tests + operator JSON/logs/metrics **and** umbrella-mandated **envelope execution depth** (dispatch tiers, **`[REDACTED-ID]`**, related override BLIs)—not tick-only observability. |
| `[REDACTED-ID]` | Membrane adapters | **Complete for program exit:** read-path adapters + per-profile tests + pilot call sites **and** production **coordinator / steward → scheduler job** wiring per umbrella **`acceptance_criteria`** (not read-path-only). |
| `[REDACTED-ID]` | Admin / discovery | `zqk system data-cells` (PRUNED) meets its acceptance criteria (structured output, tests). |
| `[REDACTED-ID]` | Migration | **Complete:** cross-profile migration path (**[runbook](./DATA_CELL_CROSS_PROFILE_MIGRATION_RUNBOOK.md)**, **`OperatorProfileMigrationChain`**, tests) **and** **CRIT-DATACELL-002** evidence on the umbrella—required for program exit, not an optional defer. |
| `[REDACTED-ID]` | Test matrix / bundles | Saved **`data-cell-stream-organism`** + **`data-cell-stream-organism-pkg-storage`**, **`zqk scheduler scan-tests --load-bundles data-cell-stream-organism,data-cell-stream-organism-pkg-storage`** replay, **`health.jsonl`** / scheduler events evidence—all **green** for program exit (see `scripts/save-data-cell-stream-organism-bundle.sh`). |

**Follow-on backlog (`in_progress` on CLI alpha readiness — same priority plan):** envelope dispatch policy, per-kind overrides, coordinator execution depth, bundle stability (**`[REDACTED-ID]`**), and related BLIs **close with the umbrella** for alpha—they are **not** post-v1 polish after “child rows” ship.

| BLI | Scope |
|-----|--------|
| **`[REDACTED-ID]`** | Envelope tick **dispatch policy overrides** (per-token / per–job_type; tier rules beyond global `ENVELOPE_TICK_DISPATCH_*`). |
| **`[REDACTED-ID]`** | **Per-kind operational envelope override table** — v1 table + merge order in code; **exit** = pilot + tests + JSON flags (`kindOperationalEnvelopeOverrides`); breadth / execution-depth polish = follow-on with **`[REDACTED-ID]`**. |
| **`[REDACTED-ID]`** | **Envelope token → job execution depth** — verifiable **`acceptance_criteria`** live on the BLI (`zqk object get [REDACTED-ID]`); summarized in the subsection below. |
| **`[REDACTED-ID]`** | **Alpha gate:** targeted **bundle stability** (scheduler test bundles / `health.jsonl` evidence for data-cell organism and related gates). |

**SCH-dce-tick envelope dispatch knobs (`[REDACTED-ID]`):** Normative semantics: [`pkg/scheduler/envelope_tick_dispatch.go`](../../pkg/scheduler/envelope_tick_dispatch.go). Common environment variables on **`SCH-dce-tick`**:

| Variable | Role |
|----------|------|
| `ENVELOPE_TICK_DISPATCH_MODE` | `off` / `shadow` / `on` |
| `ENVELOPE_TICK_DISPATCH_EXPAND` | Enables expanded allowlist (`object_validation`, `metrics_collection`) |
| `ENVELOPE_TICK_DISPATCH_ALLOWLIST_EXTRA` | Comma-separated extra `job_type` values |
| `ENVELOPE_TICK_DISPATCH_DENY_EXTRA` | Comma-separated deny overrides |
| `ENVELOPE_TICK_DISPATCH_MAX_TRIGGERS` | Caps shadow + on follow-ups per tick |
| `ENVELOPE_TICK_DISPATCH_SKIP_IF_TARGET_BUSY` | Skip when target job id is already running (default **on**) |
| `ENVELOPE_TICK_DISPATCH_CHECK_TRIGGER_QUEUE` | Skip when a pending envelope-tick trigger exists for the target (default **off**) |
| `ENVELOPE_TICK_DISPATCH_DENY_TOKENS_JSON` | JSON array of operational-envelope **discovery token** strings excluded before `job_type` resolution ([`envelope_tick_token_policy.go`](../../pkg/scheduler/envelope_tick_token_policy.go)) |
| `ENVELOPE_TICK_DISPATCH_ALLOW_TOKENS_JSON` | When present, only listed tokens contribute to resolved types (restrictive whitelist); invalid JSON falls back to no token filter |

Discovery-token policy JSON is wired from [`handlers_datacell_envelope_tick.go`](../../pkg/scheduler/handlers_datacell_envelope_tick.go).

**Exit checklist (machine-checkable traceability):** On **`[REDACTED-ID]`**, **`acceptance_criteria`** references **`CRIT-*`** rows with **`validation_method: automated_test`** or **`manual_check`**. Each automated criterion is satisfied by executing the linked **`test_case`** **`path_or_id`** (see **`criteria_refs`** / **`zqk object related`** / **`zqk object get`** on **`[REDACTED-ID]`** and **`[REDACTED-ID]`**). **`TestDispatchEnvelopeTickResolvedJobs_onModeTriggersRegisteredJob`** ([`pkg/scheduler/envelope_tick_dispatch_integration_test.go`](../../pkg/scheduler/envelope_tick_dispatch_integration_test.go)) additionally proves **`ENVELOPE_TICK_DISPATCH_MODE=on`** reaches **`TriggerJob`** against a registered target job. Those TEST rows are also listed under **`[REDACTED-ID]`**. Green tests are necessary evidence; **`backlog_item`** completion still uses explicit lifecycle **`status`** updates (criteria → validated/complete, then **`zqk object update`** on the BLI) unless a CI policy is added that performs those writes automatically. Bundle JSON may carry **`criteria_refs`** / **`test_case_refs`**; scheduler jobs emit **`criteria_verification_evidence`** lines into **`.zqk/logs/scheduler/cvs/test-bundles/events.jsonl`** for downstream gate automation — see **`docs/process/system-health/SCHEDULER_EVENTS_AND_METRICS.md`**.

**Convergence session:** **`[REDACTED-ID]`** tracks the umbrella; **`desired_end_state`** on that object is the live contract. When the rows above are satisfied and criteria pass, the program **is allowed to feel done**—remaining work becomes **new** BLIs, not an endless same-numbered program.

### [REDACTED-ID] — Formal mapping and sequencing (spec closure)

This subsection closes the **in_progress** backlog item *Data cell / stream organisms: unify feature-flag JSON, tray config, and storage selection* at the **architecture / sequencing** level. The BLI explicitly **defers** implementing the full assembly pipeline, adapters, and admin CLI (see object `description` / non-goals via `zqk object get [REDACTED-ID]`); that follow-on scope is owned by the **`BLI-177608*`** program on the same priority plan ([CMDV2_AND_CLI_SPLIT_RATIONALE.md](./CMDV2_AND_CLI_SPLIT_RATIONALE.md)).

**Terminology alignment**

| Phrase (BLI / product) | Meaning in this repo |
|------------------------|----------------------|
| **Data cell** | Logical unit + operational envelope — [Definition](#definition) above. |
| **“Stream organism”** (informal) | High-volume or **stream-profile** persistence — still a **cell** with a **stream** physical profile, not a separate model — see [SPEC_STREAM_CELL_PEDAGOGY.md](./SPEC_STREAM_CELL_PEDAGOGY.md) and *Streams & summaries* there. |
| **Runtime organism** | The **implemented v1** slice: feature flags + CLI hook profile + tray — [DATA_CELL_RUNTIME_ORGANISM.md](./DATA_CELL_RUNTIME_ORGANISM.md). |

**Three storage patterns (BLI) → today’s mapping**

The BLI called out (1) feature-flag / single-file JSON, (2) stream + WAL, (3) CAS + WAL. Mapped to **concrete surfaces**:

| Pattern | Role | Where it shows up today |
|---------|------|-------------------------|
| **(1) Single-file / light file** | Operator-local toggles and shortcuts | **Feature flags** (`.zqk/config/feature_flags.json`), **CLI hook profile** (`.zqk/config/cli_hook_profile.json`), **Tray** (embedded default + `.zqk/tray.yaml`) — path table and `protocol_version` in [DATA_CELL_RUNTIME_ORGANISM.md](./DATA_CELL_RUNTIME_ORGANISM.md). Wire API: `pkg/datacell` path helpers. |
| **(2) Stream + WAL** | Append-heavy volume | Stream-backed kinds, segments, stewardship — [STREAM_STORAGE.md](./STREAM_STORAGE.md); profile **`stream`** in specs / `pkg/datacell`. |
| **(3) CAS + WAL** | Spec-defined instances | Process objects under `docs/process/`; profile **`cas_entity`** by default via inheritance from `auditable`. |

**Unified model (without migrating files yet)**  
Feature flags, tray, and CLI hook profile are **not** separate “kinds” in CAS; they are **the same logical membrane** described in DATA_CELL_RUNTIME_ORGANISM — **light-file / operator-local** cells with centralized paths and optional `protocol_version`. Treating them as “super-simple data cells” in documentation matches the BLI goal; **wrapping** them as first-class persisted objects or **routing** all writes through a steward/coordinator is **explicitly deferred** (see *Cell manager / nucleus* in the table below and BLI non-goals).

**Sequencing (what is not this BLI)**

1. **Adapters** — optional wrappers so the same logical API can sit above CAS or stream when profile dictates — backlog / future spike.  
2. **Admin / zqk-admin** structured `new` / `edit` without exposing storage — **CMDv2** track ([CMDV2_AND_CLI_SPLIT_RATIONALE.md](./CMDV2_AND_CLI_SPLIT_RATIONALE.md)).  
3. **Assembly pipeline** (storage + metrics + logging + events + notifications + transceiver ports) — separate epics; see DATA_ORIGINATION_PIPELINE_VISION, convergence backlog.  
4. **External hooks** — remain governed by [CLI_EXTERNAL_HOOK_PROTOCOL.md](./CLI_EXTERNAL_HOOK_PROTOCOL.md); no version bump required for this doc-only closure.

## Data cell test matrix (operational envelope + organism)

Isolated **unit and integration** tests for the data-cell program (registry, profile contracts, **operational envelope** map, scheduler policy/handler, `zqk system data-cells` (PRUNED)) live in:

| Area | Tests (representative) | Foreground probe |
|------|------------------------|-------------------|
| **`pkg/datacell`** | `operational_envelope_test.go` (profile + per-kind augments, compact summaries); `stream_organism_e2e_test.go` (`TestStreamOrganism_*`); `membrane` path tests | `go test ./pkg/datacell -timeout 60s -run 'OperationalEnvelope|StreamOrganism|Membrane'` |
| **`pkg/objects`** | `spec_index_storage_profile_consistency_test.go`, `spec_index_high_volume_validate_test.go` — `high_volume_kinds.yaml` stream list vs materialized `spec_index` `storage_profile` (same checks as `ValidateHighVolumeStreamKindsMatchSpecIndex` on `generate-spec-index` / `RefreshMaterializedSpecIndex`) | `go test ./pkg/objects -timeout 60s -run 'SpecIndexStorageProfile|ValidateHighVolumeStreamKinds'` |
| **`pkg/scheduler`** | `datacell_envelope_test.go` — policy dry-run, envelope tick handler, metrics JSONL (+ dispatch outcome keys), `envelope_tick_dispatch_*` tests, `jobTypeHandlerRegistry` registration; **`maintenance_runner_test.go`** — isolated `runCycle` → steward enqueue JSONL + **`data_cell_envelope_tick`** drain (`TestMaintenanceRunner_runCycle_*`) | Foreground probe: `go test ./pkg/scheduler -timeout 60s -run 'DataCellEnvelope|DryRunDataCell|EnvelopeTickDispatch|JobTypeHandlerRegistry_DataCell'` — **full package via scheduler** (every `*_test.go` in the package, many `SCH-run-bundle-*` jobs): `zqk scheduler scan-tests --package ./pkg/scheduler` — for a **curated** data-cell slice use **`scan-tests --load-bundles data-cell-stream-organism,data-cell-stream-organism-pkg-storage`** instead. Logs: `.zqk/logs/scheduler/cvs/test-bundles/`; `envelope_tick_dispatch_*` and related lines land in **`.zqk/metrics/data_cell_envelope_tick.jsonl`** when the test or tick path enables metrics recording. Narrow stewardship pipeline probe: `go test ./pkg/scheduler -timeout 60s -run 'TestMaintenanceRunner_runCycle_'`. |
| **`pkg/storage`** | **`stream_stewardship_test.go`** — `PostRetentionStreamStewardship`: steward-queue JSONL **`stream_steward_kind`** per stream-backed kind / phase (`TestPostRetentionStreamStewardship_enqueuesStewardMaintenanceJSONL`; contract in [STREAM_KIND_STEWARDSHIP.md](./STREAM_KIND_STEWARDSHIP.md)), orphan segment GC + **soft-delete registry compact→GC** fixtures (`TestPostRetentionStreamStewardship_*Fixture`), **runtime_delta overlay GC** (`TestPostRetentionStreamStewardship_runtimeDeltaOverlayGC_fixture`), **runtime_delta backfill** (`TestPostRetentionStreamStewardship_runtimeDeltaBackfill_fixture` — stream `scheduler_job` + missing overlay); **runtime_delta_migration.go** backfill aligns with **`runtime_delta_fields.yaml`** when specs omit runtime_delta roles. Lower-level GC/registry: `stream_segment_gc_test.go`, `stream_registry_persistent_test.go` | `go test ./pkg/storage -timeout 90s -run 'TestPostRetentionStreamStewardship_'` |
| **`cmd/zqk/system`** | `data_cells_test.go` — JSON envelope shape, operational envelope rows, policy dry-run JSON; `data_cells_stream_organism_test.go` — table/JSON/`--json-envelope` (run `TestDataCells_StreamOrganism` separately); **`migrate_legacy_to_stream_*_test.go`** — in-process **`migrate-legacy-to-stream`** dry-run + apply with **`--remove-legacy`** on an isolated tree (`TestMigrateLegacyToStream_dryRun_inProcess`, `TestMigrateLegacyToStream_applyRemoveLegacy_inProcess`; subprocess suite remains `migrate_legacy_to_stream_integration_test.go` + env gate) | `go test ./cmd/zqk/system -timeout 60s -run 'Test(DataCellsEnvelopePolicy|OperationalEnvelopeTableFooter|BuildDataCellJSONRows|DataCellsJSONEnvelopeShape|FilterDataCellsByKind)'` — migration probes: `go test ./cmd/zqk/system -timeout 90s -run 'TestMigrateLegacyToStream_(dryRun|applyRemoveLegacy)_inProcess'` |

**Saved scheduler bundles** (stable job ids `SCH-run-data-cell-stream-organism*` / `SCH-run-data-cell-stream-organism-pkg-storage`, logs under `.zqk/logs/scheduler/cvs/test-bundles/`, rollup in `health.jsonl`): prefixes **`data-cell-stream-organism`** and **`data-cell-stream-organism-pkg-storage`** — shards `.zqk/test-bundles/data-cell-stream-organism-*.json` and `data-cell-stream-organism-pkg-storage-*.json`. The canonical test name lists are in [`scripts/save-data-cell-stream-organism-bundle.sh`](../../scripts/save-data-cell-stream-organism-bundle.sh). Regenerate after adding or renaming tests: `./scripts/save-data-cell-stream-organism-bundle.sh`. Run both: `zqk scheduler scan-tests --load-bundles data-cell-stream-organism,data-cell-stream-organism-pkg-storage` (requires scheduler). See [DATA_CELL_RUNTIME_ORGANISM.md](./DATA_CELL_RUNTIME_ORGANISM.md) (background verification).

### Data cell automated verification (bundles)

Tracked by **`[REDACTED-ID]`** (*automated test matrix + scheduler bundle coverage*).

| Step | Command / artifact |
|------|---------------------|
| Refresh bundle shards after renaming or adding tests listed in [`scripts/save-data-cell-stream-organism-bundle.sh`](../../scripts/save-data-cell-stream-organism-bundle.sh) | `./scripts/save-data-cell-stream-organism-bundle.sh` |
| Scheduler replay (fingerprints → `health.jsonl`) | `zqk scheduler scan-tests --load-bundles data-cell-stream-organism,data-cell-stream-organism-pkg-storage` |
| Logs | Per-job `*.log` under **`.zqk/logs/scheduler/cvs/test-bundles/`**; fingerprint rollup **`.zqk/logs/scheduler/cvs/test-bundles/health.jsonl`** |
| Envelope tick metrics (local workspace) | When tests run enable metrics recording (`metricsrecording`), **`.zqk/metrics/data_cell_envelope_tick.jsonl`** gains lines with `operational_envelope_*`, `operational_envelope_resolved_scheduler_job_types`, `envelope_tick_dispatch_*` counters |

Use the [table above](#data-cell-test-matrix-operational-envelope--organism) for **foreground** probes without the daemon.

**CMDv2 overlap:** Until cmdv2 admin binaries subsume discovery, **`zqk system data-cells` (PRUNED)** stays the canonical operator surface — see [CMDV2_AND_CLI_SPLIT_RATIONALE.md §7](./CMDV2_AND_CLI_SPLIT_RATIONALE.md#7-data-cell-operator-discovery-zqk-system-data-cells-vs-cmdv2).

## Spec cell integration suite (local) and test gaps

**What runs today:** `TestSpecCell_Suite` in `cmd/zqk/system/spec_cell_integration_test.go` is the **alpha gate** for subprocess CLI on a **greenfield temp project** (`ZQK_TEST_ROOT` / `SYSTEM_TEST_ENABLE_SPEC_CELL_INTEGRATION_TESTS=1`). It covers init → bootstrap DNA → `generate-spec-index` → **net-new kind** YAML + config patches → `generate-spec-index` / `validate` / `generate-instance-builders` → `update-specs` dry-runs → **`system update-specs` field define/modify/deprecate/archive/delete** on the bootstrapped `test_case` spec (sidecar YAML), `sync-glossary-from-specs` **dry-run JSON + apply**, `system validate`, feature-flags / cli-hooks list, path-cache, retention/health, and `system check --fast --clean-cache`. See the file header comment for the full list.

**Related (data cell program):** the subprocess suite does **not** replace the **isolated** data-cell tests above; use the [Data cell test matrix](#data-cell-test-matrix-operational-envelope--organism) and saved **`data-cell-stream-organism`** + **`data-cell-stream-organism-pkg-storage`** bundles when changing `pkg/datacell`, envelope scheduler code, **`pkg/storage`** post-retention stream stewardship (`PostRetentionStreamStewardship`, steward **`stream_steward_kind`** JSONL), or `data-cells` output. After changing matrix tests or bundle membership, refresh shards and replay **both** bundles — see [Data cell automated verification (bundles)](#data-cell-automated-verification-bundles).

**Gaps vs end-to-end “origination pipeline” (roadmap tests / backlog):**

| Gap | Notes |
|-----|--------|
| **New kind from zero** | **`net_new_kind_spec_pipeline`** adds `spec_cell_netnew.yaml` + patches `kind_mappings` / `id_prefixes` / `namespaces` → **`SPEC_ORIGIN_CROSS_MEMBRANE`** → `generate-spec-index` → `validate --kind` → **`generate-instance-builders`** (output under `.zqk/spec_cell_codegen/`). Draft scaffolds: **`zqk object template object_spec`**; elevated apply via **`zqk object create … --internal`** (REDACTED). **Fail closed:** do not advertise creatable until Kind Registration Membrane checklist green — snag catalog **`GLS-1786413953213934000-1c0fb3ff`** / process **`NEW_COMMAND_OBJECT_ORIGINATION.md`** (S01–S17). |
| **Field types matrix** | One probe field (`spec_cell_alpha_probe`) exercises **string**-shaped sidecar body; **table-driven** tests for each field semantic type / validation shape are not yet required by the suite. |
| **Full spec validation gate** | Field define with `--validate` (REQ-019 / `LoadSpecAndValidate`) remains **opt-in** via `ENABLE_SPEC_CELL_REQ019_VALIDATE` because the bundled `test_case` spec can still hit **full-spec** validation debt (e.g. inherited traits vs field declarations) unrelated to the probe field. |
| **Glossary apply** | **`sync_glossary_from_specs_apply`** runs **`sync-glossary-from-specs --apply --dry-run=false`** and lists `glossary_term` objects. **`createGlossaryCandidate`** allocates IDs and ensures `docs/process/glossary_terms/` exists before sequential ID gen — **tactical** until a **cell steward/coordinator** owns directory prep, ID allocation, WAL, and invalidation for glossary (and other) kinds. |
| **Storage profiles in action** | `storage_profile` appears in specs/index; **instance CRUD** that proves **CAS vs stream vs light_file** behavior for a **new** kind is not fully covered here (see `STREAM_STORAGE.md`, process object CAS, `pkg/datacell` paths for the runtime slice). |
| **Cell manager / nucleus** | **Partial (v1 contracts):** [`pkg/datacell/membrane.go`](../../pkg/datacell/membrane.go) defines `CellMembrane`, `CellCoordinator`, `CellReadModel`, spec-revision staleness helpers aligned with `SpecLoader.SpecCacheRevision` / materialized `spec_index`, and `MinimalMembrane` + `NoopCellCoordinator` for tests and incremental wiring. **Narrow steward slice:** [`pkg/datacell/steward_enqueue.go`](../../pkg/datacell/steward_enqueue.go) defines the JSONL contract ([`AppendStewardEnqueueRecord`](../../pkg/datacell/steward_enqueue.go), [`DrainStewardEnqueueLog`](../../pkg/datacell/steward_enqueue_drain.go)); [`datacell.StewardEnqueueJSONLPath`](../../pkg/datacell/paths.go) resolves via [`paths.PathAliasDatacellStewardEnqueue`](../../pkg/paths/resolver.go) with segment fallback [`paths.DataCellLogsSubdir`](../../pkg/paths/constants.go) / [`paths.StewardEnqueueJSONLFile`](../../pkg/paths/constants.go). Optional metrics append to `.zqk/metrics/data_cell_steward.jsonl` when metrics recording is enabled (`pkg/datacell/steward_metrics.go`). [`pkg/scheduler/steward_enqueue_coordinator.go`](../../pkg/scheduler/steward_enqueue_coordinator.go) (`StewardEnqueueCoordinator`, `NewStewardEnqueueCoordinator` / profile helpers) implements [CellCoordinator]; [`handlers_datacell_envelope_tick.go`](../../pkg/scheduler/handlers_datacell_envelope_tick.go) invokes drain on each `data_cell_envelope_tick` (`SCH-dce-tick`). [`handlers_cache_invalidation.go`](../../pkg/scheduler/handlers_cache_invalidation.go) best-effort enqueues `MaintenanceOpInvalidateCache` on the stream steward when project root is known (resolved from file storage). Attach via [`datacell.StreamMembraneReadPathsWithCoordinator`](../../pkg/datacell/membrane_read_path.go). Full thread pools, WAL routing, policy knobs, and semantic operator surface remain **roadmap**. Stand-ins (e.g. `MkdirAll` + `idgen` in `sync-glossary-from-specs` apply) are **not** the full manager. |
| **Admin CLI split** | Second binary for spec admin + codegen, emitting YAML for core CLI/scheduler to load — **future**; [CMDV2_AND_CLI_SPLIT_RATIONALE.md](./CMDV2_AND_CLI_SPLIT_RATIONALE.md). |

**Commands (local):** see **`scripts/README.md`** → *Local verification (no hosted CI yet)*.
