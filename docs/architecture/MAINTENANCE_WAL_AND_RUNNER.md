> [!WARNING]
> **ARCHIVED DOCUMENT**: The primary commands referenced in this architectural document have been pruned from the `zqk` CLI.

# Maintenance WAL and Runner

**Last Verified:** 2026-08-31


**Status:** Implemented  
**Purpose:** Guarantee **ordered** critical maintenance (aggregate-then-retention) with a single, durable stream and one reader—no reliance on scheduler job order or piecemeal run_wrapper jobs.

---

## Problem

Critical maintenance (audit aggregation → retention tolerance) must run in a fixed order:

1. **Aggregate** audit events (and optionally change_journal) so that raw events are compressed/archived.
2. **Retention** then deletes or archives objects per kind according to policy.

If retention runs without prior aggregation, `audit_event` volume may never drop because retention policy applies to aggregated/archived state. Coordinating this via **two separate scheduler jobs** (e.g. aggregate at 4am, retention at 5am) is brittle: jobs can be reordered, disabled, or run in the wrong order. A single **run_wrapper** that runs both commands helps but still depends on one job definition and cron; it doesn’t give a durable, replayable record or a single reader that can be extended for more phases.

---

## Design

### Maintenance WAL

- **Path:** `.zqk/wal/maintenance.wal` (same directory as other WALs: `object.wal`, `lifecycle_events.wal`).
- **Format:** One JSON object per line (same pattern as lifecycle event WAL).
- **Event type:** `cycle_requested` — indicates one full maintenance cycle should run (aggregate then retention, in order).
- **Checkpoint:** `.zqk/wal/maintenance.wal.checkpoint` stores the last processed sequence number.

Any component (scheduler job, CLI, or future trigger) that wants to run maintenance **appends** a `cycle_requested` record and returns. It does **not** run aggregation or retention itself.

### Maintenance runner

- **Single goroutine** (one reader of the maintenance WAL).
- **Loop:** Load checkpoint → replay WAL from `lastSeq` → for each `cycle_requested` event, run **aggregate then retention** in process (same handlers as scheduler jobs), then advance checkpoint.
- **Ordering:** Aggregation always runs before retention for that cycle; no parallelism between these two steps.
- **Trigger:** Scheduler (or CLI) only appends to the WAL; the runner does the work. So we need **one** scheduler job (e.g. timer) that appends `cycle_requested`; the runner, already running in the same process, processes it in order.

This matches the existing pattern: **lifecycle event WAL + single listener + single updater**. Here we have **maintenance WAL + single runner**.

### How to tell when a cycle has been processed

- **Checkpoint file:** `.zqk/wal/maintenance.wal.checkpoint` (created after the first completed cycle) contains a single line: the **last processed sequence number**. Compare with the number of lines in `maintenance.wal` to see how many cycles are pending (e.g. 220 lines in WAL and checkpoint `219` → one cycle pending).
- **Logs:** Each cycle appends to:
  - `.zqk/logs/scheduler/maintenance-aggregate/maintenance-aggregate.events.json` (started / outcome / completed)
  - `.zqk/logs/scheduler/maintenance-retention/maintenance-retention.events.json` (started / progress / outcome / completed)  
  Tail these to see cycles completing in real time.

### WAL growth and compaction

- **`maintenance.wal` is append-only.** Every `cycle_requested` adds one line. The runner writes the **checkpoint after each successful cycle** (so a single failing cycle does not prevent the checkpoint file from being created).
- **Compaction:** The scheduler **compacts the maintenance WAL at daemon startup** (before opening it for append), so entries with seq ≤ checkpoint are removed on each restart. You can also run `zqk system compact-maintenance-wal` when the daemon is stopped for an extra trim. See **WAL_AND_STATE_FILES.md**.

### Reliability

- **Durable:** Requests are persisted in the WAL before any work runs; if the process dies, the runner replays from the checkpoint on restart.
- **Ordered:** One goroutine, one replay loop; aggregation and retention for a cycle are never reordered.
- **Extensible:** Future phases (e.g. change_journal aggregation, then retention) can be added to the same cycle in code without new jobs or config.

