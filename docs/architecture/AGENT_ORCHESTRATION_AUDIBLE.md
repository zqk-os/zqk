# Agent orchestration — priority audible (2026)

**Last Verified:** 2026-08-31


## Intent

**Audible:** elevate **agent orchestration** (programmatic prompt delivery, multi-surface agents, audit trail, convergence integration) ahead of a strict queue so we can **prove** the path early.

**Why now:** Once delivery is **reliable and pluggable**, the project can attach **dedicated sub-agents** (or services) to **logical domains** as data and process objects accumulate—without each integration being a one-off script.

## Metaphor → product mapping

| Metaphor | Product anchor |
|----------|----------------|
| **Circulatory system** — blood carries signal and nutrients | **Streams** (append-only JSONL, health, audit) and **summaries** — see [DATA_STREAM_SUMMARY_PILOT.md](./DATA_STREAM_SUMMARY_PILOT.md); raw streams as “flow,” sidecars as “pressure/readouts.” |
| **Cells** — specialized units | **Data cells** (REQ-DATACELL-001 direction), **domain agents** bound to backlog/workstreams, **object kinds** with clear boundaries. |
| **Immune response** — targeted reaction | **Policies** (POL-), **autofix** / **convergence** when health or validation drifts; not generic spam. |
| **Society** — roles, norms, cohesion | **Mission / vision / goals / milestones**, **priority plans**, **glossary** and **lessons learned** — same traceability chain as [requirements-traceability-system-v1.0.md](../process/architecture/requirements-traceability-system-v1.0.md). |
| **Blackboard / town square** — durable state + one tick’s guidance | **Glossary `GLS-1776410364614477000-e6d4c940`**: **stdout + process objects** (CVS, BLI, criteria, plans) + exit codes; system **narrows** acceptable next steps; agent **chooses** inside the corridor. Toy: `scripts/cursor/math-volley-blackboard.sh`. See [CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md](./CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md) § *Blackboard handoff pattern*, [DATA_CELL_RUNTIME_ORGANISM.md](./DATA_CELL_RUNTIME_ORGANISM.md) narrative table. |

Nothing here replaces CAS or object specs; it **names** how orchestration sits beside **streams and cells** work already in flight.

## Traceability (persistent)

- **Bundle:** `test-scenarios/agent-orchestration-traceability-bundle/agent-orchestration-traceability-bundle.yaml` — `GOAL-AO-001`, **REQ-AO-001**, **CRIT-AO-001**–**006**, **BLI-AO-001**–**002**, **DOC-AO-001**–**003**.
- **Apply** (materialize objects): `zqk-scenario bundle apply -f test-scenarios/agent-orchestration-traceability-bundle/agent-orchestration-traceability-bundle.yaml --project-root .`
- **Reconciliation table:** [SCENARIO_BUNDLES_AND_TRACEABILITY.md](./SCENARIO_BUNDLES_AND_TRACEABILITY.md) (§ *Agent orchestration, prompt delivery, and organizational memory*).

## Code started (implementation spine)

| Piece | Role |
|-------|------|
| `pkg/agentprompt` | Wire format (HTML comment envelope) for paste/queue consumers — **unchanged**. |
| `pkg/agentdelivery` | **Transport** — `Deliverer` interface, file + noop + composite; audit-friendly `Result.DeliveredTo`. Extends to HTTP/MCP/clipboard bridges without coupling to CLI. |

Next steps: wire `zqk scheduler convergence measure` output through `agentdelivery` in a command or job; add a second deliverer (e.g. JSONL audit line); keep **process objects** edited only via `zqk` / bundle apply.

## Principles

1. **Content vs transport** — Prompt body is produced by existing convergence paths; **delivery** is swappable.
2. **Audit** — Every automated handoff leaves a **durable** path (CRIT-AO-002).
3. **No parallel process store** — Missions, requirements, policies, and streams stay **authoritative** in the object store and filesystem layouts already defined.

4. **Blackboard handoff** — Multi-step agent loops use **idempotent** re-invocation, **stdout** for the tick’s guidance, and **structured** completion back into **`zqk`** / handoff files — see **`GLS-1776410364614477000-e6d4c940`**. Chat is not the system of record for progress.
