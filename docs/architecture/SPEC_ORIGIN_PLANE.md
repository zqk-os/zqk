> [!WARNING]
> **ARCHIVED DOCUMENT**: The primary commands referenced in this architectural document have been pruned from the `zqk` CLI.

# Spec origin plane: canonical model, derived indexes, and traceability

**Last Verified:** 2026-08-31


**Status:** Architecture (normative direction + inventory of what exists today)  
**Audience:** Implementers working on spec loading, validation, codegen, scenario/traceability bundles, storage, and future stream/data-cell work.

**Process tracking:** **P1** backlog item **`[REDACTED-ID]`** (`priority_plan_ref` = Object maintenance redesign). List/filter: `zqk object list backlog_item --filter id=[REDACTED-ID]`. Roadmap: [PRIORITY_ROADMAP_CURRENT.md §8](./PRIORITY_ROADMAP_CURRENT.md).

## Summary

Object **specs** (`docs/process/_internal/object_specs/*.yaml`) are the declarative definition of the **persisted data model**: kinds, fields, traits, validation, inheritance (`extends`), and lifecycle-related metadata. Everything that makes the system **correct, fast, and safe** to operate on stored objects should ultimately **derive from** a single notion of “what specs are loaded and current,” not from ad hoc re-parsing or parallel truth sources.

This document captures that **spec origin plane** idea, maps **existing** code to it, names the **gaps**, and links to **traceability bundles**, **scenario apply**, and **stream/cell** pilots so requirements and implementation stay aligned.

## Design principles

1. **Single spec plane** — One conceptual layer answers: which spec revision is authoritative, and which **materialized** artifacts (indexes, graphs, kind lists) are valid for that revision.

2. **Detail cache → dependent caches** — A **detail** layer holds spec bytes or normalized spec state. **Thin** layers (field index, dependency graph, kind enumeration for tools) subscribe to the same **invalidation** story: when the detail layer changes, dependents rebuild or clear. Avoid unrelated caches re-reading disks without coordination.

3. **Operational paths** — Pre-built structures for CRUD, list/search, single-type groups, and multi-type composition should be **spec-driven** and **revision-consistent**, eventually aligning with **data cell / stream** boundaries (see [DATA_STREAM_SUMMARY_PILOT.md](./DATA_STREAM_SUMMARY_PILOT.md), [STREAM_STORAGE.md](./STREAM_STORAGE.md)).

4. **Sources** — Today specs load from **local files**. Longer term, the same logical model may load from **secure remote** artifacts or **graph** stores; see [graph-spec-storage-v1.0.md](../process/architecture/graph-spec-storage-v1.0.md). The **plane** abstraction is the stable seam; backends plug in behind it.

## What exists today (grounded)

| Concern | Role | Primary location / notes |
|--------|------|---------------------------|
| Load + cache resolved specs | Detail-ish: per-ontology and per-path caches, `mtime` checks | `pkg/objects/spec_loader.go` (`SpecLoader`, `GetGlobalSpecLoader`) |
| Invalidate specs | Manual: full clear or per-ontology / per-file | `ClearCache`, `InvalidateSpec`, `InvalidateSpecByFile` on `SpecLoader` |
| Dependency graph (`extends`) | Structural graph: build, cycles, topo order; **`BuiltAtSpecCacheRevision`** / **`LoaderSpecCacheInvalidatedSinceBuild`** correlate with [SpecLoader.SpecCacheRevision] | `pkg/objects/spec_dependency.go` (`SpecDependencyGraph`) |
| Field snapshot per kind | Read-optimized index for tooling | `pkg/objects/spec_index.go`, `zqk system generate-spec-index` (PRUNED) → `docs/process/_internal/spec_index.json` (JSON may include **`builder_spec_cache_revision`** / **`global_spec_cache_revision`** after regeneration; `omitempty` when zero) |
| Pipeline **Outcome** map wire keys | Declarative registry (internal YAML) → generated Go consts for `pipeline.Context.Outcome` | `docs/process/_internal/pipeline_outcome_keys.yaml`, `zqk system generate-pipeline-outcome-keys` (PRUNED) → `pkg/pipeline/outcome_keys.go` (same naming rules as **`objects.FieldKeyConstName`**) |
| Kind strings from YAML | Stateless walk: unique `ontology` values, **or** derive the same `map[string]struct{}` from a loaded `SpecIndex` | `pkg/kindnames/load_from_specs.go` (`LoadKindNamesFromSpecsDir`); when `spec_index.json` / `SpecIndex` is already materialized, prefer `pkg/objects/spec_index.go` (`KindNamesFromSpecIndex`) so the kind set matches the index snapshot |
| Spec load order / DAG | Design doc | [spec-loading-order-v1.0.md](../process/architecture/spec-loading-order-v1.0.md) |
| Traceability artifacts | REQ / CRIT / BLI / TEST / goals / … as **objects** whose kinds are spec-defined | [SCENARIO_BUNDLES_AND_TRACEABILITY.md](./SCENARIO_BUNDLES_AND_TRACEABILITY.md) |
| Bundle apply create order | Uses **spec index** ordering for creates (goals, criteria, requirements, …) | `pkg/scenario/apply.go`; rationale in [CMDV2_AND_CLI_SPLIT_RATIONALE.md](./CMDV2_AND_CLI_SPLIT_RATIONALE.md) |
| Spec YAML edits via **`update-specs`** | Refreshes **`spec_index.json`** after successful writes | `objects.RefreshMaterializedSpecIndex` from `cmd/zqk/system/update_specs.go` and `spec_writer_field_ops.go` (same **`BuildAndWriteMaterializedSpecIndex`** as **`generate-spec-index`**) |

