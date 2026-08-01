# Scheduler Architecture

**Date:** 2026-01-12  
**Status:** Active Documentation  
**Version:** 1.0.0  
**Related:** SCHEDULER_TRANSCEIVER_ARCHITECTURE.md, SCHEDULER_VALIDATION.md

## Overview

The scheduler system is a robust, event-driven job execution engine that supports multiple trigger types, callbacks, notifications, and comprehensive audit logging. This document describes the complete architecture, execution flow, and integration points.

## Core Components

### 1. Scheduler

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

### 3. Job Trigger Queue

The trigger queue (`pkg/scheduler/job_trigger_queue.go`) manages:
- Immediate job triggers (fire-on-load)
- Manual triggers (via CLI or API)
- Event-based triggers (future)
- Trigger deduplication and rate limiting

### 4. Job Handlers

Job handlers (`pkg/scheduler/handlers_*.go`) execute jobs:
- **RunWrapperHandler**: Executes shell commands with timeout/retry
- **TestIOHandler**: Tests transceiver routing channels
- **CallbackListenerHandler**: Runs HTTP server for receiving callbacks
- **AggregationHandlers**: Process audit events and change journals

### 5. Notification Context

Notification context (`pkg/scheduler/notification_context.go`) handles:
- Job execution notifications (start, completion, failure)
- Priority-based notification delivery
- Integration with external notification systems (future)

### 6. Transceiver Router

Transceiver router (`pkg/scheduler/transceiver/`) provides:
- Async message routing (queued, non-blocking)
- Protocol adapters (webhook, command, event)
- Routing rules (configurable message routing)
- Worker pool for concurrent execution

## Execution Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         SCHEDULER ARCHITECTURE                          │
└─────────────────────────────────────────────────────────────────────────┘

┌──────────────────┐
│  Job Definition  │  (YAML in storage)
│  - ID, Type      │
│  - Trigger Type  │
│  - Command/Config│
│  - Callbacks     │
└────────┬─────────┘
         │
         │ Load & Validate
         ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                          JOB LOADER                                      │
│  - Load from storage                                                     │
│  - Validate configuration                                                │
│  - Create ScheduledJob instance                                          │
│  - Auto-detect callback types                                            │
└───────────────────────────────┬─────────────────────────────────────────┘
                                │
                                │ Jobs loaded
                                ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                          SCHEDULER                                       │
│                                                                           │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    TRIGGER TYPES                                 │   │
│  │                                                                   │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │   │
│  │  │   Manual     │  │  Immediate   │  │  Scheduled   │          │   │
│  │  │  (on-demand) │  │  (fire-on-   │  │  (cron/      │          │   │
│  │  │              │  │   load)      │  │   interval)  │          │   │
│  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘          │   │
│  │         │                  │                  │                  │   │
│  │         │                  │                  │                  │   │
│  │         └──────────────────┴──────────────────┘                  │   │
│  │                           │                                      │   │
│  │                           ▼                                      │   │
│  │                  ┌─────────────────┐                             │   │
│  │                  │ TRIGGER QUEUE   │                             │   │
│  │                  │ - Immediate jobs│                             │   │
│  │                  │ - Manual triggers│                            │   │
│  │                  │ - Deduplication │                             │   │
│  │                  └────────┬────────┘                             │   │
│  └───────────────────────────┼───────────────────────────────────────┘   │
│                              │                                            │
│                              │ Job ready to execute                       │
│                              ▼                                            │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    JOB EXECUTION                                │   │
│  │                                                                   │   │
│  │  1. Create execution context (with timeout)                      │   │
│  │  2. Select handler (based on job_type)                           │   │
│  │  3. Execute handler.Execute(ctx, job)                            │   │
│  │  4. Monitor for completion/timeout                               │   │
│  │  5. Record metrics                                               │   │
│  └─────────────────────────────┬───────────────────────────────────┘   │
│                                │                                        │
│                                │ Execution completes                    │
│                                ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    POST-EXECUTION                               │   │
│  │                                                                   │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │   │
│  │  │   AUDIT      │  │ NOTIFICATION │  │  CALLBACKS   │          │   │
│  │  │   EVENTS     │  │   CONTEXT    │  │              │          │   │
│  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘          │   │
│  │         │                  │                  │                  │   │
│  │         │                  │                  │                  │   │
│  │         └──────────────────┴──────────────────┘                  │   │
│  │                           │                                      │   │
│  │                           ▼                                      │   │
│  │                  ┌─────────────────┐                             │   │
│  │                  │   TRANSCEIVER   │                             │   │
│  │                  │   ASYNC ROUTER  │                             │   │
│  │                  │  (queued, non-  │                             │   │
│  │                  │   blocking)     │                             │   │
│  │                  └────────┬────────┘                             │   │
│  └───────────────────────────┼───────────────────────────────────────┘   │
└──────────────────────────────┼───────────────────────────────────────────┘
                               │
                               │ Messages routed
                               ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                    TRANSCEIVER ASYNC ROUTER                              │
