# Runtime Goroutine Manager

**Last Verified:** 2026-08-31


**Version:** 1.0  
**Status:** Active  
**Last Updated:** 2026-01-13  
**Package:** `pkg/runtime`

## Overview

The GoroutineManager provides OS-level tracking and management of all goroutines in the system. It integrates with the coordination framework to emit events for observability and ensures no goroutine leaks.

**Critical for Operating System:** Since we're building an operating system, tracking every process and thread (goroutine) is vital for system reliability, observability, and resource management.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    GoroutineManager                         │
│                                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │   Tracking   │  │  Lifecycle    │  │  Observability│     │
│  │              │  │  Management  │  │              │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
│         │                 │                    │           │
│         └─────────────────┴────────────────────┘           │
│                            │                               │
│                            ▼                               │
│              ┌─────────────────────────┐                  │
│              │  EventCoordinator        │                  │
│              │  (Spinal Cord)           │                  │
│              └─────────────────────────┘                  │
│                            │                               │
│         ┌──────────────────┼──────────────────┐           │
│         ▼                  ▼                  ▼           │
│   ┌──────────┐      ┌──────────┐      ┌──────────┐      │
│   │ Logging  │      │  Audit   │      │ Metrics   │      │
│   └──────────┘      └──────────┘      └──────────┘      │
└─────────────────────────────────────────────────────────────┘
```

## Key Capabilities

### 1. **Complete Lifecycle Tracking**

Every goroutine is tracked from start to finish:
- **Start**: When goroutine starts, with metadata
- **Running**: Active state monitoring
- **Stop**: When goroutine stops (normal or error)
- **Leak Detection**: Identifies goroutines that should have stopped

### 2. **Resource Management**

Tracks and manages resources used by goroutines:
- **Tickers**: Automatic cleanup on stop
- **Channels**: Tracked for leak detection
- **Files**: File handles tracked
- **Connections**: Network connections tracked
- **Custom Resources**: Extensible resource tracking

### 3. **Observability Integration**

All goroutine lifecycle events are emitted through the coordination framework:
- **Logging**: Structured logs for all lifecycle events
- **Audit**: Audit events for compliance and debugging
- **Metrics**: Metrics for goroutine counts, durations, errors
- **Operational**: Events for process coordination

### 4. **Graceful Shutdown**

Ensures all goroutines stop cleanly:
- **Context Cancellation**: All goroutines receive cancellation signal
- **Resource Cleanup**: All resources are cleaned up
- **Timeout Protection**: Prevents indefinite shutdown
- **Leak Detection**: Identifies goroutines that didn't stop

### 5. **OS-Level Process Management**

Provides operating system-level capabilities:
- **Process Tracking**: Every goroutine is a tracked "process"
- **Thread Management**: Goroutines are the OS threads
- **Resource Limits**: Enforce max concurrent goroutines
- **Health Monitoring**: Track system health via goroutine stats

## Usage

### Basic Usage

```go
import (
    "github.com/lanceman/zqk/pkg/runtime"
    "github.com/lanceman/zqk/pkg/coordination"
)

// Get coordinator
coordinator := coordination.GetCoordinator()

// Create manager
manager := runtime.NewGoroutineManager(coordinator)

// Start a goroutine
id, ctx, err := manager.Start(runtime.GoroutineConfig{
    Name:     "periodic_compression",
    Purpose:  "Compress metrics periodically",
    Category: "background",
    Resources: []runtime.Resource{
        {
            Type:        "ticker",
            ID:          "compression_ticker",
            Description: "Periodic compression ticker",
            CleanupFunc: func() error {
                ticker.Stop()
                return nil
            },
        },
    },
}, func(ctx context.Context) error {
    ticker := time.NewTicker(24 * time.Hour)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return nil
        case <-ticker.C:
            // Do compression
        }
    }
})

// On shutdown
if err := manager.Shutdown(); err != nil {
    // Handle shutdown errors (leaks, timeouts)
}
```

### Advanced Usage

```go
// Set limits
manager.SetMaxGoroutines(100)
manager.SetShutdownTimeout(30 * time.Second)

// Monitor stats
stats := manager.GetRuntimeStats()
fmt.Printf("Active: %d, Total Started: %d\n", 
    stats["active_count"], stats["total_started"])

// Detect leaks
leaks := manager.DetectLeaks()
for _, leak := range leaks {
    fmt.Printf("Leaked goroutine: %s\n", leak.ID)
}

