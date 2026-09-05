# Data pipeline pilot selection

**Last Verified:** 2026-08-31


**Version:** 0.1  
**Status:** Active  
**Related:** [Standardized data pipeline lifecycle](./data-pipeline-lifecycle.md), [REDACTED-ID] (Select pilot flow for pipeline adoption).

## Pilot: Autofix batch processing

**Selected flow:** Autofix batch processing (system check auto-fix / scheduler batches).

**Rationale:** Clear entry points (CLI and scheduler), existing batch boundary (one AUTOFIX-*.json file = one batch), and alignment with system-check pipeline. Well-understood flow with ingest (read batch file), normalize (parse, validate), decide (which fixes to apply), and commit (apply fixes to storage).

---

## What / where

| Item | Value |
|------|--------|
| **Pilot name** | `autofix_batch` |
| **Entry points** | (1) **CLI:** `zqk system check --auto-fix` (and related flags) → produces fixable issues → `handleAutoFixBatching` creates batches and submits to scheduler. (2) **Scheduler:** Job `SCH-autofix-process-pending` runs `zqk system auto-fix-process-pending`; per-batch jobs process individual AUTOFIX-*.json files under `.zqk/autofix/`. |
| **Input** | One AUTOFIX-*.json file (batch of fixable issues: object ID, kind, issue type, fix payload). |
| **Output** | Applied fixes (object updates via storage), optional park for retry, and finalize (cleanup/mark batch done). |
| **Code locations** | `cmd/zqk/system/auto_fix_batch_cmd.go`, `auto_fix_batch_handler.go`, `auto_fix_scheduler_batch.go`; `pkg/pipeline/pipeline.go` (existing builder). |

---

## Envelope schema (minimal)

Per [data-pipeline-lifecycle.md](./data-pipeline-lifecycle.md), each batch run carries:

| Field | Type | Purpose |
|-------|------|---------|
| `trace_id` | string | Single id for the batch run (e.g. UUID or job_id + batch_file basename). |
| `source` | string | `cli` or `scheduler`; job_id when from scheduler. |
| `received_at` | timestamp | When the batch was accepted (ISO8601). |
| `idempotency_key` | string | See below. |
| `partition_key` | string | See below. |

Payload: parsed batch (list of fixable issues). Envelope is immutable once created at INGEST.

---

## Idempotency key and partition key

| Key | Definition | Rationale |
|-----|------------|-----------|
| **Idempotency key** | `autofix_batch:<project_root_normalized>:<batch_file_basename>` or, when from scheduler, `autofix_batch:<job_id>:<batch_file_basename>`. Same key on retry → same batch file → re-applying fixes must be idempotent (object update with same content is no-op or use conditional write). | Ensures scheduler or CLI retries do not double-apply the same batch. |
| **Partition key** | Per-issue: `object_id` (the object being fixed). For the batch envelope: `project_root` or `job_id` so all batches for the same job/process are ordered if needed; within a batch, apply fixes in order by `object_id` to avoid concurrent writes to the same object. | Ordered processing per object; avoids conflicting concurrent fixes to the same object. |

**Explicit definitions for pilot:**

- **Idempotency key:** `autofix_batch:{project_root}:{AUTOFIX-*.json basename}` (e.g. `autofix_batch:/repo/zqk:AUTOFIX-abc123.json`). When processing is triggered by a scheduler job, include job_id: `autofix_batch:{job_id}:{basename}`.
- **Partition key (batch level):** `project_root` for ordering batches per project; **(stage-level)** for COMMIT stage, process issues in order by `object_id` (or use a per-object partition key for parallel commit with one writer per object).

---

## Migration success criteria

1. **Behavior parity:** After migration to the standardized pipeline (INGEST → NORMALIZE → DECIDE → COMMIT → [PARK] → FINALIZE), output of autofix batch processing matches current behavior (same objects updated, same errors reported, no double-apply on retry).
2. **Idempotency:** Re-running the same batch (same idempotency key) does not double-apply fixes (achieved via conditional writes or no-op when state already matches).
3. **Observability:** Per-stage outcomes (see below) are emitted and available in logs/metrics.
4. **Documentation:** This pilot selection doc and the lifecycle spec are the source of truth; code comments or runbooks reference them.

