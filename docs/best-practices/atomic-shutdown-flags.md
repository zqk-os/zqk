# Atomic Shutdown Flags - Best Practice

## Overview

When implementing graceful shutdown in concurrent Go applications, use atomic boolean flags instead of mutex-protected booleans for shutdown detection. This provides lock-free, thread-safe shutdown detection that can be checked from any goroutine without blocking.

## Problem

Traditional shutdown detection uses a mutex-protected boolean:

```go
type Server struct {
    shutdownMu         sync.Mutex
    shutdownInProgress bool
}

func (s *Server) isShuttingDown() bool {
    s.shutdownMu.Lock()
    defer s.shutdownMu.Unlock()
    return s.shutdownInProgress
}
```

**Issues:**
1. **Mutex contention**: Every shutdown check requires acquiring a mutex, which can block
2. **Deadlock risk**: If shutdown is initiated while holding other locks, checking shutdown can cause deadlocks
3. **Performance**: Mutex operations are slower than atomic operations
4. **Loop overhead**: Checking shutdown in tight loops adds significant overhead

## Solution: Atomic Boolean Flag

Use `sync/atomic` with an `int32` to represent a boolean flag:

```go
type Server struct {
    shutdownFlag int32 // Atomic boolean: 0 = running, 1 = shutting down
}

// Set shutdown (only in shutdownSequence)
func (s *Server) shutdownSequence(reason string) {
    // Use CompareAndSwapInt32 to ensure shutdown only runs once
    if !atomic.CompareAndSwapInt32(&s.shutdownFlag, 0, 1) {
        return // Already shutting down
    }
    // ... rest of shutdown logic
}

// Check shutdown (anywhere, any time)
func (s *Server) isShuttingDown() bool {
    return atomic.LoadInt32(&s.shutdownFlag) == 1
}
```

## Benefits

1. **Lock-Free**: Atomic operations don't require mutex locks
2. **Thread-Safe**: Multiple goroutines can check shutdown simultaneously without blocking
3. **Fast**: Atomic loads are very fast (single CPU instruction)
4. **Non-Blocking**: No risk of deadlock from shutdown checks
5. **Loop-Safe**: Can be checked on every loop iteration without performance penalty

## Implementation Pattern

### 1. Server Struct

```go
type Server struct {
    shutdownFlag int32 // Atomic boolean: 0 = running, 1 = shutting down
    shutdownMu   sync.Mutex // Only used for shutdownReason and other non-atomic fields
    shutdownReason string
    shutdownCtx   context.Context
    shutdownCancel context.CancelFunc
}
```

### 2. Setting Shutdown

```go
func (s *Server) shutdownSequence(reason string) {
    // CRITICAL: Use atomic compare-and-swap to ensure shutdown only runs once
    // This is thread-safe and lock-free - multiple goroutines can call this safely
    if !atomic.CompareAndSwapInt32(&s.shutdownFlag, 0, 1) {
        return // Already shutting down (another goroutine set the flag)
    }
    
    // Store shutdown reason (requires mutex for string assignment)
    s.shutdownMu.Lock()
    s.shutdownReason = reason
    s.shutdownMu.Unlock()
    
    // Cancel context to notify all registered hooks
    s.shutdownCancel()
    // ... rest of shutdown logic
}
```

### 3. Checking Shutdown

```go
func (s *Server) isShuttingDown() bool {
    // CRITICAL: Use atomic load for lock-free, thread-safe check
    // This is the primary shutdown check - fast and non-blocking
    if atomic.LoadInt32(&s.shutdownFlag) == 1 {
        return true
    }
    // Also check context as a secondary indicator (defensive)
    select {
    case <-s.shutdownCtx.Done():
        return true
    default:
        return false
    }
}
```

### 4. Checking in Loops

**CRITICAL**: Every loop must check the atomic flag on each iteration:

```go
func (s *Server) ServeLoop() error {
    for {
        // CRITICAL: Check atomic shutdown flag on EACH loop iteration
        // This ensures we exit immediately when shutdown is ordered
        if atomic.LoadInt32(&s.shutdownFlag) == 1 {
            return s.handleExplicitShutdown()
        }
        
        // ... process message ...
    }
}
```

### 5. Checking Before Lock Acquisition

```go
func (s *Server) withClientsReadLock(callback func() bool) bool {
    // GUARD 1: Check atomic flag before attempting lock
    if atomic.LoadInt32(&s.shutdownFlag) == 1 {
        return false // Shutdown in progress, don't acquire lock
    }
    
    // Use TryRLock() for non-blocking lock acquisition
    if !s.clientsMu.TryRLock() {
        return false // Lock not available
    }
    defer s.clientsMu.RUnlock()
    
    // GUARD 2: Check atomic flag after acquiring lock
    if atomic.LoadInt32(&s.shutdownFlag) == 1 {
        return false // Shutdown started, release lock and return
    }
    
    return callback()
}
```

## Guard Locations

When implementing shutdown detection, add atomic flag checks at these critical points:

