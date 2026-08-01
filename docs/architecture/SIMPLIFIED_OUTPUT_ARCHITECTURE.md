# Simplified Output Architecture

## Design Principles

1. **Single Writer Pattern**: One writer goroutine handles all I/O
2. **Queue-Based**: Workers enqueue data, writer dequeues and writes
3. **Minimal Contention**: Workers acquire lock, enqueue, release lock, return
4. **FIFO Processing**: Writer processes queue head, waits if empty

## Architecture Option 1: Single Writer with Channel Selector

```
┌─────────────────────────────────────────────────────────────┐
│                    Workers (N goroutines)                   │
│                                                              │
│  For each validation result:                                 │
│    1. Acquire queue lock (write)                            │
│    2. Enqueue: {data, channel_id, flush_hint}               │
│    3. Release lock                                           │
│    4. Return to pool                                         │
└─────────────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│              Output Queue (thread-safe FIFO)                │
│                                                              │
│  Queue: []OutputPacket                                      │
│    - data: []byte or interface{}                            │
│    - channel_id: string (e.g., "progress", "metrics")       │
│    - flush_hint: bool (should flush after this?)            │
│                                                              │
│  Mutex: queueMu (sync.Mutex)                                │
│    - Only for enqueue/dequeue operations                    │
│    - Minimal contention (fast operations)                    │
└─────────────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│              Single Writer Goroutine                         │
│                                                              │
│  Loop:                                                       │
│    1. Acquire queue lock                                    │
│    2. Dequeue packet from head (FIFO)                       │
│    3. Release lock                                           │
│    4. Channel Selector: Route to appropriate channel        │
│       - progressChan -> progress handler                     │
│       - metricsChan -> metrics handler                       │
│       - stdout -> os.Stdout                                  │
│       - stderr -> os.Stderr                                  │
│    5. Write data to channel                                 │
│    6. Flush if flush_hint is true                           │
│    7. If queue empty, wait (blocking)                       │
│    8. Repeat                                                │
└─────────────────────────────────────────────────────────────┘
                        │
            ┌───────────┼───────────┐
            ▼           ▼           ▼
    ┌──────────┐ ┌──────────┐ ┌──────────┐
    │ Progress │ │ Metrics  │ │  Stdout  │
    │ Handler  │ │ Handler  │ │ Handler  │
    └──────────┘ └──────────┘ └──────────┘
```

## Architecture Option 2: Multiple Writers (One Per Output Stream)

```
┌─────────────────────────────────────────────────────────────┐
│                    Workers (N goroutines)                   │
│                                                              │
│  For each validation result:                                 │
│    1. Acquire router lock (write)                           │
│    2. Enqueue to router: {data, channel_id}                 │
│    3. Release lock                                           │
│    4. Return to pool                                         │
└─────────────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│              Router/Sorter Goroutine                        │
│                                                              │
│  Loop:                                                       │
│    1. Acquire router lock                                    │
│    2. Dequeue packet                                        │
│    3. Release lock                                           │
│    4. Route to appropriate output queue:                     │
│       - channel_id="progress" -> progressQueue              │
│       - channel_id="metrics" -> metricsQueue                │
│       - channel_id="stdout" -> stdoutQueue                  │
│    5. Signal queue writer (if needed)                        │
│    6. Repeat                                                │
└─────────────────────────────────────────────────────────────┘
            │
    ┌───────┼───────┐
    ▼       ▼       ▼
┌────────┐ ┌────────┐ ┌────────┐
│Progress│ │Metrics │ │ Stdout │
│ Queue  │ │ Queue  │ │ Queue  │
└────────┘ └────────┘ └────────┘
    │       │       │
    ▼       ▼       ▼
┌────────┐ ┌────────┐ ┌────────┐
│Progress│ │Metrics │ │ Stdout │
│ Writer │ │ Writer │ │ Writer │
│(1 gor) │ │(1 gor) │ │(1 gor) │
└────────┘ └────────┘ └────────┘
```

## Implementation: Option 1 (Single Writer)

### Core Components