## Gaps (honest)

- **No unified “spec revision” token across all derived views** — `SpecLoader` now exposes **`SpecCacheRevision()`** (increments on `ClearCache`, `InvalidateSpec`, `InvalidateSpecByFile`). `SpecIndex` JSON may record **`builder_spec_cache_revision`** / **`global_spec_cache_revision`** (after regeneration); `SpecDependencyGraph` correlates via **`BuiltAtSpecCacheRevision`**. There is still **no** single cross-plane event channel; wire callers to compare **`SpecCacheRevision()`** before/after regenerating artifacts as an incremental step.

- **Ad hoc projections** — Stateless walks of `object_specs` (`LoadKindNamesFromSpecsDir`) remain valid for **custom** specs dirs and tests. **`zqk system analyze-drift-hotspots` (PRUNED)** uses **`KindNamesFromSpecIndex`** when the default **`docs/process/_internal/object_specs`** layout applies and **`spec_index.json`** loads; otherwise it walks the specs directory. One-shot codegen/CLI without a loader still uses the walk where no index is in play.

- **Work envelope vs universal fields** — `estimated_effort` / `actual_effort` live on the `work_unit` mixin (not `base_object`); `started_at` / `completed_at` live on `work_interval`; `claimed_by` / `claimed_at` live on the `occupancy` mixin (`occupiable` trait; extends `work_interval`, not `work_unit`). Timesheet kinds **compose** occupancy. See [WORK_ENVELOPE_AND_EFFORT_FACETS.md](./WORK_ENVELOPE_AND_EFFORT_FACETS.md).

- **Graph edge ownership** — typed `*_ref`/`*_refs` must be one-way with **`edge_role`** (`membership` \| `composition` \| `associate`); `related_object_refs` is not a typed edge. See [GRAPH_EDGE_OWNERSHIP.md](./GRAPH_EDGE_OWNERSHIP.md) (`BLI-KERNEL-REF-GRAPH-ACYCLIC-001`).

- **Remote / graph spec backends** — Design exists; full unification with file-based loading is tracked with graph spec work, not complete in one place.

## First foray (recommended slice)

A defensible first step toward the full plane:

1. **Name a single “materialization” boundary** after spec changes: which **derived** structures must refresh in the same transaction or immediately after (dependency graph, spec index file, optional in-memory kind set). **Implemented (partial):** `SpecLoader.SpecCacheRevision()`; **`zqk system update-specs`** (after successful writes) and **`runFieldOperation`** call **`objects.RefreshMaterializedSpecIndex`** so **`docs/process/_internal/spec_index.json`** stays aligned with on-disk YAML (same **`BuildAndWriteMaterializedSpecIndex`** contract as **`zqk system generate-spec-index` (PRUNED)**). Dependency graph refresh remains separate.