1. **Start of functions** that might acquire locks
2. **Before lock acquisition** (to avoid deadlocks)
3. **After lock acquisition** (defensive check)
4. **On each loop iteration** (ensures immediate shutdown detection)
5. **Before blocking operations** (I/O, network calls, etc.)

## Example: Complete Implementation

```go
package server

import (
    "context"
    "sync"
    "sync/atomic"
)

type Server struct {
    shutdownFlag int32 // Atomic boolean: 0 = running, 1 = shutting down
    shutdownMu   sync.Mutex
    shutdownReason string
    shutdownCtx   context.Context
    shutdownCancel context.CancelFunc
    clientsMu    sync.RWMutex
    clients      map[string]*Client
}

func NewServer() *Server {
    shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
    return &Server{
        shutdownFlag:   0, // Initialize to 0 (running)
        shutdownCtx:    shutdownCtx,
        shutdownCancel: shutdownCancel,
        clients:        make(map[string]*Client),
    }
}

func (s *Server) shutdownSequence(reason string) {
    // Set shutdown flag atomically
    if !atomic.CompareAndSwapInt32(&s.shutdownFlag, 0, 1) {
        return // Already shutting down
    }
    
    // Store reason (requires mutex)
    s.shutdownMu.Lock()
    s.shutdownReason = reason
    s.shutdownMu.Unlock()
    
    // Cancel context
    s.shutdownCancel()
    
    // ... cleanup logic ...
}

func (s *Server) isShuttingDown() bool {
    return atomic.LoadInt32(&s.shutdownFlag) == 1
}

func (s *Server) ServeLoop() error {
    for {
        // Check shutdown on each iteration
        if atomic.LoadInt32(&s.shutdownFlag) == 1 {
            return nil // Exit immediately
        }
        
        // ... process messages ...
    }
}

func (s *Server) SendMessage(msg string) error {
    // Check shutdown before acquiring lock
    if atomic.LoadInt32(&s.shutdownFlag) == 1 {
        return nil // Exit immediately
    }
    
    s.clientsMu.RLock()
    defer s.clientsMu.RUnlock()
    
    // Check shutdown after acquiring lock
    if atomic.LoadInt32(&s.shutdownFlag) == 1 {
        return nil // Exit immediately
    }
    
    // ... send message ...
    return nil
}
```

## Testing

When testing shutdown behavior, verify:

1. **Atomic flag is set correctly** when shutdown is initiated
2. **Loops exit immediately** when flag is set
3. **Lock acquisition is avoided** when flag is set
4. **No deadlocks occur** during shutdown
5. **Multiple goroutines can check shutdown** simultaneously

Example test:

```go
func TestShutdownAtomicFlag(t *testing.T) {
    server := NewServer()
    
    // Start goroutine that checks shutdown in loop
    done := make(chan bool)
    go func() {
        for {
            if atomic.LoadInt32(&server.shutdownFlag) == 1 {
                done <- true
                return
            }
            time.Sleep(1 * time.Millisecond)
        }
    }()
    
    // Initiate shutdown
    server.shutdownSequence("test")
    
    // Verify goroutine exits quickly
    select {
    case <-done:
        // Success
    case <-time.After(100 * time.Millisecond):
        t.Fatal("Goroutine did not detect shutdown")
    }
}
```

## When to Use

Use atomic boolean flags for shutdown detection when:

- ✅ You have multiple goroutines that need to check shutdown status
- ✅ You have tight loops that need to check shutdown frequently
- ✅ You need to avoid mutex contention during shutdown
- ✅ You need lock-free shutdown detection to prevent deadlocks
- ✅ Performance is critical (high-frequency shutdown checks)

## When NOT to Use

Don't use atomic boolean flags when:

- ❌ You only have a single goroutine (mutex is fine)
- ❌ You need to store complex shutdown state (use mutex for complex data)
- ❌ You need to coordinate shutdown across multiple components (use context.Context)

## Related Patterns

- **Context Cancellation**: Use `context.Context` for propagating shutdown signals
- **Shutdown Hooks**: Register hooks to be notified when shutdown is ordered
- **TryRLock()**: Use non-blocking lock acquisition during shutdown

## References

