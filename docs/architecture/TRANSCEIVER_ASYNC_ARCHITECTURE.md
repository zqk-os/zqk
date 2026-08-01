# Transceiver Async Architecture

**Date:** 2026-01-05  
**Status:** Implemented  
**Version:** 1.0.0  
**Related:** SCHEDULER_TRANSCEIVER_ARCHITECTURE.md

## Overview

The transceiver router uses async execution with worker pools to prevent blocking job execution. Messages are queued and processed by a pool of workers, ensuring that slow or hanging connections don't tie up the scheduler.

## Architecture

### Async Execution Model

```
Job Execution → RouteAsync() → Queue → Worker Pool → Protocol Adapters
     ↓              ↓            ↓          ↓              ↓
  Non-blocking   Immediate   Buffered   Concurrent    HTTP/Command/Event
  Return         Return      Queue     Workers        (with timeouts)
```

### Components

1. **AsyncRouter**: Wraps Router with async execution
   - Non-blocking `RouteAsync()` method
   - Queue-based message buffering
   - Worker pool for concurrent execution
   - Graceful shutdown

2. **WorkerPool**: Manages goroutine pool
   - Configurable worker count
   - Task queue with buffering
   - Panic recovery
   - Resource management

3. **Router**: Core routing engine (synchronous)
   - Rule matching
   - Protocol adapter execution
   - Retry logic
   - Payload transformation

## Why Async?

### Problem: Blocking Execution

**Synchronous Routing (Bad):**
```go
// This blocks job execution
err := router.Route(ctx, message)
// Job waits for all webhooks/commands to complete
// Slow connection = slow job completion
```

**Issues:**
- Slow webhook (5s timeout) blocks job for 5 seconds
- Network issues cause job to hang
- Multiple callbacks multiply blocking time
- No resource limits (unbounded goroutines)

### Solution: Async Execution

**Async Routing (Good):**
```go
// This returns immediately
err := asyncRouter.RouteAsync(ctx, message)
// Job continues immediately
// Routing happens in background worker pool
```

**Benefits:**
- Job execution not blocked
- Worker pool limits resource usage
- Queue buffers messages during high load
- Timeouts handled in worker pool
- Graceful degradation (queue full = log warning)

## Worker Pool Design

### Configuration

```go
asyncRouter := NewAsyncRouter(
    router,           // Core router
    10,               // maxWorkers (goroutine pool size)
    100,              // queueSize (message buffer)
    logger,
)
```

### Worker Pool Sizing

**Guidelines:**
- **Small deployments**: 5-10 workers
- **Medium deployments**: 10-20 workers
- **Large deployments**: 20-50 workers
- **Queue size**: 2-5x worker count (buffers bursts)

**Considerations:**
- Each worker can handle one routing task at a time
- Routing tasks are I/O bound (HTTP, commands)
- More workers = more concurrent connections
- Too many workers = resource exhaustion

### Queue Behavior