│                                                                           │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    QUEUE                                        │   │
│  │  - Buffered channel (default: 100)                              │   │
│  │  - Non-blocking enqueue (with timeout)                          │   │
│  │  - Queue depth metrics                                          │   │
│  └─────────────────────────────┬───────────────────────────────────┘   │
│                                │                                        │
│                                │ Process queue                          │
│                                ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    WORKER POOL                                  │   │
│  │  - Configurable workers (default: 10)                           │   │
│  │  - Concurrent execution                                         │   │
│  │  - Panic recovery                                               │   │
│  └─────────────────────────────┬───────────────────────────────────┘   │
│                                │                                        │
│                                │ Route message                          │
│                                ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    ROUTER                                       │   │
│  │  - Match routing rules                                          │   │
│  │  - Execute actions (protocol adapters)                          │   │
│  │  - Retry logic (if configured)                                  │   │
│  │  - Payload transformation (if configured)                        │   │
│  └─────────────────────────────┬───────────────────────────────────┘   │
│                                │                                        │
│                                │ Protocol adapter                       │
│                                ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    PROTOCOL ADAPTERS                            │   │
│  │                                                                   │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │   │
│  │  │   Webhook    │  │   Command    │  │    Event     │          │   │
│  │  │   (HTTP)     │  │   (exec)     │  │  (internal)  │          │   │
│  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘          │   │
│  │         │                  │                  │                  │   │
│  │         └──────────────────┴──────────────────┘                  │   │
│  │                           │                                      │   │
│  │                           ▼                                      │   │
│  │                  ┌─────────────────┐                             │   │
│  │                  │   DESTINATION   │                             │   │
│  │                  │   (external     │                             │   │
│  │                  │    systems)     │                             │   │
│  │                  └─────────────────┘                             │   │
│  └─────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
```

## Detailed Flow: Job Execution

### Phase 1: Job Triggering

```
┌─────────────────────────────────────────────────────────────────┐
│                    TRIGGER TYPES                                │
└─────────────────────────────────────────────────────────────────┘

1. MANUAL TRIGGER
   ┌─────────────┐
   │ CLI/API     │  "zqk scheduler trigger SCH-001"
   └──────┬──────┘
          │
          ▼
   ┌─────────────┐
   │ TriggerQueue│  Enqueue job for execution
   └──────┬──────┘
          │
          ▼
   ┌─────────────┐
   │ Execute Job │
   └─────────────┘

2. IMMEDIATE TRIGGER
   ┌─────────────┐
   │ Job Loaded  │  trigger_type: immediate
   └──────┬──────┘
          │
          ▼
   ┌─────────────┐
   │scheduleImme-│  Execute immediately on load
   │diateJob()   │
   └──────┬──────┘
          │
          ▼
   ┌─────────────┐
   │ Execute Job │
   └─────────────┘

3. SCHEDULED TRIGGER
   ┌─────────────┐
   │ Cron Expr   │  schedule: "0 2 * * *" (daily at 2am)
   └──────┬──────┘
          │
          ▼
   ┌─────────────┐
   │ Calculate   │  NextRunTime = cron.Next(now)
   │ NextRunTime │
   └──────┬──────┘
          │
          ▼
   ┌─────────────┐
   │ Schedule    │  Add to cron scheduler (if enabled)
   │ Timer Job   │
   └──────┬──────┘
          │
          │ Time expires
          ▼
   ┌─────────────┐
   │ Execute Job │
   └─────────────┘
```

### Phase 2: Job Execution

```
┌─────────────────────────────────────────────────────────────────┐
│                    JOB EXECUTION FLOW                           │
└─────────────────────────────────────────────────────────────────┘

1. PRE-EXECUTION SETUP
   ┌─────────────┐
   │ Execute Job │
   └──────┬──────┘
          │
          ├─── Create execution context (with timeout)
          ├─── Mark job as Running
          ├─── Store execution context for cancellation
          └─── Record start metrics
          