// Get specific goroutine info
tracked, err := manager.GetGoroutine(id)
if err == nil {
    fmt.Printf("Goroutine %s: %s\n", tracked.Name, tracked.Status)
}
```

## Integration with Coordination Framework

The GoroutineManager emits events through the coordination framework:

### Event Types

1. **`goroutine_lifecycle.start`**
   - Emitted when goroutine starts
   - Includes: ID, name, purpose, category, resources

2. **`goroutine_lifecycle.stop`**
   - Emitted when goroutine stops normally
   - Includes: ID, duration, status

3. **`goroutine_lifecycle.error`**
   - Emitted when goroutine exits with error
   - Includes: ID, error, duration

4. **`goroutine_lifecycle.leaked`**
   - Emitted when leak is detected
   - Includes: ID, duration, threshold

5. **`goroutine_lifecycle.cleanup_error`**
   - Emitted when resource cleanup fails
   - Includes: ID, resource, error

### Event Channels

All events are routed to:
- **Logging**: Structured logs
- **Audit**: Audit trail
- **Metrics**: System metrics
- **Operational**: Process coordination

## Benefits for Operating System

### 1. **Complete Visibility**

Every goroutine is tracked:
- Know exactly what's running
- Track resource usage
- Monitor system health
- Debug issues quickly

### 2. **Resource Safety**

No resource leaks:
- All resources tracked
- Automatic cleanup
- Leak detection
- Graceful shutdown

### 3. **Observability**

Full observability:
- Metrics for all goroutines
- Audit trail of lifecycle
- Logs for debugging
- Operational events for coordination

### 4. **System Reliability**

Prevents system degradation:
- Max goroutine limits
- Leak detection
- Graceful shutdown
- Error tracking

## Migration Guide

### Step 1: Replace Direct `go func()` Calls

**Before:**
```go
go func() {
    for range ticker.C {
        // Do work
    }
}()
```

**After:**
```go
manager.Start(runtime.GoroutineConfig{
    Name:     "worker",
    Purpose:  "Process items",
    Category: "worker",
}, func(ctx context.Context) error {
    for {
        select {
        case <-ctx.Done():
            return nil
        case <-ticker.C:
            // Do work
        }
    }
})
```

### Step 2: Add Resource Tracking

**Before:**
```go
ticker := time.NewTicker(interval)
go func() {
    for range ticker.C {
        // Do work
    }
}()
// Ticker never stopped!
```

**After:**
```go
ticker := time.NewTicker(interval)
manager.Start(runtime.GoroutineConfig{
    Name:     "periodic_task",
    Resources: []runtime.Resource{
        {
            Type:        "ticker",
            ID:          "task_ticker",
            CleanupFunc: func() error { ticker.Stop(); return nil },
        },
    },
}, func(ctx context.Context) error {
    for {
        select {
        case <-ctx.Done():
            return nil
        case <-ticker.C:
            // Do work
        }
    }
})
```

### Step 3: Integrate with Shutdown

**Before:**
```go
// No shutdown handling
```

**After:**
```go
func (s *Server) shutdownSequence() {
    // Stop all managed goroutines
    if err := s.goroutineManager.Shutdown(); err != nil {
        // Handle shutdown errors
    }
}
```

## Testing

### Leak Detection Test

```go
func TestNoGoroutineLeaks(t *testing.T) {
    manager := runtime.NewGoroutineManager(mockCoord)
    
    // Start goroutines
    manager.Start(...)
    
    // Shutdown
    if err := manager.Shutdown(); err != nil {
        t.Errorf("Shutdown failed: %v", err)
    }
    
    // Verify no leaks
    leaks := manager.DetectLeaks()
    if len(leaks) > 0 {
        t.Errorf("Detected %d leaked goroutines", len(leaks))
    }
}
```

### Integration Test

```go
func TestGoroutineManagerIntegration(t *testing.T) {
    coordinator := coordination.GetCoordinator()
    manager := runtime.NewGoroutineManager(coordinator)
    
    // Start goroutine
    id, ctx, err := manager.Start(...)
    require.NoError(t, err)
    
    // Verify tracking
    tracked, err := manager.GetGoroutine(id)
    require.NoError(t, err)
    assert.Equal(t, runtime.StatusRunning, tracked.Status)
    
    // Stop
    require.NoError(t, manager.Stop(id))
    
    // Verify stopped
    time.Sleep(100 * time.Millisecond)
    assert.Equal(t, 0, manager.ActiveCount())
}
```

## Performance Considerations

- **Overhead**: Minimal - uses atomic counters for stats
- **Memory**: ~200 bytes per tracked goroutine
- **Lock Contention**: Uses RWMutex for concurrent access
- **Event Emission**: Async, non-blocking

## Future Enhancements

1. **Goroutine Pools**: Pre-allocated goroutine pools
2. **Priority Scheduling**: Priority-based goroutine scheduling
3. **Resource Limits**: Per-goroutine resource limits
4. **Distributed Tracking**: Track goroutines across processes
5. **Visualization**: Dashboard for goroutine monitoring

## References

- [Go Context Package](https://pkg.go.dev/context)
- [Coordination Framework](../coordination/README.md)
- [Goroutine Lifecycle Management](../mcp-goroutine-lifecycle-v1.0.md)
