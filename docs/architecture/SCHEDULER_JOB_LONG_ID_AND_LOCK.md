# Scheduler job long ID and lock file

**Status:** Resolved  
**Purpose:** Document the "file name too long" lock error for scheduler jobs with very long IDs, the root cause, and the fixes.

## Problem

When a `scheduler_job` object has an id like `SCH-1772046602-scheduler-job-SCH-1772046601-scheduler-job-...` (hundreds of characters), the job lock path becomes `.zqk/scheduler/locks/<id>.lock`. Filesystems have `NAME_MAX` (e.g. 255 bytes), so opening the lock file fails with "file name too long" and the daemon logs repeated errors.

## Root cause

- **Source of long IDs:** Legacy one-off job ID formula was `SCH-<timestamp>-<kind>-<id>` with `kind` normalized (e.g. `scheduler_job` → `scheduler-job`). When the object being "created" was a scheduler_job and `<id>` was already a long chain (e.g. a parent job id), the new id became longer each time. That formula is no longer used in current code (submit, go test, auto-fix use short ids); existing long-id jobs are **legacy objects** already in `docs/architecture/scheduler_jobs/`.
- **Why it hurts:** The lock file path uses `jobID` as the filename; long ids exceed `NAME_MAX`.

## Fixes (implemented)

1. **Lock filename (symptom)**  
   In `pkg/scheduler/job_lock.go`, when `len(jobID) > 200`, the lock filename is now `sha256(jobID).hex + ".lock"` instead of `jobID + ".lock"`. Existing long-id jobs can acquire a lock without "file name too long".

2. **Quarantine after repeated failures**  
   In `pkg/scheduler/job_execution.go` and `scheduler.go`: after 3 consecutive lock create/acquire failures for a job, we skip execution and stop logging the error; after 5 failures we disable the job in storage so it drops out of the process flow. Prevents log spam and stacking.

3. **Prevent new long IDs**  
   In `pkg/storage/object_storage_file_create.go` (`ensureObjectID`), for `kind == "scheduler_job"`, if `len(id) > 200` we return an error. No new scheduler_job objects with long ids can be created.

## Cleanup (existing long-id jobs)

- **List long ids:**  
  `zqk object list scheduler_job --format json | jq -r '.objects[] | select((.id | length) > 200) | .id'`

- **Disable or delete:**  
  Use `zqk object update <id> --field "enabled=false"` to disable, or `zqk object bulk delete --file ids.yaml` to remove (see `object-bulk-delete-not-loop.mdc`).

- Jobs that hit 5 lock failures are auto-disabled; they can be re-enabled after fixing the id (e.g. delete and recreate with a short id) or deleted.

## Activity cache

The scheduler activity cache (`.zqk/cache/scheduler_activity_cache.json`) does not retain legacy long-id jobs: on load, entries with `len(job_id) > 200` are dropped; on save, such entries are not written; and `UpdateEvent` ignores events for job IDs longer than 200 characters. So after a restart and the next cache save, long-id job entries disappear from the cache.

## References

- `pkg/scheduler/job_lock.go` – `lockFilenameForJobID`, `maxLockFilenameJobIDLen`
- `pkg/scheduler/job_execution.go` – lock failure counting, skip, disable
- `pkg/scheduler/handlers_test.go` – `TestValidationJobIDWouldExceedMaxLength` (documents old formula)
- `pkg/storage/object_storage_file_create.go` – `maxSchedulerJobIDLen` check in `ensureObjectID`