**Queue Full:**
- `RouteAsync()` returns error after 100ms timeout
- Message is not queued (prevents unbounded growth)
- Logged as warning (doesn't fail job)
- Job continues normally

**Queue Empty:**
- Workers idle (minimal resource usage)
- Ready to process new messages immediately

## Usage

### Basic Usage

```go
// Create router with default adapters
router := NewRouterWithDefaults(logger)

// Create async router with worker pool
asyncRouter := NewAsyncRouter(router, 10, 100, logger)

// Start async router (starts worker pool)
ctx := context.Background()
err := asyncRouter.Start(ctx)
if err != nil {
    log.Fatal(err)
}
defer asyncRouter.Stop()

// Route message asynchronously (non-blocking)
message := CreateCompletionMessage(job, duration, stdout, stderr, exitCode)
err = asyncRouter.RouteAsync(ctx, message)
if err != nil {
    // Queue full or router shutting down
    // Job continues normally (best-effort delivery)
    logger.Warn("Failed to queue message", logging.Error(err))
}
```

### Integration with Scheduler

```go
// In RunWrapperHandler.Execute()
func (h *RunWrapperHandler) Execute(ctx context.Context, job *ScheduledJob) error {
    // ... execute command ...
    
    // Route completion message (async, non-blocking)
    if h.asyncRouter != nil {
        message := CreateCompletionMessage(job, duration, stdout, stderr, exitCode)
        _ = RouteJobMessageAsync(ctx, h.asyncRouter, message)
        // Error ignored - routing is best-effort, doesn't affect job
    }
    
    return nil
}
```

## Resource Management

### Connection Pooling

**HTTP Adapter:**
- Uses `http.Client` with timeout (5s default)
- Connection reuse via HTTP client
- No explicit connection pool (handled by Go runtime)

**Command Adapter:**
- Spawns processes (no persistent connections)
- Process lifecycle managed by Go runtime
- Timeout protection (10s default)

**Event Adapter:**
- In-memory (no connections)
- Minimal resource usage

### Goroutine Management

**Worker Pool:**
- Fixed number of goroutines (configurable)
- No goroutine leaks (proper cleanup)
- Panic recovery (worker restarts on panic)

**Queue:**
- Bounded size (prevents memory growth)
- Non-blocking enqueue (timeout on full queue)

## Performance Characteristics

### Latency

- **Enqueue**: < 1ms (in-memory queue)
- **Routing**: 50-500ms (network dependent)
- **Total**: Job sees < 1ms (async), routing happens in background

### Throughput

- **Workers**: 10 workers = 10 concurrent routing tasks
- **Queue**: 100 messages buffered
- **Burst handling**: Queue absorbs bursts, workers process steadily

### Resource Usage

- **Memory**: ~1KB per queued message
- **Goroutines**: Fixed (worker count)
- **Connections**: Managed by HTTP client (connection pooling)

## Failure Handling

### Queue Full

**Scenario**: High message volume, queue fills up

**Behavior:**
- `RouteAsync()` returns error after 100ms
- Message not queued
- Logged as warning
- Job continues (best-effort delivery)

**Mitigation:**
- Increase queue size
- Increase worker count
- Add rate limiting at source

### Worker Panic

**Scenario**: Protocol adapter panics

**Behavior:**
- Panic recovered in worker
- Error logged
- Worker continues processing
- Other workers unaffected

### Network Timeout

**Scenario**: Webhook times out (5s)

**Behavior:**
- Timeout handled by HTTP adapter
- Error logged
- Worker available for next task
- Job not affected

## Monitoring

### Metrics to Track

1. **Queue Size**: Current queue length
2. **Queue Capacity**: Maximum queue size
3. **Active Workers**: Currently processing workers
4. **Routing Errors**: Failed routing attempts
5. **Queue Full Events**: Messages rejected due to full queue

### Example Monitoring

```go
// Get queue metrics
queueSize := asyncRouter.GetQueueSize()
queueCapacity := asyncRouter.GetQueueCapacity()
utilization := float64(queueSize) / float64(queueCapacity) * 100

// Alert if queue > 80% full
if utilization > 80 {
    logger.Warn("Queue utilization high",
        logging.Float64("utilization", utilization),
        logging.Int("queue_size", queueSize),
        logging.Int("queue_capacity", queueCapacity))
}
```

## Comparison: Async vs Sync

| Aspect | Synchronous | Async (Worker Pool) |
|--------|------------|---------------------|
| **Job Blocking** | Yes (waits for routing) | No (immediate return) |
| **Resource Usage** | Unbounded goroutines | Fixed worker pool |
| **Failure Impact** | Can block job | Best-effort, non-blocking |
| **Throughput** | Limited by slowest route | Concurrent processing |
| **Complexity** | Simple | Moderate |
| **Use Case** | Testing, simple cases | Production, high volume |

## Best Practices

1. **Always Use Async in Production**
   - Prevents job blocking
   - Better resource management
   - Graceful degradation

2. **Size Worker Pool Appropriately**
   - Start with 10 workers
   - Monitor queue utilization
   - Adjust based on load

3. **Monitor Queue Metrics**
   - Alert on high utilization
   - Track routing errors
   - Monitor worker health

4. **Handle Queue Full Gracefully**
   - Log warnings (don't fail jobs)
   - Consider increasing queue size
   - Add retry logic if needed

5. **Use Sync for Testing**
   - Easier to test
   - Deterministic behavior
   - No async timing issues

## Future Enhancements

1. **Scheduler Integration**
   - Use scheduler jobs for routing (leverage retry/timeout infrastructure)
   - Guaranteed delivery via scheduler queue
   - Better monitoring via scheduler metrics

2. **Dynamic Worker Scaling**
   - Auto-scale workers based on queue size
   - Reduce workers during low load
   - Increase workers during high load

3. **Priority Queues**
   - High-priority messages processed first
   - Low-priority messages can wait
   - Better QoS for critical routes

4. **Circuit Breakers**
   - Stop routing to failing endpoints
   - Automatic recovery after cooldown
   - Prevent cascading failures

## Related Documentation

- [Scheduler Transceiver Architecture](./SCHEDULER_TRANSCEIVER_ARCHITECTURE.md)
- [Transceiver Implementation Plan](./TRANSCEIVER_IMPLEMENTATION_PLAN.md)
- [Job Output Mechanisms](../system-health/JOB_OUTPUT_MECHANISMS.md)