---

## Implementation notes

- **Handlers:** The runner (in `pkg/scheduler/maintenance_runner.go`) uses the same `HandlerFactory` as the scheduler. It builds minimal `ScheduledJob` values (ID, JobType) for `audit_event_aggregation` and `retention_tolerance`, creates handlers via the factory, and calls `Execute(ctx, job)` in sequence. No subprocess or CLI exec.
- **Log job IDs:** The runner does **not** create scheduler_job objects. It uses **fixed** job IDs for log output (`MaintenanceLogJobIDAggregate`, `MaintenanceLogJobIDRetention`), so all cycles write to the same two directories: `.zqk/logs/scheduler/maintenance-aggregate/` and `.zqk/logs/scheduler/maintenance-retention/`. Events are appended and trimmed by `max_lines` (scheduler_logs_config). This avoids creating one directory per cycle (which would otherwise accumulate until scheduler_job_retention orphan cleanup). The scheduler_job_retention handler skips these two dirs when removing orphaned log dirs.
- **Where the runner runs:** Started alongside the lifecycle listener when the scheduler daemon starts (same process). If the scheduler is not running, maintenance can still be triggered by appending to the WAL (e.g. via CLI); the next time the scheduler starts, the runner will process pending events.
- **Trigger from scheduler:** A single job type **`maintenance`** is supported: when such a job runs, the handler appends `cycle_requested` to the WAL and returns; the runner processes it in order. Create one timer job with `job_type: maintenance` and the desired schedule (e.g. `0 5 * * *`). No need for two jobs or a run_wrapper script for this flow.
- **Ensuring the trigger job (SCH-101):** If no `job_type=maintenance` job exists, nothing ever appends `cycle_requested`, so the maintenance runner has no work and aggregation/retention never run. The scheduler **ensures** the maintenance WAL trigger job at daemon load (`ensureMaintenanceWALTriggerJobExists`), creating SCH-101 with schedule `*/15 * * * *` when missing. You can also run `zqk system ensure-retention-jobs` to create the full maintenance bundle (including this job).

---

## Catch-up without conflicting with scheduled maintenance

**Do not** run `zqk system aggregate-audit` (PRUNED) or `zqk system retention-tolerance` (PRUNED) in a separate process while the scheduler daemon is running. That would be a second process touching the same streams and CAS; it can conflict with the maintenance runner and cause duplicate work or inconsistent state.

**Safe ways to get ahead of cleanup:**

1. **Queue extra cycles (no conflict)**  
   Run `zqk system maintenance-request-cycle` one or more times. Each call appends a `cycle_requested` event; the **same** maintenance runner processes them in order (single goroutine). No second process—you are only adding work to the runner’s queue. The timer job can still append its own cycles; they are processed in WAL order.  
   Example (e.g. 20 extra cycles):
   ```bash
   for i in $(seq 1 20); do zqk system maintenance-request-cycle; done
   ```
   Each cycle uses a 1h aggregation window. For a large backlog, use option 2.

2. **Large one-off with daemon stopped (no conflict)**  
   When audit_event count is very high and you want a single 7d catch-up:
   - Stop the scheduler daemon.
   - Run: `zqk system aggregate-audit --window 7d --delete` (PRUNED) then `zqk system retention-tolerance` (PRUNED).
   - Start the daemon again.  
   Only one process touches the data, so no conflict. After that, the regular maintenance cycles (via the WAL) can maintain the lean baseline.

3. **Aggressive one-off retention (scheduler stopped)**  
   When retention is too slow (e.g. 85k audit_events and you need them down now), stop the scheduler and run retention with **large batches** and **no job timeout** (CLI runs until done):
   - Stop the scheduler daemon.
   - Run (from project root):
     ```bash
     zqk system retention-tolerance --kind audit_event --batch-size 5000 --max-batches 20 --bulk-delete-workers 32 (PRUNED)
     ```
   This processes up to 100k deletes (20 × 5000) in 20 batches with 32 parallel workers. No scheduler job timeout—the CLI runs until finished. Repeat if count is still over target, or run again with higher `--max-batches`. Then start the daemon again.

