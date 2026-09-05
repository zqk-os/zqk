# Scheduler Startup Sequence and Job Execution Flow

**Last Verified:** 2026-08-31


**Date:** 2026-02-11  
**Status:** Active Documentation  
**Version:** 1.0.0  
**Related:** SCHEDULER_ARCHITECTURE.md, SCHEDULER_DIAGNOSTICS_VIEWING.md

## Overview

This document describes the **critical startup sequence** and **job execution flows** for the scheduler daemon. Understanding these flows is essential for debugging issues related to job scheduling, context management, and thread/goroutine lifecycle.

## Critical Startup Sequence

The scheduler startup order is **critical** to prevent race conditions and ensure all job types execute correctly.

### Startup Flow Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                    SCHEDULER STARTUP SEQUENCE                    │
└─────────────────────────────────────────────────────────────────┘

1. Permission Check
   ┌─────────────┐
   │ Check       │  manage:scheduler permission
   │ Permission  │
   └──────┬──────┘
          │ ✓
          ▼
2. State Validation
   ┌─────────────┐
   │ Check       │  - Already running in this process?
   │ Running     │  - Already running in another process (PID file)?
   │ State       │
   └──────┬──────┘
          │ ✓
          ▼
3. Write PID File
   ┌─────────────┐
   │ Write       │  .zqk/scheduler/scheduler.pid
   │ PID File    │  (for cross-process detection)
   └──────┬──────┘
          │ ✓
          ▼
4. ⚠️ CREATE CHANNEL AND START WORKER POOL (CRITICAL ORDER)
   ┌─────────────────────────────────────┐
   │ Create triggeredJobCh (512 buffer)   │
   │ Create triggeredJobCtx               │
   │ Start 64 worker goroutines          │  ⚠️ Workers MUST run before
   │   (each: read channel → executeJob  │     loadAndScheduleJobs so
   │    → work.cancel())                 │     immediate jobs can be
   │                                     │     dispatched to the pool
   │ ⚠️ MUST be before cron.Start()      │  Prevents: cron fires → nil
   │    so cron callbacks see valid      │  channel → job skipped
   │    channel and workers              │
   └──────┬──────────────────────────────┘
          │ ✓
          ▼
5. Load and Schedule Jobs
   ┌─────────────────────────────────────┐
   │ loadAndScheduleJobs()               │
   │   ├─ Load jobs from storage         │
   │   ├─ Hydrate jobs (parallel pool)  │
   │   └─ Schedule by trigger_type:      │
   │      ├─ timer → scheduleTimerJob() │  Registers cron callbacks
   │      ├─ immediate → submit work     │  Sends to triggeredJobCh
   │      │              to pool         │  (no per-job goroutine)
   │      └─ event/lifecycle → register │  Registers for triggers
   │         TriggeredJob()              │
   └──────┬──────────────────────────────┘
          │ ✓
          ▼
6. Start Cron Scheduler
   ┌─────────────────────────────────────┐
   │ cron.Start()                        │  Channel and workers exist
   └──────┬──────────────────────────────┘
          │ ✓
          ▼
7. Start Background Goroutines
   ┌─────────────────────────────────────┐
   │ ├─ watchJobChanges()               │  Reloads jobs every 30s
   │ ├─ WatchTriggerQueue()              │  Processes manual triggers
   │ ├─ healthMonitor()                 │  Detects missed jobs
   │ └─ keepAliveHeartbeat()            │  Writes keep-alive file
   └──────┬──────────────────────────────┘
          │ ✓
          ▼
8. Start Async Router (if available)
   ┌─────────────────────────────────────┐
   │ Load routing rules                  │
   │ Start async router                  │
   └──────┬──────────────────────────────┘
          │ ✓
          ▼
10. Wait for Shutdown
   ┌─────────────────────────────────────┐
   │ <-ctx.Done()                        │  Blocks until context cancelled
   │                                     │  (SIGTERM, shutdown signal)
   └─────────────────────────────────────┘
