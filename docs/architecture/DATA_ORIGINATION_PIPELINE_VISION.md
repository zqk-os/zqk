# Data origination from specs — pipeline vision (target)

**Status:** Architecture (**target / roadmap** — not a single shipped “one command does everything” today)  
**Audience:** Platform designers, spec authors, and agent–human collaboration workstreams who want a **repeatable path** from **`object_specs`** to **storages**, **lifecycles**, **access**, and **CLI-manipulable** structured data.

## Intent

Provide a **conceptual pipeline** — aligned with [data-pipeline-lifecycle.md](./data-pipeline-lifecycle.md) stage vocabulary — that describes how zqk could **originate** a full data surface from a kind (or family of kinds) declared in the **spec plane**:

- **What** shape of data exists (fields, traits, validation).
- **Where** it lives: **singular** object rows (e.g. CAS), **append-only streams** / summaries, **graph** projections, or **future** backends (“**x**”).
- **How** instances **transition** (lifecycle specs and listeners).
- **Who** may read or write **which fields** (field- and object-level privilege / policy materialization).
- **How** humans and agents **mutate** instances through the **CLI** (and APIs) with shared **schema_version** and **membrane** contracts — supporting collaboration, expectations, expertise, memory-like objects, transactions, metrics, and other structured blobs **without** forking truth outside the spec plane.

This document **ties the spec / stream / cell pedagogy** ([SPEC_STREAM_CELL_PEDAGOGY.md](./SPEC_STREAM_CELL_PEDAGOGY.md)) to **pipelines** and to the **operating-system metaphor** in [CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md](./CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md): a **realm of shared understanding** requires **one declarative genome** (specs) and **explicit origination steps** that materialize storages, gates, and tools.

---

## Pipeline spine (conceptual stages)

The following maps **origination** to the standard pipeline legend (envelope, stages, outcomes). Implementations may **batch** stages in one job or split across scheduler work; the **contract** is that each concern is **explicit** and **observable** ([`pkg/pipeline`](../pkg/pipeline); registry [`pipeline_outcome_keys.yaml`](../process/_internal/pipeline_outcome_keys.yaml); regenerate constants via **`zqk system generate-pipeline-outcome-keys` (PRUNED)**).

| Stage | Origination concern | Example outputs (target) |
|-------|---------------------|---------------------------|
| **INGEST** | Accept **spec inputs**: `ontology` YAML, traits, lifecycle refs, storage hints (stream vs document vs graph), policy templates | Normalized **origination plan** (versioned struct), idempotency key = spec content hash or spec revision |
| **NORMALIZE** | Resolve **extends**, traits, `ResolvedFields`, enums; merge with **spec_index** rules | Canonical **kind profile**: fields, types, required, validation hooks |
| **DECIDE** | Choose **storage binding**: CAS object, stream alias + layout, graph kind mapping, future adapter | **Storage decision record** (which writer paths activate; bucketing per [FILESYSTEM_DATA_LAYOUT.md](./FILESYSTEM_DATA_LAYOUT.md)) |
| **COMMIT** | **Materialize**: write/update **lifecycle** objects or bindings, register stream **path aliases**, create **graph** schema hooks if applicable, refresh **derived indexes** | Persisted config under process/internal or runtime registries **without** duplicating field semantics in payloads |
| **(AGGREGATE)** | **Summaries** for high-volume paths (existing pilot pattern) | `.zqk/stream_summary/*` rows, health sidecars ([DATA_STREAM_SUMMARY_PILOT.md](./DATA_STREAM_SUMMARY_PILOT.md)) |
| **(TRIGGER)** | **Glossary**, **builder regen**, **agent context refresh** hints | `sync-glossary-from-specs`, `generate-instance-builders`, policy interrupt surfacing |
| **FINALIZE** | **Verification**: validators, sample CRUD, privilege checks | Origination **Outcome** map + logs; “green” means CLI can **create/list/get/update** per kind |

**Principle:** Origination **consumes** the spec plane; it does **not** embed a second ontology inside stream cells. See [SPEC_ORIGIN_PLANE.md](./SPEC_ORIGIN_PLANE.md) (**Compatibility with data cells and streams**).

---

## Encapsulation in `pkg/pipeline` (v1 contract)