2. SELECT HANDLER
   ┌─────────────┐
   │ Job Type    │  job_type: run_wrapper, test_io, etc.
   └──────┬──────┘
          │
          ▼
   ┌─────────────┐
   │ Handler     │  RunWrapperHandler, TestIOHandler, etc.
   │ Factory     │
   └──────┬──────┘
          │
          ▼
   ┌─────────────┐
   │ Handler.    │  Execute(ctx, job)
   │ Execute()   │
   └──────┬──────┘
          │
          ▼
   ┌───────────────────────────────────────────────────────┐
   │            HANDLER-SPECIFIC EXECUTION                │
   │                                                       │
   │  RunWrapperHandler:                                   │
   │    - Parse command and arguments                      │
   │    - Set environment variables                        │
   │    - Execute command with timeout                     │
   │    - Capture stdout/stderr                            │
   │    - Handle retries (if configured)                   │
   │    - Execute callbacks (on completion/error)          │
   │    - Route messages through transceiver               │
   │                                                       │
   │  TestIOHandler:                                       │
   │    - Send test messages to transceiver                │
   │    - Verify routing channels                          │
   │                                                       │
   │  CallbackListenerHandler:                             │
   │    - Start HTTP server                                │
   │    - Register routes                                  │
   │    - Handle incoming callbacks                        │
   └───────────────────────┬───────────────────────────────┘
                           │
                           │ Execution completes (success or failure)
                           ▼
```

### Phase 3: Post-Execution

```
┌─────────────────────────────────────────────────────────────────┐
│                    POST-EXECUTION FLOW                          │
└─────────────────────────────────────────────────────────────────┘

┌─────────────┐
│ Job Executed│  (success or failure)
└──────┬──────┘
       │
       ├──────────────────────────────────────────────────────┐
       │                                                      │
       ▼                                                      ▼
┌──────────────┐                                      ┌──────────────┐
│ AUDIT EVENTS │                                      │ NOTIFICATIONS│
│              │                                      │              │
│ Created for: │                                      │ Created for: │
│ - started    │                                      │ - completion │
│ - completed  │                                      │ - failure    │
│ - failed     │                                      │ - timeout    │
│              │                                      │              │
│ Stored in:   │                                      │ Delivered via│
│ - audit_event│                                      │ - Notification│
│   storage    │                                      │   Context    │
└──────┬───────┘                                      └──────┬───────┘
       │                                                      │
       │                                                      │
       └──────────────────┬──────────────────────────────────┘
                          │
                          ▼
                  ┌──────────────┐
                  │   CALLBACKS  │
                  │              │
                  │ Types:       │
                  │ - completion │  callback_on_completion
                  │ - error      │  callback_on_error
                  │ - status     │  callback_on_status
                  │              │
                  │ Mechanisms:  │
                  │ - webhook    │  HTTP POST
                  │ - command    │  Execute command
                  │ - event      │  Emit event (future)
                  └──────┬───────┘
                         │
                         │ (Currently: Direct execution)
                         │ (Future: Route through transceiver)
                         ▼
                  ┌──────────────┐
                  │ TRANSCEIVER  │
                  │ ASYNC ROUTER │
                  │              │
                  │ Routes:      │
                  │ - Messages   │  scheduler_job_completed
                  │ - Messages   │  scheduler_job_failed
                  │ - Messages   │  scheduler_job_status
                  │ - Callbacks  │  scheduler_job_callback_completion
                  │ - Callbacks  │  scheduler_job_callback_error
                  │ - Callbacks  │  scheduler_job_callback_status
                  └──────┬───────┘
                         │
                         │ Queued, processed by worker pool
                         ▼
                  ┌──────────────┐
                  │  DESTINATIONS│
                  │              │
                  │ - Webhooks   │
                  │ - Commands   │
                  │ - Events     │
                  └──────────────┘