```

### Critical Order: Why Channel Before Cron?

**Problem:** If `triggeredJobCh` is created **after** `cron.Start()`, there's a race condition:

1. Cron starts → timer callbacks can fire immediately
2. Callback executes → checks `if s.triggeredJobCh == nil { return }`
3. Channel not created yet → callback silently returns, job skipped
4. Channel created → but job already missed its window

**Solution:** Create channel **before** `cron.Start()` so all callbacks see a valid channel. Start workers **before** `loadAndScheduleJobs()` so immediate jobs can be dispatched to the pool instead of spawning a goroutine per immediate job.

**Code Location:** `pkg/scheduler/scheduler.go` (Start method)

### Goroutine budget (no unbounded growth)

Job execution must **not** spawn goroutines per job; otherwise thread count grows without bound.

| Operation | Previous (bad) | Current (bounded) |
|-----------|----------------|-------------------|
| Activity cache + coordinator emit | 2 goroutines × 3 events/job = **6 per job** | **0** (sync in worker) |
| Health metric record + emit | 1 + 2 goroutines per health cycle | **0** (sync) |
| Immediate jobs at startup | 1 goroutine per immediate job | **0** (submit to pool) |
| Timer / event / lifecycle / manual | Already used pool | Pool (64 workers) |

**Rule:** Do not add "fire-and-forget" goroutines in paths that run per job or per event. Use the existing worker pool or do the work synchronously in the caller.

## Job Execution Flows by Trigger Type

### 1. Timer Jobs (Cron)

```
┌─────────────────────────────────────────────────────────────────┐
│                    TIMER JOB EXECUTION FLOW                     │
└─────────────────────────────────────────────────────────────────┘

Cron Timer Fires
    │
    ▼
┌─────────────────────────────────────┐
│ Cron Callback (scheduleTimerJob)   │
│                                     │
│ 1. Create jobCtx with timeout      │  context.WithTimeout(...)
│ 2. Create cancel function           │  cancel := jobCtx.CancelFunc
│ 3. Check triggeredJobCh != nil     │  (should always be true)
│ 4. Wait under goroutine ceiling    │  WaitUnderGoroutineCeiling()
│ 5. Build triggeredJobWork:          │
│    - job                            │
│    - handler                        │
│    - ctx: jobCtx                    │
│    - cancel: cancel                 │  ⚠️ Pass cancel, don't defer
│ 6. Send to triggeredJobCh          │  Non-blocking select
└──────┬──────────────────────────────┘
       │ ✓ Sent
       │
       │ ⚠️ Callback returns WITHOUT calling cancel()
       │    Worker owns the context and will cancel when done
       │
       ▼
┌─────────────────────────────────────┐
│ Worker Pool (64 workers)            │
│                                     │
│ Worker receives work from channel   │
│   ├─ Execute: executeJob(work.ctx) │
│   └─ Cancel: work.cancel()          │  ⚠️ Worker cancels when done
└─────────────────────────────────────┘
```

**Key Points:**
- **Context ownership:** Worker owns the context, cancels when job completes
- **Producer (cron callback):** Must NOT call `cancel()` or defer it; passes cancel to worker
- **Worker:** Calls `work.cancel()` after `executeJob()` returns

### 2. Event-Triggered Jobs

```
┌─────────────────────────────────────────────────────────────────┐
│                  EVENT-TRIGGERED JOB EXECUTION FLOW             │
└─────────────────────────────────────────────────────────────────┘

External Event Occurs
    │
    ▼
┌─────────────────────────────────────┐
│ TriggerJobByEvent()                 │
│                                     │
│ 1. Find matching jobs (event filter)│
│ 2. For each job:                    │
│    ├─ Create jobCtx with timeout    │  Job-owned context
│    ├─ Attach event data             │  context.WithValue(...)
│    ├─ Propagate operation callback  │  From caller's context
│    ├─ Build triggeredJobWork        │
│    │   - cancel: cancel             │  ⚠️ Pass cancel to worker
│    └─ Send to triggeredJobCh        │  Non-blocking select
│                                     │
│ ⚠️ Caller's context may be short-  │  HTTP request, storage call
│    lived; job needs own context     │
└──────┬──────────────────────────────┘
       │ ✓ Sent
       │
       │ ⚠️ Function returns; caller's context may be cancelled
       │    but jobCtx remains valid (worker owns it)
       │
       ▼
┌─────────────────────────────────────┐
│ Worker Pool                         │
│   ├─ Execute: executeJob(work.ctx) │
│   └─ Cancel: work.cancel()          │
└─────────────────────────────────────┘
```

**Key Points:**
- **Job-owned context:** Each job gets its own context with timeout
- **Event data propagation:** Event data attached via `context.WithValue()`
- **Operation callback:** Propagated from caller so callbacks still fire

### 3. Lifecycle-Triggered Jobs

```
┌─────────────────────────────────────────────────────────────────┐
│              LIFECYCLE-TRIGGERED JOB EXECUTION FLOW             │
└─────────────────────────────────────────────────────────────────┘

Object Lifecycle Transition
    │
    ▼
┌─────────────────────────────────────┐
│ TriggerJobByLifecycle()             │
│                                     │
│ 1. Find matching jobs (lifecycle   │
│    filter)                          │
│ 2. For each job:                    │
│    ├─ Create jobCtx with timeout    │  Job-owned context
│    ├─ Propagate operation callback  │  From caller's context
│    ├─ Build triggeredJobWork        │
│    │   - cancel: cancel             │  ⚠️ Pass cancel to worker
│    └─ Send to triggeredJobCh       │  Non-blocking select
└──────┬──────────────────────────────┘
       │ ✓ Sent
       │
       ▼