---

## Centralized maintenance job configuration

**Required maintenance jobs** (including the maintenance WAL trigger SCH-101, cache prewarm SCH-007, object_validation SCH-val, retention, audit aggregation, autofix, pre-commit, etc.) are defined in a **single source of truth**:

- **Config:** `docs/process/_internal/configs/scheduler_maintenance_config.yaml`
- **Override:** `ZQK_SCHEDULER_MAINTENANCE_CONFIG` to point at a different file.

The config lists `required_jobs` (id, job_type, template_file). All create/ensure flows use this config so behavior is **observable, reliable, accurate, and efficient**. No scattered "ensure" logic in the daemon load path: the daemon runs **one** ensure at start (`zqk system ensure-retention-jobs` logic), which reads the config and creates or corrects jobs from templates. Reload cycles no longer create or correct jobs.

- **CLI:** `zqk system ensure-retention-jobs` (and init `--with-maintenance-jobs`, prepare-onboarding `--with-maintenance`) use this config.
- **Daemon start:** Runs the same ensure once so SCH-101 and the rest exist before the first LoadJobs.

**Data cell operational envelope (v1):** **`SCH-dce-tick`** (`job_type` **`data_cell_envelope_tick`**, scheduler category **`data_cell_envelope`**) is included in the same `required_jobs` list. Template: **`scripts/scheduler_jobs/data_cell_envelope_tick_six_hourly.yaml`**. The handler records a structured log line with the stream-profile operational envelope summary ([`pkg/scheduler/handlers_datacell_envelope_tick.go`](../../pkg/scheduler/handlers_datacell_envelope_tick.go)). Product context: [DATA_CELL_MODEL.md](./DATA_CELL_MODEL.md) (operational envelope row).

### Rationale: maintenance config as a project-customizable plan

This pattern is intentionally **project-customizable** and **intuitive**:

- **Single list, one place:** Each project can define its own set of maintenance jobs (this repo has more—e.g. pre-commit lint/policy/integrity, autofix—because we are building the product; other projects may have fewer or different jobs). The system does not hard-code the set; it reads from the config.
- **Ensure and go:** Throwing jobs on the maintenance config (and adding a template per job) is enough: the system creates or corrects them at daemon start and treats them as **maintenance** (same category, same prioritization and lifecycle behavior). No extra "register this job type" or per-job daemon code.
- **Robust customization:** Projects can add or remove required jobs by editing the YAML (or swapping the file via env). The daemon and CLI stay generic; prioritization and scheduling apply uniformly to whatever is in the config and in storage.

---

## Related: cleanup maintenance (on-demand)

A **cleanup** extension to the maintenance pattern is designed for on-demand (or timer) config-driven tasks: delete, truncate, archive, move, group/archive, lightweight analysis (no metric duplication), and delegation to make/ant/gradle/npm. See **[CLEANUP_MAINTENANCE_DESIGN.md](./CLEANUP_MAINTENANCE_DESIGN.md)** for config schema, step types, and build-tool flexibility.

---

## References

- `docs/architecture/LIFECYCLE_EVENT_LISTENER_AND_CRITERIA.md` — same WAL + single-reader pattern.
- `docs/architecture/CLEANUP_MAINTENANCE_DESIGN.md` — on-demand cleanup config and job type (design).
- `docs/architecture/INTERNAL_OBJECTS_AS_DEDICATED_WALS.md` — direction for dedicated WALs.
- `docs/process/_internal/configs/retention_tolerance.yaml` — coordination note: aggregate before retention.
- `docs/process/_internal/configs/scheduler_maintenance_config.yaml` — required scheduler maintenance jobs (single source of truth).
- `scripts/scheduler_jobs/maintenance_wal_trigger.yaml` — template for SCH-101.
- `scripts/scheduler_jobs/data_cell_envelope_tick_six_hourly.yaml` — template for SCH-dce-tick (data cell operational envelope tick).
- `scripts/scheduler_jobs/maintenance_aggregate_then_retention.yaml` — legacy run_wrapper; can be replaced by a single “maintenance” job that requests a cycle via the WAL.
