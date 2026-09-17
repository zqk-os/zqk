# Process Group Management - Best Practice

## Overview

Components that spawn goroutines or subprocesses must use a **ProcessGroupManager** to track and control all spawned processes. This ensures that shutdown can properly terminate all processes, preventing leaks and ensuring clean shutdown.

## Philosophy

**Every component that spawns processes must be able to control them.** No orphaned goroutines, no hanging subprocesses, no resource leaks. The process group manager provides centralized control and ensures all processes can be terminated during shutdown.

## Problem

Without process group management:

1. **Goroutine leaks**: Fire-and-forget goroutines that don't check shutdown
2. **Subprocess leaks**: Child processes that aren't killed when parent exits
3. **Resource accumulation**: Processes that continue running after shutdown
4. **Uncontrolled shutdown**: No way to ensure all processes exit cleanly

## Solution: ProcessGroupManager

The `ProcessGroupManager` provides:

1. **Tracking**: All goroutines and subprocesses are registered
2. **Control**: Centralized shutdown that cancels/kills all processes
3. **Lifecycle**: Context-based cancellation for graceful shutdown
4. **Safety**: Critical vs non-critical process handling

## Implementation Pattern

### 1. Create Process Group Manager

```go
type Component struct {
    processGroupManager *ProcessGroupManager
}

func NewComponent() *Component {
    // Create with 5 second shutdown timeout
    // This allows critical operations to complete, but prevents hanging
    processGroupManager := NewProcessGroupManager(5 * time.Second)
    
    return &Component{
        processGroupManager: processGroupManager,
    }
}
```

### 2. Spawn Goroutines Through Manager

```go
// BAD: Fire-and-forget goroutine
go func() {
    doWork() // Might run during shutdown, no control
}()

// GOOD: Tracked goroutine
ctx, unregister := component.processGroupManager.SpawnGoroutine(
    "worker-1",
    "Background Worker",
    "Processes background tasks",
    false, // Not critical - can be cancelled
    func(ctx context.Context) {
        // Check shutdown context in loop
        for {
            select {
            case <-ctx.Done():
                return // Shutdown ordered, exit immediately
            default:
                doWork()
            }
        }
    },
)
defer unregister() // Cleanup on exit
```

### 3. Register Subprocesses

```go
cmd := exec.CommandContext(ctx, "command", "args")
cmd.SysProcAttr = &syscall.SysProcAttr{
    Setpgid: true, // Create process group
}

if err := cmd.Start(); err != nil {
    return err
}

processPID := cmd.Process.Pid
processGroupID := -processPID // Negative PID for process group

killFunc := func() error {
    return syscall.Kill(processGroupID, syscall.SIGKILL)
}

// Register subprocess
component.processGroupManager.RegisterSubprocess(
    "subprocess-1",
    "job-id",
    "Description of subprocess",
    processPID,
    processGroupID,
    false, // Not critical
    killFunc,
)

// Unregister on completion
defer component.processGroupManager.UnregisterSubprocess("subprocess-1")
```

### 4. Shutdown Process Group

```go
func (c *Component) Shutdown(reason string) error {
    // Shutdown process group manager
    // This cancels all goroutine contexts and kills all subprocesses
    if c.processGroupManager != nil {
        if err := c.processGroupManager.Shutdown(reason); err != nil {
            // Some critical processes may still be running
            return fmt.Errorf("process group shutdown: %w", err)
        }
    }
    return nil
}
```

## Critical vs Non-Critical Processes

### Non-Critical Processes

- Can be cancelled immediately on shutdown
- Examples: logging goroutines, metrics recording, background workers
- Use `critical: false` when spawning

```go
processGroupManager.SpawnGoroutine(
    "metrics-recorder",
    "Metrics Recorder",
    "Records metrics asynchronously",
    false, // Non-critical - can be cancelled
    func(ctx context.Context) {
        // ... work ...
    },
)
```

### Critical Processes

