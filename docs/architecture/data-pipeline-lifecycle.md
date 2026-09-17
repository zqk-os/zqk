# Standardized data pipeline lifecycle

**Last Verified:** 2026-08-31

**Version:** 0.2  
**Status:** Active (canonical)  
**Scope:** Reusable pipeline contract for system flows (streams, metrics, audit/change events, validation/autofix, scheduler jobs). Implementations in `pkg/pipeline` and pilot flows (e.g. autofix batch) follow this contract.

## One-line model

\[
\textbf{INGEST} \rightarrow \textbf{NORMALIZE} \rightarrow \textbf{DECIDE} \rightarrow \textbf{COMMIT} \rightarrow (\textbf{PARK}) \rightarrow (\textbf{AGGREGATE}) \rightarrow (\textbf{TRIGGER}) \rightarrow \textbf{FINALIZE}
\]

## Legend (shared vocabulary)

- **Envelope**: Immutable wrapper carrying payload + metadata: `trace_id`, `source`, `received_at`, `idempotency_key`, `partition_key`. See "Minimal implementation surface" below.
- **Stage**: A named step with explicit input/output and a bounded set of allowed side effects.
- **Outcome**: Structured result of a stage (success, skipped, parked, retriable, fatal) plus diagnostics.
- **Idempotency key**: Stable key ensuring retries do not double-apply effects.
- **Partition key**: Key used for ordered processing *per key* (not global ordering).

## Stage contract (succinct)

| Stage | Allowed effects | Purpose | Output |
|------:|------------------|---------|--------|
| **INGEST** | I/O read only | Acquire input + attach metadata (source, received_at, trace_id, idempotency_key) | `Envelope<Raw>` |
| **NORMALIZE** | None (pure) | Canonicalize encodings and types (timestamps, schema_version, YAML typing) | `Envelope<Norm>` + diagnostics |
| **DECIDE** | None (pure) | Compute intended actions (writes, park policy, triggers) | `Plan` |
| **COMMIT** | Authoritative writes only | Apply the plan with idempotency + optimistic locking | `CommitResult` |
| **PARK** *(opt)* | Hot storage write | Persist recoverable input/result for replay or later processing | `ParkRef` |
| **AGGREGATE** *(opt)* | Aggregate state write | Update summaries/counters (count, uniq, percentiles, group-by) | `AggregateResult` |
| **TRIGGER** *(opt)* | Side-effects only | Fan-out notifications/callbacks/jobs; must be idempotent | `FanoutResult` |
| **FINALIZE** | Cleanup only | Archive/delete/close resources; update caches as needed | `FinalState` |

## Invariants (non-negotiable)

- **Normalize is pure**: no storage writes, no scheduler jobs, no notifications.
- **Commit is authoritative**: only COMMIT mutates canonical domain state.
- **Idempotency is required** for COMMIT/TRIGGER/AGGREGATE when retries are possible.
- **Ordering is per partition key** when required; never assume global ordering.
- **Backpressure is explicit**: PARK or shed-load policies are chosen, not accidental.
- **Errors are classified**:
  - **InvalidInput** (drop/reject)
  - **NonDeterminable** (needs human/data; park + signal)
  - **Transient** (retry with backoff)
  - **Conflict** (re-read/optimistic-lock retry)
  - **InvariantViolation** (stop; bug)

## Why this exists

The system has multiple high-volume and/or schema-driven flows. A shared lifecycle prevents “same bug, new encoding” issues, reduces ambiguity about where side effects are allowed, and makes observability consistent (per-stage outcomes instead of ad-hoc logs).

## Minimal implementation surface (target)

- `Envelope<T>` with metadata: `trace_id`, `source`, `received_at`, `idempotency_key`, `partition_key`.
- `PipelineBuilder` (functional/builder style) to wire stages with explicit side-effect boundaries.
- Per-stage `Outcome` with standardized fields for logging/metrics.

## Implementation alignment (`pkg/pipeline`)

The reference implementation lives in **`pkg/pipeline`**. It provides:

