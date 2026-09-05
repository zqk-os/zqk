# MCP Protocol Metrics

**Last Verified:** 2026-08-31


## Overview

Comprehensive metrics instrumentation for all MCP protocol operations, providing observability into server performance, usage patterns, and health.

## Metrics Categories

### 1. Lifecycle Metrics

Tracks server initialization and shutdown:

- **InitializeCount**: Total initialize requests
- **InitializeDuration**: Latency distribution (min, max, avg, P50, P95, P99)
- **InitializeErrors**: Failed initialization attempts
- **ShutdownCount**: Total shutdown requests
- **ShutdownDuration**: Shutdown latency

### 2. Tools Metrics

Tracks tool operations:

- **ToolsListCount**: Total tools/list calls
- **ToolsListDuration**: Latency distribution
- **ToolsListErrors**: Failed list operations
- **ToolCallCount**: Total tool calls
- **ToolCallDuration**: Latency distribution
- **ToolCallErrors**: Failed tool calls
- **ToolCallByTool**: Per-tool metrics (call count, avg duration, errors, last called)

### 3. Resources Metrics

Tracks resource operations:

- **ResourcesListCount**: Total resources/list calls
- **ResourcesListDuration**: Latency distribution
- **ResourcesListErrors**: Failed list operations
- **ResourceGetCount**: Total resource retrievals
- **ResourceGetDuration**: Latency distribution
- **ResourceGetErrors**: Failed retrievals

### 4. Prompts Metrics

Tracks prompt operations:

- **PromptsListCount**: Total prompts/list calls
- **PromptsListDuration**: Latency distribution
- **PromptsListErrors**: Failed list operations
- **PromptGetCount**: Total prompt retrievals
- **PromptGetDuration**: Latency distribution
- **PromptGetErrors**: Failed retrievals

### 5. Roots Metrics

Tracks roots operations:

- **RootsListCount**: Total roots/list calls
- **RootsListDuration**: Latency distribution
- **RootsListErrors**: Failed list operations

### 6. Notification Metrics

Tracks server-to-client notifications:

- **LogMessageCount**: Total log messages sent
- **LogMessageDropped**: Log messages dropped (queue full)
- **EventCount**: Total events emitted
- **MessageCount**: Total messages sent

### 7. Async Operation Metrics

Tracks asynchronous operations:

- **AsyncToolCallCount**: Total async tool calls
- **AsyncToolCallErrors**: Failed async tool calls
- **BatchToolCallCount**: Total batch operations
- **BatchToolCallSize**: Batch size distribution
- **BatchToolCallErrors**: Errors in batch operations

### 8. Queue Metrics

Tracks message queue performance:

- **QueueDepth**: Current queue depth
- **QueueDropped**: Total messages dropped
- **QueueSent**: Total messages sent successfully
- **QueueErrors**: Total queue errors

### 9. Concurrency Metrics

Tracks concurrent operations:

- **ConcurrentOperations**: Current concurrent operations
- **MaxConcurrentOps**: Peak concurrent operations

## Usage

### Accessing Metrics

```go
// Get metrics from server
server := mcp.NewServer()
metrics := server.GetMCPMetrics()

// Get snapshot
snapshot := server.GetMCPMetricsSnapshot()

// Access specific metrics
fmt.Printf("Tool calls: %d\n", snapshot.Tools.CallCount)
fmt.Printf("Avg tool call duration: %v\n", snapshot.Tools.CallDuration.Average)
fmt.Printf("Tool call errors: %d\n", snapshot.Tools.CallErrors)

// Per-tool metrics
for toolName, toolMetrics := range snapshot.Tools.ByTool {
    fmt.Printf("Tool %s: %d calls, avg %v\n", 
        toolName, toolMetrics.CallCount, toolMetrics.AverageDuration)
}
```

### Via Interface

```go
adapter := mcp.NewMCPServerAdapter(server)
snapshot := adapter.GetMetricsSnapshot()
```

### Metrics Snapshot Structure

