# Scheduler Degraded-Mode Guardrails

This document defines expected CLI behavior when the scheduler daemon is not running.

## Why this exists

Some commands rely on scheduler-maintained state (maintenance jobs, cache freshness, convergence loops).  
Allowing these commands to run silently while the daemon is stopped creates confusing partial-state outcomes that look like defects.

## Guardrail model

- **Required scheduler commands**: block when scheduler is down, unless explicitly overridden with `--allow-degraded`.
- **Optional scheduler commands**: run, but emit a warning that results may be partial or stale.
- **Independent commands**: run normally.

## Current implementation scope

Implemented via command-group `PersistentPreRunE` guards for:

- `zqk system`
- `zqk object`
- `zqk internal`

### `zqk system`

- **Required** (blocked without override):
  - `aggregate-audit`
  - `retention-tolerance`
  - `compact-stream-state`
  - `compact-wal`
  - `compact-maintenance-wal`
  - `cleanup-duplicates`
  - `cleanup-quarantine`
  - `auto-fix-batch`
  - `auto-fix-process-pending`

- **Optional** (warning/degraded mode allowed):
  - `maintenance-request-cycle`
  - `retention-status`
  - `health-data`

### `zqk object`

- **Required** (blocked without override):
  - `bulk`
  - `bulk-delete`
  - `bulk-update`
  - `import`
- **Optional**:
  - `list`
  - `count`

### `zqk internal`

- **Required** (blocked without override):
  - `bulk`
  - `process`
- **Optional**:
  - `list`
  - `count`

## Operator behavior

- Preferred: start scheduler first: `zqk scheduler start`
- If intentionally running without scheduler: use `--allow-degraded` and treat output as degraded-mode results.

## Testing expectations

Guard behavior must have unit tests for:

- required command blocks when scheduler down
- required command allows override with `--allow-degraded`
- optional command warns but does not block
- independent command unaffected

This ensures degraded-mode behavior remains intentional and does not regress into accidental partial execution.