┌─────────────────────────────────────┐
│ Worker Pool                         │
│   ├─ Execute: executeJob(work.ctx) │
│   └─ Cancel: work.cancel()          │
└─────────────────────────────────────┘
```

**Key Points:**
- Same pattern as event-triggered: job-owned context, worker cancels

### 4. Manual Triggers (TriggerJob)

```
┌─────────────────────────────────────────────────────────────────┐
│                  MANUAL TRIGGER JOB EXECUTION FLOW              │
└─────────────────────────────────────────────────────────────────┘

CLI/HTTP/Queue Request
    │
    ▼
┌─────────────────────────────────────┐
│ TriggerJob()                        │
│                                     │
│ 1. Lookup job by ID                 │
│ 2. Validate trigger_type allows     │
│    manual triggering                │
│ 3. Create jobCtx with timeout       │  Job-owned context
│ 4. Propagate operation callback      │  From caller's context
│                                     │
│ IF triggeredJobCh != nil:           │  ⚠️ Scheduler started
│    ├─ Build triggeredJobWork        │
│    │   - cancel: cancel             │
│    └─ Send to triggeredJobCh        │  → Worker pool (bounded)
│                                     │
│ ELSE:                               │  ⚠️ Scheduler not started
│    └─ Spawn goroutine directly      │  → Fallback (tests, edge cases)
│       └─ defer cancel()             │
└──────┬──────────────────────────────┘
       │
       ├─→ Worker Pool (if channel exists)
       │   ├─ Execute: executeJob(work.ctx)
       │   └─ Cancel: work.cancel()
       │
       └─→ Direct Goroutine (if channel nil)
           ├─ Execute: executeJob(jobCtx)
           └─ Cancel: defer cancel()
```

**Key Points:**
- **Bounded pool:** Manual triggers use same worker pool as event/lifecycle (prevents thread explosion)
- **Fallback:** If scheduler not started, spawns goroutine directly (for tests)

### 5. Immediate Jobs

```
┌─────────────────────────────────────────────────────────────────┐
│                  IMMEDIATE JOB EXECUTION FLOW                   │
└─────────────────────────────────────────────────────────────────┘

Job Loaded (trigger_type: immediate)
    │
    ▼
┌─────────────────────────────────────┐
│ scheduleImmediateJob()              │
│                                     │
│ 1. Create jobCtx with timeout       │  Job-owned context
│ 2. Spawn goroutine:                 │
│    └─ executeJob(jobCtx)            │
│    └─ defer cancel()                │  Goroutine cancels when done
└─────────────────────────────────────┘
```

**Key Points:**
- **Direct execution:** Runs immediately in own goroutine (not via worker pool)
- **One-time:** If `execution_mode: one_time`, marked as executed

### 6. Missed Job Recovery

```
┌─────────────────────────────────────────────────────────────────┐
│                  MISSED JOB RECOVERY FLOW                        │
└─────────────────────────────────────────────────────────────────┘

Health Monitor Detects Missed Job
    │
    ▼
┌─────────────────────────────────────┐
│ recoverMissedJob()                   │
│                                     │
│ 1. Create recoveryCtx with timeout  │  Job-owned context
│ 2. Create cancel function           │
│ 3. Wait under goroutine ceiling    │
│ 4. Build triggeredJobWork:          │
│    - cancel: cancel                 │  ⚠️ Pass cancel to worker
│ 5. Send to triggeredJobCh          │
└──────┬──────────────────────────────┘
       │ ✓ Sent
       │
       ▼
┌─────────────────────────────────────┐
│ Worker Pool                         │
│   ├─ Execute: executeJob(work.ctx) │
│   └─ Cancel: work.cancel()          │
└─────────────────────────────────────┘
```

**Key Points:**
- Same pattern as timer: worker owns context, cancels when done

## Context Ownership and Cancellation

### The Critical Pattern

**Problem:** If the producer (cron callback, event trigger, etc.) calls `cancel()` or defers it, the context is cancelled as soon as the producer returns, **before** the worker can execute the job.

**Solution:** Worker owns the context and cancels it when the job completes.

### Context Lifecycle Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│              CONTEXT LIFECYCLE (CORRECT PATTERN)                 │
└─────────────────────────────────────────────────────────────────┘

Producer (Cron/Event/Lifecycle/Manual)
    │
    ├─ Create: jobCtx, cancel := context.WithTimeout(...)
    │
    ├─ Build: triggeredJobWork{ctx: jobCtx, cancel: cancel}
    │
    ├─ Send: triggeredJobCh <- work
    │
    └─ Return: ⚠️ DO NOT call cancel() here
              ⚠️ DO NOT defer cancel()
              Worker will cancel when done
    │
    ▼
Worker Pool
    │
    ├─ Receive: work := <-triggeredJobCh
    │
    ├─ Execute: executeJob(work.ctx, work.job, work.handler)
    │   │
    │   └─ Job runs with valid context
    │
    └─ Cancel: work.cancel()  ⚠️ Worker cancels when done
```

