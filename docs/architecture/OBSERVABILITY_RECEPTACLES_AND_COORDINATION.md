# Observability receptacles and coordination

**Last Verified:** 2026-08-31


This note fixes vocabulary for **how** the system exposes measurements and **where** they land, without requiring heartbeat-style aggregation for every subsystem or user dataset.

## Observability receptacle

A **receptacle** is a **narrow, stable emission surface** where code attaches measurements. Examples:

- **`coordination.EventContext`** — carries per-emission **channel flags** (`EmitLogging`, `EmitAudit`, `EmitMetrics`, `EmitOperational`) so one observation can be routed to logging only, audit only, metrics only, or a combination. See `pkg/coordination/event_context.go` and `Coordinator.Emit` in `pkg/coordination/coordinator.go`.
- **`pkg/observability.Recorder`** — `Record(operation, builder)` for structured fields, tags, duration, and errors; implementations decide persistence.
- Scheduler hooks that publish **coordination** or **audit**-shaped events after real state changes (sparse signals, not periodic rollups by default).

Receptacles stay **thin**; policy and storage stay in **routers** and **pipeline** configuration.

## Configurable observers (routers)

The **observer** is not a single daemon; it is the **set of configured routers** behind the coordinator:

- **Audit** — e.g. `StorageAuditRouter` → `CreateAuditEventWithBuilder` / audit trail semantics.
- **Metrics** — e.g. `MetricPipelineRouter` → `MetricPipeline` (sampling, batching, volume-aware behavior).
- **Operational** — subscribers on the operational channel for coordination-style outcomes.

Swapping or tuning these implementations changes **how** measurements become durable artifacts, without rewriting every call site.

## Coordinator

The **coordinator** (`pkg/coordination`) composes an **emit pipeline** that runs the configured routers. Channel selection is explicit on `EventContext`, so emission is **event-driven** and **scoped**, not a global heartbeat.

## Scheduler transceiver

The **transceiver** (`pkg/scheduler/transceiver`) sits beside coordinator concerns for the scheduler: **protocol adapters** (`Router`, `RegisterDefaultAdapters`) and **async delivery** (`AsyncRouter`) so observation and command paths can respect **delivery semantics** (async, bounded) separate from **what** is measured.

Together, coordinator + transceiver let **outcome events** match **characteristics** of the data (audit vs high-volume metrics vs operational coordination) without conflating them.

## Storage and characteristics

Different channels intentionally map to **different persistence patterns** (audit events vs metric pipeline vs logs). Choose channels and routers for **origination, growth, mutation, termination, and preservation** demands of the signal—not every signal needs every channel.

## Related code

| Concept | Primary location |
|--------|------------------|
| Event context and channel flags | `pkg/coordination/event_context.go` |
| Emit pipeline stages | `pkg/coordination/coordinator_emit_pipeline.go` |
| Metric pipeline router | `pkg/coordination/routers.go` (`MetricPipelineRouter`) |
| Observability recorder API | `pkg/observability/recorder.go` |
| Transceiver router | `pkg/scheduler/transceiver/router.go`, `async_router.go` |
| Unified metrics + coordinator bridge | `pkg/coordination/metrics_collector_adapter.go`, `pkg/storage/unified_metrics_collector_example.go` |

## Process traceability

Glossary terms (`glossary_term`) for this vocabulary use **`machine_hints`** JSON with `primary_doc_entry` and `path` so agents and tooling resolve **canonical definitions** to this document and the registered `doc_entry`.

| Object | ID |
|--------|-----|
| doc_entry (this document) | `DOC-1775094665414254000-3c45a697` |
| glossary_term — Observability receptacle | `GLS-1775094698968884000-d73a1e85` |
| glossary_term — Coordinator event emission | `GLS-1775094700736406000-b51d7612` |
| glossary_term — Scheduler transceiver | `GLS-1775094702525421000-b2c2d2c5` |
