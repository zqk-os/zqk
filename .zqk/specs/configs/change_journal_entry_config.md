# Change Journal Entry: Lifecycle, Collection, Aggregation, Bucketing

This document defines how **change_journal_entry** (internal object) is configured for lifecycle, collection, aggregation, and storage. The object spec points here for a single definition.

## Lifecycle

- **Definition:** `.zqk/specs/lifecycles/change_journal_entry_lifecycle.yaml`
- **Resolution:** By convention (kind name → `{kind}_lifecycle.yaml`); see `pkg/objects/lifecycle_loader.go`.
- **Statuses:** pending (initial) → completed | failed | aggregated; completed → reverted; any → archived | error.
- **aggregated:** Set by the change_journal_aggregation job after entries are rolled up into an aggregation metric; eligible for compaction or retention cleanup.

## Collection configuration

- **When entries are created:**
  - **Object changes:** On create/update/delete/move (storage layer calls `createChangeJournalEntry`); `change_type` in create, update, delete, import.
  - **Health checks:** On monitor run via `storage.CreateHealthCheckChangeJournalEntry`; `change_type=health_check`, `object_ref=health_monitor:<monitor_id>`.
- **No samplers/gauges/timers:** Unlike base_metric (which has scalar_metric_sampler, list_metric_sampler, collection_count, etc.), change_journal_entry is an append-only event log. Collection is push-based (on change or health run), not pull/sample-based.

## Aggregation strategy

- **Job:** change_journal_aggregation (scheduler job type `change_journal_aggregation`).
- **Behavior:** Query entries in a time window → group by `change_type` → create audit_aggregation_metric → mark entries `status=aggregated`.
- **Compaction (micro-GC):** Separate pipeline compresses old entries (including aggregated) into `.cjournal` artifacts (dictionary + snapshot); can archive/delete originals. Configurable interval to free memory/disk.

## Bucketing / storage strategy

- **Default:** Chronological, monthly (`created_at` → `2006-01`). change_journal_entry is a high-volume kind in `pkg/storage/bucketing_strategy_defaults.go` and in `.zqk/specs/bucketing_strategies/KIND_BASED_STRATEGIES.md`.
- **Retention:** `.zqk/specs/configs/retention_tolerance.yaml` (e.g. archive_after 12h, cleanup_after 72h, max_count 200). Overridable by a `bucketing_strategy` object with `applies_to: [change_journal_entry]` and `retention_tolerance`.

## Difference from metrics

Metrics (base_metric, scheduler_health_metric, etc.) use:

- **Samplers:** scalar_metric_sampler, list_metric_sampler, status_history_metric_sampler (gauges, timers, list aggregation).
- **Collection:** Pull/sample-based (metrics collection job reads objects and produces metric instances); `collection_count`, `aggregation_window_start/end`, etc.

Change_journal_entry is event-log style: append on occurrence (object change or health run), then aggregate by time window and compact. No sampler config; collection is defined by “who calls CreateChangeJournalEntry / CreateHealthCheckChangeJournalEntry.”
