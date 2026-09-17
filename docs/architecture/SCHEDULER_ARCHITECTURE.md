# Scheduler Architecture

**Status:** Active Documentation  
**Version:** 1.1.0  
**Related:** 
- [Scheduler Transceiver Architecture](SCHEDULER_TRANSCEIVER_ARCHITECTURE.md)
- [Scheduler Host Service and Cluster Status](SCHEDULER_HOST_SERVICE_AND_CLUSTER_STATUS.md)
- [Scheduler Status and Health Check Alignment](SCHEDULER_STATUS_AND_HEALTH_CHECK_ALIGNMENT.md)

## Overview

The scheduler system is a robust, event-driven job execution engine that supports multiple trigger types, callbacks, notifications, and comprehensive audit logging. It also manages concurrent background operations and interacts with the cluster status plane.

## Architecture & Daemon Topology

### 1. Scheduler Daemon
The main scheduler (`pkg/scheduler/scheduler.go`) manages:
- Job lifecycle (loading, scheduling, execution, cleanup)
- Trigger queue for immediate/manual jobs
- Job execution context and timeouts
- Audit event generation
- Notification delivery

### 2. Job Loader
The job loader (`pkg/scheduler/job_loader.go`) handles:
- Loading job definitions from storage
- Validating job configurations
- Creating `ScheduledJob` instances
- Auto-detecting callback types

### 3. Job Handlers
Job handlers execute jobs based on type (e.g., `RunWrapperHandler`, `TestIOHandler`, `CallbackListenerHandler`, `AggregationHandlers`, `CacheInvalidationHandler`, `CascadeUpdateHandler`, `OperationExecutionHandler`).

### 4. Transceiver Router
Provides async message routing (queued, non-blocking), protocol adapters, routing rules, and a worker pool for concurrent execution.

## Startup & Execution Flow

### Critical Startup Sequence
The scheduler startup order is **critical** to prevent race conditions and ensure all job types execute correctly:
1. Permission Check (`manage:scheduler`)
2. State Validation (check for already running process/PID)
3. Write PID File (`.zqk/scheduler/scheduler.pid`)
4. **⚠️ CREATE CHANNEL AND START WORKER POOL (CRITICAL ORDER)**:
   Create `triggeredJobCh` (512 buffer), create context, start 64 worker goroutines. This must happen before `cron.Start()` so cron callbacks see valid channels.
5. Load and Schedule Jobs: Hydrate jobs and schedule by trigger_type.
6. Start Cron Scheduler (`cron.Start()`).
7. Start Background Goroutines (e.g. `watchJobChanges()`, `WatchTriggerQueue()`, `healthMonitor()`, `keepAliveHeartbeat()`).
8. Start Async Router.
9. Wait for Shutdown.

### Worker Pool Architecture
- **Workers**: 64 concurrent workers
- **Queue**: 512 buffered channel (`triggeredJobCh`)
- **Context Ownership**: The worker owns the execution context and cancels it when the job completes. The producer (cron callback, event trigger) must NOT cancel the context itself.

### Job Trigger Types and Flow
1. **Manual Trigger**: Enqueue in trigger queue -> Execute.
2. **Immediate Trigger**: Execute immediately on load (`trigger_type: immediate`).
3. **Scheduled Trigger**: Cron-based or interval timer -> Wait -> Execute.
4. **Event/Lifecycle Trigger**: Execute in response to system events (e.g., cache invalidation).

## Storage & State Management
### Scheduler Integration with Operations
Instead of spawning goroutines directly, background operations are managed through `scheduler_job` objects (e.g., `SCH-009` for cache invalidation, `SCH-010` for cascade updates).
This provides:
- **Managed Execution**: Scheduler handles job lifecycle, timeouts, retries
- **Audit Events**: All background operations generate audit events
- **Status Tracking**: `last_run_at`, `next_run_at` track execution

Fallback behavior exists for critical paths (like cache invalidation) if the scheduler is not available, executing the operation directly and emitting specific log wires (`cache_invalidation_schedule_fallback`, `cache_invalidation_failed`).

## Cluster Status Plane & Host Services
The scheduler integrates with the host services and cluster status mechanisms. 
*See companion spec: [Scheduler Host Service and Cluster Status](SCHEDULER_HOST_SERVICE_AND_CLUSTER_STATUS.md) for details.*

## Concurrency & Health Alignment
Job execution is bounded. The scheduler does not spawn goroutines per job; instead, it uses a fixed worker pool of 64 concurrent workers. 
*See companion spec: [Scheduler Status and Health Check Alignment](SCHEDULER_STATUS_AND_HEALTH_CHECK_ALIGNMENT.md) for deeper details on health alignment.*

## Callbacks & Notifications
Callbacks are executed at specific points: `callback_on_completion`, `callback_on_error`, `callback_on_status`. Mechanisms include Webhook, Command, and Event.
Notifications are sent via the `NotificationContext` for user-facing alerts (started, completed, failed, timeout).

## Relevant CLI Commands

```bash
# Trigger a specific job manually
zqk scheduler trigger <JOB-ID>

# Submit a job
zqk scheduler submit <JOB-FILE>

# Run scheduler daemon
zqk scheduler run

# Manage jobs
zqk object create scheduler_job --data "..."
```
