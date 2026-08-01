# Non-Deterministic Goroutine Patterns

This document lists all non-deterministic goroutine usage patterns across production and test code. Non-deterministic patterns include:
- `wg.Wait()` without timeout
- `select` statements without timeout cases
- `time.Sleep()` used for waiting (not retry backoff)
- Channel operations without timeouts
- `go func()` without proper cancellation/timeout mechanisms

## Production Code Violations

### Critical: WaitGroup.Wait() Without Timeout

#### cmd/zqk/system/snapshot_expand.go
- **Line 299**: `wg.Wait()` - Direct wait without timeout in context cancellation handler
- **Line 307**: `wg.Wait()` - Direct wait without timeout after closing jobs channel
  - **Issue**: If workers hang, this will block indefinitely
  - **Fix**: Use deterministic timeout pattern with channel + select

#### cmd/zqk/system/hierarchical_fix_batch.go
- **Line 197**: `wg.Wait()` - Direct wait without timeout
  - **Issue**: Blocks indefinitely if any goroutine hangs
  - **Fix**: Use deterministic timeout pattern

#### pkg/storage/cas_orphan_cleanup_queue.go
- **Line 506**: `q.wg.Wait()` - Direct wait in `Shutdown()` method
  - **Issue**: Shutdown can hang indefinitely
  - **Fix**: Use deterministic timeout pattern
- **Line 526**: `q.wg.Wait()` - Wait in goroutine without timeout in `Drain()`
  - **Status**: Has timeout via `ctx.Done()` in select, but wait goroutine itself has no timeout
  - **Fix**: Ensure wait goroutine has timeout

#### pkg/storage/waitgroup_manager.go
- **Line 151**: `entry.wg.Wait()` - Direct wait without timeout
  - **Issue**: Blocks indefinitely if goroutines hang
  - **Fix**: Use deterministic timeout pattern

#### pkg/storage/hash_registry_manager.go
- **Line 81**: `wg.Wait()` - Wait in goroutine, but no timeout on the wait itself
  - **Status**: Has timeout via `ctx.Done()` in select, but wait goroutine has no timeout
  - **Fix**: Ensure wait goroutine has timeout

#### pkg/storage/id_generation/queue.go
- **Line 385**: `qm.wg.Wait()` - Direct wait in `Stop()` method
  - **Issue**: Stop can hang indefinitely
  - **Fix**: Use deterministic timeout pattern
- **Line 410**: `qm.wg.Wait()` - Wait in goroutine without timeout in `Drain()`
  - **Status**: Has timeout via `ctx.Done()` in select, but wait goroutine itself has no timeout
  - **Fix**: Ensure wait goroutine has timeout

#### pkg/scheduler/handlers.go
- **Line 144**: `wgTier2.Wait()` - Wait in goroutine, has timeout via context
  - **Status**: Has timeout via `tier2Ctx.Done()` in select, but wait goroutine itself has no timeout
  - **Fix**: Ensure wait goroutine has timeout
- **Line 202**: `wgTier3.Wait()` - Wait in goroutine, has timeout via context
  - **Status**: Has timeout via `tier3Ctx.Done()` in select, but wait goroutine itself has no timeout
  - **Fix**: Ensure wait goroutine has timeout

### Critical: time.Sleep() for Waiting (Not Retry Backoff)

#### pkg/scheduler/transceiver/async_router.go
- **Line 452**: `time.Sleep(asyncRouterCheckInterval)` - Polling wait in worker loop
  - **Issue**: Non-deterministic polling interval
  - **Fix**: Use context-based ticker or channel-based signaling

#### pkg/storage/operation_executor.go
- **Line 318**: `time.Sleep(operationExecutorCheckInterval)` - Polling wait in worker loop
  - **Issue**: Non-deterministic polling interval
  - **Fix**: Use context-based ticker or channel-based signaling

#### pkg/storage/cas_index_write_queue.go
- **Line 638**: `time.Sleep(50 * time.Millisecond)` - Wait for queue creation
  - **Issue**: Non-deterministic wait - queue might not exist yet
  - **Fix**: Use deterministic retry with timeout
