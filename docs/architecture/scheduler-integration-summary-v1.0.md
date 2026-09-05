# Scheduler Integration Summary

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2026-01-01  
**Status**: Implementation Complete

## Overview

Integrated the concurrent operations system with `scheduler_job` objects to manage all background operations. This replaces direct goroutine spawning with managed scheduler jobs.

## Key Changes

### 1. Scheduler Job Handlers Created

**New Handlers**:
- `pkg/scheduler/handlers_cache_invalidation.go`: Handles cache invalidation jobs
- `pkg/scheduler/handlers_cascade_update.go`: Handles cascade update jobs
- `pkg/scheduler/handlers_operation_execution.go`: Handles operation execution jobs

**Integration**: Added handlers to `createJobHandler()` in `pkg/scheduler/scheduler.go`:
```go
case "cache_invalidation":
    return NewCacheInvalidationHandler(s.storage)
case "cascade_update":
    return NewCascadeUpdateHandler(s.storage)
case "operation_execution":
    return NewOperationExecutionHandler(s.storage)
```

### 2. Scheduler Job Objects Created

**SCH-009** (`cache_invalidation`):
- `trigger_type`: `event`
- `event_filter`: `cache_invalidation`
- `category`: `cache`
- `max_runtime_seconds`: 300

**SCH-010** (`cascade_update`):
- `trigger_type`: `event`
- `event_filter`: `cascade_update`
- `category`: `maintenance`
- `max_runtime_seconds`: 600

**SCH-011** (`operation_execution`):
- `trigger_type`: `event`
- `event_filter`: `operation`
- `category`: `operations`
- `max_runtime_seconds`: 1800

### 3. Scheduler Integration Layer

**`pkg/storage/scheduler_integration.go`**:
- `SchedulerJobManager`: Manages scheduling of background operations
- `ScheduleCacheInvalidation`: Schedules cache invalidation via event trigger
- `ScheduleCascadeUpdate`: Schedules cascade updates via event trigger
- `ScheduleOperation`: Schedules operation execution via event trigger

### 4. Enhanced Operation Executor Integration

**`pkg/storage/operation_executor_enhanced.go`**:
- `CacheManager` now uses `SchedulerJobManager` instead of direct goroutines
- Cache invalidations are scheduled via scheduler jobs
- Falls back to direct execution if scheduler unavailable

### 5. Event Data Passing

**Updated `TriggerJobByEvent`** in `pkg/scheduler/scheduler.go`:
- Passes event data via context: `context.WithValue(ctx, "event_data", eventData)`
- Handlers extract event data from context
- Enables handlers to access operation parameters

### 6. Object Spec Updates

**`docs/process/_internal/object_specs/scheduler_job.yaml`**:
- Added `cache_invalidation`, `cascade_update`, `operation_execution` to `job_type` enum

## Benefits

### 1. Managed Execution
- Scheduler handles job lifecycle
- Automatic timeout protection (`max_runtime_seconds`)
- Retry logic built-in (via scheduler job configuration)
- Status tracking (`last_run_at`, `next_run_at`)

### 2. Auditability
- All background operations generate audit events
- Execution history tracked in scheduler_job objects
- Performance metrics (execution time, success/failure)

### 3. Monitoring
- Job health monitoring via `last_run_at`
- Performance tracking
- Failure detection
- System health awareness

### 4. Configuration
- Jobs can be enabled/disabled
- Timeout configuration per job type
- Retry configuration
- Event filter configuration

### 5. Consistency
- Jobs are tracked in system objects
- Status is queryable
- Operations are auditable
- System state is observable

## Usage Flow

### Cache Invalidation Flow

```
Storage Operation (Create/Update/Delete)
    ↓
CacheManager.InvalidateAsync()
    ↓
SchedulerJobManager.ScheduleCacheInvalidation()
    ↓
Scheduler.TriggerJobByEvent("cache_invalidation", "cache_operation", eventData)
    ↓
Scheduler matches SCH-009 (event_filter: "cache_invalidation")
    ↓
CacheInvalidationHandler.Execute()
    ↓
cache_invalidation_job_start → per-ID cacheOperationHandler; per-object errors: cache_invalidation_entry_failed
    ↓
cache_invalidation_job_completed
```

### Cascade Update Flow

```
Delete Operation
    ↓
Detect dependents
    ↓
SchedulerJobManager.ScheduleCascadeUpdate()
    ↓
Scheduler.TriggerJobByEvent("cascade_update", "cascade_operation", eventData)
    ↓
Scheduler matches SCH-010 (event_filter: "cascade_update")
    ↓
CascadeUpdateHandler.Execute()
    ↓
cascade_update_job_start → cascade_update_processing
    ↓
Update dependent objects (nullify/set_null)
    ↓
cascade_update_job_completed
```

## Integration Points

### 1. Scheduler Registration

The scheduler registers itself with the storage layer:

