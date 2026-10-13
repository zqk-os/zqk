# Test Suite Concurrency Hygiene and Goroutine Supervision

## Executive Summary

As a distributed daemon and micro-kernel platform, ZQK demands rigorous concurrency invariants not only in core production runtime paths, but equally across its unit, integration, and stress test suites. Unmanaged goroutines (`go func()`) in test suites introduce insidious failure modes:
1. **Unobservable Panics and Silenced Crashes:** Raw goroutines that panic can tear down the entire test binary without meaningful telemetry or clean stack frames attributed to test sub-contexts.
2. **Goroutine Leakage and Resource Creep:** Unmanaged background goroutines often outlive individual test functions, polluting subsequent test executions, triggering data races on shared memory, or pinning network sockets.
3. **Deadlocks on Runner Teardown:** Tests waiting indefinitely on unmonitored channels without lifecycle context cancellation cause CI hangs and test timeouts.

This specification codifies the supervised goroutine lifecycle invariant, mandating the adoption of `pkg/goroutinelabels` across all test suites, paired with deterministic synchronization (`sync.WaitGroup`), explicit timeout contexts, and bounded shutdown.

---

## Core Invariants

### 1. The Supervised Goroutine Invariant (Zero Raw Goroutines)
Every background task initiated within a test or helper function MUST use `goroutinelabels.NewGoroutine(name, purpose)`:
- **`name`**: Follows standard dot-delimited hierarchical naming: `test.<subsystem>.<component>` (e.g., `test.mcp.concurrent_reader`, `test.ideadapter.session_heartbeat`).
- **`purpose`**: A concise, human-legible sentence describing the operational intent of the routine (e.g., `"concurrent transport read loop"`, `"accept single dial from proxy"`).

```go
// CORRECT: Named, supervised goroutine with automatic panic recovery
goroutinelabels.NewGoroutine("test.mcp.proxy_start", "run proxy daemon in test").
    StartSimple(func() {
        _ = proxy.Start(ctx)
    })

// INCORRECT: Raw, unmonitored goroutine
go proxy.Start(ctx)
```

### 2. Deterministic Synchronization Invariant
Test goroutines that execute concurrent work MUST be tracked deterministically. When using `sync.WaitGroup`, the builder method `.WithWaitGroup(&wg)` MUST be used:
- The builder automatically increments the waitgroup (`wg.Add(1)`) prior to goroutine spawn and ensures decrement (`wg.Done()`) in a deferred safety wrapper even in the presence of panics.
- Test authors must **never** manually call `wg.Add(1)` or `defer wg.Done()` when using `.WithWaitGroup(&wg)`.

```go
var wg sync.WaitGroup
for i := 0; i < concurrencyLevel; i++ {
    goroutinelabels.NewGoroutine("test.mcp.concurrent_reader", "concurrent transport read loop").
        WithWaitGroup(&wg).
        StartSimple(func() {
            // Test execution payload
        })
}
wg.Wait()
```

### 3. Fail-Closed Panic Containment
`goroutinelabels` wraps all execution in structured `recover()` handlers. Any uncaught panic is logged to standard error with complete stack trace attribution, preventing unhandled runtime faults from corrupting adjacent test suites.

### 4. Lifecycle Context Boundedness
All mock listeners, echo servers, and background dispatchers started within tests MUST respect a cancellation context (`context.WithCancel` or `context.WithTimeout`). Goroutines must immediately release underlying OS resources (sockets, pipes, file descriptors) upon context expiration to ensure zero socket leakage between test cases.

---

## Architectural Pattern Catalog

### Pattern A: Short-Lived Concurrent Workers with WaitGroup
Used for testing race conditions, message throughput, and multi-tenant reader/writer locks:

```go
var wg sync.WaitGroup
for i := 0; i < workerCount; i++ {
    goroutinelabels.NewGoroutine("test.module.worker", "execute concurrent stress iteration").
        WithWaitGroup(&wg).
        StartSimple(func() {
            for j := 0; j < iterations; j++ {
                // Perform thread-safe operation
            }
        })
}
wg.Wait()
```

### Pattern B: Mock Network Listener with Lifecycle Context
Used for testing client reconnect, proxy forwarders, and protocol handshakes:

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

goroutinelabels.NewGoroutine("test.network.mock_listener", "accept mock connections").
    StartSimple(func() {
        for {
            conn, err := listener.Accept()
            if err != nil {
                return // Listener closed by test teardown
            }
            c := conn
            goroutinelabels.NewGoroutine("test.network.conn_worker", "process incoming request").
                StartSimple(func() {
                    defer c.Close()
                    // Serve request
                })
        }
    })
```

### Pattern C: Background Daemon Service with Channel Handshake
Used for testing long-running processes that run asynchronously until cancellation:

```go
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()

done := make(chan error, 1)
goroutinelabels.NewGoroutine("test.service.runner", "run background daemon service").
    StartSimple(func() {
        done <- service.Start(ctx)
    })

// Assert service behavior...

cancel()
select {
case err := <-done:
    // Verify graceful shutdown
case <-time.After(1 * time.Second):
    t.Fatal("service did not shut down within timeout")
}
```

---

## Verification & Race Freedom

All test suites refactored under this specification are verified continuously under Go's race detector:
```bash
go test -v -race ./pkg/mcp/... ./pkg/goroutinelabels/...
```

Adherence ensures zero race conditions, zero orphaned goroutines, and transparent profiling under Go diagnostic tooling.
