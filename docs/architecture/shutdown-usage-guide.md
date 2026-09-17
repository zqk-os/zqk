# Queue Shutdown Usage Guide

**Last Verified:** 2026-08-31


**Version:** 1.0  
**Status:** Active  
**Date:** 2026-01-17  
**Purpose:** Guide for using the queue shutdown coordinator

## Overview

The `QueueShutdownCoordinator` provides graceful shutdown management for all storage queues, ensuring no data loss during shutdown.

## Usage

### Basic Shutdown

```go
coordinator := storage.GetGlobalShutdownCoordinator()

// Initiate shutdown (stops accepting new operations)
if err := coordinator.InitiateShutdown(); err != nil {
    log.Fatal(err)
}

// Drain all queues (processes all pending operations)
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

if err := coordinator.DrainAll(ctx); err != nil {
    log.Fatal(err)
}

// Shutdown complete - safe to exit
```

### Integration with System Shutdown

```go
// In your shutdown handler (e.g., signal handler)
func handleShutdown() {
    coordinator := storage.GetGlobalShutdownCoordinator()
    
    // Initiate shutdown
    if err := coordinator.InitiateShutdown(); err != nil {
        log.Error("Failed to initiate shutdown", err)
    }
    
    // Drain with timeout
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    if err := coordinator.DrainAll(ctx); err != nil {
        log.Error("Shutdown drain failed", err)
        // Force shutdown if configured
    }
}
```

## Registered Queues

The following queues are automatically registered during system initialization:

1. **CAS Orphan Cleanup Queue** (non-critical)
2. **CAS Index Write Queue** (critical)
3. **I/O Queue Manager** (critical)
4. **Hash Registry Manager** (critical - tracks all registries)
5. **ID Generation Queue Manager** (non-critical)

## Queue Classification

### Critical Queues
- Must drain before force shutdown
- Data loss would be unacceptable
- Examples: CAS Index Write Queue, Hash Registry, I/O Queue Manager

### Non-Critical Queues
- Can be deferred during shutdown
- Data loss is acceptable or can be recovered
- Examples: CAS Orphan Cleanup Queue, ID Generation Queue Manager

## Configuration

```go
config := &storage.ShutdownConfig{
    Timeout:       30 * time.Second,  // Max time to wait
    ForceShutdown: true,              // Force after timeout
    LogIncomplete: true,              // Log incomplete operations
    CheckInterval: 100 * time.Millisecond,
}

coordinator := storage.GetGlobalShutdownCoordinator()
coordinator.SetConfig(config)
```

## Shutdown Sequence

1. **InitiateShutdown()**
   - Sets shutdown flag (atomic)
   - Notifies all queues to stop accepting new work
   - All enqueue operations check this flag and reject new work

2. **DrainAll()**
   - Processes all pending operations in all queues
   - Waits for all workers to complete
   - Respects timeout configuration

3. **Verification**
   - Checks if all queues are drained
   - Verifies critical queues completed
   - Logs incomplete operations if any

4. **Completion**
   - All queues drained
   - System ready for termination

## Error Handling

- **Shutdown Timeout**: Logs incomplete operations, optionally forces shutdown
- **Critical Queue Not Drained**: Prevents force shutdown (if configured)
- **Drain Errors**: Logged but don't block shutdown (best effort)

## Best Practices

1. **Always call InitiateShutdown() first** - prevents new work from being queued
2. **Use appropriate timeout** - balance between graceful shutdown and responsiveness
3. **Monitor incomplete operations** - review logs to identify issues
4. **Respect critical queues** - don't force shutdown if critical queues aren't drained
5. **Test shutdown scenarios** - ensure all queues drain properly
