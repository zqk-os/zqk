# ADR: Scheduler Daemon Scan-Churn Elimination and Scheduled Resource-Hygiene

## Status
Accepted

## Date
2026-11-21

## Context
Long-running `zqk scheduler` daemon processes experience unbounded heap growth and elevated baseline CPU utilization when idle or under light load. Profiling and lifecycle inspection traced this regression to four compounding behaviors:

1. **Unconditional Periodic Scan & Cron Teardown (`watchJobChanges`)**: Every 30 seconds, `Scheduler.watchJobChanges` (`pkg/scheduler/lifecycle_coordination.go`) unconditionally invokes `maybeReconcileSchedulerJobCASIndexBeforeReload` and `loadAndScheduleJobs(ctx, true)`, invalidating the `scheduler_job` list cache and stream registry cache, scanning CAS storage and YAML definitions across all persisted jobs, re-hydrating every job struct, and tearing down and rebuilding cron entries even when no job definition has changed on disk. Under 30s/45s sweep cadences, this creates continuous allocation churn and GC pressure.
2. **Missing `SCH-resource-hygiene` Provisioning**: While `SCH-scheduler-job-retention` and other core maintenance jobs are declared in `.zqk/specs/configs/scheduler_maintenance_config.yaml` and provisioned via `zqk system ensure-retention-jobs` (`cmd/zqk/system/ensure_retention_jobs.go`), `SCH-resource-hygiene` is not yet provisioned in the required maintenance bundle, leaving workspace resource-hygiene sweeps unscheduled.
3. **Unbounded Stale `SCH-AUTOFIX-*` Jobs and Orphan Scheduler Logs**: One-off and manual `SCH-AUTOFIX-*` jobs accumulated from `zqk system check --auto-fix-scheduler` remain persisted after completion or disablement, inflating job lists. Meanwhile, `.zqk/logs/scheduler/*` log cleanup lacks an O(1) `idSet`-backed lookup against live job IDs, risking slow scans or retained orphan log files.
4. **Conflict-Drop Re-Enqueue Loops for Periodic Timer Jobs**: When a periodic `TriggerTypeTimer` job is already executing and hits a conflict (`ConflictManager.CanRun` returns false), re-enqueueing the dropped trigger on the trigger queue causes tight retry/re-enqueue churn across overlapping long-running sweeps instead of letting the next scheduled cron tick fire naturally.

## Decision

1. **Stat-Gated Reload & Single-Job Admission (`AdmitJobFromStorage`)**:
   - Gate `watchJobChanges` periodic reloads on filesystem/CAS metadata change detection (stat-gated mtime/digest check) so unchanged storage produces zero CAS index scans, zero full-list YAML parsing, and zero cron teardown/rebuild cycles.
   - Rely on single-job `AdmitJobFromStorage` (`pkg/scheduler/job_admission.go`) on cache miss (in `TriggerJob` and trigger-queue batch processing) to hydrate newly created cross-process jobs directly via `storage.Read` without triggering a full-store scan.

2. **Provision `SCH-resource-hygiene` in Maintenance Bundle**:
   - Add `SCH-resource-hygiene` to `.zqk/specs/configs/scheduler_maintenance_config.yaml` (`required_jobs` and, where appropriate, pause exemptions) with its corresponding template so `zqk system ensure-retention-jobs` and daemon startup automatically provision and reconcile the job.

3. **Stale `SCH-AUTOFIX-*` Reaping & O(1) `idSet`-Backed Orphan Log Pruning**:
   - Extend `scheduler_job_retention` (`pkg/scheduler/handlers_scheduler_job_retention.go`, `pkg/scheduler/scheduler_job_retention_pipeline.go`) to reap completed or disabled manual `SCH-AUTOFIX-*` jobs subject to bounded TTL and count caps.
   - Build an O(1) `idSet` of active/retained `scheduler_job` IDs during retention sweeps to prune orphaned `.zqk/logs/scheduler/*` files and directories without repeated directory/store scans.

4. **Suppress Conflict-Drop Re-Enqueue Loops for Periodic `TriggerTypeTimer` Jobs**:
   - When a `TriggerTypeTimer` job is dropped due to an in-flight execution conflict, suppress trigger-queue re-enqueueing so overlapping long-running sweeps do not amplify queue depth or spin CPU; the periodic timer schedule naturally governs the next eligible execution.

## Consequences
- Eliminates idle 30s/45s full-CAS scan churn, YAML re-hydration, and cron teardown/rebuild allocations in `watchJobChanges`.
- Preserves immediate cross-process job visibility on cache miss via O(1) `AdmitJobFromStorage`.
- Ensures `SCH-resource-hygiene` is deterministically provisioned and maintained across environments via `zqk system ensure-retention-jobs`.
- Bounds `scheduler_job` count and `.zqk/logs/scheduler/*` disk footprint with O(1) `idSet` orphan pruning.
- Prevents conflict-induced trigger-queue feedback loops for periodic `TriggerTypeTimer` jobs.

## References
- `pkg/scheduler/lifecycle_coordination.go` (`watchJobChanges`, `TriggerJob`)
- `pkg/scheduler/job_management.go` (`loadAndScheduleJobs`)
- `pkg/scheduler/job_admission.go` (`AdmitJobFromStorage`)
- `pkg/scheduler/job_trigger_queue.go` (trigger queue batch processing & re-enqueue)
- `pkg/scheduler/handlers_scheduler_job_retention.go` (`scheduler_job_retention`)
- `cmd/zqk/system/ensure_retention_jobs.go` (`ensure-retention-jobs`)
- `.zqk/specs/configs/scheduler_maintenance_config.yaml`
