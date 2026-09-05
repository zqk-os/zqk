# Scheduler Execution Boundaries & Robustness

**Last Verified:** 2026-08-31


**Date:** 2026-01-05  
**Status:** Architecture Documentation  
**Version:** 1.0.0  
**Related:** Scheduler Architecture

## Overview

The scheduler already provides comprehensive execution boundaries and retry mechanisms through job configuration. No separate "retryable job" concept is needed - we just need to ensure the executor (handlers) are robust and properly respect these boundaries.

## Existing Scheduler Features

### Execution Boundaries

The scheduler job specification already provides all necessary boundaries:

```yaml
# Execution boundaries
max_runtime_seconds: 300        # Maximum execution time (0 = no timeout)
retry_count: 3                  # Maximum retry attempts (0 = no retries)
retry_delay_seconds: 5         # Delay between retries
execution_mode: reusable        # "reusable" or "one_time"
```

### How Boundaries Are Enforced

1. **Max Runtime**: `MaxRuntimeSeconds` field
   - Enforced by scheduler before job execution
   - Context timeout set to `max_runtime_seconds`
   - Job killed if exceeds timeout

2. **Retry Count**: `RetryCount` field
   - Handled by individual handlers (e.g., `RunWrapperHandler`)
   - Retries on failure up to `retry_count`
   - Each retry waits `retry_delay_seconds`

3. **Execution Mode**: `ExecutionMode` field
   - `reusable`: Job can run multiple times
   - `one_time`: Job runs once, then disabled

4. **Timeout Protection**: Context cancellation
   - All handlers receive context with timeout
   - Context cancelled on timeout
   - Handlers should check `ctx.Done()`

## Robust Executor Requirements

### Current Handler Implementation

**RunWrapperHandler** already implements:
- ✅ Retry logic (respects `RetryCount`)
- ✅ Retry delays (respects `RetryDelaySeconds`)
- ✅ Timeout handling (context cancellation)
- ✅ Error propagation
- ✅ Logging and audit events

### What Makes an Executor Robust

1. **Context Awareness**
   ```go
   func (h *Handler) Execute(ctx context.Context, job *ScheduledJob) error {
       // Check for cancellation
       select {
       case <-ctx.Done():
           return ctx.Err()
       default:
       }
       
       // Long-running operations should check context periodically
       for i := 0; i < iterations; i++ {
           if ctx.Err() != nil {
               return ctx.Err()
           }
           // Do work...
       }
   }
   ```

2. **Graceful Shutdown**
   - Clean up resources on cancellation
   - Release locks, close connections
   - Save partial state if needed

3. **Error Handling**
   - Distinguish retryable vs non-retryable errors
   - Log errors with context
   - Return appropriate error types

4. **Resource Management**
   - Limit resource usage (memory, connections)
   - Clean up resources even on error
   - Use timeouts for external calls

5. **Idempotency**
   - Operations should be safe to retry
   - Check for already-completed work
   - Use idempotency keys when possible

## Execution Boundary Examples

### Example 1: Long-Running Job with Timeout

```yaml
id: SCH-001
kind: scheduler_job
job_type: run_wrapper
command: ./long-running-script.sh
max_runtime_seconds: 3600  # 1 hour timeout
retry_count: 0              # No retries (one-shot)
execution_mode: reusable
```

**Behavior:**
- Job runs for up to 1 hour
- Killed if exceeds 1 hour
- No retries on failure
- Can be triggered multiple times

### Example 2: Retryable Job with Bounded Time

```yaml
id: SCH-002
kind: scheduler_job
job_type: run_wrapper
command: ./unreliable-api-call.sh
max_runtime_seconds: 300   # 5 minute timeout per attempt
retry_count: 3              # Retry up to 3 times
retry_delay_seconds: 10     # Wait 10 seconds between retries
execution_mode: reusable
```

**Behavior:**
- Each attempt has 5 minute timeout
- Up to 3 retries on failure
- 10 second delay between retries
- Total possible time: (5 min × 4 attempts) + (10s × 3 delays) = ~20 minutes

### Example 3: One-Time Job with Strict Timeout

```yaml
id: SCH-003
kind: scheduler_job
job_type: run_wrapper
command: ./migration-script.sh
max_runtime_seconds: 1800   # 30 minute timeout
retry_count: 1              # One retry allowed
retry_delay_seconds: 60     # Wait 1 minute before retry
execution_mode: one_time     # Runs once, then disabled
```

**Behavior:**
- Runs once (or twice if first attempt fails)
- 30 minute timeout per attempt
- Disabled after completion (success or failure)

## Handler Robustness Checklist

### ✅ Current Implementation Status

