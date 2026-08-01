# Semantic kernel and the CLI façade

**Status:** Concept (architecture). Process traceability: `doc_entry` **DOC-EXAMPLE**; `glossary_term` **GLS-EXAMPLE** (via `zqk object create`).

## One-line idea

**ZQK is a semantic kernel:** a stable core of meaning (object kinds, lifecycles, validation, storage, scheduler, observability) that humans and agents address through a **thin, spec-first command surface**—the CLI as **façade**, not the whole system.

## Why “semantic kernel”

A traditional OS kernel mediates hardware and processes. Here the “hardware” is **persisted objects and streams**; the “processes” are **workflows** (scheduler jobs, scenarios, convergence). The **semantic** layer is what makes those pieces **speak the same language**: specs, enums, references, and the **object graph** so that behavior stays coherent as the platform grows.

The project name **ZQK** doubles as a mnemonic—**zen quantum kernel**: a small, calm center that **collapses** many possible behaviors into **deterministic, inspectable** operations (commands, validations, audits). That is not marketing fluff; it is a **design metaphor** for how the system should feel: small surface, deep coherence.

## CLI as façade

The CLI does not duplicate business logic in ad hoc scripts. It **routes** to:

- **Spec-backed objects** (builders, validation, lifecycle).
- **Declarative command specs** (`.zqk/cli/specs/…` + generated command builders).
- **Scheduler and observability** where work is long-running or needs traceability.

New verbs should start as **YAML command specs** and codegen (`zqk system generate-command-builders`), keeping the **declarative contract** visible and reviewable—see `docs/architecture/NEW_COMMAND_OBJECT_ORIGINATION.md` for the `zqk new` pipeline.

## Vocabulary and the evolving mesh

The knowledge base is not a pile of markdown files alone. It is a **mesh** of:

- **Glossary terms** and **policies** (shared vocabulary).
- **Doc entries** (stable pointers into the repo’s narrative docs).
- **Requirements, criteria, backlog** (traceability).
- **Scheduler jobs and metrics** (what actually ran and what it proved).

Together, that mesh behaves less like a static wiki and more like an **organism**: local changes propagate through validation, indexing, and reporting; the system **remembers** what was decided and what was executed.

## Related reading

- `docs/architecture/NEW_COMMAND_OBJECT_ORIGINATION.md` — spec-first origination (`zqk new`).
- `docs/architecture/PRE_CHANGE_CHECKLIST.md` §1 — process instance data via CLI only.
- `docs/architecture/SCS_FACADE_METRICS_EXCHANGE_CONTRACT.md` — facade-level metrics contract and config-object pattern.
- `docs/onboarding/AI_AGENT_ONBOARDING.md` — discovering `doc_entry` and `glossary_term`.