- Must complete before shutdown completes
- Examples: saving state, flushing buffers, completing transactions
- Use `critical: true` when spawning

```go
processGroupManager.SpawnGoroutine(
    "state-saver",
    "State Saver",
    "Saves component state to disk",
    true, // Critical - must complete
    func(ctx context.Context) {
        // ... save state ...
    },
)
```

## Shutdown Behavior

### Graceful Shutdown

1. **Cancel contexts**: All goroutine contexts are cancelled
2. **Wait for non-critical**: Non-critical processes are given time to exit
3. **Wait for critical**: Critical processes are allowed to complete
4. **Force kill**: If timeout exceeded, force kill remaining processes

### Timeout Handling

- **Non-critical processes**: Cancelled immediately, short timeout for cleanup
- **Critical processes**: Allowed to complete, but with overall timeout
- **Subprocesses**: SIGTERM first (graceful), then SIGKILL (force)

## Integration with Components

### MCP Server

```go
type Server struct {
    processGroupManager *ProcessGroupManager
}

func NewServer() *Server {
    processGroupManager := NewProcessGroupManager(5 * time.Second)
    return &Server{
        processGroupManager: processGroupManager,
    }
}

// Spawn welcome message goroutine
_, _ = s.processGroupManager.SpawnGoroutine(
    "welcome-message",
    "Welcome Message Sender",
    "Sends welcome message to client",
    false, // Non-critical
    func(ctx context.Context) {
        // ... send welcome message ...
    },
)
```

### Scheduler

```go
type RunWrapperHandler struct {
    processGroupManager *ProcessGroupManager
}

func NewRunWrapperHandler(...) *RunWrapperHandler {
    processGroupManager := NewProcessGroupManager(30 * time.Second)
    return &RunWrapperHandler{
        processGroupManager: processGroupManager,
    }
}

// Register subprocess when command starts
h.processGroupManager.RegisterSubprocess(
    subprocessID,
    job.ID,
    "Command execution",
    processPID,
    processGroupID,
    false, // Non-critical
    killFunc,
)
```

## Status Monitoring

The process group manager provides status information:

```go
status := processGroupManager.GetStatus()
// status.ShuttingDown - whether shutdown is in progress
// status.Goroutines - list of tracked goroutines
// status.Subprocesses - list of tracked subprocesses
// status.GoroutineCount - number of active goroutines
// status.SubprocessCount - number of active subprocesses
```

## Best Practices

1. **Always use process group manager**: Never spawn goroutines or subprocesses without tracking
2. **Check shutdown in loops**: Every loop must check `ctx.Done()`
3. **Unregister on completion**: Clean up registrations when processes complete
4. **Mark critical processes**: Identify which processes must complete before shutdown
5. **Set appropriate timeouts**: Balance between allowing completion and preventing hangs
6. **Handle shutdown errors**: Log warnings but don't block shutdown for non-critical failures

## Testing

Test process group management:

```go
func TestProcessGroupShutdown(t *testing.T) {
    manager := NewProcessGroupManager(1 * time.Second)
    
    // Spawn goroutine
    ctx, _ := manager.SpawnGoroutine("test", "Test", "Test goroutine", false, func(ctx context.Context) {
        for {
            select {
            case <-ctx.Done():
                return
            default:
                time.Sleep(10 * time.Millisecond)
            }
        }
    })
    
    // Shutdown
    err := manager.Shutdown("test")
    if err != nil {
        t.Fatal(err)
    }
    
    // Verify context is cancelled
    select {
    case <-ctx.Done():
        // Success
    case <-time.After(100 * time.Millisecond):
        t.Fatal("Context not cancelled after shutdown")
    }
}
```

## References

- `pkg/mcp/process_group_manager.go` - MCP server process group manager
- `pkg/scheduler/process_group_manager.go` - Scheduler process group manager
- `pkg/mcp/server.go` - MCP server integration
- `pkg/scheduler/handlers_run_wrapper.go` - Scheduler subprocess registration