```go
// In scheduler package initialization
storage.SetGlobalSchedulerGetter(func() interface{} {
    return GetGlobalScheduler()
})
```

This enables `SchedulerJobManager` to access the scheduler without circular dependencies.

### 2. Event Data Format

Event data passed to handlers:

**Cache Invalidation**:
```json
{
  "object_ids": ["GOAL-001", "GOAL-002"],
  "reason": "Updated GOAL-001",
  "type": "cache_invalidation"
}
```

**Cascade Update**:
```json
{
  "parent_id": "CRIT-001",
  "parent_kind": "criteria",
  "cascade_type": "nullify",
  "dependent_ids": ["TEST-001", "REQ-001"],
  "type": "cascade_update"
}
```

**Operation Execution**:
```json
{
  "operation_type": "update",
  "object_id": "GOAL-001",
  "object_kind": "goal",
  "data": {"title": "New Title"},
  "type": "operation"
}
```

## Fallback Behavior

If scheduler is not available or `TriggerJobByEvent` fails:
- **Cache Invalidation**: Direct execution via `cacheOperationHandler`; schedule failure logs **`cache_invalidation_schedule_fallback`** (`pkg/storage/scheduler_integration.go`, `operation_executor_cache.go`). Execution failure logs **`cache_invalidation_failed`**.
- **Cascade Updates**: Direct execution logs **`cascade_update_direct`**; schedule failure logs **`cascade_update_schedule_fallback`**.
- **Operations**: Returns error (operations require scheduler)

## Stable log event keys (POL-CODE-007)

Handlers use snake_case wires suitable for dashboards and grep (see handlers under `pkg/scheduler/handlers_*.go` listed in **Files Created/Modified**):

| Area | Example wires |
|------|----------------|
| Cache invalidation job | `cache_invalidation_job_start`, `cache_invalidation_job_completed`, … |
| Cascade update job | `cascade_update_job_start`, `cascade_update_processing`, `cascade_update_job_completed` |
| Operation execution job | `operation_execution_job_start`, `operation_execution_processing`, `operation_execution_job_completed` |
| Integrity check job | `integrity_check_job_start`, `integrity_check_processing`, `integrity_check_command_failed`, `integrity_check_job_completed` |
| Object validation job | `object_validation_processing`, `object_validation_skip_batch_item`, … |
| Scheduler job retention | `scheduler_job_retention_job_start`, `scheduler_job_retention_config`, `scheduler_job_retention_job_completed`, … |
| Metrics collection (file lock) | `metrics_collection_job_start`, `metrics_collection_processing`, `metrics_collection_failed`, `metrics_collection_job_completed` |
| Context refresh | `context_refresh_job_start`, `context_refresh_list_failed`, `context_refresh_schedule_refreshed`, `context_refresh_job_completed`, … |
| Autofix batch cleanup | `autofix_batch_cleanup_job_start`, `autofix_batch_cleanup_job_completed`, `autofix_batch_cleanup_partial_errors`, … |
| Cleanup (config-driven job) | `cleanup_skip_no_project_root`, `cleanup_deleted_file`, `cleanup_truncated_file`, … |
| Convergence session tick | `convergence_session_tick_applied_measure`, `convergence_session_tick_rollup_completed`, `convergence_session_tick_terminal_measurement_followup_warn`, … |
| Run wrapper | `run_wrapper_executing_command`, `run_wrapper_dynamic_timeout`, `run_wrapper_panicked`, … |
| Storage fallbacks | `cache_invalidation_schedule_fallback`, `cache_invalidation_failed`, `cascade_update_schedule_fallback`, `cascade_update_direct` |

## Next Steps

1. **Handler Implementation**: Complete cascade update logic in `CascadeUpdateHandler`
2. **Testing**: Test scheduler integration with background operations
3. **Monitoring**: Add metrics for job execution and health
4. **Documentation**: Add usage examples and best practices
5. **Event Filter Enhancement**: Support more complex event filters if needed

## Files Created/Modified

### New Files
- `pkg/storage/scheduler_integration.go`
- `pkg/scheduler/handlers_cache_invalidation.go`
- `pkg/scheduler/handlers_cascade_update.go`
- `pkg/scheduler/handlers_operation_execution.go`
- `docs/process/scheduler_jobs/SCH-009.yaml`
- `docs/process/scheduler_jobs/SCH-010.yaml`
- `docs/process/scheduler_jobs/SCH-011.yaml`
- `docs/process/architecture/scheduler-integration-v1.0.md`

### Modified Files
- `pkg/scheduler/scheduler.go`: Added new job types to `createJobHandler()`, updated `TriggerJobByEvent()` to pass event data
- `pkg/storage/operation_executor_enhanced.go`: Integrated `SchedulerJobManager` into `CacheManager`
- `pkg/storage/object_storage_file.go`: Added `GetCacheOperationHandler()`
- `docs/process/_internal/object_specs/scheduler_job.yaml`: Added new job types to enum