---

## Observability plan

| Stage | Outcome / metrics |
|-------|-------------------|
| **INGEST** | `autofix_batch.ingest.count` (batches read), `autofix_batch.ingest.errors` (read/parse failures). |
| **NORMALIZE** | `autofix_batch.normalize.count`, `autofix_batch.normalize.skipped` (invalid entries). |
| **DECIDE** | `autofix_batch.decide.count` (issues to fix), `autofix_batch.decide.parked` (if any). |
| **COMMIT** | `autofix_batch.commit.success`, `autofix_batch.commit.conflict`, `autofix_batch.commit.fatal` (per batch or per object as needed). |
| **PARK** (optional) | `autofix_batch.park.count` when batch/issue is parked for retry. |
| **FINALIZE** | `autofix_batch.finalize.count` (batches completed). |

Logs: structured log per stage with `trace_id`, `stage`, `duration`, `outcome` (success, skipped, retriable, fatal). Existing logger from context; use `pkg/pipeline` MetricsSink or equivalent so pipeline runs record stage timing and outcome.

---

## Migration (before/after)

- **Before:** Autofix batch was a linear flow in `runAutoFixBatch`: read file → parse → `ProcessBatch` → update file, emit events, rename. No shared Envelope or named stages.
- **After:** Single-batch path (`zqk system auto-fix-batch --batch-file ...`) and process-pending path both use `runOneBatchViaPipeline`, which builds a pipeline with stages INGEST → NORMALIZE → COMMIT → FINALIZE. `pkg/pipeline.Envelope` carries trace_id, idempotency_key, partition_key; per-stage outcomes are set in `Context.Outcome` and recorded via `MetricsSink.RecordStage`. Behavior (writes, rename, metrics, audit) is unchanged; observability is improved (per-stage timing and outcome keys).

## Next steps (post–pilot selection)

1. ~~**Adopt data pipeline lifecycle spec + legend**~~ Done: [data-pipeline-lifecycle.md](./data-pipeline-lifecycle.md) is canonical; `pkg/pipeline` package doc references it.
2. ~~**Build pipeline API + migrate one pilot flow**~~ Done: Shared API (`Envelope`, `Builder`, `Run`, unit tests); autofix_batch migrated; per-stage outcomes observable.
3. ~~**Migrate second client flow: scheduler events aggregation**~~ Done: SCH-evag (scheduler_events_aggregation) runs via `RunAggregationViaPipeline` in `pkg/scheduler/events_aggregation_pipeline.go` (INGEST → NORMALIZE → COMMIT → FINALIZE); handler calls it from `SchedulerEventsAggregationHandler.Execute`.
4. ~~**Migrate third client flow: change journal aggregation**~~ Done: Scheduler job type `change_journal_aggregation` runs via `RunChangeJournalAggregationViaPipeline` in `pkg/scheduler/change_journal_aggregation_pipeline.go` (INGEST → NORMALIZE → COMMIT → FINALIZE). Handler calls it from `ChangeJournalAggregationHandler.Execute`. INGEST parses job config (batch size, window, retention); NORMALIZE does health check, optional aggressive cleanup at 3k limit, and `AggregateChangeJournalEntries`; COMMIT does optional delete-after-aggregation, cleanup old aggregated metrics, and retention-by-age.
5. ~~**Migrate fourth client flow: audit event aggregation**~~ Done: Scheduler job type `audit_event_aggregation` runs via `RunAuditAggregationViaPipeline` in `pkg/scheduler/audit_aggregation_pipeline.go` (INGEST → NORMALIZE → FINALIZE). Handler calls it from `AuditAggregationHandler.Execute`. NORMALIZE runs `executeAuditAggregationCore` (health check, CAS recovery, cache build, retention, catch-up, aggressive/proactive cleanup, aggregation, post-cleanup, retention second pass). CLI context is set in pipeline before Run.
6. ~~**Migrate fifth–ninth: cleanup/retention flows**~~ Done: aggregation_metrics_cleanup, generic_metrics_cleanup, retention_tolerance, scheduler_job_retention, autofix_batch_cleanup each run via `Run*ViaPipeline` (INGEST → NORMALIZE → FINALIZE); handlers call pipeline; core logic in `execute*Core`.