```

## Trigger Types

### 1. Manual Trigger

**Usage:** On-demand execution via CLI or API

```bash
zqk scheduler trigger SCH-001
```

**Flow:**
1. Command/API call
2. Job enqueued in trigger queue
3. Scheduler picks up job
4. Job executed immediately

**Use Cases:**
- One-time operations
- Testing
- Manual intervention
- Event-driven triggers (future)

### 2. Immediate Trigger

**Usage:** Fire immediately when job is loaded

```yaml
trigger_type: immediate
```

**Flow:**
1. Job loaded from storage
2. `scheduleImmediateJob()` called
3. Job executed immediately (fire-on-load)
4. Job marked as executed (if `execution_mode: one_time`)

**Use Cases:**
- Initialization jobs
- One-time setup tasks
- Jobs submitted via `zqk scheduler submit`

### 3. Scheduled Trigger

**Usage:** Cron-based or interval-based scheduling

```yaml
trigger_type: scheduled
schedule: "0 2 * * *"  # Daily at 2am
# OR
interval_seconds: 3600  # Every hour
```

**Flow:**
1. Job loaded with schedule
2. Next run time calculated
3. Job scheduled with cron/interval timer
4. Timer fires → job executed
5. Next run time calculated again (if recurring)

**Use Cases:**
- Periodic maintenance
- Regular aggregation
- Scheduled backups
- Daily/weekly reports

### 4. Event Trigger (Future)

**Usage:** Trigger based on system events

```yaml
trigger_type: event
event_filter: scheduler_job_completed
event_conditions:
  job_id: SCH-002
```

**Flow:**
1. Event occurs (e.g., job completion)
2. Event matches filter/conditions
3. Job triggered automatically

**Use Cases:**
- Dependent jobs (job B runs after job A completes)
- Cascade operations
- Event-driven workflows

## Callbacks

Callbacks are executed at specific points in job execution:

### Callback Types

1. **Completion Callback** (`callback_on_completion`)
   - Executed when job completes successfully
   - Receives: job_id, command, duration, success, stdout, exit_code

2. **Error Callback** (`callback_on_error`)
   - Executed when job fails (after all retries)
   - Receives: job_id, command, error, stderr, attempts, exit_code

3. **Status Callback** (`callback_on_status`)
   - Executed at job start (if configured)
   - Receives: job_id, command, status, timestamp

### Callback Mechanisms

1. **Webhook** (`callback_type: webhook`)
   - HTTP POST to URL
   - JSON payload
   - 5-second timeout
   - Route through transceiver (async, queued)
   - Falls back to direct execution if routing fails

2. **Command** (`callback_type: command`)
   - Execute local command
   - JSON payload via stdin
   - 10-second timeout
   - Route through transceiver (async, queued)
   - Falls back to direct execution if routing fails

3. **Event** (`callback_type: event`)
   - Emit system event
   - Placeholder for future event system
   - Currently: Logged only

### Callback Flow (Implemented)

```
Job Execution
    │
    ├─── On Start → executeCallback("status", payload)
    │         │
    │         └─── CreateCallbackMessage("status", payload)
    │                   │
    │                   └─── RouteJobMessageAsync(callbackMsg)
    │                             │
    │                             ├─── Success → Async Router Queue
    │                             │                  │
    │                             │                  └─── Worker Pool → Router → Routing Rules → Webhook/Command
    │                             │
    │                             └─── Failure → Fallback to executeCallbackDirect()
    │                                                │
    │                                                └─── Direct HTTP POST / Command execution
    │
    ├─── On Success → executeCallback("completion", payload)
    │         │
    │         └─── CreateCallbackMessage("completion", payload)
    │                   │
    │                   └─── RouteJobMessageAsync(callbackMsg)
    │                             │
    │                             └─── (same flow as above)
    │
    └─── On Failure → executeCallback("error", payload)
              │
              └─── CreateCallbackMessage("error", payload)
                        │
                        └─── RouteJobMessageAsync(callbackMsg)
                                  │
                                  └─── (same flow as above)
```

## Notifications

Notifications are sent via the `NotificationContext` for user-facing alerts:

### Notification Types

1. **Job Started**
   - Priority: Medium
   - Contains: job_id, job_type, category, command

2. **Job Completed**
   - Priority: Medium
   - Contains: job_id, duration, command, stdout

3. **Job Failed**
   - Priority: High
   - Contains: job_id, error, command, stderr, attempts

4. **Job Timeout**
   - Priority: High
   - Contains: job_id, max_runtime_seconds

### Notification Flow

```
Job Execution
    │
    ├─── On Start → CreateJobNotification("started", ...)
    │         │
    │         └─── NotificationContext.Notify(notification)
    │
    ├─── On Success → CreateJobNotification("completed", ...)
    │         │
    │         └─── NotificationContext.Notify(notification)
    │
    └─── On Failure → CreateJobNotification("failed", ...)
              │
              └─── NotificationContext.Notify(notification)