```go
type MetricsSnapshot struct {
    Lifecycle     LifecycleMetrics
    Tools         ToolsMetrics
    Resources     ResourcesMetrics
    Prompts       PromptsMetrics
    Roots         RootsMetrics
    Notifications NotificationsMetrics
    Async         AsyncMetrics
    Queue         QueueMetrics
    Concurrency   ConcurrencyMetrics
    Timestamp     time.Time
}
```

## Duration Histograms

All duration metrics use histograms that track:

- **Count**: Total number of operations
- **Total**: Cumulative duration
- **Average**: Mean duration
- **Min**: Minimum duration
- **Max**: Maximum duration
- **P50**: 50th percentile (median)
- **P95**: 95th percentile
- **P99**: 99th percentile

Percentiles are calculated from a rolling sample (default 1000 samples) for efficiency.

## Automatic Instrumentation

Metrics are automatically recorded for:

1. **All MCP Protocol Methods**: Via `MCPServerAdapter`
2. **Message Queue Operations**: Via queue metrics callback
3. **Async Operations**: Via async method wrappers
4. **Batch Operations**: Via batch method wrappers

## Thread Safety

All metrics operations are thread-safe:

- Atomic operations for counters
- Mutex-protected histograms
- Lock-free reads where possible

## Performance Impact

Metrics collection is designed to be lightweight:

- Atomic operations for counters (very fast)
- Histogram sampling (configurable, default 1000 samples)
- Minimal allocations
- No blocking operations

## Exporting Metrics

Metrics can be exported to:

1. **Logs**: Periodic snapshot logging
2. **HTTP Endpoint**: Expose via MCP tool (future)
3. **Prometheus**: Convert to Prometheus format (future)
4. **File**: Periodic snapshot to file (future)

## Example: Monitoring Tool Performance

```go
snapshot := server.GetMCPMetricsSnapshot()

// Check tool call performance
if snapshot.Tools.CallDuration.P95 > 5*time.Second {
    log.Warn("Tool calls are slow", 
        "p95", snapshot.Tools.CallDuration.P95)
}

// Check error rate
errorRate := float64(snapshot.Tools.CallErrors) / 
    float64(snapshot.Tools.CallCount)
if errorRate > 0.1 {
    log.Warn("High tool call error rate", 
        "rate", errorRate)
}

// Check specific tool
if toolMetrics, ok := snapshot.Tools.ByTool["my_tool"]; ok {
    if toolMetrics.ErrorCount > 0 {
        log.Warn("Tool has errors", 
            "tool", "my_tool",
            "errors", toolMetrics.ErrorCount,
            "calls", toolMetrics.CallCount)
    }
}
```

## Example: Monitoring Queue Health

```go
snapshot := server.GetMCPMetricsSnapshot()

// Check queue health
if snapshot.Queue.Depth > 500 {
    log.Warn("Queue depth is high", 
        "depth", snapshot.Queue.Depth)
}

// Check drop rate
if snapshot.Queue.Dropped > 0 {
    dropRate := float64(snapshot.Queue.Dropped) / 
        float64(snapshot.Queue.Sent + snapshot.Queue.Dropped)
    log.Warn("Messages being dropped", 
        "dropped", snapshot.Queue.Dropped,
        "rate", dropRate)
}
```

## Example: Monitoring Concurrency

```go
snapshot := server.GetMCPMetricsSnapshot()

// Check current load
if snapshot.Concurrency.Current > 50 {
    log.Warn("High concurrency", 
        "current", snapshot.Concurrency.Current,
        "max", snapshot.Concurrency.Max)
}
```

## Reset Metrics

For testing, metrics can be reset:

```go
metrics := server.GetMCPMetrics()
metrics.Reset()
```

## Future Enhancements

1. **Metrics Export**: HTTP endpoint, Prometheus format
2. **Time-Series Storage**: Store metrics over time
3. **Alerting**: Alert on thresholds
4. **Dashboards**: Visualize metrics
5. **Rate Calculations**: Automatic rate calculations
6. **Custom Metrics**: Allow custom metric registration
