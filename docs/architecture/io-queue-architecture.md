# I/O Queue Architecture

**Last Verified:** 2026-08-31


**Version:** 1.0  
**Status:** Design  
**Date:** 2026-01-17  
**Purpose:** Design a queue-based I/O system to prevent deadlocks and enable load distribution

## Executive Summary

All I/O operations (inbound and outbound) **MUST** flow through queues to prevent deadlocks. When a single queue worker cannot keep up with volume, the system should automatically create additional queues and workers, with enqueue operations alternating between queues to distribute load.

## Critical Requirements

1. **Zero Deadlocks**: All I/O operations must be asynchronous and queued
2. **Load Distribution**: Multiple queues/workers when volume exceeds single-worker capacity
3. **Queue Selection**: Round-robin or work-grouping strategies for enqueue operations
4. **Dynamic Scaling**: Automatically add queues/workers when queue depth exceeds threshold
5. **Work Grouping**: Option to keep related work grouped per queue

## Architecture

### Core Components

#### 1. I/O Queue Manager

Manages multiple I/O queues and workers for load distribution.

```go
type IOQueueManager struct {
    queues      []*IOQueue  // Multiple queues for load distribution
    queueIndex  int32       // Atomic counter for round-robin selection
    mu          sync.RWMutex
    maxQueueDepth int       // Threshold for creating new queue/worker
    minQueues   int         // Minimum number of queues
    maxQueues   int         // Maximum number of queues
}
```

#### 2. I/O Queue

Individual queue with dedicated worker for processing I/O operations.

```go
type IOQueue struct {
    operations  chan *IOOperation
    workerRunning int32
    queueDepth  int64      // Atomic counter for queue depth
    ctx         context.Context
    cancel      context.CancelFunc
    wg          sync.WaitGroup
}
```

#### 3. I/O Operation

Represents a single I/O operation (read or write).

```go
type IOOperation struct {
    Type        IOOperationType  // Read or Write
    FilePath    string
    Data        []byte           // For writes
    Result      chan IOResult    // For async results
    GroupID     string           // Optional: for work grouping
}
```

### Queue Selection Strategies

#### Round-Robin (Default)

Enqueue operations alternate between queues to distribute load evenly.

```go
func (m *IOQueueManager) enqueueRoundRobin(op *IOOperation) error {
    index := atomic.AddInt32(&m.queueIndex, 1) % int32(len(m.queues))
    return m.queues[index].Enqueue(op)
}
```

#### Work Grouping

Related operations (same file, same kind, etc.) go to the same queue.

```go
func (m *IOQueueManager) enqueueGrouped(op *IOOperation) error {
    queueIndex := hashGroupID(op.GroupID) % len(m.queues)
    return m.queues[queueIndex].Enqueue(op)
}
```

### Dynamic Scaling

When queue depth exceeds threshold, automatically create new queue and worker:

```go
func (m *IOQueueManager) checkAndScale() {
    for _, queue := range m.queues {
        depth := atomic.LoadInt64(&queue.queueDepth)
        if depth > int64(m.maxQueueDepth) && len(m.queues) < m.maxQueues {
            // Create new queue and worker
            m.addQueue()
        }
    }
}
```

## Implementation Plan

### Phase 1: Core Infrastructure

1. **Create I/O Queue Manager**
   - Multi-queue support
   - Round-robin queue selection
   - Dynamic queue creation

2. **Create I/O Queue**
   - On-demand worker pattern
   - Queue depth tracking
   - Operation processing

3. **Create I/O Operation Types**
   - Read operations
   - Write operations
   - Result channels

### Phase 2: Route File Operations

1. **File Reads**
   - Replace direct `readObjectFile()` calls with queued operations
   - Maintain backward compatibility with async results

2. **File Writes**
   - Replace direct `writeObjectFileWithPermAndData()` calls with queued operations
   - Batch writes when possible

### Phase 3: Work Grouping

1. **Group ID Assignment**
   - By file path (same file operations grouped)
   - By object kind (same kind operations grouped)
   - By operation type (reads vs writes)

2. **Grouped Queue Selection**
   - Hash-based queue assignment
   - Consistent queue for same group

### Phase 4: Dynamic Scaling

1. **Queue Depth Monitoring**
   - Track queue depth per queue
   - Periodic checks for scaling

2. **Automatic Queue Creation**
   - Create new queue when threshold exceeded
   - Start new worker for new queue

## Benefits

1. **Deadlock Prevention**: All I/O is asynchronous and queued
2. **Load Distribution**: Multiple queues/workers handle high volume
3. **Scalability**: Automatically scales with load
4. **Work Grouping**: Related operations stay together
5. **Backpressure**: Queue depth limits prevent memory issues

## Migration Strategy

1. **Gradual Migration**: Start with new operations, migrate existing
2. **Feature Flag**: Enable/disable queue-based I/O
3. **Fallback**: Direct I/O if queue system unavailable
4. **Monitoring**: Track queue depths, worker counts, operation latencies

## Configuration

```go
type IOQueueConfig struct {
    MinQueues      int           // Minimum queues (default: 1)
    MaxQueues      int           // Maximum queues (default: 10)
    MaxQueueDepth  int           // Threshold for new queue (default: 1000)
    BatchSize      int           // Batch size for processing (default: 50)
    BatchTimeout   time.Duration // Batch timeout (default: 100ms)
    IdleTimeout    time.Duration // Worker idle timeout (default: 5min)
    Strategy       QueueStrategy // Round-robin or grouped
}
```

## Related Patterns

- **On-Demand Worker Pattern**: Workers wake on work, shut down when idle
- **Batch Processing**: Group operations for efficiency
- **Load Balancing**: Distribute work across multiple queues