**Status:** Stage names and order are **defined** in code (`pkg/specorigination` + `pipeline_kind` **`spec.origination`**). **Implementations** that call `zqk` subprocesses or storage APIs per stage are still **incremental**; this section is the contract to converge on.

**Runner semantics:** [`pkg/pipeline`](../pkg/pipeline) runs **`AddStage` in strict sequence** ([data-pipeline-lifecycle.md](./data-pipeline-lifecycle.md) — no parallel stages inside one `Run`). Anything that can run concurrently must be **inside one stage** (e.g. TRIGGER) or **another job** (scheduler), not interleaved `AddStage` order.

| Order | Stage name (constant) | Lifecycle mapping | Primary resources / artifacts | Serial deps |
|------:|----------------------|-------------------|------------------------------|------------|
| 1 | `SPEC_ORIGIN_INGEST` | INGEST | Draft paths, optional `object_specs/*.yaml`, patches for `kind_mappings` / `id_prefixes` / `namespaces` | — |
| 2 | `SPEC_ORIGIN_NORMALIZE` | NORMALIZE | Resolved spec (`LoadSpecWithInheritance`), effective `storage_profile`, traits | after INGEST inputs exist |
| 3 | `SPEC_ORIGIN_DECIDE` | DECIDE | Plan: which regen steps apply (index, builders, glossary, stream list alignment) | after NORMALIZE |
| 4 | `SPEC_ORIGIN_COMMIT` | COMMIT | **Authoritative writes** to object specs and internal config (CLI or tooling only; no hand-edit `docs/architecture/` instances) | after DECIDE |
| 5 | `SPEC_ORIGIN_MATERIALIZE_INDEXES` | COMMIT (continued) | `spec_index.json`, `generate-field-keys`, other `_internal` indexes | after spec files committed |
| 6 | `SPEC_ORIGIN_TRIGGER` | TRIGGER | Idempotent: `generate-instance-builders`, `sync-glossary-from-specs`, path-cache / `detect-spec-changes` as needed | after indexes |
| 7 | `SPEC_ORIGIN_FINALIZE` | FINALIZE | `system validate --kind`, optional `system check --fast`, smoke `object list` / `count` | after TRIGGER |

**Parallelism (explicit):**

- **Between stages:** not supported by the runner; order is **serial** as above.
- **Inside `SPEC_ORIGIN_TRIGGER`:** safe to run **in parallel** only steps that do not depend on each other’s outputs (e.g. glossary apply vs builder regen may be ordered; follow existing CLI constraints). Prefer **one stage** that runs a small worker pool with **idempotent** commands and stable ordering **per kind** if needed.
- **Net-new kind:** `kind_mappings` / `id_prefixes` / `namespaces` patches must be **committed before** `MATERIALIZE_INDEXES` (loader skips unknown kinds).

**Alpha integration coverage:** The subprocess ordering exercised in `cmd/zqk/system/spec_cell_integration_test.go` (init → bootstrap → `generate-spec-index` → validate → `generate-instance-builders`, etc.) is a **reference run**; align future `spec.origination` wiring to this table.

---

## Dimensions to originate (the “whole enchilada”)

| Dimension | Spec / config source (today or target) | Notes |
|-----------|------------------------------------------|--------|
| **Type of storage** | Traits, future `storage_profile` / stream hints on kind or internal config | **Singular** instances (default CAS path), **streams** (JSONL + summaries), **graph** (MemGraph / future), **x** (pluggable adapter contract). |
| **Lifecycle** | `docs/architecture/_internal/lifecycles/*.yaml`, lifecycle builders | Listener and transition rules stay **spec-linked**; origination ensures **objects** and **jobs** that enforce them exist. |
| **Privileges** | Field `access` / `permissions` in specs, POL objects, keystore / MCP roles | Target: **materialized** checks on hot paths (already directionally in storage and verification authority docs) — origination should **register** which gates apply per kind/field. |
| **CLI surface** | `object` CRUD, command specs, format/membrane | **Shared understanding** for humans + agents: same **`--format`**, **`schema_version`** on payloads, glossary links ([SPEC_ORIGIN_PLANE.md](./SPEC_ORIGIN_PLANE.md) glossary section). |
| **Human–agent collaboration** | Process objects (REQ, criteria, `glossary_term`), convergence sessions, agent delivery | Structured blobs (expectations, expertise, “memory” objects, metrics) are **instances of spec-defined kinds** — origination ensures they are **first-class** and **policy-bound**, not ad hoc files. |

