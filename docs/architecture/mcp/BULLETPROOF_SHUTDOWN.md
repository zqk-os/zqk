# Bulletproof Shutdown Implementation

## Philosophy

**Servers can fail, but they must never leave a mess behind.** Every goroutine, every resource, every process must be properly cleaned up. No leaks, no hanging processes, no orphaned goroutines.

## Core Principles

1. **Atomic Shutdown Detection**: Use atomic boolean flags for lock-free, thread-safe shutdown detection
2. **Guard Every Path**: Check shutdown at the start of every function, before every lock acquisition, in every loop iteration
3. **Clean Exit**: All goroutines must check shutdown and exit cleanly - no orphaned workers
4. **Resource Cleanup**: All resources (files, connections, processes) must be closed/stopped on shutdown
5. **Process Group Management**: Child processes must be killed with their parent (process groups)

## Implementation

### 1. Atomic Shutdown Flag

```go
type Server struct {
    shutdownFlag int32 // Atomic boolean: 0 = running, 1 = shutting down
    shutdownCtx  context.Context
    shutdownCancel context.CancelFunc
}

// Set shutdown (only in shutdownSequence)
func (s *Server) shutdownSequence(reason string) {
    // Use CompareAndSwapInt32 to ensure shutdown only runs once
    if !atomic.CompareAndSwapInt32(&s.shutdownFlag, 0, 1) {
        return // Already shutting down
    }
    // Cancel context to notify all components
    s.shutdownCancel()
    // ... cleanup ...
}

// Check shutdown (anywhere, any time)
func (s *Server) isShuttingDown() bool {
    return atomic.LoadInt32(&s.shutdownFlag) == 1
}
```

### 2. Guard Locations

**CRITICAL**: Every function that might acquire locks or spawn goroutines must check shutdown:

1. **Function Entry**: Check atomic flag at the very start (before ANY work)
2. **Before Lock Acquisition**: Check before every mutex lock
3. **After Lock Acquisition**: Check again (defensive, handles race conditions)
4. **Loop Iterations**: Check on every loop iteration
5. **Before Goroutine Spawn**: Check before spawning any goroutine
6. **Goroutine Entry**: Check at the start of every goroutine

### 3. Example: SendLogMessage

```go
func (s *Server) SendLogMessage(level LogLevel, message string, fields map[string]interface{}) error {
    // GUARD 0: Check atomic flag FIRST (before ANY work, including time.Now())
    if atomic.LoadInt32(&s.shutdownFlag) == 1 {
        // Only write to trace file, skip all other operations
        // ... trace file write ...
        return nil // Exit immediately
    }
    
    // GUARD 1: Check shutdown context (defensive)
    select {
    case <-s.shutdownCtx.Done():
        // ... trace file write ...
        return nil
    default:
    }
    
    // ... rest of function with additional guards ...
}
```

### 4. Example: SendMessageToClientByID

```go
func (s *Server) SendMessageToClientByID(clientID, message, messageType, priority string) error {
    // GUARD 0: Check atomic flag at the very start
    if atomic.LoadInt32(&s.shutdownFlag) == 1 {
        return nil // Exit immediately
    }
    
    // GUARD 0.5: Double-check shutdown context
    select {
    case <-s.shutdownCtx.Done():
        return nil
    default:
    }
    
    // ... multiple additional guards throughout function ...
}
```

### 5. Goroutine Cleanup

**CRITICAL**: Every goroutine must check shutdown and exit cleanly:

```go
// BAD: Fire-and-forget goroutine without shutdown check
go func() {
    s.clientMetricsStore.RecordEvent(...) // Might run during shutdown
}()

// GOOD: Check shutdown before spawning, and at start of goroutine
if atomic.LoadInt32(&s.shutdownFlag) == 1 {
    return // Don't spawn goroutine during shutdown
}
go func() {
    // Check shutdown at start of goroutine
    if atomic.LoadInt32(&s.shutdownFlag) == 1 {
        return
    }
    s.clientMetricsStore.RecordEvent(...)
}()
```

### 6. Loop Shutdown Checks

**CRITICAL**: Every loop must check shutdown on each iteration:

```go
// Main serve loop
for {
    // Check shutdown on EACH iteration
    if atomic.LoadInt32(&s.shutdownFlag) == 1 {
        return // Exit immediately
    }
    // ... process message ...
}

// Client iteration loop
for _, client := range s.clients {
    // Check shutdown on EACH iteration
    if atomic.LoadInt32(&s.shutdownFlag) == 1 {
        return false // Exit immediately
    }
    // ... process client ...
}
```

### 7. Process Group Management

**CRITICAL**: Child processes must be killed with their parent:

```go
cmd := exec.CommandContext(ctx, command, args...)
cmd.SysProcAttr = &syscall.SysProcAttr{
    Setpgid: true, // Create new process group
}

if err := cmd.Start(); err != nil {
    return err
}

// Kill entire process group on cancellation
processPID := cmd.Process.Pid
go func() {
    <-ctx.Done()
    if processPID > 0 {
        syscall.Kill(-processPID, syscall.SIGKILL) // Negative PID = process group
    }
}()

err := cmd.Wait()
```

## Complete Guard Checklist

For every function that might acquire locks or spawn goroutines:

- [ ] GUARD 0: Atomic flag check at function entry (before ANY work)
- [ ] GUARD 1: Shutdown context check (defensive)
- [ ] GUARD 2: Atomic flag check before lock acquisition
- [ ] GUARD 3: Atomic flag check after lock acquisition
- [ ] GUARD 4: Atomic flag check in loops (on each iteration)
- [ ] GUARD 5: Atomic flag check before spawning goroutines
- [ ] GUARD 6: Atomic flag check at goroutine entry

## Resource Cleanup Checklist

On shutdown, ensure:

- [ ] All goroutines are cancelled (context cancellation)
- [ ] All tickers are stopped
- [ ] All message queues are stopped and flushed
- [ ] All file handles are closed
- [ ] All process groups are killed
- [ ] All locks are released
- [ ] All metrics are saved

## Testing

Every shutdown scenario must be tested:

- [ ] Shutdown during normal operation
- [ ] Shutdown during message processing
- [ ] Shutdown during lock acquisition
- [ ] Shutdown during goroutine execution
- [ ] Concurrent shutdown requests (idempotent)
- [ ] Shutdown with hanging processes (must be killed)
- [ ] Shutdown with active goroutines (must exit cleanly)

## Failure Modes

The server must handle these gracefully:

1. **Sudden shutdown**: All resources cleaned up, no leaks
2. **Timeout shutdown**: Processes killed, goroutines exited
3. **Error shutdown**: State saved, resources released
4. **Concurrent shutdown**: Idempotent, no double-cleanup
5. **Partial shutdown**: Remaining resources still cleaned up

## References

- `pkg/mcp/server.go` - Main shutdown implementation
- `pkg/mcp/logging_channel.go` - Logging with shutdown guards
- `pkg/mcp/locking_helpers.go` - Lock acquisition with shutdown checks
- `pkg/scheduler/handlers_run_wrapper.go` - Process group management
- `docs/best-practices/atomic-shutdown-flags.md` - Best practices guide
