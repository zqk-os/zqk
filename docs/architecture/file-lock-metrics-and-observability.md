# File Lock Metrics and Observability

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2026-01-02  
**Status**: Active  
**Purpose**: Document metrics collection and observability for cross-process file locking

## Overview

The `FileLock` implementation includes comprehensive metrics collection to understand system busyness, lock contention, and performance characteristics. This enables:

1. **Early bailout**: Processes can detect high contention and bail out early
2. **System monitoring**: Track lock performance and identify bottlenecks
3. **Capacity planning**: Understand when system is approaching limits
4. **Debugging**: Identify lock-related performance issues

## Metrics Collected

### Acquisition Metrics

- **TotalAcquisitions**: Total number of successful lock acquisitions
- **TotalFailures**: Total number of failed lock attempts (errors)
- **TotalTimeouts**: Total number of timeout failures
- **TotalContention**: Total number of times lock was already held (TryLock returned false)

### Timing Metrics

- **TotalAcquisitionTime**: Total time spent acquiring locks (successful only)
- **TotalWaitTime**: Total time spent waiting for locks (including timeouts)
- **MaxAcquisitionTime**: Maximum time to acquire a lock
- **MaxWaitTime**: Maximum wait time (including timeouts)

### Contention Metrics

- **CurrentHolders**: Current number of processes holding locks (approximate)
- **PeakContention**: Maximum number of concurrent lock attempts

## Derived Metrics

### AverageAcquisitionTime

Average time to successfully acquire a lock:
```
AverageAcquisitionTime = TotalAcquisitionTime / TotalAcquisitions
```

**Use Case**: Baseline performance. If this increases over time, system may be under load.

### AverageWaitTime

Average time spent waiting (including timeouts):
```
AverageWaitTime = TotalWaitTime / (TotalTimeouts + TotalAcquisitions)
```

**Use Case**: Understanding typical wait times. High values indicate contention.

### ContentionRate

Rate of lock contention:
```
ContentionRate = TotalContention / TotalAttempts
```

**Use Case**: System busyness indicator. Values > 0.1 (10%) suggest high contention.

### SuccessRate

Rate of successful acquisitions:
```
SuccessRate = TotalAcquisitions / TotalAttempts
```

**Use Case**: System health indicator. Low values suggest problems.

## Early Bailout

### Configuration

```go
config := FileLockConfig{
    EarlyBailoutThreshold: 100 * time.Millisecond, // Bail out after 100ms
    EnableMetrics: true,
}
fileLock, err := NewFileLockWithConfig(lockFile, config)
```

### Behavior

When `EarlyBailoutThreshold` is set and exceeded:
- `LockWithTimeout()` returns an error immediately
- Error message includes elapsed time and threshold
- Suggests retrying later
- Prevents processes from waiting unnecessarily

### Use Cases

1. **High-priority operations**: Don't wait if system is busy
2. **User-facing operations**: Fail fast rather than blocking
3. **Batch operations**: Skip if contention is high, retry later

## Observability

### Accessing Metrics

```go
metrics := storage.GetFileLockMetrics()
snapshot := metrics.GetSnapshot()

fmt.Printf("Acquisitions: %d\n", snapshot.TotalAcquisitions)
fmt.Printf("Contention Rate: %.2f%%\n", snapshot.ContentionRate()*100)
fmt.Printf("Avg Acquisition Time: %v\n", snapshot.AverageAcquisitionTime())
fmt.Printf("Current Holders: %d\n", snapshot.CurrentHolders)
```

### Integration with Monitoring

Metrics can be exported to:
- **Prometheus**: Via metrics endpoint
- **Logging**: Periodic snapshot logs
- **CLI**: `zqk system metrics` command
- **MCP**: Exposed as metrics tools

## Performance Characteristics

### Typical Performance

- **Lock acquisition (no contention)**: < 1ms
- **Lock acquisition (with contention)**: 10-50ms (depends on lock holder)
- **TryLock (no contention)**: < 100μs
- **TryLock (with contention)**: < 100μs (returns immediately)

### When to Use Early Bailout

Set `EarlyBailoutThreshold` when:
- Average wait time > 50ms
- Contention rate > 10%
- User-facing operations (fail fast)
- System is known to be busy

### When NOT to Use Early Bailout

Don't use early bailout for:
- Critical operations (must complete)
- Background jobs (can wait)
- Low-priority operations (timeout is sufficient)

## Example: Monitoring Lock Performance

```go
// Get metrics snapshot
snapshot := storage.GetFileLockMetrics().GetSnapshot()

// Check system health
if snapshot.ContentionRate() > 0.2 {
    log.Warn("High lock contention detected",
        "contention_rate", snapshot.ContentionRate(),
        "current_holders", snapshot.CurrentHolders,
        "peak_contention", snapshot.PeakContention)
}

// Check performance degradation
if snapshot.AverageAcquisitionTime() > 10*time.Millisecond {
    log.Warn("Lock acquisition time increased",
        "avg_time", snapshot.AverageAcquisitionTime(),
        "max_time", snapshot.MaxAcquisitionTime)
}
```

## Future Enhancements

### Lock Availability Notification

**Problem**: Processes wait unnecessarily when lock becomes available.

**Proposed Solution**: 
- Use file system events (inotify, FSEvents) to notify when lock file changes
- Processes can subscribe to lock availability
- Reduces polling overhead

**Complexity**: High (requires platform-specific implementations)

**Alternative**: 
- Use shorter polling intervals when contention is detected
- Adaptive timeout based on metrics

### Lock Queue

**Problem**: First-come-first-served may not be optimal.

**Proposed Solution**:
- Maintain a queue of waiting processes
- Priority-based lock acquisition
- Fairness guarantees

**Complexity**: Very High (requires coordination mechanism)

**Alternative**:
- Use early bailout for low-priority operations
- Retry with exponential backoff

## Best Practices

1. **Monitor metrics regularly**: Track contention rate and average wait times
2. **Set early bailout appropriately**: Balance between fail-fast and completion
3. **Use TryLock for non-critical operations**: Don't block if lock is held
4. **Log metrics periodically**: Include in system health reports
5. **Alert on high contention**: Notify when contention rate exceeds threshold

## Related Documentation

- [Cross-Process File Locking Pattern](./shared-resource-locking.md): Core pattern documentation
- [Design Patterns Library](./DESIGN_PATTERNS.md): Pattern catalog
- [Architecture Patterns Library](./ARCHITECTURE_PATTERNS.md): Architecture patterns

