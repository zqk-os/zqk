# MCP Goroutine Lifecycle Management

**Version:** 1.0  
**Status:** Active  
**Last Updated:** 2026-01-13

## Overview

This document defines best practices for managing goroutine lifecycles in the MCP server to prevent resource leaks and ensure graceful shutdown.

## Problem Statement

Goroutine leaks occur when background goroutines continue running after their parent context has been cancelled or the server has shut down. This leads to:
- High CPU usage from orphaned goroutines
- Memory leaks
- Resource exhaustion
- Process persistence after shutdown

## Best Practices

### 1. Always Use Context for Lifecycle Management

**✅ DO:**
```go
func (s *Server) StartPeriodicCompression(ctx context.Context, interval, retentionPeriod time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop() // Always defer cleanup
    
    go func() {
        defer ticker.Stop() // Ensure cleanup even if goroutine exits early
        
        for {
            select {
            case <-ctx.Done():
                return // Exit on context cancellation
            case <-ticker.C:
                // Do work
                if err := s.CompressMetrics(retentionPeriod); err != nil {
                    // Handle error
                }
            }
        }
    }()
}
```

**❌ DON'T:**
```go
func (s *Server) StartPeriodicCompression(interval, retentionPeriod time.Duration) *time.Ticker {
    ticker := time.NewTicker(interval)
    go func() {
        for range ticker.C { // No way to cancel!
            // Do work
        }
    }()
    return ticker // Caller must remember to stop
}
```

### 2. Store Resources for Cleanup

**✅ DO:**
- Store all resources (tickers, contexts, channels) that need cleanup
- Use a cleanup method that's called on shutdown
- Use `defer` for guaranteed cleanup

```go
type Server struct {
    compressionTicker   *time.Ticker
    compressionCtx     context.Context
    compressionCancel  context.CancelFunc
    compressionTickerMu sync.Mutex
}

func (s *Server) StartPeriodicCompression(ctx context.Context, ...) {
    s.compressionCtx, s.compressionCancel = context.WithCancel(ctx)
    // ... start goroutine with context
}

func (s *Server) shutdownSequence(...) {
    // Stop ticker
    s.compressionTickerMu.Lock()
    if s.compressionTicker != nil {
        s.compressionTicker.Stop()
    }
    s.compressionTickerMu.Unlock()
    
    // Cancel context
    if s.compressionCancel != nil {
        s.compressionCancel()
    }
}
```

### 3. Use sync.WaitGroup for Tracking

For multiple related goroutines, use `sync.WaitGroup`:

```go
type Server struct {
    wg sync.WaitGroup
}

func (s *Server) StartBackgroundWork(ctx context.Context) {
    s.wg.Add(1)
    go func() {
        defer s.wg.Done()
        
        for {
            select {
            case <-ctx.Done():
                return
            case <-time.After(interval):
                // Do work
            }
        }
    }()
}

func (s *Server) Shutdown() {
    // Cancel all contexts
    s.cancelAll()
    
    // Wait for all goroutines to finish (with timeout)
    done := make(chan struct{})
    go func() {
        s.wg.Wait()
        close(done)
    }()
    
    select {
    case <-done:
        // All goroutines finished
    case <-time.After(5 * time.Second):
        // Timeout - log warning
    }
}
```

### 4. Always Check Context in Loops

**✅ DO:**
```go
for {
    select {
    case <-ctx.Done():
        return // Always check context
    case data := <-inputChan:
        // Process data
    }
}
```

**❌ DON'T:**
```go
for data := range inputChan {
    // Process data - no way to cancel!
}
```

### 5. Use Buffered Channels with Care

- Unbuffered channels can cause goroutines to block indefinitely
- Buffered channels can hide leaks (goroutine exits but channel still has data)
- Always provide a way to cancel/drain channels

### 6. Document Goroutine Lifecycle

Every function that starts a goroutine should document:
- When it starts
- When it stops
- How to cancel it
- What resources it uses

```go
// StartPeriodicCompression starts a background goroutine that periodically
// compresses metrics. The goroutine runs until ctx is cancelled.
//
// Lifecycle:
//   - Starts: Immediately when called
//   - Stops: When ctx is cancelled or ticker is stopped
//   - Cleanup: Ticker is stopped automatically via defer
//
// Resources:
//   - Creates one goroutine
//   - Uses one time.Ticker
//   - Accesses ClientMetricsStore (thread-safe)
func (s *ClientMetricsStore) StartPeriodicCompression(ctx context.Context, ...) {
    // ...
}
```

## Implementation Checklist

When adding a new background goroutine:

- [ ] Accept `context.Context` as first parameter
- [ ] Check `ctx.Done()` in all loops
- [ ] Use `defer` for cleanup (ticker.Stop(), cancel(), etc.)
- [ ] Store resources (ticker, cancel func) for shutdown cleanup
- [ ] Add cleanup to `shutdownSequence()`
- [ ] Document lifecycle in function comment
- [ ] Add test to verify goroutine exits on context cancellation
- [ ] Use leak detection in tests (see Testing section)

## Testing for Leaks

### Using goleak

```go
import "go.uber.org/goleak"

func TestServerShutdown(t *testing.T) {
    defer goleak.VerifyNone(t) // Detects goroutine leaks
    
    server := NewServer()
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    server.StartPeriodicCompression(ctx, ...)
    
    // Cancel context
    cancel()
    
    // Wait a bit for goroutine to exit
    time.Sleep(100 * time.Millisecond)
    
    // goleak will detect if goroutine didn't exit
}
```

### Manual Verification

```go
func TestNoGoroutineLeaks(t *testing.T) {
    before := runtime.NumGoroutine()
    
    server := NewServer()
    ctx, cancel := context.WithCancel(context.Background())
    server.StartPeriodicCompression(ctx, ...)
    
    cancel()
    time.Sleep(100 * time.Millisecond) // Allow cleanup
    
    after := runtime.NumGoroutine()
    
    if after > before {
        t.Errorf("Goroutine leak detected: before=%d, after=%d", before, after)
    }
}
```

## Current Implementation Status

### ✅ Fixed Issues

1. **Compression Ticker** (2026-01-13)
   - Now stores ticker on Server struct
   - Stops ticker in shutdownSequence()
   - Added mutex protection

### 🔄 Needs Improvement

1. **Compression Goroutine**
   - Should use context.Context instead of relying on ticker.Stop()
   - Should check ctx.Done() in loop

2. **Read Goroutines**
   - Each loop iteration creates a new goroutine
   - Should verify old goroutines exit when new ones start

3. **Async Handler Goroutines**
   - Currently uses context properly ✅
   - Should add timeout verification

## Migration Plan

1. **Phase 1: Add Context Support** (Current)
   - Refactor `StartPeriodicCompression` to accept context
   - Update all call sites

2. **Phase 2: Add WaitGroup Tracking**
   - Add `sync.WaitGroup` to Server struct
   - Track all background goroutines

3. **Phase 3: Add Leak Detection Tests**
   - Add goleak to test dependencies
   - Create test suite for all background goroutines

4. **Phase 4: Documentation**
   - Update all function comments
   - Add architecture diagrams
   - Create developer guide

## References

- [Go Context Package](https://pkg.go.dev/context)
- [Effective Go: Goroutines](https://go.dev/doc/effective_go#goroutines)
- [Go Memory Model](https://go.dev/ref/mem)
- [uber-go/goleak](https://github.com/uber-go/goleak)