2. **Reduce duplicate walks** — Prefer deriving **kind lists** from `SpecIndex` or `SpecDependencyGraph` **after** a load/refresh where possible, or document when a stateless walk is intentional. **Implemented (partial):** **`KindNamesFromSpecIndex`**; drift hotspots analyzer prefers **`TryLoadSpecIndexForProjectRoot`** + **`KindNamesFromSpecIndex`** for the default object_specs path under project root.

3. **Document invalidation** — When adding a new spec-derived cache, hook **either** explicit invalidation next to existing `SpecLoader` clears **or** a shared helper so operational behavior stays predictable. **Contract:** any new in-memory spec-derived cache should bump or observe **`SpecCacheRevision()`** alongside `SpecLoader` invalidation, or document why it is exempt (one-shot CLI).

This does not require implementing data cells or streams; it **prepares** consistent snapshots for later cell/stream boundaries.

### Invalidation contract (first-foray closure)

For **[REDACTED-ID]** acceptance: the **first foray** above is **implemented** for the paths below, with **documented exceptions** where a stateless projection is intentional.

| Derived artifact | Invalidation / correlation | Exceptions (documented) |
|------------------|----------------------------|---------------------------|
| **`SpecLoader`** in-memory caches | **`ClearCache`**, **`InvalidateSpec`**, **`InvalidateSpecByFile`** bump **`SpecCacheRevision()`** | Long-lived processes must still call these after external spec edits; not an event bus. |
| **`docs/process/_internal/spec_index.json`** | **`BuildAndWriteMaterializedSpecIndex`**: clears **global** loader first, then writes JSON with **`builder_spec_cache_revision`** and **`global_spec_cache_revision`**. Invoked by **`zqk system generate-spec-index` (PRUNED)**, **`zqk system update-specs`** (after writes), and **field operations** that write specs. | Manual YAML edits without running **`generate-spec-index`** or **`update-specs`** can leave the file stale until someone regenerates. |
| **`SpecDependencyGraph`** | **`BuiltAtSpecCacheRevision`**, **`LoaderSpecCacheInvalidatedSinceBuild`** vs **`GetGlobalSpecLoader().SpecCacheRevision()`** | Rebuild graph after spec changes when you rely on topo order; no automatic hook from **`update-specs`** yet. |
| Kind set for tools (`map[string]struct{}`) | Prefer **`KindNamesFromSpecIndex`** when **`spec_index.json`** loads; **`analyze-drift-hotspots`** does this for default **`object_specs`** layout. | **`LoadKindNamesFromSpecsDir`** for custom **`--specs-dir`**, tests, and one-shot codegen without an index. |

New **spec-derived** in-memory caches should follow **PRE_CHANGE_CHECKLIST** (spec origin plane / section 3a): observe or bump **`SpecCacheRevision()`** with **`SpecLoader`** invalidation, **or** document a one-shot / exempt path.

## Traceability bundles and scenario apply

**Persistent traceability bundles** (requirements, criteria, backlog items, test cases, goals, fixtures) are **instances** of kinds defined in **object_specs**. They are not a parallel schema; they are the operational proof that the spec-defined model works end-to-end.

- **Pattern and commands:** [SCENARIO_BUNDLES_AND_TRACEABILITY.md](./SCENARIO_BUNDLES_AND_TRACEABILITY.md) (bundle table, `zqk-scenario bundle apply`, project root, CAS visibility).

- **Ordering:** Scenario apply respects **spec-derived** create order so references resolve; that ties bundle UX directly to the **spec index** and thus to the **spec origin** story above.

When adding REQ/CRIT/BLI objects for spec-plane work, prefer creating or extending a **traceability bundle** and linking criteria to tests, per that document.

**Agent orchestration / multi-surface prompts (2026):** Product direction (pluggable agent delivery, organizational memory, policy self-assessment) is captured as **persistent traceability** in `test-scenarios/agent-orchestration-traceability-bundle/agent-orchestration-traceability-bundle.yaml` (`REQ-AO-001`, `GOAL-AO-001`, criteria **CRIT-AO-001**–**006**). Reconciliation narrative: [SCENARIO_BUNDLES_AND_TRACEABILITY.md](./SCENARIO_BUNDLES_AND_TRACEABILITY.md) (§ *Agent orchestration, prompt delivery, and organizational memory*). Apply via `zqk-scenario bundle apply` or `zqk bundle apply`; do not hand-edit instance YAML under `docs/process/`.

## Streams, summaries, and data cells

