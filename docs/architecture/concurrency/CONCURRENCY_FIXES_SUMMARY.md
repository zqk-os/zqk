# Concurrency Fixes Summary

**Last Verified:** 2026-08-31


**Date**: 2026-01-21  
**Status**: In Progress  
**Goal**: Resolve all concurrency-related problems, make things deterministic, eliminate random timeouts

## Completed Fixes

### 1. Critical Lock Timeout Fixes ✅

**Total Fixed**: ~40+ methods across 8 critical files

#### Storage Package (`pkg/storage/`)

Fixed all `context.Background()` usages in critical initialization paths to use bounded timeouts:

#### Storage Package (`pkg/storage/`)
- **`content_addressable_storage.go`**:
  - `IDIndex.ListIDs()` - Added 10s timeout, returns empty slice on timeout
  - `IDIndex.SnapshotMappings()` - Added 10s timeout, returns empty map on timeout
  - `IDIndex.Load()` - Added 10s timeout
  - `IDIndex.GetHash()` - Added 10s timeout
  - `IDIndex.SetMapping()` - Added 10s timeout
  - `IDIndex.RemoveMapping()` - Added 10s timeout
  - `ContentAddressableStorage.setIndexMappingInMemory()` - Added 5s timeout
  - `ContentAddressableStorage.SetOrphanCleanupCallback()` - Added 5s timeout
  - `ContentAddressableStorage.SetPostSyncCallback()` - Added 5s timeout

- **`storage_orchestrator.go`**:
  - `RegisterBackend()` - Added 5s timeout
  - `GetBackend()` - Added 5s timeout
  - `GetDefaultBackend()` - Added 5s timeout
  - `RegisterOperation()` - Added 5s timeout
  - `CompleteOperation()` - Added 5s timeout
  - `HasPendingOperations()` - Added 5s timeout
  - `GetBackendForObject()` - Added 5s timeout

- **`id_generation/queue.go`**:
  - `GetOrCreateQueue()` fast path - Added 5s timeout
  - `GetOrCreateQueue()` slow path - Added 5s timeout

- **`hash_registry.go`**:
  - `Load()` - Added 10s timeout

- **`hash_registry_manager.go`**:
  - `RegisterRegistry()` - Added 5s timeout
  - `GetAllRegistries()` - Added 5s timeout

- **`waitgroup_manager.go`**:
  - `SetObserver()` - Added 5s timeout
  - `CreateGroup()` - Added 5s timeout
  - `GetGroup()` - Added 5s timeout
  - `Add()` - Added 5s timeout
  - `Done()` - Added 5s timeout
  - `Wait()` - Added 5s timeout
  - `DeleteGroup()` - Added 5s timeout
  - `GetGroupInfo()` - Added 5s timeout
  - `ListGroups()` - Added 5s timeout
  - `Count()` - Added 5s timeout

- **`audit_event_buffer.go`**:
  - `SetEnabled()` - Added 5s timeout
  - `IsEnabled()` - Added 5s timeout
  - `SetFileStorage()` - Added 5s timeout (critical for initialization)
  - `SetFlushErrorCallback()` - Added 5s timeout
  - `SetFlushProgressChannel()` - Added 5s timeout
  - `SetSecurityContext()` - Added 5s timeout
  - `AddEvent()` - Added 5s timeout (hot path)
  - `Flush()` - Added 10s timeout (may do I/O)

#### Scheduler Package (`pkg/scheduler/`)
- **`scheduler.go`**:
  - `GetGlobalScheduler()` - Added 5s timeout

### 2. Deterministic Test Synchronization ✅

- **Created `pkg/testing/deterministic_wait.go`**:
  - `WaitForCondition()` - Generic helper for waiting on conditions
  - `WaitForConditionWithTimeout()` - Convenience wrapper with timeout

- **Fixed `cmd/zqk/scheduler/scheduler_test.go`**:
  - Replaced all `time.Sleep()` calls with deterministic polling using `WaitForConditionWithTimeout()`
  - Tests now wait for actual scheduler state changes rather than fixed delays
  - All scheduler start/stop tests now use deterministic synchronization

### 3. End-to-End Integration Test ✅

- **Created `cmd/zqk/scheduler/scheduler_start_integration_test.go`**:
  - Tests actual CLI command `zqk scheduler start`
  - Verifies scheduler starts without panicking
  - Uses proper timeout handling

## Status Update

**✅ COMPLETED**: All `context.Background()` usages in lock operations in `pkg/storage` non-test files have been fixed!

Verification:
```bash
$ grep -rn "context.Background()" pkg/storage --include="*.go" | \
    grep -E "(WithLockTimeout|WithRLockTimeout)" | \
    grep -v "_test.go"
# Result: 0 matches
```

## Remaining Work

### High Priority

1. **Fix remaining `context.Background()` in lock operations in other packages**:
   - `pkg/scheduler/` - Some instances may remain
   - `pkg/mcp/` - Check for lock operations
   - Other packages as needed

2. **Replace `time.Sleep()` in tests** (247 instances remaining):
   - Use `WaitForConditionWithTimeout()` helper
   - Focus on critical test files first:
     - `pkg/scheduler/keepalive_integration_test.go`
     - `pkg/scheduler/job_lock_integration_test.go`
     - `pkg/scheduler/scheduler_submit_integration_test.go`
     - `pkg/storage/*_test.go` files
     - `cmd/zqk/scheduler/scheduler_test.go` (partially done)

3. **Investigate and fix deadlock patterns**:
   - Review lock ordering in storage initialization
   - Check for circular lock dependencies
   - Verify all locks are released before I/O operations

### Medium Priority

4. **Review race conditions**:
   - Run race detector: `go test -race ./...`
   - Fix any detected races
   - Add race condition tests where appropriate

5. **Document lock timeout patterns**:
   - Create guidelines for choosing timeout durations
   - Document when to use 5s vs 10s timeouts
   - Add comments explaining timeout choices

## Timeout Guidelines

- **5 seconds**: For quick operations (lock acquisition, simple lookups)
- **10 seconds**: For operations that may do I/O (loading indexes, file operations)
- **Context-based**: When a context is already available, use it with timeout

## Testing Strategy

1. **Unit tests**: Use `WaitForConditionWithTimeout()` instead of `time.Sleep()`
2. **Integration tests**: Use deterministic synchronization channels
3. **Race detection**: Run `go test -race ./...` regularly
4. **Deadlock detection**: Monitor for goroutine dumps and stuck locks

## Next Steps

1. Continue fixing `context.Background()` in critical paths
2. Systematically replace `time.Sleep()` in tests
3. Run full test suite with race detector
4. Document any remaining patterns that need fixing
