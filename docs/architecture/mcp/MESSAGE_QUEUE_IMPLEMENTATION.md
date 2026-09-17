# Message Queue Implementation for Backpressure Relief

**Last Verified:** 2026-08-31


## Overview

A message queue system has been implemented between the MCP server and clients to provide backpressure relief. This prevents the server from blocking when clients are slow to consume messages, improving overall system responsiveness and reliability.

## Architecture

### Components

1. **MessageQueue** (`message_queue.go`): Per-client message queue with dedicated writer goroutine
2. **QueueConfig**: Configurable queue behavior (size, drop policy, timeouts)
3. **Integration**: Automatic queue creation for client connections
4. **Metrics**: Queue statistics (depth, drops, sent, errors)

### Key Features

- **Non-blocking Enqueue**: Messages are queued without blocking server operations
- **Dedicated Writer Goroutine**: Separate goroutine drains queue and writes to client
- **Backpressure Handling**: Drops messages when queue is full (configurable)
- **Priority Support**: Messages can have priority (low, normal, high, critical)
- **Write Timeouts**: Prevents hanging on broken pipes
- **Graceful Shutdown**: Flushes remaining messages on shutdown
- **Metrics**: Tracks queue depth, drops, sent messages, and errors

## Configuration

### Config File (`config.yaml`)

```yaml
mcp_server:
  message_queue:
    max_queue_size: 1000    # Maximum messages in queue (default: 1000)
    drop_when_full: true    # Drop messages when queue is full (default: true)
    flush_interval: "0"     # How often to flush (Go duration, "0" = immediate)
    write_timeout: "5s"     # Write timeout (Go duration, default: "5s")
```

### Constants

- `DefaultMessageQueueSize = 1000`
- `DefaultMessageWriteTimeout = 5 * time.Second`

## Behavior

### Message Flow

1. **Server generates message** (notification, log, etc.)
2. **Message is enqueued** (non-blocking)
3. **If queue is full**:
   - If `drop_when_full: true`: Message is dropped (backpressure)
   - If `drop_when_full: false`: Blocks until space available (not recommended)
4. **Writer goroutine** drains queue and writes to client
5. **Write timeout** prevents hanging on broken pipes

### Priority Handling

Messages are prioritized based on:
- **Critical/High**: Error messages, critical notifications
- **Normal**: Regular notifications, info logs
- **Low**: Debug logs

Priority is determined from:
- Message type (error, critical → high priority)
- Explicit priority parameter
- Log level (error → high, debug → low)

## Integration Points

### SendMessageToClient

- Checks for client-specific queue first
- Falls back to default client queue if available
- Falls back to direct write if no queue (backward compatible)

### SendLogMessage

- Uses queue if available
- Falls back to direct write with timeout (original behavior)
- Records delivery method in metrics (queue vs direct)

### Client Connection

- Queue is created automatically when client connects
- Queue is updated when client reconnects
- Queue is stopped and flushed on shutdown

## Metrics

Queue statistics are available via `Queue.Stats()`:

```go
type QueueStats struct {
    QueueDepth  int       // Current queue depth
    MaxQueueSize int      // Maximum queue size
    Dropped     int64     // Count of dropped messages
    Sent        int64     // Count of successfully sent messages
    Errors      int64     // Count of send errors
    LastError   error     // Last error encountered
    LastErrorAt time.Time // When last error occurred
    Active      bool      // Whether queue is active
}
```

## Benefits

1. **Non-blocking Operations**: Server operations don't block on slow clients
2. **Backpressure Relief**: Queue full = message dropped (prevents memory growth)
3. **Improved Reliability**: Write timeouts prevent hanging
4. **Better Performance**: Dedicated writer goroutine optimizes I/O
5. **Observability**: Queue metrics provide visibility into message flow
6. **Graceful Degradation**: Falls back to direct write if queue unavailable

## Trade-offs

### Memory Usage

- Each queue uses `MaxQueueSize * average_message_size` memory
- Default: 1000 messages per client
- Mitigation: Configurable queue size, drop when full

### Latency

- Messages may be delayed if queue is backing up
- Priority helps ensure critical messages are sent first
- Trade-off: Better to delay than block server operations

### Message Loss

- Messages are dropped when queue is full (if `drop_when_full: true`)
- This is intentional backpressure behavior
- Critical messages should use high priority

## Future Enhancements

1. **Priority Queue**: Implement actual priority queue (currently FIFO)
2. **Queue Metrics Endpoint**: Expose queue stats via MCP tool
3. **Adaptive Queue Size**: Dynamically adjust based on client consumption rate
4. **Batching**: Batch multiple messages in single write for efficiency
5. **Retry Logic**: Retry failed writes with exponential backoff

## Testing

To test backpressure handling:

1. Configure small queue size (e.g., `max_queue_size: 10`)
2. Generate many messages rapidly
3. Observe dropped messages in queue stats
4. Verify server continues operating normally

## Migration Notes

- **Backward Compatible**: Falls back to direct write if queue unavailable
- **No Breaking Changes**: Existing code continues to work
- **Opt-in via Config**: Queue is enabled by default but can be configured
- **Gradual Rollout**: Can be enabled per-client or globally