- **`Builder`** / **`AddStage(name, fn)`** / **`Build()`** — compose a named pipeline (`kind`) with ordered stages.
- **`Pipeline.Run(ctx, payload)`** — executes stages **strictly in order**; each stage’s output becomes the next stage’s input.
- **`Pipeline.Context`** — per-run execution state: `Ctx` (standard `context.Context`), `IdempotencyKey`, `PartitionKey`, and `Outcome` (map for stage outputs). Callers should pass a **dedicated** `*pipeline.Context` per invocation (or let `Run` fill defaults when `nil` is acceptable).
- **`MetricsConfig`** — optional per-run contract: a **`MetricsSink`** plus a **`BucketingStrategy`** that produces bounded label dimensions (e.g. `partition_key`, `idempotency_key`) for breaking down per-stage telemetry. If unset on the built `*Pipeline`, **`Run` short-circuits** and does not emit per-stage metrics. If **`Sink` is non-nil** and **`Strategy` is nil**, **`Build`** resolves **`Strategy`** to **`StandardPipelineBucketing`**. **Call-site pattern:** use **`NewBuilder(kind, logger).WithMetricsConfig(...)`** everywhere — logger-backed per-stage telemetry: **`WithMetricsConfig(DefaultMetricsConfig(logger))`**; noop / high-frequency paths: **`WithMetricsConfig(&MetricsConfig{Sink: noopSink, Strategy: NoopBucketing{}})`**. **`NewInstrumentedBuilder(kind, logger)`** is equivalent to **`NewBuilder`** + **`DefaultMetricsConfig`** (shorthand). Legacy **`WithMetrics(sink)`** sets a noop **`BucketingStrategy`** only.
- **`METRICS_RESOLVE`** — when metrics are enabled, **`Build`** prepends a reserved stage with this name. It runs the bucketing strategy once (preflight) and sets **`Outcome["metrics_resolve"]`**; it does **not** emit a per-stage metrics row (so sinks never see a duplicate “stage” for this step). Label maps passed to **`BucketingMetricsSink`** are normalized via **`NormalizeBucketLabels`** after **`Buckets()`** (trim, cardinality cap, safe key characters).
- **`MetricsSink`** / **`BucketingMetricsSink`** — per-stage timing and failures; sinks that implement **`BucketingMetricsSink`** receive bucket maps from the strategy.

This doc’s **lifecycle names** (INGEST, NORMALIZE, …) are the **contract**; `pkg/pipeline` does **not** enforce stage names at compile time—wire stages that match this contract and your pilot’s needs.

## In-repo pilots (examples)

- **Test bundle matrix** (`quality.test_bundle_matrix` in `pkg/quality`): regenerates `docs/quality/TEST_BUNDLE_MATRIX.csv`, runs verify, optional convergence trigger. User entry: **`zqk system test-bundle-matrix` (PRUNED)**. For scan-tests plus wait-for-health before regen, use **`scripts/test_bundle_matrix_pipeline.py`**. Optional timer **`scheduler_job`** template: **`scripts/scheduler_jobs/test_bundle_matrix_regen_daily.yaml`**.

## Concurrency and thread-safety

- **`Pipeline.Run` is sequential** — there is **no** parallel execution of stages inside the runner (no goroutines in the runner).
- A **built `*Pipeline`** is safe to **read** from multiple goroutines: it does not mutate during `Run`.
- **Each `Run` should use its own** `*pipeline.Context` (or equivalent isolation). `Context` is **not** synchronized; it is **per execution**, not a shared bus across concurrent runs.
- **Thread-safety of stage functions** is the author’s responsibility: `StageFunc` closures must not mutate **shared** unsynchronized state. Safe patterns: capture only immutable config, or use synchronized storage; avoid hidden globals in stages.

So: **concurrent calls to `Run` on the same `*Pipeline` are fine** when stages are side-effect-safe and each invocation uses a **distinct** `*pipeline.Context`.

## Fan-out, fork, and variants

- The lifecycle places **fan-out** in the **TRIGGER** stage (notifications, callbacks, scheduler jobs). The **runner** does not implement a graph or parallel fork: **fan-out is implemented inside a stage** (or by a separate orchestrator that TRIGGER invokes).
- **Patterns for multiple variants**:
  - **Separate pipelines** per variant (different `AddStage` chains), sharing helpers.
  - **One linear pipeline** with a **DISPATCH** (or similar) stage that branches on envelope metadata and calls shared helpers—still one sequential chain at the runner.
  - **TRIGGER** enqueues work (jobs, channels) so downstream work runs **after** COMMIT, with idempotency keys preserved.

**Partition keys** remain the right way to express **ordered processing per key** when many parallel consumers exist; they do not imply global ordering.

## `StageFunc` and typing

- Stages are **`StageFunc func(*Context, any) (any, error)`** — **generic `any`** on purpose so pilots can evolve without regenerating types.
- **Tradeoff:** flexibility and reuse across flows; **correctness** must be enforced with **typed wrappers** at boundaries and tests. Prefer thin adapters that map `any` → concrete types at each stage edge when a pilot stabilizes.

## See also

- [DECLARATIVE_LAUNCH_AND_MATRIX_LOOP.md](./DECLARATIVE_LAUNCH_AND_MATRIX_LOOP.md) — product launch as **variable inputs / staged outputs**, routed by **measurable outcomes** (same lifecycle vocabulary; GTM URLs stay non-authoritative until promoted).