**RunWrapperHandler:**
- ✅ Respects `MaxRuntimeSeconds` (via context timeout)
- ✅ Implements retry logic (respects `RetryCount`)
- ✅ Implements retry delays (respects `RetryDelaySeconds`)
- ✅ Checks context cancellation
- ✅ Logs errors and audit events
- ✅ Handles command execution errors
- ✅ Captures stdout/stderr

**IntegrityCheckHandler:**
- ✅ Respects context timeout
- ✅ Handles errors gracefully
- ✅ Logs results

**AuditAggregationHandler:**
- ✅ Respects context timeout
- ✅ Handles aggregation errors
- ✅ Logs results

### 🔄 Potential Enhancements

1. **Better Context Checking**
   - More frequent context checks in long loops
   - Context checks before expensive operations
   - Early return on cancellation

2. **Resource Limits**
   - Memory limits for handlers
   - Connection pool limits
   - File descriptor limits

3. **Circuit Breakers**
   - Stop retrying if endpoint is consistently failing
   - Automatic recovery after cooldown
   - Configurable failure thresholds

4. **Exponential Backoff**
   - Currently uses fixed delay (`retry_delay_seconds`)
   - Could add exponential backoff option
   - Better for transient failures

5. **Partial Progress Tracking**
   - Save progress for long-running jobs
   - Resume from last checkpoint
   - Useful for data processing jobs

## No Need for Separate "Retryable Job" Concept

### Why Not?

1. **Scheduler Already Provides It**
   - `retry_count` = max failure count
   - `max_runtime_seconds` = execution time bound
   - `retry_delay_seconds` = delay between retries
   - `execution_mode` = run-once vs reusable

2. **Flexibility Through Configuration**
   - Each job can configure its own boundaries
   - No need for separate job types
   - Configuration is declarative (YAML)

3. **Consistency**
   - All jobs use same execution model
   - Same timeout/retry mechanisms
   - Same monitoring and logging

4. **Simplicity**
   - One job type, many configurations
   - Less code to maintain
   - Easier to understand

### What We Need Instead

**Robust Executor Implementation:**
- ✅ Proper context handling
- ✅ Resource cleanup
- ✅ Error classification (retryable vs not)
- ✅ Graceful degradation
- ✅ Comprehensive logging

**Enhanced Configuration (if needed):**
- Exponential backoff option
- Circuit breaker configuration
- Resource limits per job
- Progress tracking

## Best Practices for Handler Implementation

### 1. Always Check Context

```go
func (h *Handler) Execute(ctx context.Context, job *ScheduledJob) error {
    // Check at start
    if ctx.Err() != nil {
        return ctx.Err()
    }
    
    // Check in loops
    for i := 0; i < count; i++ {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }
        // Do work...
    }
}
```

### 2. Clean Up Resources

```go
func (h *Handler) Execute(ctx context.Context, job *ScheduledJob) error {
    resource := acquireResource()
    defer releaseResource(resource) // Always cleanup
    
    // Use resource...
}
```

### 3. Distinguish Error Types

```go
func (h *Handler) Execute(ctx context.Context, job *ScheduledJob) error {
    err := doWork()
    if err != nil {
        // Retryable: network errors, timeouts
        if isRetryable(err) {
            return err // Scheduler will retry
        }
        // Non-retryable: validation errors, permanent failures
        return fmt.Errorf("permanent failure: %w", err)
    }
    return nil
}
```

### 4. Use Timeouts for External Calls

```go
func (h *Handler) Execute(ctx context.Context, job *ScheduledJob) error {
    // Use context timeout for external calls
    callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()
    
    err := externalAPI.Call(callCtx, data)
    return err
}
```

### 5. Log with Context

```go
func (h *Handler) Execute(ctx context.Context, job *ScheduledJob) error {
    h.logger.Info("Starting execution",
        logging.String("job_id", job.ID),
        logging.Int("max_runtime", job.MaxRuntimeSeconds),
        logging.Int("retry_count", job.RetryCount))
    
    // ...
}
```

## Conclusion

**The scheduler already provides all necessary execution boundaries:**
- ✅ Max execution time (`max_runtime_seconds`)
- ✅ Max failure count (`retry_count`)
- ✅ Retry delays (`retry_delay_seconds`)
- ✅ Execution mode (`execution_mode`)

**What we need is robust executor implementation:**
- ✅ Proper context handling
- ✅ Resource cleanup
- ✅ Error classification
- ✅ Graceful degradation

**No separate "retryable job" concept needed** - the scheduler's flexibility through configuration is sufficient.

## Related Documentation

- [Scheduler Architecture](./scheduler-coordination-kernel.md)
- [Scheduler Job Specification](../_internal/object_specs/scheduler_job.yaml)
- [Transceiver Async Architecture](./TRANSCEIVER_ASYNC_ARCHITECTURE.md)

