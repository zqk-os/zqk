# MCP Metrics Instrumentation

## Overview

Comprehensive metrics instrumentation has been added to all MCP protocol operations, providing detailed observability into server performance, usage patterns, and health.

## Metrics Architecture

### Core Components

1. **MCPMetrics** (`mcp_metrics.go`): Central metrics collector
   - Thread-safe atomic counters
   - Duration histograms with percentiles
   - Per-tool metrics tracking
   - Queue metrics integration

2. **Automatic Instrumentation**: Via `MCPServerAdapter`
   - All protocol methods automatically instrumented
   - Latency tracking for all operations
   - Error rate tracking
   - Concurrent operation tracking

3. **Queue Metrics Integration**: Message queues report metrics via callbacks
   - Real-time queue depth
   - Drop rate tracking
   - Send/error counts

4. **Metrics Tools**: Exposed via MCP protocol
   - `mcp_get_metrics`: Get full metrics snapshot
   - `mcp_get_tool_metrics`: Get metrics for specific tool

## Metrics Collected

### 1. Lifecycle Metrics
- Initialize: count, duration (min/max/avg/P50/P95/P99), errors
- Shutdown: count, duration

### 2. Tools Metrics
- List: count, duration, errors
- Call: count, duration, errors
- **Per-Tool**: call count, avg duration, errors, last called

### 3. Resources Metrics
- List: count, duration, errors
- Get: count, duration, errors

### 4. Prompts Metrics
- List: count, duration, errors
- Get: count, duration, errors

### 5. Roots Metrics
- List: count, duration, errors

### 6. Notification Metrics
- Log messages: sent, dropped
- Events: count
- Messages: count

### 7. Async Operation Metrics
- Async tool calls: count, errors
- Batch operations: count, size distribution, errors

### 8. Queue Metrics
- Current depth
- Total sent
- Total dropped
- Total errors
- Drop rate

### 9. Concurrency Metrics
- Current concurrent operations
- Peak concurrent operations

## Usage

### Accessing Metrics

```go
// Direct access
server := mcp.NewServer()
snapshot := server.GetMCPMetricsSnapshot()

// Via interface
adapter := mcp.NewMCPServerAdapter(server)
snapshot := adapter.GetMetricsSnapshot()
```

### Via MCP Tools

```json
// Get full metrics
{
  "name": "mcp_get_metrics",
  "arguments": {
    "format": "summary"  // or "json"
  }
}

// Get tool-specific metrics
{
  "name": "mcp_get_tool_metrics",
  "arguments": {
    "tool_name": "my_tool"
  }
}
```

## Duration Histograms

All duration metrics use histograms that track:

- **Count**: Total operations
- **Total**: Cumulative duration
- **Average**: Mean duration
- **Min/Max**: Duration bounds
- **P50/P95/P99**: Percentiles (approximate, from rolling sample)

Histograms use a configurable sample size (default 1000-10000 samples) for efficient percentile calculation.

## Per-Tool Metrics

Each tool gets individual metrics tracking:

- Call count
- Average duration
- Error count
- Last called timestamp

This enables identifying slow or problematic tools.

## Thread Safety

All metrics operations are thread-safe:

- **Atomic operations** for counters (lock-free, very fast)
- **Mutex-protected** histograms (for percentile calculation)
- **No blocking** during metrics collection

## Performance Impact

Metrics collection is designed to be lightweight:

- Atomic operations: ~1-2ns per operation
- Histogram sampling: O(1) insertion, O(n log n) percentile calculation (only when needed)
- Minimal allocations: Pre-allocated structures
- No I/O: All in-memory

## Automatic Instrumentation Points

Metrics are automatically recorded at:

1. **Protocol Method Entry/Exit**: Via adapter wrappers
2. **Queue Operations**: Via queue metrics callbacks
3. **Async Operations**: Via async method wrappers
4. **Batch Operations**: Via batch method wrappers

## Example: Monitoring Performance

```go
snapshot := server.GetMCPMetricsSnapshot()

// Check tool call performance
if snapshot.Tools.CallDuration.P95 > 5*time.Second {
    log.Warn("Tool calls are slow",
        "p95", snapshot.Tools.CallDuration.P95,
        "p99", snapshot.Tools.CallDuration.P99)
}

// Check error rates
toolErrorRate := float64(snapshot.Tools.CallErrors) / 
    float64(snapshot.Tools.CallCount)
if toolErrorRate > 0.1 {
    log.Warn("High tool error rate", "rate", toolErrorRate)
}

// Check specific tool
if metrics, ok := snapshot.Tools.ByTool["slow_tool"]; ok {
    if metrics.AverageDuration > 2*time.Second {
        log.Warn("Tool is slow", 
            "tool", "slow_tool",
            "avg", metrics.AverageDuration,
            "calls", metrics.CallCount)
    }
}
```

## Example: Monitoring Queue Health

```go
snapshot := server.GetMCPMetricsSnapshot()

// Check queue depth
if snapshot.Queue.Depth > 500 {
    log.Warn("Queue depth is high", "depth", snapshot.Queue.Depth)
}

// Check drop rate
if snapshot.Queue.Dropped > 0 {
    total := snapshot.Queue.Sent + snapshot.Queue.Dropped
    dropRate := float64(snapshot.Queue.Dropped) / float64(total)
    if dropRate > 0.05 {
        log.Warn("High message drop rate",
            "rate", dropRate,
            "dropped", snapshot.Queue.Dropped,
            "sent", snapshot.Queue.Sent)
    }
}
```

## Example: Monitoring Concurrency

```go
snapshot := server.GetMCPMetricsSnapshot()

// Check current load
if snapshot.Concurrency.Current > 50 {
    log.Warn("High concurrency",
        "current", snapshot.Concurrency.Current,
        "peak", snapshot.Concurrency.Max)
}

// Check if we're approaching limits
if snapshot.Concurrency.Current > snapshot.Concurrency.Max*0.8 {
    log.Info("Approaching peak concurrency",
        "current", snapshot.Concurrency.Current,
        "peak", snapshot.Concurrency.Max)
}
```

## Metrics Export

Metrics can be accessed via:

1. **Direct API**: `server.GetMCPMetricsSnapshot()`
2. **MCP Tools**: `mcp_get_metrics`, `mcp_get_tool_metrics`
3. **Future**: HTTP endpoint, Prometheus format, file export

## Reset Metrics

For testing, metrics can be reset:

```go
metrics := server.GetMCPMetrics()
metrics.Reset()
```

## Best Practices

1. **Monitor P95/P99**: These catch tail latencies
2. **Track Error Rates**: Error rate > 1% may indicate issues
3. **Watch Queue Depth**: High depth = backpressure
4. **Monitor Per-Tool**: Identify problematic tools
5. **Track Concurrency**: Ensure we're not hitting limits

## Future Enhancements

1. **Time-Series Storage**: Store metrics over time
2. **Alerting**: Alert on thresholds
3. **Dashboards**: Visualize metrics
4. **Export Formats**: Prometheus, StatsD, etc.
5. **Custom Metrics**: Allow custom metric registration
6. **Metrics Aggregation**: Aggregate across time windows