### What Happens If Producer Cancels Too Early?

```
┌─────────────────────────────────────────────────────────────────┐
│              INCORRECT PATTERN (BUG)                             │
└─────────────────────────────────────────────────────────────────┘

Producer
    │
    ├─ Create: jobCtx, cancel := context.WithTimeout(...)
    │
    ├─ defer cancel()  ⚠️ WRONG: Cancels when function returns
    │
    ├─ Send: triggeredJobCh <- work
    │
    └─ Return: cancel() called immediately
    │
    ▼
Worker Pool
    │
    ├─ Receive: work := <-triggeredJobCh
    │
    ├─ Execute: executeJob(work.ctx, ...)
    │   │
    │   └─ ⚠️ Context already cancelled!
    │   └─ Job fails with "context canceled"
    │
    └─ Cancel: work.cancel()  (no-op, already cancelled)
```

## Worker Pool Architecture

### Pool Configuration

- **Workers:** 64 concurrent workers
- **Queue:** 512 buffered channel (`triggeredJobCh`)
- **Job Types:** Timer, Event, Lifecycle, Manual (all use same pool)

### Worker Pool Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                    WORKER POOL ARCHITECTURE                     │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────┐
│ triggeredJobCh (512 buffer)         │
│                                     │
│ Producers:                          │
│ ├─ Cron callbacks (timer jobs)      │
│ ├─ TriggerJobByEvent()              │
│ ├─ TriggerJobByLifecycle()          │
│ ├─ TriggerJob() (manual)            │
│ └─ recoverMissedJob()               │
└──────┬──────────────────────────────┘
       │
       │ Work items queued
       ▼
┌─────────────────────────────────────┐
│ 64 Worker Goroutines                │
│                                     │
│ Each worker:                        │
│   for {                             │
│     select {                        │
│     case <-triggeredJobCtx.Done():  │  Shutdown signal
│       return                         │
│     case work := <-triggeredJobCh:   │  Receive work
│       executeJob(work.ctx, ...)      │  Execute job
│       work.cancel()                  │  Cancel context
│     }                                │
│   }                                  │
└─────────────────────────────────────┘
```

### Why Bounded Pool?

**Before Fix:** Manual triggers spawned one goroutine per trigger → unbounded growth (2700+ threads)

**After Fix:** All triggers use bounded pool → max 64 concurrent + 512 queued

## Job Execution Details

### executeJob Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                    executeJob() INTERNAL FLOW                   │
└─────────────────────────────────────────────────────────────────┘

executeJob(ctx, job, handler)
    │
    ├─ Re-read job from storage (check enabled status)
    │
    ├─ Check conflicts (ConflictManager.CanRun())
    │
    ├─ Create operation callback
    │
    ├─ Execute handler.Execute(ctx, job)
    │   │
    │   └─ Handler-specific execution:
    │      ├─ RunWrapperHandler: Execute command
    │      ├─ AggregationHandler: Process events
    │      ├─ CachePrewarmHandler: Warm caches
    │      └─ ... (other handlers)
    │
    ├─ Record metrics
    │
    ├─ Create audit events
    │
    ├─ Send notifications
    │
    └─ Emit coordinator events
```

## Startup Order Summary

**Critical sequence (must be preserved):**

1. ✅ Permission check
2. ✅ State validation
3. ✅ Write PID file
4. ✅ **Create triggeredJobCh** ← Must be before cron.Start()
5. ✅ Load and schedule jobs (registers cron callbacks)
6. ✅ **Start cron** ← Channel already exists
7. ✅ Start worker pool (64 workers)
8. ✅ Start background goroutines
9. ✅ Start async router
10. ✅ Wait for shutdown

**Why this order matters:**
- Channel before cron prevents race condition (callbacks see nil)
- Workers before cron ensures pool is ready when jobs fire
- Background goroutines start after core components

## Related Documentation

- [Scheduler Architecture](SCHEDULER_ARCHITECTURE.md) - Overall architecture
- [Scheduler Diagnostics Viewing](../system-health/SCHEDULER_DIAGNOSTICS_VIEWING.md) - How to diagnose issues
- [Scheduler Concurrency Analysis](scheduler-concurrency-analysis.md) - Concurrency patterns