- **Onboarding / analogy (non-normative):** [SPEC_STREAM_CELL_PEDAGOGY.md](./SPEC_STREAM_CELL_PEDAGOGY.md) — technical + practical call-out + shared metaphor (specs ↔ DNA, streams/cells ↔ circulation) so newcomers have a **common picture** before file paths and commands.

- **Pilot:** Summarized stream sidecars and health metrics: [DATA_STREAM_SUMMARY_PILOT.md](./DATA_STREAM_SUMMARY_PILOT.md).

- **Origination roadmap (target):** [DATA_ORIGINATION_PIPELINE_VISION.md](./DATA_ORIGINATION_PIPELINE_VISION.md) — pipeline-shaped path from spec to storage choices, lifecycle, access materialization, and CLI-manipulable instances (aligned with [data-pipeline-lifecycle.md](./data-pipeline-lifecycle.md)).

- **Storage layout:** [STREAM_STORAGE.md](./STREAM_STORAGE.md), [FILESYSTEM_DATA_LAYOUT.md](./FILESYSTEM_DATA_LAYOUT.md).

Full **data cell** coordination (REQ-DATACELL-001–style) is **out of scope** for the first spec-plane slice; the spec origin plane is the **schema and revision** foundation that later cell boundaries should trust.

### Compatibility with data cells and streams (no forked truth)

These layers stay **orthogonal** so stream and cell work can proceed without replacing spec-plane artifacts:

- **Schema vs instance:** The spec plane answers **what kinds and fields exist** (ontology, inheritance, validation, materialized **`spec_index.json`**). **Stream segments**, **runtime deltas**, and **stream summaries** ([DATA_STREAM_SUMMARY_PILOT.md](./DATA_STREAM_SUMMARY_PILOT.md), [STREAM_STORAGE.md](./STREAM_STORAGE.md)) answer **how high-volume instance rows move** and **how operational logs aggregate**. Cells and membranes should **consume** resolved specs through **`SpecLoader` / field registry / validators**—not embed a parallel copy of ontology in stream payloads or sidecars.

- **Revision story:** **`SpecCacheRevision`** and on-disk **`spec_index.json`** revisions are **read-model correlation** for tooling. A future graph-backed spec backend or cell-hosted cache should preserve the **same invalidation contract** (monotonic revision or explicit generation) described in the **Invalidation contract** table above, per [graph-spec-storage-v1.0.md](../process/architecture/graph-spec-storage-v1.0.md)—not introduce a second ad hoc “spec generation” for streams.

- **Naming / storage API:** Hash-addressed **file** storage (“CAS” in `pkg/storage`) is an **implementation** of object persistence. It is **not** the same concern as **data cells** or **`.zqk/stream_summary/`** rows. Neutral entry-point aliases (**`GetObjectStorageMetrics`**, **`GetGlobalOrphanCleanupQueue`**, etc.) are documented in [STORAGE_PUBLIC_API_NEUTRAL_ALIASES.md](./STORAGE_PUBLIC_API_NEUTRAL_ALIASES.md); full migration off **CAS\*** symbols is tracked as **[REDACTED-ID]** and stays **orthogonal** to cell/stream rollout.

### Measurements, reports, and response (interpreter / synthesizer stage)

Any **measurement or report** the system produces (object counts, stream summaries, health aggregates, improvement reports, etc.) should have a **paired interpretation stage**: policy or thresholds that turn raw numbers into **decisions**, and **actions** delivered through the same **coordinator** path as the rest of the platform — `pkg/coordination` **`EventContext`**, **`Coordinator.Emit`**, and the **`pkg/pipeline`** emit graph (logging, audit, metrics, operational subscribers), not ad-hoc side channels. Configurable follow-ups (events, webhooks, scheduler follow-on jobs, routing rules) belong **after** that synthesis step so delivery semantics stay consistent with **metrics, logging, and audit** already wired there. This aligns with **data cell / stream** direction (summaries and membranes without duplicating pipelines) and with the completed spec-origin slice (**[REDACTED-ID]**): extend existing jobs and coordinators rather than inventing parallel alert paths.

## Glossary as a derived index (target)

**Intent:** **`glossary_term`** (`GLS-*`) should act as a **live index** that ties **vocabulary** (agents, operators, docs) to **machine structures**: object specs, lifecycles, and canonical configs. That supports goals stated in [CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md](./CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md) (membrane / cell / organelle vocabulary) without duplicating truth outside the spec plane.