```

**Future:** Notifications can be routed through transceiver for external delivery (Slack, email, etc.)

## Audit Events

Audit events are created for all job operations and stored in the audit event storage:

### Audit Event Types

1. **scheduler_job_started**
   - Created when job execution begins
   - Contains: job_id, job_type, category, command

2. **scheduler_job_completed**
   - Created when job completes successfully
   - Contains: job_id, duration, success, exit_code, stdout

3. **scheduler_job_failed**
   - Created when job fails (after retries)
   - Contains: job_id, error, attempts, exit_code, stderr

### Audit Event Flow

```
Job Execution
    │
    ├─── On Start → CreateAuditEvent("scheduler_job_started", ...)
    │         │
    │         └─── Store in audit_event storage
    │
    ├─── On Success → CreateAuditEvent("scheduler_job_completed", ...)
    │         │
    │         └─── Store in audit_event storage
    │
    └─── On Failure → CreateAuditEvent("scheduler_job_failed", ...)
              │
              └─── Store in audit_event storage
```

Audit events are also routed through the transceiver async router for downstream processing (aggregation, metrics, etc.)

## Transceiver Integration

The transceiver async router provides asynchronous, queued message routing:

### Message Types Routed

1. **Job Completion Messages**
   - Event type: `scheduler_job_completed`
   - Source: `scheduler`
   - Payload: job_id, duration, success, exit_code, stdout

2. **Job Failure Messages**
   - Event type: `scheduler_job_failed`
   - Source: `scheduler`
   - Payload: job_id, error, attempts, exit_code, stderr

3. **Job Status Messages**
   - Event type: `scheduler_job_status`
   - Source: `scheduler`
   - Payload: job_id, status, progress

### Transceiver Flow

```
Job Execution
    │
    ├─── On Success → CreateCompletionMessage(job, ...)
    │         │
    │         └─── RouteJobMessageAsync(message)
    │                   │
    │                   └─── AsyncRouter.RouteAsync()
    │                             │
    │                             ├─── Queue (buffered channel)
    │                             │
    │                             ├─── Worker Pool (concurrent processing)
    │                             │
    │                             ├─── Router (rule matching)
    │                             │
    │                             └─── Protocol Adapters (webhook/command/event)
    │
    └─── On Failure → CreateErrorMessage(job, ...)
              │
              └─── RouteJobMessageAsync(message)
```

### Routing Rules

Routing rules configure how messages are delivered:

```yaml
routes:
  - name: route_completion_to_webhook
    enabled: true
    priority: 100
    match:
      event_type: scheduler_job_completed
      source: scheduler
      job_category: maintenance
    actions:
      - protocol: webhook
        endpoint: https://api.example.com/webhooks/job-complete
        timeout: 5s
        retry:
          max_attempts: 3
          initial_delay: 1s
```

## Metrics and Monitoring

The scheduler records metrics for:

1. **Job Execution**
   - Execution count (success/failure)
   - Execution duration
   - Timeout count

2. **Transceiver Router**
   - Messages routed
   - Rules matched
   - Actions executed (success/failure)
   - Queue depth
   - Queue full events
   - Messages dropped

3. **Worker Pool**
   - Active workers
   - Tasks queued
   - Tasks completed

## Error Handling and Resilience

### Timeout Handling

- Job execution timeouts: Configurable per job (`max_runtime_seconds`)
- Webhook timeouts: 5 seconds (hardcoded)
- Command callback timeouts: 10 seconds (hardcoded)
- Async router queue timeout: 100ms (default)

### Retry Logic

1. **Job Retries**
   - Configurable per job (`retry_count`)
   - Exponential backoff (future)
   - Retry delay configurable

2. **Callback Retries**
   - Currently: No retries (direct execution)
   - Future: Via routing rules (configurable retry)

3. **Transceiver Routing Retries**
   - Configurable per routing rule
   - Exponential backoff
   - Max attempts limit

### Failure Modes

1. **Job Execution Failure**
   - Error callback executed
   - Failure audit event created
   - Failure notification sent
   - Error message routed through transceiver

2. **Callback Failure**
   - Logged as warning (non-fatal)
   - Job execution continues
   - Future: Retry via transceiver routing rules

3. **Transceiver Queue Full**
   - Message dropped (logged)
   - Metrics recorded
   - Job execution not affected (non-blocking)

## Related Documentation

- [Scheduler Transceiver Architecture](SCHEDULER_TRANSCEIVER_ARCHITECTURE.md)
- [Scheduler Validation Guide](../system-health/SCHEDULER_VALIDATION.md)
- [Callback Validation Guide](../system-health/CALLBACK_VALIDATION.md)
- [Job Output Mechanisms](../system-health/JOB_OUTPUT_MECHANISMS.md)
- [Router Testing Guide](../system-health/ROUTER_TESTING_GUIDE.md)
