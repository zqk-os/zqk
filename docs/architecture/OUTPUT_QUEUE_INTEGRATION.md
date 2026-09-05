# Output Queue Integration Guide

**Last Verified:** 2026-08-31


## Overview

The new output queue system replaces the current mutex-heavy progress tracking with a simple FIFO queue pattern. This eliminates lock contention and simplifies the architecture.

## Current Architecture (Before)

```
Workers → progressChan → Drain Goroutine → mu.Lock() → Update counters → mu.Unlock()
                                                          ↓
Main Thread → mu.RLock() → Read counters → mu.RUnlock()
```

**Problems**:
- High lock contention (drain goroutine vs main thread)
- Complex completion detection
- Potential deadlocks
- Hard to add new output channels

## New Architecture (After)

```
Workers → OutputQueue.Enqueue() → Single Writer → Route to Handler → I/O
```

**Benefits**:
- Minimal lock contention (microsecond lock holds)
- Simple FIFO processing
- Easy to add new channels
- No deadlock risk

## Integration Steps

### Step 1: Create Output Queue and Writer

```go
// In runCheckAsync or showValidationProgressWithMetrics
ctx := context.Background()
queue := validation.NewOutputQueue(10000) // Max 10k packets
writer := validation.NewOutputWriter(ctx, queue, logger)

// Register handlers
writer.RegisterHandler("progress", validation.NewWriterOutputHandler(progressWriter, true))
writer.RegisterHandler("stderr", validation.NewWriterOutputHandler(os.Stderr, false))
writer.RegisterHandler("stdout", validation.NewWriterOutputHandler(os.Stdout, false))

// Start writer
writer.Start()
defer writer.Stop()
```

### Step 2: Replace progressChan with Queue

**Before**:
```go
// In AsyncValidator worker
select {
case av.progressChan <- ValidationProgress{...}:
default:
    // Channel full, drop
}
```

**After**:
```go
// In AsyncValidator worker
queue.EnqueueProgress(ValidationProgress{
    Status: "completed",
    CurrentObject: objectID,
}, false) // Don't flush every update
```

### Step 3: Replace Direct Writes

**Before**:
```go
fmt.Fprintf(os.Stderr, "Progress: %d/%d\n", completed, total)
os.Stderr.Sync()
```

**After**:
```go
queue.EnqueueStderr(fmt.Sprintf("Progress: %d/%d\n", completed, total), true)
```

### Step 4: Remove Mutex from Progress Tracking

**Before**:
```go
var mu sync.RWMutex
var completed int
var completedObjectIDs map[string]bool

// Drain goroutine
mu.Lock()
completed++
completedObjectIDs[objectID] = true
mu.Unlock()
```

**After**:
```go
// No mutex needed! Workers just enqueue, writer processes
// If you need counters, track them in the progress handler
```

### Step 5: Update Completion Detection

**Before**:
```go
mu.RLock()
currentCompleted := completed
mu.RUnlock()
```

**After**:
```go
// Track completion in progress handler
// Or query AsyncValidator for stats directly
stats := validator.GetValidationStats()
```

## Example: Progress Handler

```go
type ProgressHandler struct {
    completed int
    failed    int
    mu        sync.Mutex // Only for counter updates
    writer    io.Writer
}

func (h *ProgressHandler) Write(data interface{}) error {
    progress, ok := data.(ValidationProgress)
    if !ok {
        return fmt.Errorf("invalid progress data")
    }
    
    // Update counters (minimal lock time)
    h.mu.Lock()
    if progress.Status == "completed" {
        h.completed++
    } else if progress.Status == "error" {
        h.failed++
    }
    h.mu.Unlock()
    
    // Write to output (if needed)
    if h.writer != nil {
        fmt.Fprintf(h.writer, "Progress: %s - %s\n", progress.Status, progress.CurrentObject)
    }
    
    return nil
}

func (h *ProgressHandler) GetCounts() (completed, failed int) {
    h.mu.Lock()
    defer h.mu.Unlock()
    return h.completed, h.failed
}
```

## Migration Checklist

### Phase 1: Setup (Non-Breaking)
- [x] Create `OutputQueue` and `OutputWriter`
- [ ] Add queue to `AsyncValidator` (optional field)
- [ ] Create progress handler
- [ ] Test queue in isolation

### Phase 2: Replace Progress Channel
- [ ] Update `AsyncValidator.Enqueue()` to use queue
- [ ] Update workers to enqueue instead of sending to channel
- [ ] Remove `progressChan` from `AsyncValidator`
- [ ] Test with existing code (both paths active)

### Phase 3: Remove Drain Goroutine
- [ ] Remove drain goroutine from `showValidationProgressWithMetrics`
- [ ] Use progress handler for completion detection
- [ ] Remove `mu` mutex from progress tracking
- [ ] Test completion detection

### Phase 4: Replace Direct Writes
- [ ] Replace all `fmt.Fprintf(os.Stderr, ...)` with queue
- [ ] Replace all `os.Stdout.Write()` with queue
- [ ] Test output correctness

### Phase 5: Cleanup
- [ ] Remove unused mutexes
- [ ] Remove unused channels
- [ ] Simplify completion detection
- [ ] Performance testing

## Performance Considerations

### Queue Size
- **Default**: 10,000 packets (configurable)
- **Monitoring**: Track queue depth, warn if > 80% full
- **Backpressure**: If queue full, workers can:
  - Wait (blocking)
  - Drop (non-blocking)
  - Retry with backoff

### Writer Efficiency
- **Batching**: Writer can batch multiple packets to same channel
- **Buffering**: Use buffered handlers for high-frequency channels
- **Flush Strategy**: 
  - Flush on `flush_hint=true`
  - Periodic flush (every N packets or M milliseconds)

### Lock Contention
- **Workers**: Lock held for ~1 microsecond (enqueue)
- **Writer**: Lock held for ~1 microsecond (dequeue)
- **Total**: Minimal contention, no blocking

## Testing

### Unit Tests
- [ ] Test queue enqueue/dequeue
- [ ] Test queue full behavior
- [ ] Test writer routing
- [ ] Test handler write/flush

### Integration Tests
- [ ] Test with real validation workload
- [ ] Test queue under high load
- [ ] Test writer shutdown
- [ ] Test error handling

### Performance Tests
- [ ] Measure queue throughput
- [ ] Measure lock contention
- [ ] Compare with old system
- [ ] Verify no performance regression

## Rollback Plan

If issues arise:
1. Keep old code path active (feature flag)
2. Switch back to `progressChan` if needed
3. Gradually migrate back

## Success Metrics

- ✅ No deadlocks
- ✅ Lock wait time < 1ms (99th percentile)
- ✅ Queue depth < 1000 (normal operation)
- ✅ No performance regression (< 5% overhead)
- ✅ Output correctness maintained