---

## Pipeline candidate list (all scheduler job types)

| Job type | Handler | Pipeline status | Notes |
|----------|---------|-----------------|-------|
| (CLI) autofix_batch | (batch handler) | Done | `runOneBatchViaPipeline` |
| scheduler_events_aggregation | SchedulerEventsAggregationHandler | Done | `RunAggregationViaPipeline` |
| change_journal_aggregation | ChangeJournalAggregationHandler | Done | `RunChangeJournalAggregationViaPipeline` |
| audit_event_aggregation | AuditAggregationHandler | Done | `RunAuditAggregationViaPipeline` |
| aggregation_metrics_cleanup | AggregationMetricsCleanupHandler | Done | `RunAggregationMetricsCleanupViaPipeline` |
| generic_metrics_cleanup | GenericMetricsCleanupHandler | Done | `RunGenericMetricsCleanupViaPipeline` |
| retention_tolerance | RetentionToleranceHandler | Done | `RunRetentionToleranceViaPipeline` |
| scheduler_job_retention | SchedulerJobRetentionHandler | Done | `RunSchedulerJobRetentionViaPipeline` |
| autofix_batch_cleanup | AutofixBatchCleanupHandler | Done | `RunAutofixBatchCleanupViaPipeline` |
| cache_prewarm | CachePrewarmHandler | Done | `RunCachePrewarmViaPipeline` (INGEST → NORMALIZE → FINALIZE) |
| lifecycle_check | LifecycleCheckHandler | Done | `RunLifecycleCheckViaPipeline` (INGEST → NORMALIZE → FINALIZE) |
| maintenance | MaintenanceRequestHandler | Done | `RunMaintenanceViaPipeline` (INGEST → NORMALIZE → FINALIZE) |
| metrics_collection | (varies) | Done | `RunMetricsCollectionViaPipeline` (INGEST → NORMALIZE → FINALIZE) |
| cache_invalidation | CacheInvalidationHandler | Done | `RunCacheInvalidationViaPipeline` (INGEST → NORMALIZE → FINALIZE) |
| integrity_check | IntegrityCheckHandler | Done | `RunIntegrityCheckViaPipeline` (INGEST → NORMALIZE → FINALIZE) |
| cascade_update | CascadeUpdateHandler | Done | `RunCascadeUpdateViaPipeline` (INGEST → NORMALIZE → FINALIZE) |
| operation_execution | OperationExecutionHandler | Done | `RunOperationExecutionViaPipeline` (INGEST → NORMALIZE → FINALIZE) |
| object_validation | ObjectValidationHandler | Done | `RunObjectValidationViaPipeline` (INGEST → NORMALIZE → FINALIZE) |
| run_wrapper | RunWrapperHandler | Done | `RunRunWrapperViaPipeline` (INGEST → NORMALIZE → FINALIZE) |
| callback_listener | CallbackListenerHandler | Done | `RunCallbackListenerViaPipeline` (INGEST → NORMALIZE → FINALIZE) |
| test_io | TestIOHandler | Done | `RunTestIOViaPipeline` (INGEST → NORMALIZE → FINALIZE) |
| context_refresh | ContextRefreshHandler | Done | `RunContextRefreshViaPipeline` (INGEST → NORMALIZE → FINALIZE) |
| cleanup | CleanupConfigHandler | Done | `RunCleanupConfigViaPipeline` (INGEST → NORMALIZE → FINALIZE) |