```go
// OutputPacket represents a single output operation
type OutputPacket struct {
    Data      interface{} // The data to write
    ChannelID string      // Which channel/stream to write to
    FlushHint bool        // Should flush after this write?
    Timestamp time.Time   // When was this enqueued?
}

// OutputQueue manages the FIFO queue of output packets
type OutputQueue struct {
    mu    sync.Mutex
    queue []OutputPacket
    cond  *sync.Cond // For signaling when queue has data
}

// Enqueue adds a packet to the queue (thread-safe, fast)
func (oq *OutputQueue) Enqueue(packet OutputPacket) {
    oq.mu.Lock()
    defer oq.mu.Unlock()
    
    oq.queue = append(oq.queue, packet)
    oq.cond.Signal() // Wake up writer if waiting
}

// Dequeue removes and returns the head packet (blocks if empty)
func (oq *OutputQueue) Dequeue() OutputPacket {
    oq.mu.Lock()
    defer oq.mu.Unlock()
    
    // Wait for data if queue is empty
    for len(oq.queue) == 0 {
        oq.cond.Wait()
    }
    
    // FIFO: take from head
    packet := oq.queue[0]
    oq.queue = oq.queue[1:]
    return packet
}

// OutputWriter handles all I/O operations
type OutputWriter struct {
    queue      *OutputQueue
    channels   map[string]io.Writer // channel_id -> writer
    handlers   map[string]OutputHandler // channel_id -> handler
    ctx        context.Context
    logger     logging.Logger
}

// OutputHandler processes data for a specific channel
type OutputHandler interface {
    Write(data interface{}) error
    Flush() error
}

// Start begins the writer goroutine
func (ow *OutputWriter) Start() {
    go func() {
        for {
            select {
            case <-ow.ctx.Done():
                return
            default:
                // Dequeue packet (blocks if empty)
                packet := ow.queue.Dequeue()
                
                // Route to appropriate handler
                handler := ow.handlers[packet.ChannelID]
                if handler != nil {
                    if err := handler.Write(packet.Data); err != nil {
                        ow.logger.Warn("Failed to write output",
                            logging.String("channel", packet.ChannelID),
                            logging.Error(err))
                    }
                    
                    if packet.FlushHint {
                        if err := handler.Flush(); err != nil {
                            ow.logger.Warn("Failed to flush output",
                                logging.String("channel", packet.ChannelID),
                                logging.Error(err))
                        }
                    }
                }
            }
        }
    }()
}
```

## Benefits

1. **Minimal Lock Contention**: 
   - Workers: Lock → Enqueue → Unlock (microseconds)
   - Writer: Lock → Dequeue → Unlock (microseconds)
   - No long-held locks

2. **FIFO Guarantee**: 
   - Data processed in order
   - No race conditions on output

3. **Single Point of I/O**:
   - All file operations in one place
   - Easy to add buffering, batching, etc.

4. **Scalable**:
   - Workers don't block on I/O
   - Queue absorbs bursts
   - Writer can batch writes if needed

5. **Simple Error Handling**:
   - Errors isolated to writer
   - Workers unaffected by I/O failures

## Migration Strategy

### Phase 1: Create OutputQueue and OutputWriter
- [ ] Implement `OutputQueue` with thread-safe enqueue/dequeue
- [ ] Implement `OutputWriter` with channel routing
- [ ] Create handlers for each output type (progress, metrics, stdout)

### Phase 2: Replace progressChan
- [ ] Workers enqueue to OutputQueue instead of sending to progressChan
- [ ] Writer routes progress packets to progress handler
- [ ] Remove drain goroutine (writer handles it)

### Phase 3: Replace direct writes
- [ ] Replace all `fmt.Fprintf(os.Stderr, ...)` with queue enqueue
- [ ] Replace all `os.Stdout.Write()` with queue enqueue
- [ ] Writer handles all I/O

### Phase 4: Remove mutexes
- [ ] Remove `mu` from `showValidationProgressWithMetrics`
- [ ] Remove contention points
- [ ] Simplify completion detection

## Performance Considerations

### Queue Size
- **Bounded**: Prevent unbounded growth
- **Monitoring**: Track queue depth, warn if > threshold
- **Backpressure**: If queue full, workers can wait or drop

### Writer Efficiency
- **Batching**: Batch multiple packets to same channel
- **Buffering**: Use buffered writers where appropriate
- **Flush Strategy**: Flush on flush_hint or periodic

### Channel Switching Cost
- **Measure**: Time to switch between channels
- **Optimize**: Batch writes to same channel
- **If inefficient**: Switch to Option 2 (multiple writers)

## Comparison: Option 1 vs Option 2

| Aspect | Option 1 (Single Writer) | Option 2 (Multiple Writers) |
|--------|---------------------------|------------------------------|
| **Complexity** | Lower | Higher |
| **Channel Switching** | May be inefficient | No switching needed |
| **Queue Management** | Single queue | Multiple queues |
| **Lock Contention** | Minimal (single writer) | Minimal (per-channel) |
| **Scalability** | Good for few channels | Better for many channels |
| **Resource Usage** | 1 writer goroutine | N writer goroutines |

## Recommendation

**Start with Option 1** (Single Writer):
- Simpler to implement
- Easier to debug
- Sufficient for current use case (progress, metrics, stdout)
- Can migrate to Option 2 if channel switching becomes bottleneck

