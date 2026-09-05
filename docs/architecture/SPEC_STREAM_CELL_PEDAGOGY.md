# Spec plane, streams, and data cells — shared mental model

**Last Verified:** 2026-08-31


**Status:** Architecture (pedagogy + alignment with implementation)  
**Audience:** Anyone first encountering **object_specs**, **stream summaries**, or **data cell** language in zqk — operators, implementers, and agents who need a **stable picture** before diving into file paths and commands.

## Why this document exists

Complex systems are easier to reason about when **inputs and outputs** share a **common scaffold**. This note gives:

1. A **technical** mapping to what the repo actually implements (with pointers to normative docs).
2. A **practical call-out** — one concrete contrast that shows how two different failure modes feel in practice.
3. A **biological analogy** you can extend (carefully) to other parallels: nutrition, immunity, signaling, etc.

The analogy is **illustrative**, not a substitute for the spec plane, storage layout, or stream contracts.

---

## Technical core (what is what)

| Layer | Role in zqk | Think of it as |
|-------|----------------|-----------------|
| **Spec plane** | `docs/process/_internal/object_specs`, loaders, **`spec_index.json`**, validators, generated field keys / builders | The **rules** for valid persisted shape and behavior of kinds — **schema + lifecycle + validation**, not the live traffic. |
| **Data cell (logical + envelope)** | A **logical grouping** of data plus **operational packaging** (cleanup, scan, cache, retention, archive, bucketing, …) — may use **light files**, **CAS**, or **streams** as physical profiles; low-volume kinds may **share** one cell (twins / triplets / …). The **spec loader** already **auto-wires** **persistence** and the **standard query surface** (filtering, sorting, grouping, listing, pagination, counts) from **traits**; the cell/stream pipeline **extends** that contract — [DATA_CELL_MODEL.md](./DATA_CELL_MODEL.md) (*Spec-driven plane*). | A **contained data-management unit** — not “only small JSON under `.zqk/`”; that runtime slice is [DATA_CELL_RUNTIME_ORGANISM.md](./DATA_CELL_RUNTIME_ORGANISM.md). |
| **Streams & summaries** | High-volume JSONL / segments, **`.zqk/stream_summary/`** sidecars, bounded filesystem layout ([STREAM_STORAGE.md](./STREAM_STORAGE.md), [DATA_STREAM_SUMMARY_PILOT.md](./DATA_STREAM_SUMMARY_PILOT.md)) | **Flow and aggregation** for **stream-profile** cells — how the system **moves and summarizes** volume without re-parsing the whole genome on every read. |
| **Object persistence (e.g. CAS)** | Hash-addressed storage for **instances** | The **built structures** that exist **according to** specs — bricks laid to plan, not the plan itself. |

Normative detail and invalidation contracts: [SPEC_ORIGIN_PLANE.md](./SPEC_ORIGIN_PLANE.md) (especially **Streams, summaries, and data cells** and **Compatibility with data cells and streams**).

End-to-end **origination** story (spec → storages, lifecycle, access, CLI — roadmap): [DATA_ORIGINATION_PIPELINE_VISION.md](./DATA_ORIGINATION_PIPELINE_VISION.md).

---

## Practical call-out: two unhealthy states, two different fixes

**Scenario A — Spec / index drift**  
Someone edits **`object_specs`** YAML by hand (or merges a branch) but **does not** refresh **`spec_index.json`** or regenerate dependent materializations. Symptom: completions, drift analyzers, or scenario apply order **disagree** with what is on disk; “impossible” validation or missing fields in tooling.

**Fix direction:** Regenerate or use **`zqk system update-specs`** / **`generate-spec-index`** / builders as appropriate; align with **SPEC_ORIGIN_PLANE** invalidation table.

