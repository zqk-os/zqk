# CLI membrane and system anatomy (cells, organelles, systems)

**Last Verified:** 2026-08-31


**Status:** Architecture (vocabulary + current vs target)  
**Audience:** CLI designers, convergence orchestration, agents automating cross-component workflows.

## Purpose

Describe a **shared vocabulary** for how the `zqk` CLI exposes **routing** (what runs), **formatting** (how results are serialized or rendered), and **delivery** (clipboard, HTTP, streams) so that **distinct components** can collaborate through **stable, event-aware surfaces** instead of ad hoc strings. This aligns with nested **convergence_session** work, **JSONL** evidence lines, and the **spec origin plane** (canonical specs → derived indexes).

## Biological analogy (domain vocabulary)

| Term | Meaning in this codebase | Examples / anchors |
|------|-------------------------|---------------------|
| **Membrane** | The **boundary** where data leaves or enters a component under **explicit contracts**: flags, `schema_version`, append-only logs, HTTP hooks. | **Storage membrane:** data cell coordinator (see glossary **Data cell**, GLS data-cell template). **CLI operational membrane:** global `--format` / `--context`, command-specific renderers, `agent_prompt_runs.jsonl`, `cvs_orchestrate_runs.jsonl`, `rollup_status_core.jsonl`. |
| **Cell** | A **closed collaboration**: one primary **versioned payload** or session plus its **measures** and **one primary output contract** for that scope. | A **convergence_session** (`CVS-*`) with bundle + gate measures; a **data cell** around one storage domain. |
| **Organelle** | Several **cells** (or hybrid units) with a **unified goal**, sharing **surfaces** (e.g. `health.jsonl`, fingerprints, `related_object_refs`) and **rollup** semantics. | **Nested CVS** (parent + children), **convergence-overseer** BFS tree (`pkg/convergerollup.CollectCVSTreeBFS`), multi-session coordination (**CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md** Appendix F). |
| **System** | The full **operational** stack: process objects, scheduler, storage, policy, backlog — **systems** compose **organelles** and shared membranes. | Scheduler + CAS + `scheduler_job`; process **priority plans** and **convergence_session** lifecycles. |

The analogy is **normative for communication**, not a one-to-one map to Go packages. Prefer linking to **glossary_term** (`GLS-*`) and architecture docs when naming these layers in backlog or criteria.

### Membrane tightness (security)

Treat the operational membrane as a **snug orifice**: only command surfaces that are **registered** under `.zqk/cli/specs/` and pass **`zqk system validate-command-specs`** against the built binary are part of the supported contract. Un-spec’d or stray subcommands **widen the aperture**—bootstrap archives, generated builders, and agent-facing help drift from what actually ships, and downstream automation loses a single source of truth. **Spec first**, then `zqk-admin system generate-command-builders --overwrite`, then wire `RunE` in `cmd/zqk/`, then run the native validator. Same idea as other fail-closed gates: the membrane stays **selectively permeable**, not leaky.

### CLI DNA source of truth

`.zqk/cli/specs/` is the sole authoring source for command names, arguments, flags,
and help. Generated builders under `pkg/cli/bldr_cli_cmd_v1/` are projections;
`cmd/zqk/` owns runtime behavior and wiring only. Process `command_spec` (`CSPEC-*`)
objects are not an authoring peer and must not be a default code-generation input.
They may become a generated discovery mirror later. This boundary is recorded by
`DEC-1786732826125502000-ef80a104`.

The committed command-coverage baseline is an enforcement debt inventory, not a
second definition source. The native validator permits removal of baseline drift
but fails on any newly exposed command without file DNA or newly orphaned spec.

## Where the code sits today

- **Global serialization:** Root **`--format`** / **`--context`** (json, yaml, table, …) applies broadly; see `cmd/zqk/app/root.go`, internal tests for format coverage.
- **Agent-oriented markdown + delivery** is **not** global today: **`--format agent-prompt`**, **`--copy`**, **`--paste-cursor`**, attention modes, and HTTP delivery live on **`zqk scheduler convergence measure`** and the **`convergence_agent_prompt`** pipeline (`cmd/zqk/scheduler/convergence_agent_prompt.go`, `test_failures_conchestrate.go`).
- **Stream / audit membranes** (append-only JSONL) are **per concern**: `rollup_status_core.jsonl`, `agent_prompt_runs.jsonl`, `cvs_orchestrate_runs.jsonl`, test-bundle `health.jsonl` — each has a **schema** or line shape documented in architecture or command help.
- **Orchestrate shell** (`scripts/cvs_convergence_orchestrate.sh`) composes persist + Python rollup; **Go** validates **`cvs_orchestrate_run_v1`** via **`zqk scheduler record-cvs-orchestrate-run`**.

## Desired end state (direction)

1. **Uniform output facade** — Any command that produces a **structured result** should be able to opt into the same **renderers** (including **agent-prompt**) and **delivery adapters** via shared helpers, not duplicated flags per command.
2. **Versioned payloads** — Every cross-membrane artifact carries **`schema_version`** (or equivalent) so **components** (CLI, scheduler, agent, coordinator) agree on fields.
3. **Glossary as live index** — **Glossary terms** (`glossary_term`, `GLS-*`) should **track** object specs, lifecycles, and config schemas so high-level intent maps to **explicit, configurable** operations. See **SPEC_ORIGIN_PLANE.md** (section *Glossary as a derived index (target)*) and **PRE_CHANGE_CHECKLIST.md** (§3b).

## Related documents

- [CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md](./CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md) — orchestrate path, nested CVS, JSONL.
- [CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md](./CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md) — parity and convergence target.
- [EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md](./EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md) — when to tick, JSONL, object update.
- [SPEC_ORIGIN_PLANE.md](./SPEC_ORIGIN_PLANE.md) — spec revision, derived indexes, **glossary coupling (target)**.
- [DATA_STREAM_SUMMARY_PILOT.md](./DATA_STREAM_SUMMARY_PILOT.md) — stream/cell pilots.

## Glossary terms

Operational terms for this analogy are registered as **`glossary_term`** objects (`GLS-*`). Created with this document (retrieve with `zqk object get <id>`):

| Title | Id |
|-------|-----|
| CLI operational membrane | `GLS-1775714853460470000-de54fd9a` |
| Collaboration organelle | `GLS-1775714854943622000-27c7f216` |
| Glossary–spec coupling | `GLS-1775714856428268000-e11b9c89` |
| Wrapper inner-context guard | `GLS-1775965558831066000-25c5083a` |

Templates for recreating or adapting: **`scripts/templates/glossary_cli_operational_membrane.yaml`**, **`glossary_collaboration_organelle.yaml`**, **`glossary_spec_glossary_coupling.yaml`**, **`glossary_wrapper_inner_context_guard.yaml`**. Storage **Data cell** remains **`GLS-1774504405009664000-c1c04629`** (existing).

**Wrapper types:** Methods on `*cli.Context` that delegate to the embedded `*context.Context` follow **[WRAPPER_INNER_CONTEXT_GUARD_PATTERN.md](./WRAPPER_INNER_CONTEXT_GUARD_PATTERN.md)** (`withInnerContext` + `check-cli-inner-guard.sh`).
