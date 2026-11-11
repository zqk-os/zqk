# Test Concurrency & Timing Invariants Guide

## 1. Executive Architectural Summary

In multi-core CI runners and high-concurrency swarms, asynchronous pipeline tests and connection pool tests often suffer from timing variance flakes when relying on wall-clock latency assumptions. Under heavy CPU load or virtualization preemption, a goroutine may experience arbitrary scheduling latency between context creation and operation execution, or take longer than microsecond/millisecond budgets.

This guide codifies the core design patterns and timing invariants required across the ZQK Knowledge Kernel test suite under **REQ-001** (addressing `TDE-001` and `TDE-001`).

---

## 2. Core Invariants & Anti-Patterns

### Anti-Pattern 1: Tight Wall-Clock Sleep or Window Assertions
```go
// ❌ FLAKY: Fails when CI runner CPU contention deschedules goroutine or fires deadline early
start := time.Now()
_, err := pool.GetConnection(ctx)
elapsed := time.Since(start)
if elapsed < 5*time.Millisecond || elapsed > 50*time.Millisecond {
    t.Errorf("Expected timeout around 10ms, got %v", elapsed)
}
```

#### Why it fails:
1. If the runner preempts between `context.WithTimeout(ctx, 10*time.Millisecond)` and `start := time.Now()`, the context deadline may expire before or immediately after `GetConnection` starts, causing `time.Since(start)` to measure tens of microseconds (e.g., `10.51µs`).
2. If the runner is starved during execution, `time.Since(start)` can exceed 50ms without any pool deadlock or bug.

#### Invariant Pattern:
```go
// ✅ RESILIENT: Assert context cancellation and ceiling bound without tight microsecond bounds
_, err := pool.GetConnection(ctx)
elapsed := time.Since(start)

if !errors.Is(err, context.DeadlineExceeded) {
    t.Errorf("Expected context.DeadlineExceeded, got %v", err)
}
if ctx.Err() != context.DeadlineExceeded {
    t.Errorf("Expected ctx.Err() == context.DeadlineExceeded, got %v", ctx.Err())
}
// Sanity bound to prevent test suite hangs without asserting microsecond precision
if elapsed < 0 || elapsed > 5*time.Second {
    t.Errorf("Elapsed time out of reasonable bounds: %v", elapsed)
}
```

---

### Anti-Pattern 2: Arbitrary `time.Sleep` for Asynchronous Processing
```go
// ❌ FLAKY: Arbitrary sleep assumes background workers finish within 100ms
testValidator.Enqueue(task)
time.Sleep(100 * time.Millisecond)
total, _, _, queueSize := testValidator.GetValidationStats()
```

#### Why it fails:
1. Under heavy concurrent load (e.g. `go test -race ./...`), 100ms is frequently insufficient for worker goroutines to wake up, acquire locks, read fixtures, and complete processing.
2. If tasks fail silently or are still in flight, stats return 0, leading to silent test bypasses or flaky assertions.

#### Invariant Pattern:
```go
// ✅ RESILIENT: Synchronize on deterministic completion callbacks or channels
taskDone := make(chan struct{}, numTasks)
testValidator.SetResultCallback(func(state *validation.ValidationState, err error) {
    taskDone <- struct{}{}
})

for i := 0; i < numTasks; i++ {
    testValidator.Enqueue(...)
}

timeout := time.After(5 * time.Second)
for i := 0; i < numTasks; i++ {
    select {
    case <-taskDone:
    case <-timeout:
        t.Fatalf("timed out waiting for task %d to complete", i)
    }
}
```

---

### Anti-Pattern 3: Shared Pointer Mutation in Queues & Goroutines
```go
// ❌ RACE CONDITION: Enqueueing shared pointer causes data race on retry
taskCopy := task // Copies pointer only (*ValidationTask)
taskCopy.RetryCount++
av.priorityQueue.Enqueue(taskCopy) // Other worker dequeues and reads while this goroutine logs
```

#### Invariant Pattern:
```go
// ✅ SAFE: Clone task struct by value for each worker and retry
taskCopy := *task // Deep/shallow struct value copy
retryCount := taskCopy.RetryCount + 1
retryTask := taskCopy
retryTask.RetryCount = retryCount
av.priorityQueue.Enqueue(&retryTask)
av.notifyWorkers()
logging.Fluent(av.logger).Debug("Retrying validation").Int("retry_count", retryCount).Log()
```

---

## 3. Verification & CI Safeguards

When authoring or modifying concurrency or timing-sensitive code:
1. Always run tests under Go race detector with high repetition:
   ```bash
   go test -v -race -count=5 <target_package>
   ```
2. Verify that progress and error channels are subscribed **prior** to enqueuing operations.
3. Use `testkit.WriteTestObjectStandalone(t, projectRoot, content)` so file read fixtures exist deterministically on disk.
