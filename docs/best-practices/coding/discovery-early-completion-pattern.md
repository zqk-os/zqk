# Discovery Early Completion Pattern

**Version**: 1.0.0  
**Created**: 2026-01-27  
**Status**: Active  
**Purpose**: Pattern for implementing early completion detection in long-running discovery operations

## Overview

The Discovery Early Completion Pattern provides a robust mechanism for detecting when a discovery operation has stabilized (no new items found for a period) and completing early rather than waiting for timeouts or all goroutines to finish. This pattern prevents unnecessary waiting when discovery has effectively completed.

## Problem Statement

When performing parallel discovery operations across multiple object kinds:
- Discovery goroutines may complete at different times
- Some kinds may finish quickly while others continue searching
- Without early completion detection, the system waits unnecessarily for all goroutines to finish or timeout
- This leads to poor user experience with apparent "hanging" behavior

## Solution Pattern

### Core Components

1. **Stable Count Detection**: Track when the discovered item count stabilizes (no change for a threshold duration)
2. **Early Completion Signal**: Use a channel to signal early completion when stability is detected
3. **Double-Close Prevention**: Use `sync.Once` to prevent panics from multiple close attempts
4. **Progress Emission**: Emit progress updates at regular intervals (independent of stability checks)

### Implementation Pattern

```go
// 1. Create completion channel and sync.Once for safe closing
discoveryComplete := make(chan struct{})
var discoveryCompleteOnce sync.Once

// 2. Track count stability
var lastCount int64
lastCountChangeTime := startTime

// 3. Progress emission tracking (independent of stability checks)
lastProgressEmitTime := startTime

// 4. Ticker goroutine for stability detection and progress emission
goroutinelabels.NewGoroutine("discovery_progress_ticker", "emitting periodic discovery progress").
    WithContext(ctx).
    StartSimple(func() {
        t := time.NewTicker(1 * time.Second) // Check frequently for fast response
        defer t.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-discoveryComplete:
                return
            case <-t.C:
                currentCount := atomic.LoadInt64(&totalFound)
                now := time.Now()

                // Track count changes
                if currentCount != lastCount {
                    lastCount = currentCount
                    lastCountChangeTime = now
                }

                // Early completion: count stable for threshold duration
                if lastCount > 0 && now.Sub(lastCountChangeTime) >= 3*time.Second {
                    discoveryCompleteOnce.Do(func() {
                        close(discoveryComplete)
                    })
                    return
                }

                // Progress emission: independent of stability checks
                if now.Sub(lastProgressEmitTime) >= 2*time.Second {
                    lastProgressEmitTime = now
                    emitProgress(...)
                }
            }
        }
    })

// 5. Main select: wait for completion signal OR timeout
select {
case <-waitDone:
    // All goroutines completed normally
case <-discoveryComplete:
    // Early completion triggered
case <-time.After(timeout):
    // Timeout fallback
case <-ctx.Done():
    // Context cancelled
}

// 6. Safe cleanup: use sync.Once to prevent double-close
discoveryCompleteOnce.Do(func() {
    close(discoveryComplete)
})
```

## Key Design Decisions

### 1. Ticker Frequency vs. Stability Threshold

- **Ticker**: Check every 1 second for fast response
- **Stability Threshold**: 3 seconds of no change triggers early completion
- **Rationale**: Frequent checks enable fast detection while threshold prevents premature completion from brief pauses

### 2. Progress Emission Independence

- **Progress Emission**: Every 2 seconds (independent of stability checks)
- **Rationale**: Users need regular feedback even when discovery is still active

### 3. Double-Close Prevention

- **Pattern**: Use `sync.Once` for channel closing
- **Rationale**: Both ticker goroutine and cleanup code may attempt to close the channel; `sync.Once` ensures only one close occurs

### 4. Atomic Count Tracking

- **Pattern**: Use `atomic.LoadInt64` and `atomic.AddInt64` for thread-safe count updates
- **Rationale**: Multiple goroutines update the count concurrently; atomic operations prevent race conditions

## Best Practices

### ✅ DO

1. **Use `sync.Once` for channel closing**: Prevents panics from double-close attempts
2. **Track count changes atomically**: Use atomic operations for thread-safe updates
3. **Separate stability detection from progress emission**: Different concerns, different intervals
4. **Initialize `lastCountChangeTime` to `startTime`**: Ensures accurate timing from the beginning
5. **Use context cancellation**: Respect context cancellation in all goroutines
6. **Log early completion**: Provide visibility into when and why early completion occurred

### ❌ DON'T

1. **Don't close channels without `sync.Once`**: Multiple goroutines may attempt to close
2. **Don't use non-atomic operations for shared counters**: Race conditions will occur
3. **Don't emit progress on every tick**: Reduces spam and improves performance
4. **Don't use very short stability thresholds**: May trigger premature completion
5. **Don't ignore context cancellation**: Goroutines should respect cancellation signals

## Configuration Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| Ticker Interval | 1 second | How frequently to check for stability |
| Stability Threshold | 3 seconds | Duration of no change before early completion |
| Progress Emission Interval | 2 seconds | How frequently to emit progress updates |
| Overall Timeout | 45 seconds | Maximum time to wait before timeout fallback |

## Testing Considerations

When testing this pattern:

1. **Test stable count detection**: Verify early completion triggers after threshold
2. **Test double-close prevention**: Ensure `sync.Once` prevents panics
3. **Test progress emission timing**: Verify progress emits at correct intervals
4. **Test context cancellation**: Ensure goroutines respect cancellation
5. **Test concurrent updates**: Verify atomic operations prevent race conditions

See `cmd/zqk/system/discovery_early_completion_test.go` for comprehensive test examples.

## Related Patterns

- **Concurrent Validation Pattern**: Discovery streams files to validation concurrently
- **Timeout Pattern**: Fallback timeout ensures completion even if early detection fails
- **Progress Reporting Pattern**: Regular progress updates provide user feedback

## Example Usage

This pattern is implemented in `cmd/zqk/system/async_check.go` in the `discoverObjectsParallel` function. It enables:

- Fast completion when discovery stabilizes (typically 3-4 seconds after last file found)
- Concurrent validation starting immediately as files are discovered
- Improved user experience with responsive feedback

## Performance Impact

- **Reduced Wait Time**: Early completion typically saves 10-40 seconds per discovery operation
- **Lower CPU Usage**: Fewer goroutines running unnecessarily
- **Better Responsiveness**: Users see results faster when discovery completes quickly

## Maintenance Notes

- Monitor stability threshold: Adjust if false positives/negatives occur
- Review ticker frequency: Balance between responsiveness and CPU usage
- Track early completion frequency: High frequency indicates good performance

## References

- Implementation: `cmd/zqk/system/async_check.go` (lines 548-606)
- Tests: `cmd/zqk/system/discovery_early_completion_test.go`
- Related: Concurrent validation pattern in `cmd/zqk/system/async_check_helpers.go`
