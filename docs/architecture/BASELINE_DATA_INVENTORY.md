> [!WARNING]
> **ARCHIVED DOCUMENT**: The primary commands referenced in this architectural document have been pruned from the `zqk` CLI.

# Baseline Data Inventory — `.zqk` metrics, reports, and aggregations

**Last Verified:** 2026-08-31


Single reference for all **data artifacts** under `.zqk` (project data directory): what they are, how to establish a solid baseline, and how they can be offloaded or removed.

**Purpose:** Get a good solid baseline for all data so you can later offload or get rid of it with confidence.

## Canonical layout (logs vs metrics vs cache / scheduler / WAL)

All paths use constants from `pkg/paths/constants.go`. Top-level separation:

- **logs/** — CLI and daemon logs, scheduler job logs, **reports/** (e.g. object-count-report). Raw logs and report outputs.
- **metrics/** — Command metrics, **object_volume/** time series. Signals and summaries.
- **cache/** — Object ID, validation, reverse-reference index, scheduler activity. Hot-path caches.
- **scheduler/** — Daemon config, events, triggers, locks. Scheduler state only.
- **wal/** — Object WAL (object.wal), maintenance WAL (maintenance.wal), lifecycle WAL (lifecycle_events.wal), rollback_points. Durability and recovery.

This keeps “where to look for signals” (metrics) vs “raw logs and reports” (logs) obvious and keeps WAL/cache/scheduler distinct.

---

## 1. Cache (`.zqk/cache/`)

| Artifact | Purpose | How to (re)establish baseline | Offload / remove |
|----------|---------|------------------------------|-------------------|
| **object-id-cache.json** | ID → file path lookup; speeds list/count and validation. | **Scheduler:** cache_prewarm (Tier 3) builds it. **CLI:** `zqk system check` (and create/update flows) update it incrementally via CAS callback. Run `zqk system check` or let scheduler run cache_prewarm. | Safe to delete; repopulates on next check or prewarm. Export: copy JSON; offload = archive then delete. |
| **high-volume-events-cache.json** | Time-window queries for audit/metrics (minimal: `created_at` per id). | Scheduler aggregation (`EnsureHighVolumeEventCacheReady`) and create/delete flows. Run scheduler or system check; cache fills as events are written. | Safe to delete; repopulates. Minimal value per entry; offload = optional archive. |
| **reverse-reference-index.json** | Reverse refs (who references this ID). Used for dependency/impact. | **Scheduler:** cache_prewarm Tier 3 (`prewarmReverseReferenceIndex`). **Storage:** updated on create/update/delete. Run cache_prewarm or full system check. | Safe to delete; repopulates on prewarm or as objects change. Can be large; good candidate to offload/rebuild on demand. |
| **validation_cache.json** | Validation state cache (e.g. object validation results). | **Scheduler:** cache_prewarm Tier 3 (`prewarmValidationStateCache`). **CLI:** `zqk system check --refresh-cache` / validation runs. | Safe to delete; repopulates. Small; low offload priority. |
| **scheduler_activity_cache.json** | Scheduler activity view (recent jobs, history). | Written by scheduler/CLI when showing activity or history. | Safe to delete; repopulates when activity/history is queried. Offload = archive for analytics. |

**Baseline in one go:** After a clean rebuild/restart, run **cache_prewarm** (e.g. trigger the cache_prewarm job or run a full `zqk system check`). That repopulates object-id, reverse-reference-index, and validation caches; high-volume-events and scheduler_activity fill as traffic runs.

---

## 2. Metrics (`.zqk/metrics/`)

| Artifact | Purpose | How to (re)establish baseline | Offload / remove |
|----------|---------|------------------------------|-------------------|
| **command_metrics.json** | Per-command invocation counts and timing (CLI usage metrics). | Written by CLI on each run (root.go). Grows over time. | **Audit report:** `zqk system audit-report` (PRUNED) reads it. To baseline: leave as-is or archive and truncate. Offload = copy for analytics; then delete or rotate. |
| **profiles/** | Metrics profiles (if used). | Created when using metrics profiles. | Optional; remove if not using profiles. |
| **baseline_*.json**, **baseline_*.prof** | Script output from `scripts/collect_baseline_metrics.sh` (validation run with metrics + CPU profile). | Run `scripts/collect_baseline_metrics.sh`. | One-time baseline; archive then delete. |

---

## 3. Scheduler (`.zqk/scheduler/`)

| Artifact | Purpose | How to (re)establish baseline | Offload / remove |
|----------|---------|------------------------------|-------------------|
| **diagnostics.jsonl** (+ rotated .1–.4) | JSONL daemon events (job started/completed/failed, health, progress). Rolling: 10MB × 5 files. | Daemon writes continuously. No manual baseline; ensure daemon has run so file exists. | Rolling already limits size. Offload = copy JSONL for analytics; rotation handles removal. |
| **scheduler-metrics-summary.json** | Aggregated job stats (completed/failed, duration) from events. | **CLI:** `zqk scheduler events aggregate`. Reads diagnostics.jsonl, writes summary. Run after daemon has been up to get a current baseline. | Safe to delete; regenerate with `zqk scheduler events aggregate`. Good candidate to offload (small); keep one current copy locally. |
| **config.yaml** | Scheduler config. | Edit via `zqk scheduler config` or create manually. | Config, not baseline data; keep or version-control. |
| **scheduler.pid**, **scheduler.keepalive** | Runtime state. | Created when daemon runs. | Remove when daemon stopped; no offload. |
| **triggers/queue.json** | Trigger queue. | Scheduler-managed. | Do not delete while daemon runs; offload = backup. |
| **locks/** | Lock files. | Scheduler-managed. | Clean when daemon stopped. |
| **diagnostics/** | SIGUSR1 dumps (goroutines, threads, processes, heap). | Sent when you send SIGUSR1 to daemon. | Archive for debugging then delete; safe to clear. |

**Baseline:** Run `zqk scheduler events aggregate` to refresh `scheduler-metrics-summary.json` from current `diagnostics.jsonl`.

---

## 4. Logs (`.zqk/logs/`)

| Artifact | Purpose | How to (re)establish baseline | Offload / remove |
|----------|---------|------------------------------|-------------------|
| **log-events.json**, **log-events-human.log** | Structured log events and human-readable log. | Written by logging framework. | Rotate or truncate; offload = archive then delete. |
| **scheduler/<job_id>/** | Per-job logs: `<job_id>.events.json`, `.stdout`, `.stderr`. | Created when scheduler runs jobs. | Retention is manual or via cleanup job. Offload = archive by job_id; then delete old dirs. |
| **reports/** | Object count and other reports (e.g. `object-count-report-*.txt` / `.json`). | **CLI:** `zqk system object-count-report` (PRUNED) (writes here by default). | Run report to refresh; archive reports then delete. |
| **post-commit-tests.log**, **components/** | Test and component logs. | Written by test runner / components. | Archive then delete or rotate. |
| **cpu-*.prof** | CPU profile (SIGUSR2). | Created when SIGUSR2 sent to process. | One-off; archive then delete. |

**Baseline:** No single “baseline” for logs; they’re append/rolling. For reports, run `zqk system object-count-report` (PRUNED) to get a current snapshot.

---

## 5. CLI commands that generate reports

Commands that produce report-like output or write to files. Use **`--output`** / **`--report-file`** (or command-specific flags) to write to a path; otherwise many print to stdout.

| Command | What it generates | Default or flag for file output | Location / notes |
|---------|-------------------|----------------------------------|------------------|
| **zqk system object-count-report (PRUNED)** | Congruence report (disk vs index, counts by kind, cache status, integrity). | **Default:** writes to `.zqk/logs/reports/object-count-report-<timestamp>.txt` and same path with `.json`. Override: `--report-file <path>`. | §4 reports/. |
| **zqk system audit-report (PRUNED)** | Analysis of command metrics (failure rates, timeouts, slow commands, churn). | **Optional:** `--output <path>` (e.g. report.md, report.json). Reads `.zqk/metrics/command_metrics.json`. | No default path; stdout or `--output`. |
| **zqk system metrics** | View/filter command metrics. | **Optional:** `--output <path>`. `--summary` for detailed analysis. | Reads command_metrics.json. |
| **zqk system quarantine-report (PRUNED)** | Quarantine folder contents (file counts by source). | **Optional:** `--output` for JSON/YAML. Data for dashboard. | Reads `.zqk/system-health/quarantine/`. |
| **zqk system health-data (PRUNED)** | Aggregated system-health payload (quarantine + check summary). | **Optional:** `--output` for JSON/YAML. | Reads `.zqk/system-health/tier1-latest.json` (if present) and quarantine. |
| **zqk system cache-audit (PRUNED)** | Stale object ID cache entries (files deleted outside CLI). | **Optional:** `--output <path>`. `--format json`, yaml, or table. | No default path; stdout or `--output`. |
| **zqk system retention-status (PRUNED)** | Object counts vs retention targets. | **Optional:** `--output`. | Stdout or `--output`. |
| **zqk system check-baseline (PRUNED)** | Baseline metrics from sync check (for comparing async). | **Optional:** `--baseline-output <file>` (e.g. baseline.json). | User-chosen path. |
| **zqk system check-async-baseline (PRUNED)** | Compare async validator to baseline. | **Optional:** `--baseline-file <path>` (default baseline), `--comparison-output <path>`. | User-chosen paths. |
| **zqk system snapshot-expand** | Expand snapshot to directory. | **Optional:** `--output-dir <dir>`. | User-chosen directory. |
| **zqk scheduler events aggregate** | Aggregated job stats from diagnostics.jsonl. | **Fixed:** writes `.zqk/scheduler/scheduler-metrics-summary.json`. | §3. |
| **zqk scheduler events health** | Health view (failures, slow jobs) from summary. | Stdout only; no file. | Reads scheduler-metrics-summary.json. |
| **zqk reports quick** | Quick report (preset: questions, milestones-overdue). | **Optional:** `--output`. | Stdout or `--output`. |
| **zqk reports pcs** | Project Confidence Score (PCS). | **Optional:** `--output`. `--format json`. | Stdout or `--output`. |
| **zqk reports edd** | Effort Distribution Discrepancy (EDD). | **Optional:** `--output`. `--format json`. | Stdout or `--output`. |
| **zqk reports blockers** | Dependencies and blockers (D&B). | **Optional:** `--output`. `--format json`. | Stdout or `--output`. |
| **zqk pre-commit lint-report** | Show lint-output.txt. | Displays `.zqk/pre-commit/lint-output.txt`. | Read-only. |
| **zqk pre-commit policy-report** | Show policy-output.txt. | Displays `.zqk/pre-commit/policy-output.txt`. | Read-only. |
| **zqk pre-commit integrity-report** | Show integrity-output.txt. | Displays `.zqk/pre-commit/integrity-output.txt`. | Read-only. |

**System-health directory (`.zqk/system-health/`):** Used by repair and health-data. **tier1-cas-corruption.tsv** (input for `repair-cas-corruption`) and **tier1-latest.json** (check summary for dashboard) are written by the integrity/check pipeline or scripts (e.g. pre-commit integrity job). **quarantine/** holds files moved by repair-cas-corruption and cleanup-duplicates. Baseline: run integrity check so tier1-* exist if needed; quarantine is populated by repair/cleanup.

---

## 6. Pre-commit (`.zqk/pre-commit/`)

| Artifact | Purpose | How to (re)establish baseline | Offload / remove |
|----------|---------|------------------------------|-------------------|
| **results.json** | Aggregated result for hook (block + categories). | **CLI:** `zqk pre-commit aggregate` (merges category files into this). | Required for hook; don’t remove. Baseline = run aggregate after category jobs. |
| **&lt;category&gt;.json** (e.g. lint, integrity, policy) | Per-category check result. | Written by background jobs or `zqk pre-commit write-result`. | Re-run jobs then aggregate. |
| **.last-&lt;category&gt;.json** | Staging file for category result. | Written by check scripts; callback reads and writes category. | Ephemeral; no baseline. |
| **lint-output.txt**, **policy-output.txt**, **integrity-output.txt** | Full output when category failed. | Written by lint/policy/integrity scripts. | Removed when category passes; optional archive. |
| **system-check.json** | Written when system check is run with `--output .zqk/pre-commit/system-check.json` (or similar). | Run `zqk system check` with that output path. | Optional; archive then delete if not needed. |

**Baseline:** Run the three category jobs (lint, policy, integrity) then `zqk pre-commit aggregate` so `results.json` reflects current state.

---

## 7. Config (`.zqk/config/`)

| Artifact | Purpose | How to (re)establish baseline | Offload / remove |
|----------|---------|------------------------------|-------------------|
| **config.yaml** | Project config. | `zqk init` or manual. | Config; keep. |
| **feature_flags.json** | Feature flags. | CLI/manual. | Config; keep or reset. |

---

## 8. Other directories

| Path | Contents | Baseline / offload |
|------|----------|--------------------|
| **.zqk/diagnostics/** | SIGUSR1 dumps from CLI (goroutines, threads, processes). | Archive for debugging; safe to delete. |
| **.zqk/state/** | State files (if any). | Depends on usage; often safe to clear when processes stopped. |
| **.zqk/callback-logs/** | Callback logs from scheduler. | Archive then delete or rotate. |
| **.zqk/migration-snapshots/** | Migration backups. | Keep until migrations verified; then archive and delete. |
| **.zqk/wal/** | Write-ahead log (object WAL). | Do not delete while storage in use; offload = backup. |

---

## 9. Object-backed “report” data (in `.zqk/process/` or storage)

These are first-class objects, not raw files under `.zqk`:

| Kind | Purpose | Baseline / offload |
|------|---------|--------------------|
| **scheduler_health_metric** | Health monitoring (missed triggers, recovered jobs). | Query via object list; export for analytics; retention by policy. |
| **audit_aggregation_metric** | Aggregated change-journal / audit metrics. | **CLI:** `zqk system aggregate-change-journal` (PRUNED) creates these from change journal; optionally delete processed journal entries. Offload = export then bulk delete by criteria. |
| **audit_event** (incl. aggregated_summary) | Audit trail. | Written by buffer flush and direct writes. High-volume; offload = export by time window then archive/delete. |
| **change_journal_entry** | Change journal. | Consumed by aggregate-change-journal; optionally deleted after aggregation. |

---

## 10. One-shot baseline checklist

Use this to get a solid baseline after rebuild/restart (or after cleaning caches):

1. **Scheduler running** (if you use it): start daemon so scheduler-events and jobs run.
2. **Caches:** Run cache_prewarm (scheduler Tier 3) or a full `zqk system check` so object-id, reverse-reference-index, and validation caches are populated.
3. **Scheduler metrics:** Run `zqk scheduler events aggregate` to refresh `scheduler-metrics-summary.json`.
4. **Pre-commit:** Run lint, policy, and integrity jobs (or their scripts), then `zqk pre-commit aggregate`.
5. **Reports (optional):** Run `zqk system object-count-report` (PRUNED) to get a current object/cache/integrity snapshot. For other report commands (audit-report, quarantine-report, retention-status, reports quick/pcs/edd/blockers), see **§5**.
6. **Metrics (optional):** Leave `command_metrics.json` as-is or archive + truncate for a fresh start.
7. **Diagnostics:** Delete or archive old `.zqk/diagnostics/` and `.zqk/scheduler/diagnostics/` so new dumps are easy to find.

After this, you have a consistent baseline; then you can decide what to offload (e.g. old logs, diagnostics, archived caches) or remove.

---

## 11. References

- **Paths:** `pkg/paths/constants.go`
- **Log naming (conventions and patterns):** `docs/architecture/LOG_NAMING_CONVENTIONS.md`
- **Scheduler events and metrics:** `docs/archive/system_health/SCHEDULER_EVENTS_AND_METRICS.md`
- **Pre-commit layout:** `docs/architecture/PRE_COMMIT_BACKGROUND_RESULTS.md`
- **Cache state after hang:** `docs/architecture/SAMPLE_AND_HANG_INVESTIGATION_20260225.md` (§7)
