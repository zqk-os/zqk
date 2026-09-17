# Scheduler status and health-check alignment

**Last Verified:** 2026-08-31


**Status:** Follow-up to investigate  
**Purpose:** Ensure health check fails when the scheduler process is not running, and improve detection/recovery when the daemon is in a bad state.

## Problem

The scheduler can appear "disconnected" from the status check: health check may not correctly fail when `zqk scheduler status` would report that the process is not running. This leads to inconsistent visibility (e.g. monitoring thinks things are ok while the daemon is actually down).

## Desired behavior

1. **Health check must fail when process is not running**  
   If `zqk scheduler status` (or equivalent) determines the process is not running, the health check should fail (e.g. write or leave `daemon-unhealthy.json`, return non-zero).

2. **PID file alignment**  
   Investigate why the PID file and actual process can get out of sync. If possible, ensure the PID file is updated or validated so status reflects reality.

3. **Bad-state recovery**  
   If the process is found to be running but in a bad state (e.g. unresponsive, keep-alive stale), the health-check script or monitor could:
   - Kill the process and start a new one, **unless**
   - An explicit stop was ordered (e.g. `zqk scheduler stop` created `no-auto-restart`), in which case do not auto-restart.

## Implementation notes

- Health-check logic (e.g. `scripts/` or wherever the HTTP/script health check lives) should call or mirror the same "is process running?" logic as `zqk scheduler status`.
- PID file path: `.zqk/scheduler/scheduler.pid`. Ensure daemon writes/updates it and that readers revalidate that the PID is still the running process.
- `SchedulerNoAutoRestartFile` (`no-auto-restart`): if present, do not restart the daemon when killing a bad state.

## References

- `pkg/scheduler/pid_file_test.go`, PID file handling
- `docs/archive/system_health/` – scheduler validation and diagnostics
- `.zqk/scheduler/daemon-unhealthy.json` – written by external health-check when daemon is down or keep-alive stale
