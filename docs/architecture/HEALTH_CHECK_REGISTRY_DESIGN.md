# Health Check Registry (Monitor/Beacon) Design

**Last Verified:** 2026-08-31


**Status:** Design + minimal implementation  
**Related:** SCHEDULER_EVENTS_AND_METRICS.md, object validation job (SCH-val), scheduler events aggregation (SCH-evag)

## Goal

- Abstract health checks into **pluggable monitors** that implement a common interface and are **auto-registered**.
- Provide a **registry** (like regedit) for listing, enabling/disabling, and bulk-operating on monitors.
- Keep monitors **separate from base objects**: different caches, non-transactional, but can be **source triggers for alert notifications** when a check fails or degrades.
- CLI: **`healthchk`** with subcommands similar to object/internal: `list`, `update`, `bulk disable`, etc.

## Concepts

| Term | Meaning |
|------|--------|
| **Monitor** | A single health check: has an ID, runs on schedule or event, produces a result (ok / degraded / fail). Can be enabled/disabled in the registry. |
| **Registry** | Persistent store of monitor *configuration* (id, enabled, optional schedule override). Not full object storage; lightweight (e.g. one JSON file under `.zqk/`). |
| **Result** | Output of a monitor run: status, summary, optional details. May be written to a component-specific file (e.g. scheduler-metrics-summary.json) or to a shared health-result cache. |
| **Alert trigger** | When a monitor reports degraded/fail, the system can create an alert or notification (future: wire into NotificationContext or a dedicated alert channel). |

## Interface (pkg/healthcheck)

```go
// Monitor is a pluggable health check. Implementations register with the global registry.
type Monitor interface {
    ID() string          // e.g. "scheduler_events", "object_validation_latency"
    Name() string         // human-readable name
    Run(ctx context.Context, projectRoot string) (*Result, error)
}

// Result is the outcome of a monitor run.
type Result struct {
    Status  string            // "ok" | "degraded" | "fail"
    Summary string            // one-line summary
    Details map[string]any    // optional (e.g. job_stats with failures)
}

// Registry holds registered monitors and their runtime config (enabled, last_run, etc.).
// Config is persisted separately from objects (e.g. .zqk/health_monitors.json).
type Registry interface {
    Register(m Monitor)
    List() []Monitor
    Get(id string) (Monitor, bool)
    IsEnabled(id string) bool
    SetEnabled(id string, enabled bool) error
    Run(ctx context.Context, projectRoot string, id string) (*Result, error)
}
```

- **Register**: Components call `Registry.Register(monitor)` at init. No YAML for monitor *definitions*; code defines them.
- **Config store**: Only overrides (enabled/disabled, optional schedule) are persisted. Default is “enabled” for all registered monitors.
- **Run**: Can be invoked by a scheduler job (timer/event) or by CLI `healthchk run <id>`.

## Naming: Monitor vs Beacon

- **Monitor** is used in this doc and in code as the *concept* (something that checks and reports status).
- **Beacon** could be the *spec/object name* if we later expose monitors as a first-class kind (e.g. for UI or cross-project dashboards). For now, monitors are **not** stored as full objects; they use a dedicated registry store to avoid transactional object storage and to keep the model simple.

## Change journal and micro-GC

Health check results are a good fit for the **change journal entry** pipeline: track change over time on a regular interval and compress at a configurable interval to free memory (micro garbage collector pattern).

- **Append**: On each monitor run (e.g. when SCH-evag runs or `healthchk run`), append a **change_journal_entry** with:
  - `change_type`: `"health_check"`
  - `object_ref`: `"health_monitor:<monitor_id>"` (e.g. `health_monitor:scheduler_events`)
  - `diff_summary`: one-line result summary (status + summary)
  - `previous_state`: optional payload (status, summary, details) for analytics and rollback-style inspection
- **Aggregate**: The existing **change journal aggregation** job (or a variant) runs over a time window; entries are grouped by `change_type`. Health_check entries are aggregated like create/update/delete, producing counts and optionally a dedicated health aggregation metric. Aggregation marks entries as `status=aggregated`.
- **Compress**: The existing **change journal compaction** pipeline compresses old entries (dictionary + snapshot encoding) into `.cjournal` artifacts and can archive/delete originals. Configurable or desirable interval (e.g. daily or weekly) keeps raw journal growth bounded and frees memory/disk.