**Today:** Terms are created with **`zqk object create glossary_term`** (see **`scripts/templates/glossary_*.yaml`**). A first-pass sync command now exists: **`zqk system sync-glossary-from-specs`** (default dry-run proposals; optional **`--apply --dry-run=false`** creates missing terms using the glossary instance builder). Coverage is intentionally conservative (title-based matching; no stale-update reconciliation yet).

**Target process (adjacent to spec codegen):**

1. **Trigger points** — After materializing specs (**`zqk system generate-spec-index` (PRUNED)**, **`zqk system generate-instance-builders`**, **`zqk system generate-config-builders`**) or an aggregated CI step that runs them, run **`zqk system sync-glossary-from-specs`**:
   - read **ontology** keys and spec **`description`** fields from the **same** spec plane revision as **`spec_index.json`**;
   - include **lifecycle** object types and **config** kinds that participate in generated builders;
   - **dry-run** by default: list **missing** or **stale** glossary candidates (title, proposed definition stub, **`machine_hints`** JSON with spec path / kind).

2. **Persistence** — Instance writes remain **CLI-only** (`zqk object create` / `bulk update`). The sync command **proposes**; humans or agents **review** and create/update objects (process-data-cli-only).

3. **Outcome** — Glossary becomes a **dynamic semantic layer** on top of the spec plane: high-level goals can reference **`GLS-*`** terms that resolve to **explicit** kinds, fields, and policies.

**Gap:** richer reconciliation is still pending (e.g., detect/update stale glossary rows, alias mapping, confidence levels). Current helper **`scripts/sync_glossary_from_specs.sh`** now delegates to the command (dry-run default).

## Related documentation

| Topic | Document |
|-------|----------|
| Human-facing doc map + linking roadmap (markdown ↔ ontology; doc-graph backlog **`[REDACTED-ID]`** **deferred**; glossary **`GLS-1776207925199440000-aa236125`**) | [ASSESSMENT_AND_ONBOARDING_INDEX.md](./ASSESSMENT_AND_ONBOARDING_INDEX.md) |
| Traceability bundles, REQ→test mapping | [SCENARIO_BUNDLES_AND_TRACEABILITY.md](./SCENARIO_BUNDLES_AND_TRACEABILITY.md) |
| Agent orchestration audible, metaphor → streams/cells | [AGENT_ORCHESTRATION_AUDIBLE.md](./AGENT_ORCHESTRATION_AUDIBLE.md) |
| CLI split, `ApplyScenarioBundle`, spec index ordering | [CMDV2_AND_CLI_SPLIT_RATIONALE.md](./CMDV2_AND_CLI_SPLIT_RATIONALE.md) |
| Spec DAG / load order (design) | [spec-loading-order-v1.0.md](../process/architecture/spec-loading-order-v1.0.md) |
| Graph-backed spec storage (design) | [graph-spec-storage-v1.0.md](../process/architecture/graph-spec-storage-v1.0.md) |
| Pre-change checklist (includes specs / tests / glossary section 3b) | [PRE_CHANGE_CHECKLIST.md](./PRE_CHANGE_CHECKLIST.md) |
| CLI membrane, cells, organelles (vocabulary) | [CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md](./CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md) |
| Stream segments, high-volume instance layout | [STREAM_STORAGE.md](./STREAM_STORAGE.md) |
| Stream summary sidecars (pilot), test-bundle health | [DATA_STREAM_SUMMARY_PILOT.md](./DATA_STREAM_SUMMARY_PILOT.md) |
| Spec / stream / cell metaphor for onboarding | [SPEC_STREAM_CELL_PEDAGOGY.md](./SPEC_STREAM_CELL_PEDAGOGY.md) |
| Data origination from specs (pipeline vision, target) | [DATA_ORIGINATION_PIPELINE_VISION.md](./DATA_ORIGINATION_PIPELINE_VISION.md) |
| Storage neutral API aliases (CAS naming vs cells) | [STORAGE_PUBLIC_API_NEUTRAL_ALIASES.md](./STORAGE_PUBLIC_API_NEUTRAL_ALIASES.md) |

---

*This file is architecture narrative under `docs/architecture/`. Process **objects** (requirements, backlog items) remain authoritative in CAS; create or update them with the `zqk` CLI per project policy, not by editing YAML by hand.*