- [Go sync/atomic package](https://pkg.go.dev/sync/atomic)
- [Go Memory Model](https://go.dev/ref/mem)
- Implementation: `pkg/mcp/server.go` (MCP server shutdown)

---

# Process Group Management for Child Process Cleanup - Best Practice

## Overview

When executing external commands that may spawn child processes, use process groups to ensure all child processes are killed when the parent process is terminated. This prevents process leaks when commands timeout or are cancelled.

## Problem

`exec.CommandContext` only kills the direct child process, not any subprocesses it spawns:

```go
cmd := exec.CommandContext(ctx, "some_command")
err := cmd.Run() // Only kills the direct child, not grandchildren
```

**Issues:**
1. **Child process leaks**: When a command spawns subprocesses, they become orphaned
2. **Resource accumulation**: Orphaned processes consume memory and CPU
3. **Zombie processes**: Dead processes that aren't reaped by their parent
4. **Timeout failures**: Commands that spawn children may not terminate on timeout

## Solution: Process Groups

Use `syscall.SysProcAttr{Setpgid: true}` to create a new process group, then kill the entire group when the context is cancelled:

```go
cmd := exec.CommandContext(ctx, "some_command")
cmd.SysProcAttr = &syscall.SysProcAttr{
    Setpgid: true, // Create new process group
}

if err := cmd.Start(); err != nil {
    return err
}

// Set up cleanup goroutine to kill process group on cancellation
processPID := cmd.Process.Pid
go func() {
    <-ctx.Done()
    // Kill entire process group using negative PID
    if processPID > 0 {
        syscall.Kill(-processPID, syscall.SIGKILL)
    }
}()

err := cmd.Wait()
```

## Implementation Pattern

### 1. Set Process Group Attribute

```go
cmd := exec.CommandContext(ctx, command, args...)
cmd.SysProcAttr = &syscall.SysProcAttr{
    Setpgid: true, // Create new process group (Unix only)
}
```

### 2. Start Command and Capture PID

```go
if err := cmd.Start(); err != nil {
    return err
}

processPID := cmd.Process.Pid
```

### 3. Set Up Process Group Cleanup

```go
go func() {
    <-ctx.Done()
    // Context cancelled (timeout or explicit cancellation)
    // Kill the entire process group using negative PID
    // This kills the process and all its children
    if processPID > 0 {
        // Negative PID targets the process group
        _ = syscall.Kill(-processPID, syscall.SIGKILL) //nolint:errcheck // Best effort cleanup
    }
}()
```

### 4. Wait for Completion

```go
err := cmd.Wait()
```

## Complete Example

```go
package scheduler

import (
    "context"
    "os/exec"
    "syscall"
)

func executeCommand(ctx context.Context, command string, args []string) error {
    cmd := exec.CommandContext(ctx, command, args...)
    
    // CRITICAL: Set process group to ensure child processes are killed on timeout
    // This prevents process leaks when commands spawn subprocesses
    cmd.SysProcAttr = &syscall.SysProcAttr{
        Setpgid: true, // Create new process group (Unix only)
    }
    
    // Start command
    if err := cmd.Start(); err != nil {
        return err
    }
    
    // Set up goroutine to kill process group when context is cancelled
    // This ensures child processes are killed, not just the direct child
    processPID := cmd.Process.Pid
    go func() {
        <-ctx.Done()
        // Context cancelled (timeout or explicit cancellation)
        // Kill the entire process group using negative PID
        // This kills the process and all its children
        if processPID > 0 {
            // Negative PID targets the process group
            _ = syscall.Kill(-processPID, syscall.SIGKILL) //nolint:errcheck // Best effort cleanup
        }
    }()
    
    // Wait for command to complete
    return cmd.Wait()
}
```

## Key Points

1. **Setpgid: true**: Creates a new process group for the command and its children
2. **Negative PID**: `syscall.Kill(-pid, signal)` targets the entire process group
3. **SIGKILL**: Use `SIGKILL` for forced termination (no cleanup), or `SIGTERM` for graceful shutdown
4. **Unix Only**: Process groups are Unix-specific; Windows requires different handling

## Platform Considerations

### Unix/Linux/macOS

```go
cmd.SysProcAttr = &syscall.SysProcAttr{
    Setpgid: true,
}
// Use negative PID to kill process group
syscall.Kill(-pid, syscall.SIGKILL)
```

### Windows

Windows doesn't support process groups in the same way. Use Job Objects or Windows-specific APIs:

```go
// Windows-specific implementation would use Job Objects
// This is more complex and requires platform-specific code
```

For cross-platform code, use build tags:

```go
// +build !windows

cmd.SysProcAttr = &syscall.SysProcAttr{
    Setpgid: true,
}
```

## When to Use

Use process groups when:

- ✅ Commands may spawn child processes
- ✅ Commands have timeouts that must be enforced
- ✅ You need to prevent process leaks
- ✅ Commands are executed in long-running services (schedulers, daemons)

## Testing

Test process group cleanup:

```go
func TestProcessGroupCleanup(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
    defer cancel()
    
    // Command that spawns a child process
    cmd := exec.CommandContext(ctx, "sh", "-c", "sleep 10 & sleep 10")
    cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
    
    if err := cmd.Start(); err != nil {
        t.Fatal(err)
    }
    
    processPID := cmd.Process.Pid
    go func() {
        <-ctx.Done()
        if processPID > 0 {
            syscall.Kill(-processPID, syscall.SIGKILL)
        }
    }()
    
    err := cmd.Wait()
    // Should timeout and kill all processes
    if err == nil {
        t.Fatal("Expected timeout")
    }
    
    // Verify no orphaned processes
    // (check process list)
}
```

## References

- [Go exec package](https://pkg.go.dev/os/exec)
- [Go syscall package](https://pkg.go.dev/syscall)
- Implementation: `pkg/scheduler/handlers_run_wrapper.go` (scheduler job execution)