So: **append** (every run) → **aggregate** (e.g. daily window) → **compact** (e.g. weekly). No separate health-specific store; reuse the same journal, filters, and retention. Optional: separate retention policy for `change_type=health_check` (e.g. aggregate every 6h, compact after 7 days).

## Storage

- **Registry config**: `.zqk/config/health_monitors.json` — list of `{ "id": "scheduler_events", "enabled": true }`. No schema version or object semantics; just a small config file.
- **Results**: Component-specific files (e.g. `scheduler-metrics-summary.json`) plus **change journal**: each monitor run can append a `change_journal_entry` (change_type=health_check, object_ref=health_monitor:&lt;id&gt;) for time-series and micro-GC (see above).
- **Caches**: Monitors may use existing caches (e.g. validation cache, activity cache). No new global cache on the object path; health-check runs are read-heavy and optional.

## CLI: healthchk

- **healthchk list** [--filter key=value] [--format table|json]  
  List registered monitors and their config (id, name, enabled, last_run if stored).
- **healthchk update &lt;id&gt;** --enabled true|false  
  Enable or disable a monitor.
- **healthchk run** [id]  
  Run one monitor by id, or all enabled monitors if id omitted. Output result(s) to stdout (and optionally persist).
- **healthchk bulk disable** --ids-file &lt;file&gt; | --ids id1,id2  
  Disable multiple monitors (same pattern as object bulk delete).

Subcommands and flags should align with object/internal where it makes sense (e.g. --filter, --format) so the UX feels consistent.

## Event/timer integration

- Existing **SCH-evag** job already runs scheduler-events aggregation on a timer. That job can be refactored to call the **scheduler_events** monitor’s `Run()` and then the current “events health” view is just one monitor’s result.
- Optional: a single **health-runner** scheduler job that runs every N minutes and calls `Registry.Run(ctx, projectRoot, "")` for all enabled monitors, then optionally emits alerts for any with status degraded/fail. That would centralize “run all health checks” in one place.
- Monitors do not have to be tied to a scheduler job; they can be run only via CLI or via other triggers (e.g. post-deploy hook).

## Alerts

- When a monitor returns `Result.Status == "degraded"` or `"fail"`, the runner (scheduler job or CLI) can:
  - Log the result.
  - Call a small **alert hook** (e.g. `OnHealthFailure(monitorID, result)`) that creates a notification (NotificationContext) or writes to an alert queue. No need for full object semantics for alerts in v1.

## First monitor: scheduler_events

- **ID**: `scheduler_events`
- **Name**: "Scheduler events (failures and slow jobs)"
- **Run**: Reads `scheduler-metrics-summary.json` (produced by SCH-evag), evaluates failures and slow threshold, returns `Result{ Status: "ok"|"degraded"|"fail", Summary: "...", Details: job_stats }`.
- **Registration**: Registered at init from the same package that wires SCH-evag (or from a small `pkg/healthcheck/monitors` package).
- Current `zqk scheduler events health` can become a thin wrapper: resolve project root, call `Registry.Run(ctx, projectRoot, "scheduler_events")`, print result. Alternatively we keep `scheduler events health` as-is and add `healthchk run scheduler_events` as the generic way to run that check.

## Implementation order

1. **pkg/healthcheck**: Interface (`Monitor`, `Result`), in-memory registry with optional persistence for enabled/disabled.
2. **Monitor: scheduler_events**: Implement `Monitor` that wraps current scheduler-metrics-summary logic; register it.
3. **healthchk list**: List registered monitors and config.
4. **healthchk update &lt;id&gt; --enabled**: Persist enabled/disabled to `.zqk/health_monitors.json`.
5. **healthchk run [id]**: Run one or all enabled monitors.
6. **healthchk bulk disable**: Bulk disable by --ids or --ids-file.
7. **Change journal**: Append a change_journal_entry per monitor run (change_type=health_check, object_ref=health_monitor:<id>); reuse existing aggregation and compaction to compress at interval (micro-GC).
8. (Later) Scheduler job or event that runs all enabled monitors and triggers alerts on fail/degraded.
9. (Later) Additional monitors (e.g. object_validation_latency, file_lock_contention) and optional “beacon” object kind if we want cross-project visibility.

## Out of scope (for now)

- Full object kind for monitors (no `object create monitor`).
- Transactional or WAL-backed store for health config.
- Dashboard/UI; CLI and scheduler-driven runs only.
