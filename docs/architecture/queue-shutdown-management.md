# Queue Shutdown Management

**Last Verified:** 2026-08-31


**Version:** 1.0  
**Status:** Design  
**Date:** 2026-01-17  
**Purpose:** Ensure graceful shutdown of all queues without data loss

## Critical Requirements

1. **No Data Loss**: All queued operations must complete before shutdown
2. **Graceful Drain**: Stop accepting new work, process all pending operations
3. **Shutdown Coordination**: All queues coordinate shutdown together
4. **Timeout Protection**: Prevent indefinite shutdown waits
5. **Process Disabling**: System can be disabled without losing critical updates

## Architecture

### Shutdown Phases

1. **Initiate Shutdown**
   - Set shutdown flag (prevents new operations)
   - Notify all queue managers
   - Start drain phase

2. **Drain Phase**
   - Stop accepting new operations
   - Process all pending operations
   - Wait for all workers to complete

3. **Shutdown Complete**
   - All queues drained
   - All workers stopped
   - System ready for termination

### Components

#### 1. Shutdown Coordinator

Manages global shutdown state and coordinates all queues.

```go
type ShutdownCoordinator struct {
    shutdownInitiated int32  // Atomic flag
    shutdownComplete  chan struct{}
    queues            []QueueShutdownHandler
    timeout           time.Duration
}
```

#### 2. Queue Shutdown Handler

Interface for queues to implement graceful shutdown.

```go
type QueueShutdownHandler interface {
    InitiateShutdown() error
    Drain(timeout time.Duration) error
    IsDrained() bool
    GetPendingCount() int64
}
```

#### 3. Shutdown-Aware Enqueue

All enqueue operations check shutdown state before accepting work.

```go
func (m *IOQueueManager) Enqueue(op *IOOperation) error {
    if atomic.LoadInt32(&shutdownCoordinator.shutdownInitiated) == 1 {
        return ErrShutdownInProgress
    }
    // ... enqueue logic
}
```

## Implementation Strategy

### Phase 1: Shutdown Coordinator

1. Create global shutdown coordinator
2. Register all queue managers
3. Implement shutdown initiation
4. Implement drain coordination

### Phase 2: Queue Integration

1. Add shutdown handlers to all queues:
   - CAS Orphan Cleanup Queue
   - ID Generation Queue Manager
   - CAS Index Write Queue
   - Hash Registry Save Worker
   - I/O Queue Manager
   - Operation Executor

2. Implement drain logic for each queue

### Phase 3: Shutdown-Aware Operations

1. Check shutdown state before enqueue
2. Reject new operations during shutdown
3. Log rejected operations for visibility

### Phase 4: Timeout Protection

1. Configurable shutdown timeout
2. Force shutdown after timeout
3. Log incomplete operations

## Shutdown Sequence

```
1. System receives shutdown signal
2. ShutdownCoordinator.InitiateShutdown()
   - Set shutdown flag
   - Notify all registered queues
3. All queues enter drain mode
   - Stop accepting new operations
   - Process pending operations
4. Wait for all queues to drain (with timeout)
5. Force shutdown if timeout exceeded
6. Log any incomplete operations
7. Shutdown complete
```

## Expedited shutdown (scheduler stop and write-behind)

When **shutdown is ordered** (e.g. `zqk scheduler stop` or SIGTERM), the following keep shutdown time bounded so the process can exit promptly:

- **Triggered job pool**: Scheduler waits at most **5 seconds** for the pool to stop. If in-flight jobs do not respect context cancellation by then, shutdown continues; remaining work is abandoned when the process exits.
- **Write-behind worker**: When `FileObjectStorage.Shutdown()` calls the worker’s `Stop()`, the worker drains the in-memory buffer for at most **5 seconds** (`shutdownDrainDeadline`). Any remaining buffer entries remain in the WAL and are replayed on next start. No new WAL backlog is pulled during drain.

This “shutdown ordered” behavior avoids long blocks (e.g. full WAL catch-up or a long-running retention job) so that `scheduler stop` responds and the daemon can exit within a few seconds.

## Error Handling

- **Shutdown Timeout**: Log incomplete operations, force shutdown
- **Queue Drain Failure**: Log error, continue with other queues
- **Critical Operation Loss**: Alert and log, prevent shutdown if critical

## Configuration

```go
type ShutdownConfig struct {
    Timeout           time.Duration // Max time to wait for drain (default: 30s)
    ForceShutdown     bool          // Force shutdown after timeout (default: true)
    LogIncomplete     bool          // Log incomplete operations (default: true)
    CriticalQueues    []string      // Queues that must drain (no force shutdown)
}
```
