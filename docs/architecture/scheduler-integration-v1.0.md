# Scheduler Integration for Background Operations

**Version**: 1.0.0  
**Created**: 2026-01-01  
**Status**: Active  
**Purpose**: Integrate concurrent operations system with scheduler_job objects for managed background processes

## Overview

Instead of spawning goroutines directly, background operations are managed through `scheduler_job` objects. This provides:
- **Managed Execution**: Scheduler handles job lifecycle, timeouts, retries
- **Audit Events**: All background operations generate audit events
- **Status Tracking**: `last_run_at`, `next_run_at` fields track execution
- **Monitoring**: Job health and performance monitoring
- **Configuration**: Jobs can be enabled/disabled, scheduled, configured

## Integration Points

### 1. Cache Invalidation

**Job**: `SCH-009` (cache_invalidation)  
**Trigger**: `event` (event_type: `cache_invalidation`)  
**Category**: `cache`

**Usage**:
```go
schedulerManager.ScheduleCacheInvalidation(ctx, objectIDs, reason)
```

**Benefits**:
- Non-blocking cache invalidation
- Audit events for all invalidations
- Status tracking (last_run_at, execution time)
- Retry logic if invalidation fails
- Timeout protection (max_runtime_seconds)

### 2. Cascade Updates

**Job**: `SCH-010` (cascade_update)  
**Trigger**: `event` (event_type: `cascade_update`)  
**Category**: `maintenance`

**Usage**:
```go
schedulerManager.ScheduleCascadeUpdate(ctx, parentID, parentKind, cascadeType, dependentIDs)
```

**Benefits**:
- Background cascade updates don't block delete operations
- Audit events for cascade operations
- Status tracking
- Retry logic for failed cascade updates
- Timeout protection

### 3. Operation Execution

**Job**: `SCH-011` (operation_execution)  
**Trigger**: `event` (event_type: `operation`)  
**Category**: `operations`

**Usage**:
```go
schedulerManager.ScheduleOperation(ctx, operationType, objectID, objectKind, data)
```

**Benefits**:
- Managed operation execution
- Progress reporting via callbacks
- Status tracking
- Retry logic
- Timeout protection

## Scheduler Job Types

### Cache Invalidation Job (SCH-009)

**Configuration**:
- `job_type`: `cache_invalidation`
- `trigger_type`: `event`
- `max_runtime_seconds`: 300 (5 minutes)
- `execution_mode`: `reusable`

**Event Data**:
```json
{
  "object_ids": ["GOAL-001", "GOAL-002"],
  "reason": "Updated GOAL-001",
  "type": "cache_invalidation"
}
```

### Cascade Update Job (SCH-010)

**Configuration**:
- `job_type`: `cascade_update`
- `trigger_type`: `event`
- `max_runtime_seconds`: 600 (10 minutes)
- `execution_mode`: `reusable`

**Event Data**:
```json
{
  "parent_id": "CRIT-001",
  "parent_kind": "criteria",
  "cascade_type": "nullify",
  "dependent_ids": ["TEST-001", "REQ-001"],
  "type": "cascade_update"
}
```

### Operation Execution Job (SCH-011)

**Configuration**:
- `job_type`: `operation_execution`
- `trigger_type`: `event`
- `max_runtime_seconds`: 1800 (30 minutes)
- `execution_mode`: `reusable`

**Event Data**:
```json
{
  "operation_type": "update",
  "object_id": "GOAL-001",
  "object_kind": "goal",
  "data": {"title": "New Title"},
  "type": "operation"
}
```

## Implementation

### SchedulerJobManager

The `SchedulerJobManager` provides a unified interface for scheduling background operations:

```go
type SchedulerJobManager struct {
    storage   ObjectStorageProvider
    scheduler SchedulerInterface
    logger    *logging.EventLogger
}
```

### Methods

1. **ScheduleCacheInvalidation**: Schedules cache invalidation via event trigger
2. **ScheduleCascadeUpdate**: Schedules cascade update via event trigger
3. **ScheduleOperation**: Schedules operation execution via event trigger

### Fallback Behavior

If scheduler is not available or job trigger fails:
- **Cache invalidation**: Direct execution via `cacheOperationHandler`. Schedule/trigger failure: **`cache_invalidation_schedule_fallback`**. Execution error: **`cache_invalidation_failed`** (`pkg/storage`).
- **Cascade updates**: Direct path: **`cascade_update_direct`**. Schedule failure: **`cascade_update_schedule_fallback`**.
- **Operations**: Returns error (operations require scheduler)

Successful handler runs emit POL-CODE-007 wires keyed by **`job_type`** (for example **`cache_invalidation_job_completed`**, **`cascade_update_job_completed`**, **`operation_execution_job_completed`**, **`integrity_check_job_completed`**, **`scheduler_job_retention_job_completed`**); see `pkg/scheduler/handlers_*.go`.

## Benefits

### 1. Managed Execution
- Scheduler handles job lifecycle
- Automatic timeout protection
- Retry logic built-in
- Status tracking

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
- Schedule configuration (for timer-based jobs)

### 5. Consistency
- Jobs are tracked in system objects
- Status is queryable
- Operations are auditable
- System state is observable

## Usage Example

```go
// Create scheduler job manager
schedulerManager := GetSchedulerJobManager(storage)

// Schedule cache invalidation (triggers SCH-009)
err := schedulerManager.ScheduleCacheInvalidation(ctx, 
    []string{"GOAL-001"}, 
    "Updated GOAL-001")

// Schedule cascade update (triggers SCH-010)
err = schedulerManager.ScheduleCascadeUpdate(ctx,
    "CRIT-001",
    "criteria",
    "nullify",
    []string{"TEST-001", "REQ-001"})

// Schedule operation (triggers SCH-011)
err = schedulerManager.ScheduleOperation(ctx,
    "update",
    "GOAL-001",
    "goal",
    map[string]interface{}{"title": "New Title"})
```

## Integration with Existing Systems

### Cache Manager Integration

`CacheManager` now uses `SchedulerJobManager`:

```go
func (cm *CacheManager) executeInvalidation(ctx context.Context, inv *PendingInvalidation) {
    // Schedule via scheduler_job instead of direct goroutine
    err := cm.schedulerManager.ScheduleCacheInvalidation(ctx, inv.ObjectIDs, inv.Reason)
    // ... handle error, fallback to direct execution
}
```

### Operation Executor Integration

`EnhancedOperationExecutor` can use scheduler for long-running operations:

```go
// For long-running operations, schedule via scheduler
if operationDuration > threshold {
    return sjm.ScheduleOperation(ctx, op.Type, op.ObjectID, op.ObjectKind, op.Data)
}
```

## Next Steps

1. **Job Handlers**: Implement handlers for `cache_invalidation`, `cascade_update`, `operation_execution` job types
2. **Event Registration**: Register event types with scheduler
3. **Job Creation**: Ensure scheduler_job objects exist and are enabled
4. **Testing**: Test scheduler integration with background operations
5. **Monitoring**: Add monitoring for job execution and health

