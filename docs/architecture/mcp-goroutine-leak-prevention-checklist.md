# Goroutine Leak Prevention Checklist

**Last Verified:** 2026-08-31


**Quick Reference Guide** - Use this checklist when adding background goroutines.

## ✅ Pre-Implementation Checklist

Before writing code that starts a goroutine:

- [ ] **Do I really need a goroutine?** 
  - Can this be done synchronously?
  - Can this use existing async infrastructure?

- [ ] **What is the lifecycle?**
  - When does it start?
  - When does it stop?
  - What triggers shutdown?

- [ ] **What resources does it use?**
  - Files, network connections, timers?
  - How are these cleaned up?

## ✅ Implementation Checklist

When implementing a background goroutine:

- [ ] **Accept `context.Context` as first parameter**
  ```go
  func StartBackgroundWork(ctx context.Context, ...) {
      // NOT: func StartBackgroundWork(...) {
  }
  ```

- [ ] **Check `ctx.Done()` in all loops**
  ```go
  for {
      select {
      case <-ctx.Done():
          return // Always exit on cancellation
      case data := <-inputChan:
          // Process data
      }
  }
  ```

- [ ] **Use `defer` for cleanup**
  ```go
  ticker := time.NewTicker(interval)
  defer ticker.Stop() // Always cleanup
  ```

- [ ] **Store resources for shutdown**
  ```go
  type Server struct {
      backgroundCtx    context.Context
      backgroundCancel context.CancelFunc
      backgroundTicker *time.Ticker
      mu sync.Mutex
  }
  ```

- [ ] **Add cleanup to shutdown sequence**
  ```go
  func (s *Server) shutdownSequence() {
      s.mu.Lock()
      if s.backgroundCancel != nil {
          s.backgroundCancel()
      }
      if s.backgroundTicker != nil {
          s.backgroundTicker.Stop()
      }
      s.mu.Unlock()
  }
  ```

- [ ] **Document lifecycle in function comment**
  ```go
  // StartBackgroundWork starts a background goroutine that...
  //
  // Lifecycle:
  //   - Starts: When called
  //   - Stops: When ctx is cancelled
  //   - Cleanup: Automatic via defer
  //
  // Resources:
  //   - Creates 1 goroutine
  //   - Uses 1 time.Ticker
  func StartBackgroundWork(ctx context.Context, ...) {
  }
  ```

## ✅ Testing Checklist

Before committing:

- [ ] **Test goroutine exits on context cancellation**
  ```go
  ctx, cancel := context.WithCancel(context.Background())
  StartBackgroundWork(ctx, ...)
  cancel()
  time.Sleep(100 * time.Millisecond)
  // Verify goroutine exited
  ```

- [ ] **Test cleanup in shutdown sequence**
  ```go
  server := NewServer()
  server.StartBackgroundWork(...)
  server.shutdownSequence()
  // Verify no goroutines running
  ```

- [ ] **Use leak detection** (if available)
  ```go
  import "go.uber.org/goleak"
  
  func TestNoLeaks(t *testing.T) {
      defer goleak.VerifyNone(t)
      // Your test
  }
  ```

## ❌ Common Mistakes to Avoid

1. **Returning ticker without storing it**
   ```go
   // BAD
   ticker := store.StartPeriodicCompression(...)
   return ticker // Caller might forget to stop
   
   // GOOD
   s.compressionTicker = store.StartPeriodicCompression(ctx, ...)
   // Store it, stop it in shutdown
   ```

2. **Using unbounded loops without context**
   ```go
   // BAD
   for range ticker.C {
       // No way to cancel!
   }
   
   // GOOD
   for {
       select {
       case <-ctx.Done():
           return
       case <-ticker.C:
           // Do work
       }
   }
   ```

3. **Creating channels that never close**
   ```go
   // BAD
   never := make(chan struct{})
   timeoutChan = never // Never closes!
   
   // GOOD
   timeoutChan = nil // nil channel never triggers
   ```

4. **Forgetting to cancel contexts**
   ```go
   // BAD
   ctx, cancel := context.WithCancel(...)
   go func() { ... }()
   // cancel() never called!
   
   // GOOD
   ctx, cancel := context.WithCancel(...)
   defer cancel() // Always cleanup
   go func() { ... }()
   ```

## 🔍 How to Detect Leaks

### Runtime Detection

```bash
# Check for mcp-simple processes
ps aux | grep mcp-simple

# Sample a process to see what it's doing
sample <pid> 1000 1 > /tmp/sample.txt
```

### Test-Time Detection

```go
func TestGoroutineLeak(t *testing.T) {
    before := runtime.NumGoroutine()
    
    // Your code that starts goroutines
    server := NewServer()
    ctx, cancel := context.WithCancel(context.Background())
    server.StartBackgroundWork(ctx, ...)
    
    // Cleanup
    cancel()
    server.Shutdown()
    time.Sleep(200 * time.Millisecond) // Allow cleanup
    
    after := runtime.NumGoroutine()
    
    if after > before {
        t.Errorf("Goroutine leak: before=%d, after=%d", before, after)
    }
}
```

## 📚 Related Documentation

- [MCP Goroutine Lifecycle Management](./mcp-goroutine-lifecycle-v1.0.md) - Detailed best practices
- [Go Context Package](https://pkg.go.dev/context)
- [Effective Go: Goroutines](https://go.dev/doc/effective_go#goroutines)