**Scenario B — Stream / summary lag**  
Test bundles append to **`.zqk/logs/scheduler/cvs/test-bundles/health.jsonl`**, but the **stream summary** file (e.g. **`test_bundle_health.json`**) is stale or missing. Symptom: **`zqk system health-data` (PRUNED)** or dashboards show **old line counts / timestamps**; the organism still has a pulse in the raw log, but the **summarized “vital”** lies.

**Fix direction:** Run the documented refresh path for that stream (e.g. **`write_test_bundle_stream_summary`** for the pilot in [DATA_STREAM_SUMMARY_PILOT.md](./DATA_STREAM_SUMMARY_PILOT.md)); keep **`.zqk/stream_summary/`** layout bounded per [FILESYSTEM_DATA_LAYOUT.md](./FILESYSTEM_DATA_LAYOUT.md).

Same word “unhealthy,” **different layer**: A is **instruction / read-model** consistency; B is **operational telemetry / materialized view** freshness. Teaching that distinction early prevents “edit YAML, why didn’t the stream move?” confusion.

---

## Analogy: DNA, circulation, and systemic stress

| Idea | Rough mapping | Caution |
|------|----------------|---------|
| **DNA** | **object_specs** (+ lifecycles, traits, codegen outputs) — low-level instructions for what kinds **may** exist and how fields behave | Specs **evolve** with migrations and tooling; unlike DNA, we **intentionally** revise the “genome” — revision and materialization matter ([SPEC_ORIGIN_PLANE.md](./SPEC_ORIGIN_PLANE.md)). |
| **Cell shape, uptake, tissue cooperation** | **Data cells / membranes / streams** — high-volume paths, summaries, coordination at boundaries | No single stream is usually **fatal**; **many** stuck or inconsistent surfaces degrade trust and automation like **circulation** problems affecting the whole body. |
| **Pathogen / metabolic crisis** | Widespread inconsistency: stale indexes, broken summaries, forked truth between payload and spec | Recovery is **layered**: fix schema plane **and** fix transport/materialization — not one magic reboot. |

**Digital bricks:** Persisted objects and CAS shards are **structures built to spec** — stable only when the **plan** (specs) and **environment** (streams, layout, jobs) stay coherent.

---

## Using (and extending) the metaphor

- **Good use:** Onboarding slides, ADRs, glossaries — “we’ll use DNA/circulation language consistently in this program.”
- **Risky use:** Letting the metaphor **replace** checklists — e.g. “the cell is sick” without naming **which artifact** (spec index, JSONL, summary file, scheduler job).

When you draw a **new** parallel (immune system, hormones, repair), tie it to a **named** technical surface in [SPEC_ORIGIN_PLANE.md](./SPEC_ORIGIN_PLANE.md), [STREAM_STORAGE.md](./STREAM_STORAGE.md), or [CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md](./CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md) so it stays **falsifiable**.

---

## Related documentation

| Topic | Document |
|-------|----------|
| **Data cell model** (logical unit, profiles, multi-kind cells) | [DATA_CELL_MODEL.md](./DATA_CELL_MODEL.md) |
| Spec revision, derived indexes, stream orthogonality | [SPEC_ORIGIN_PLANE.md](./SPEC_ORIGIN_PLANE.md) |
| Stream storage layout | [STREAM_STORAGE.md](./STREAM_STORAGE.md) |
| Test-bundle health stream + summary pilot | [DATA_STREAM_SUMMARY_PILOT.md](./DATA_STREAM_SUMMARY_PILOT.md) |
| Membrane / cell / organelle vocabulary (CLI) | [CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md](./CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md) |
| Audible / metaphor hooks for orchestration | [AGENT_ORCHESTRATION_AUDIBLE.md](./AGENT_ORCHESTRATION_AUDIBLE.md) |
| Pre-change habits (specs, streams, glossary) | [PRE_CHANGE_CHECKLIST.md](./PRE_CHANGE_CHECKLIST.md) |
| Spec → full data surface (target pipeline) | [DATA_ORIGINATION_PIPELINE_VISION.md](./DATA_ORIGINATION_PIPELINE_VISION.md) |