---

## Practical slice (what is already directionally true)

- **`zqk system spec-origination` (PRUNED):** Implements the `spec.origination` pipeline in [`pkg/specorigination`](../pkg/specorigination); use **`--apply-trigger`** (not with `--dry-run`) to run glossary apply, `generate-instance-builders --overwrite`, and `path-cache` as subprocesses after materialize; otherwise the outcome records manual trigger hints. Outcome map keys are registered in [`pipeline_outcome_keys.yaml`](../process/_internal/pipeline_outcome_keys.yaml) (`zqk system generate-pipeline-outcome-keys` (PRUNED)).
- **Specs → indexes → tooling:** `generate-spec-index`, `generate-field-keys`, instance builders, validators — the **genome** side of origination exists; see [SPEC_ORIGIN_PLANE.md](./SPEC_ORIGIN_PLANE.md).
- **Pipeline pattern for runs:** `pkg/pipeline` + stage outcomes + codegen keys ([`pipeline_outcome_keys.yaml`](../process/_internal/pipeline_outcome_keys.yaml)) — good **spine** for observability when origination becomes automated jobs.
- **Streams / cells pilot:** Test-bundle health stream + summary ([DATA_STREAM_SUMMARY_PILOT.md](./DATA_STREAM_SUMMARY_PILOT.md)) — pattern for **declared alias → JSONL → summary**.
- **Membrane vocabulary:** [CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md](./CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md) — how CLI and agents share **versioned** cross-boundary payloads.

---

## Gaps (honest)

- **No single `zqk system originate-data-from-spec` (yet)** that chains all materializations with one idempotent run.
- **Storage profile** as a first-class, spec-driven **choice** (stream vs graph vs document) is **not** uniformly declared per kind; much is convention + code paths.
- **Privilege materialization** from field-level spec metadata to **enforced** uniform gates is **partial** — origination should eventually **emit** a machine-readable “access matrix” or hook list per kind.
- **Graph** origination (schema sync from spec to graph backend) remains **design-heavy** relative to CAS + file streams.

---

## Shared understanding (humans, agents, swarms)

A **realm of shared understanding** in this model means:

1. **Same spec revision** (or correlated `SpecCacheRevision` / `spec_index` generation) for humans editing YAML and agents calling tools.
2. **Same CLI and object IDs** for process and domain objects (`zqk object …`); instance data under `docs/architecture/` stays **CLI-only** (see enforcement index under `docs/architecture/enforcement/`).
3. **Same membranes** for delivery: structured results, JSONL evidence, optional HTTP/agent delivery ([CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md](./CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md)).
4. **Pipelines** for **origination** and **convergence** so “we added a kind” is not a tribal ritual but a **repeatable, measurable** flow ([data-pipeline-lifecycle.md](./data-pipeline-lifecycle.md), [EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md](./EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md)).

---

## Related documentation

| Topic | Document |
|-------|----------|
| Spec vs stream vs cell (pedagogy) | [SPEC_STREAM_CELL_PEDAGOGY.md](./SPEC_STREAM_CELL_PEDAGOGY.md) |
| Spec plane, revision, glossary target | [SPEC_ORIGIN_PLANE.md](./SPEC_ORIGIN_PLANE.md) |
| Pipeline stage contract | [data-pipeline-lifecycle.md](./data-pipeline-lifecycle.md) |
| Spec origination stage names (`spec.origination`) | [`pkg/specorigination`](../pkg/specorigination/stage_names.go) |
| Stream storage layout | [STREAM_STORAGE.md](./STREAM_STORAGE.md) |
| CLI membrane, cells, organelles | [CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md](./CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md) |
| Pre-change habits (specs, pipelines, glossary) | [PRE_CHANGE_CHECKLIST.md](./PRE_CHANGE_CHECKLIST.md) |

---

*Process **backlog / requirements** that implement this vision belong in CAS as objects created via the `zqk` CLI; this file is narrative architecture only.*
