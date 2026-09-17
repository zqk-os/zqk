# CEF: event path decision and globals inventory

**Last Verified:** 2026-08-31


**TRACK:** `BLI-CEF-ARCH-EVENTS-GLOBALS` / `REQ-CEF-ARCH-002` / `CRIT-CEF-ARCH-002A`  
**Status:** Accepted decision (2026-08-17)  
**Related:** [EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md](./EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md), CEF Agent A finding **F-ARCH-006**

## Decision (single recommended event path)

| Concern | Use | Do not use for |
|---------|-----|----------------|
| **In-process fan-out** (scheduler ticks, metrics/audit routers, lifecycle-adjacent Emit) | **`pkg/coordination`** (`GetGlobalCoordinator` / Emit routers) | New ad-hoc channels or a second global bus |
| **Durable, sparse, machine-parseable evidence** across runs (JSONL under `.zqk/metrics/`) | **`pkg/contextevents`** (`context_events.jsonl`) | Coupling callers to coordinator globals for evidence-only writes |
| **Request/context pipeline hooks** (pending→processing→completed listeners) | **`pkg/context.ContextListenerRegistry`** | Cross-process or durable audit; expanding into a general PubSub |

**Rule of thumb:** prefer **causal edges** (something happened → named listener) over new timers ([EVENT_PIPELINE…](./EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md)). When adding a signal, pick **one** row above; do not invent a fourth bus.

### Why this split

- **`pkg/coordination`** already has the largest production footprint (~200 call sites for coordinator/Emit patterns) and owns unified metrics / audit routing used by scheduler and CLI.
- **`pkg/contextevents`** is explicitly append-only evidence (POL-OBS-001 style), not in-process fan-out — keep it for matrix/criteria/operator notes.
- **`ContextListenerRegistry`** is a small, purpose-built registry for **context object state** transitions (~3 production references). Expanding it into a general event bus would recreate F-ARCH-006.

### Migration posture

- **New code:** follow the table; link this doc in package comments when touching event surfaces.
- **Existing dual use:** leave working call sites; do not mass-rewire in CEF S5. Tranche work removes *new* ambiguity, not every historical Emit.

## Globals inventory (CEF snapshot)

High-frequency symbols (non-test `pkg/` + `cmd/` scan, 2026-08-17). These are **not** all forbidden — they are the coupling surface F-ARCH-003 called out.

| Symbol / pattern | Role | Notes |
|------------------|------|-------|
| `DefaultBudget` / `goroutinelabels` budget | Process goroutine cap | Prefer pool + budget over raw `go` |
| `GetGlobalRegistry` / `globalRegistry` | Spec/builder registries | Spec plane; warm at process start |
| `GetGlobalSpecLoader` / lifecycle / field / kind mappers | Ontology loaders | Cache; avoid reload on hot path |
| `GetGlobalHighVolumeEventCache` | HV event indexes | Storage observability |
| `globalCoordinator` / `GetGlobalCoordinator` | **Recommended in-process bus** | See decision table |
| `globalListCache` / listing index write queue | List/count performance | Incremental invalidation only |
| `GetGlobalShutdownCoordinator` | Process shutdown | Keep single |
| `globalObjectIDCache` (storage) | ID→path index | Test isolation must reset via teardown helpers |
| `ContextListenerRegistry` | Context pipeline | Keep narrow |

**Test isolation:** prefer `testkit` / `RunProjectTestTeardown` over inventing new global reset hooks. New globals need an explicit reset path or constructor injection.

## Acceptance evidence (CRIT-CEF-ARCH-002A)

- This document is the **documented decision** among coordination / ContextListenerRegistry / contextevents.
- Package docs in `pkg/coordination`, `pkg/contextevents`, and `pkg/context` point here.
