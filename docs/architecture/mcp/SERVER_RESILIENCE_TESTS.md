# Server Resilience Tests

## Overview

Comprehensive unit tests that stress-test the MCP server to ensure it's "indestructible" against unintended shutdown requests and various error conditions.

## Test Categories

### 1. Server Loop Resilience (`TestServerLoopResilience`)

Tests that the server loop continues operating despite various error conditions:

- **`handles_concurrent_shutdown_requests`**: Verifies shutdown is idempotent when called from multiple goroutines
- **`handles_panic_in_read_goroutine`**: Ensures server state remains consistent after panics
- **`handles_write_errors_gracefully`**: Server continues operating after write failures
- **`handles_concurrent_message_sends`**: Server handles 1000+ concurrent message sends
- **`handles_malformed_requests`**: Server gracefully handles malformed JSON requests
- **`handles_rapid_initialize_requests`**: Server handles 50 concurrent initialize requests

### 2. Message Queue Resilience (`TestMessageQueueResilience`)

Tests the message queue under heavy load and error conditions:

- **`handles_queue_overflow`**: Queue properly drops messages when full (1000 messages, queue size 10)
- **`handles_slow_writer`**: Queue handles slow writers without blocking (200 messages, 50ms delay)
- **`handles_writer_failures`**: Queue marks itself inactive after fatal write errors
- **`handles_concurrent_enqueue`**: Queue handles 1000 concurrent enqueue operations (100 goroutines × 10 messages)
- **`handles_priority_messages`**: Queue handles priority-based message ordering

### 3. Shutdown Resilience (`TestShutdownResilience`)

Tests that shutdown is properly handled:

- **`shutdown_is_idempotent`**: Shutdown can be called multiple times safely
- **`shutdown_cleans_up_queues`**: Shutdown flushes remaining messages in queues
- **`shutdown_handles_nil_resources`**: Shutdown handles nil resources gracefully

### 4. Concurrent Operations (`TestConcurrentOperations`)

Tests that concurrent operations don't cause race conditions:

- **`concurrent_tool_registration`**: 100 concurrent tool registrations
- **`concurrent_message_sends_with_queue`**: 500 concurrent message sends with queue
- **`concurrent_resource_registration`**: 100 concurrent resource registrations

### 5. Error Recovery (`TestErrorRecovery`)

Tests that the server recovers from errors:

- **`recovers_from_marshal_errors`**: Server continues after JSON marshal failures
- **`recovers_from_transport_errors`**: Server continues after transport write failures

### 6. Stress Test (`TestServerLoopStressTest`)

Comprehensive stress test that runs for 5 seconds:

- 10 goroutines spamming message sends
- 5 goroutines spamming shutdown requests (should be ignored after first)
- Verifies server continues operating throughout
- Tracks operations, errors, and queue statistics

## Key Test Patterns

### 1. Concurrent Shutdown Protection

```go
// Spam shutdown requests from multiple goroutines
for i := 0; i < 100; i++ {
    go func() {
        server.RequestShutdown(fmt.Sprintf("shutdown %d", i))
    }()
}
// Verify shutdown is idempotent
```

### 2. Queue Overflow Testing

```go
// Flood queue with messages
for i := 0; i < 1000; i++ {
    queue.Enqueue(data, format, "normal")
}
// Verify some messages are dropped
stats := queue.Stats()
assert stats.Dropped > 0
```

### 3. Error Injection

```go
// Create failing writer
failingWriter := &failingWriter{}
// Verify server handles errors gracefully
err := server.SendMessageToClient("test", "test", "normal")
// Server should still be operational
assert !server.shutdownRequested
```

### 4. Concurrent Operations

```go
// Run operations concurrently
var wg sync.WaitGroup
for i := 0; i < 100; i++ {
    wg.Add(1)
    go func(id int) {
        defer wg.Done()
        server.RegisterTool(...)
    }(i)
}
wg.Wait()
// Verify all operations completed
```

## Test Helpers

### Helper Types

- **`panicReader`**: Reader that panics on read (tests panic recovery)
- **`panicTransport`**: Transport that panics (tests panic recovery)
- **`failingWriter`**: Writer that always fails (tests error handling)
- **`slowWriter`**: Writer with configurable delay (tests backpressure)

## Running Tests

### Run All Resilience Tests

```bash
go test -v ./pkg/mcp -run "TestServerLoopResilience|TestMessageQueueResilience|TestShutdownResilience|TestConcurrentOperations|TestErrorRecovery"
```

### Run Specific Test

```bash
go test -v ./pkg/mcp -run TestServerLoopResilience/handles_concurrent_shutdown_requests
```

### Run Stress Test (5 seconds)

```bash
go test -v ./pkg/mcp -run TestServerLoopStressTest
```

### Skip Stress Test in Short Mode

The stress test automatically skips in short mode:

```bash
go test -short ./pkg/mcp  # Stress test skipped
```

## Expected Behavior

### Server Should:

✅ Continue operating after write errors
✅ Handle concurrent shutdown requests idempotently
✅ Recover from panics in goroutines
✅ Handle malformed requests gracefully
✅ Process messages concurrently without races
✅ Drop messages when queue is full (backpressure)
✅ Flush remaining messages on shutdown
✅ Handle nil resources gracefully

### Server Should NOT:

❌ Shutdown due to write errors
❌ Shutdown due to malformed requests
❌ Shutdown due to concurrent operations
❌ Crash due to panics
❌ Block on slow clients
❌ Leak goroutines
❌ Corrupt state under concurrent access

## Coverage

These tests provide comprehensive coverage for:

- **Concurrency**: 100+ concurrent operations
- **Error Handling**: Write errors, marshal errors, transport errors
- **Backpressure**: Queue overflow, slow writers
- **Shutdown**: Idempotency, cleanup, nil resources
- **Resilience**: Panic recovery, error recovery
- **Stress**: 5-second stress test with multiple goroutines

## Future Enhancements

Potential additions:

1. **Network Transport Tests**: Test with actual network connections
2. **Memory Leak Detection**: Long-running tests with memory profiling
3. **Performance Benchmarks**: Measure throughput under load
4. **Race Condition Detection**: Use `go test -race`
5. **Timeout Tests**: Test various timeout scenarios
6. **Resource Exhaustion**: Test behavior when resources are exhausted