- **Line 653**: `time.Sleep(100 * time.Millisecond)` - Additional wait for queue creation
  - **Issue**: Non-deterministic wait
  - **Fix**: Use deterministic retry with timeout
- **Line 687**: `time.Sleep(batchTimeout + 200*time.Millisecond)` - Wait for batch processing
  - **Issue**: Non-deterministic wait
  - **Fix**: Use channel-based signaling or deterministic timeout

#### pkg/storage/cas_orphan_cleanup_queue.go
- **Line 302**: `time.Sleep(backoffDelay)` - Retry backoff
  - **Status**: Acceptable - this is retry backoff, not waiting for completion
  - **Note**: Retry backoff is acceptable, but should be bounded

### Acceptable Patterns (With Notes)

#### pkg/storage/object_storage_file.go
- **Line 766**: `time.Sleep(delay)` - Exponential backoff retry
  - **Status**: Acceptable - retry backoff with bounded delay
- **Line 799**: `time.Sleep(delay)` - File system consistency delay
  - **Status**: Acceptable - deterministic delay for file system operations
- **Line 2137**: `time.Sleep(10 * time.Millisecond)` - OS sync delay
  - **Status**: Acceptable - small deterministic delay for OS operations

#### pkg/storage/audit_events.go
- **Line 138**: `time.Sleep(delay)` - Exponential backoff retry
  - **Status**: Acceptable - retry backoff with bounded delay
- **Line 156**: `time.Sleep(100 * time.Millisecond)` - File system consistency delay
  - **Status**: Acceptable - deterministic delay for file system operations

#### pkg/scheduler/handlers_run_wrapper.go
- **Line 348**: `time.Sleep(retryDelay)` - Retry backoff
  - **Status**: Acceptable - retry backoff with bounded delay

## Test Code Violations

Test code has many `wg.Wait()` calls without timeouts. While test code may have different requirements, it's still recommended to use deterministic patterns to prevent test hangs.

### Test Files with Non-Deterministic Patterns

- `cmd/zqk/system/async_check_deadlock_test.go` - Has some timeout patterns, but also direct waits
- `cmd/zqk/system/output_stream_test.go` - Multiple `wg.Wait()` calls followed by `time.Sleep()`
- `cmd/zqk/system/streaming_template_cache_test.go` - Direct `wg.Wait()` calls
- `pkg/validation/concurrent_test.go` - Direct `wg.Wait()` calls (some have timeouts after)
- `pkg/validation/async_validator_deadlock_test.go` - Mix of timeout patterns and direct waits
- `pkg/mcp/server_resilience_test.go` - Direct `wg.Wait()` calls
- `pkg/storage/cas_orphan_cleanup_queue_test.go` - Direct `wg.Wait()` calls followed by `time.Sleep()`
- Many more test files...

**Note**: Test code violations are less critical but should still be addressed to prevent test hangs and improve test reliability.

## Summary Statistics

- **Production Code Violations**: ~15 critical non-deterministic patterns
- **Test Code Violations**: ~50+ non-deterministic patterns
- **Acceptable Patterns**: ~10 patterns (retry backoff, file system delays)

## Recommended Fixes

1. **Replace all `wg.Wait()` with deterministic timeout pattern**:
   ```go
   waitDone := make(chan struct{})
   goroutinelabels.NewGoroutine("wait_group", "waiting for wait group").
       WithCleanup(func() {
           close(waitDone)
       }).
       StartSimple(func() {
           wg.Wait()
       })
   
   select {
   case <-waitDone:
       // All goroutines completed
   case <-time.After(timeout):
       // Timeout - log warning but proceed
   case <-ctx.Done():
       // Context cancelled
   }
   ```

2. **Replace polling `time.Sleep()` with context-based tickers or channel signaling**

3. **Ensure all wait goroutines have timeouts** - even if the select has a timeout, the wait goroutine itself should have a timeout

4. **Use coordinator events for all goroutine lifecycle events** (start, completion, cancellation, timeout)
