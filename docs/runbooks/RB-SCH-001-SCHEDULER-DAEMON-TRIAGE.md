# RB-SCH-001: Scheduler Daemon Exit Cascades and Job Triage

## Metadata
- **Severity**: P1 / High
- **Component**: `cmd/zqk/scheduler`, `pkg/scheduler`
- **Owner**: Automation Stewards

## Symptoms & Alerts
- Error message: `scheduler daemon is not running` or `scheduler exited with code 1`
- Warning: `materialized view watermark exceeds tolerance` during `whats-next` queries.
- Periodic jobs (`SCH-retention-tolerance`, `SCH-val`, `SCH-objcount-report`) not updating `last_run_at`.

## Root Cause Analysis
1. Zombie or stale PID file at `.zqk/state/scheduler.pid` pointing to dead process.
2. Missing or malformed config in `.zqk/specs/configs/scheduler_maintenance_config.yaml`.
3. Unhandled panic in a background job runner that escaped the recovery wrapper.
4. Missing host execution permissions on scheduler job scripts (exit code 126/127).

## Step-by-Step Remediation Procedure

### Step 1: Check Scheduler Status & PID
Inspect the live status:
```bash
./bin/zqk scheduler status
```
If reported running but unresponsive, inspect the PID file:
```bash
cat .zqk/state/scheduler.pid
ps -p $(cat .zqk/state/scheduler.pid 2>/dev/null)
```

### Step 2: Clear Stale PID File
If the process does not exist but the PID file remains:
```bash
rm -f .zqk/state/scheduler.pid
```

### Step 3: Check Scheduler Logs
Examine the last 50 lines of daemon output:
```bash
tail -n 50 .zqk/state/logs/scheduler.log
```
Check for panics, syntax errors in scripts, or missing dependencies.

### Step 4: Ensure Survival Jobs & Restart
Ensure all mandatory survival jobs are configured:
```bash
./bin/zqk system ensure-retention-jobs
```
Restart the scheduler daemon:
```bash
./bin/zqk scheduler start
```
Verify the daemon is alive:
```bash
./bin/zqk scheduler status
./bin/zqk scheduler list
```

### Step 5: Verification Gate
Trigger an immediate test job:
```bash
./bin/zqk scheduler trigger SCH-val
./bin/zqk scheduler status
```
Ensure scheduler daemon status reports `Running` and job completes with status `succeeded`.
