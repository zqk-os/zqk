# Cross-Process File Locking Pattern

**Version**: 1.0.0  
**Created**: 2026-01-02  
**Status**: Active  
**Purpose**: Ensure atomic operations on shared file-based resources across multiple processes

## Overview

When a resource is shared across processes (e.g., files, queues, registries), we need to ensure atomic operations to prevent:
- **Race conditions**: Multiple processes modifying the same resource simultaneously
- **Data corruption**: Partial writes overwriting each other
- **Lost updates**: One process's changes being overwritten by another

## Design Principles

1. **Flexible**: Works for any shared file-based resource
2. **Reliable**: Prevents race conditions and data corruption
3. **Observable**: Logs lock acquisition/release, timeouts, and failures
4. **Efficient**: Non-blocking where possible, timeouts to prevent indefinite blocking
5. **Concise**: Simple API that's easy to use correctly
6. **Consistent**: Same pattern across all shared resources

## Implementation: `FileLock`

Located in `pkg/storage/file_lock.go`, this provides cross-process file locking using `flock()`.

### Features

- **Exclusive locks**: Only one process can hold the lock at a time
- **Non-blocking option**: `TryLock()` returns immediately if lock is held
- **Timeout support**: `LockWithTimeout()` prevents indefinite blocking
- **Automatic cleanup**: Lock is released when process exits (OS-level)
- **Convenience methods**: `WithLock()` and `WithLockTimeout()` for easy usage

### Usage Pattern

```go
// Create lock
fileLock, err := storagepkg.NewFileLock(lockFilePath)
if err != nil {
    return err
}
defer fileLock.Close()

// Acquire lock (with timeout)
if err := fileLock.LockWithTimeout(5 * time.Second); err != nil {
    return fmt.Errorf("failed to acquire lock: %w", err)
}
defer fileLock.Unlock()

// Perform atomic operation
// ... read/modify/write ...
```

### Or use convenience method:

```go
fileLock, err := storagepkg.NewFileLock(lockFilePath)
if err != nil {
    return err
}
defer fileLock.Close()

err = fileLock.WithLockTimeout(5*time.Second, func() error {
    // Perform atomic operation
    // ... read/modify/write ...
    return nil
})
```

## Current Applications

### 1. Job Trigger Queue (`pkg/scheduler/job_trigger_queue.go`)

**Problem**: Multiple processes could enqueue trigger requests simultaneously, causing:
- Lost requests (one process overwrites another's write)
- Corrupted JSON (partial writes)
- Race conditions

**Solution**: Use `FileLock` for both `EnqueueTriggerRequest()` and `DequeueTriggerRequests()`:
- **Enqueue**: Acquires lock, reads queue, appends request, writes queue, releases lock
- **Dequeue**: Acquires lock (non-blocking), reads queue, clears queue, releases lock

**Benefits**:
- Atomic read-modify-write operations
- No lost requests
- No corrupted JSON
- Observable (logs queue length, lock acquisition)

### 2. Hash Registry (`pkg/storage/hash_registry.go`)

**Problem**: Multiple processes updating hash registry simultaneously could corrupt it.

**Solution**: Uses `syscall.Flock()` directly (could be refactored to use `FileLock`).

### 3. Object ID Cache (`pkg/validation/state_cache.go`)

**Problem**: Multiple processes building cache simultaneously could corrupt cache file.

**Solution**: Uses `syscall.Flock()` with stale lock detection (could be refactored to use `FileLock`).

## Future Applications

### 1. PID File Updates

Currently, PID file writes are not locked. If multiple processes try to start the scheduler simultaneously, they could both write their PID.

**Solution**: Use `FileLock` when writing/reading PID file.

### 2. Scheduler Job Execution Locks

Prevent the same job from running in multiple processes simultaneously.

**Solution**: Use `FileLock` per job ID (`.zqk/scheduler/locks/{jobID}.lock`).

### 3. Change Journal Aggregation

If multiple scheduler instances run, they could both aggregate the same entries.

**Solution**: Use `FileLock` for aggregation operations.

### 4. Audit Event Aggregation

Similar to change journal aggregation.

**Solution**: Use `FileLock` for aggregation operations.

## Lock File Naming Convention

- **Lock file**: `{resource_file}.lock`
- **Example**: `queue.json.lock`, `hash_registry.json.lock`, `cache.json.lock`

## Timeout Guidelines

- **Critical operations** (enqueue): 5 seconds
- **Non-critical operations** (dequeue): Non-blocking (`TryLock()`)
- **Long-running operations** (aggregation): 30 seconds
- **Cache operations**: 2 seconds (read), 5 seconds (write)

## Observability

All lock operations should log:
- Lock acquisition (with timeout)
- Lock release
- Lock failures (timeout, already held)
- Queue length after operations

## Error Handling

- **Lock timeout**: Return error, don't retry automatically (caller decides)
- **Lock already held**: For non-blocking operations, return empty result
- **Lock file errors**: Return error immediately

## Testing

Tests should verify:
- Concurrent access from multiple goroutines
- Cross-process locking (separate processes)
- Timeout behavior
- Stale lock cleanup (if process dies while holding lock)

## Migration Path

1. ✅ **Job Trigger Queue**: Migrated to `FileLock`
2. **Hash Registry**: Refactor to use `FileLock` (currently uses `syscall.Flock()` directly)
3. **Object ID Cache**: Refactor to use `FileLock` (currently uses `syscall.Flock()` directly)
4. **PID File**: Add `FileLock` for writes
5. **Job Execution**: Add `FileLock` per job ID

## Conclusion

The `FileLock` utility provides a **flexible, reliable, observable, efficient, concise, and consistent** solution for cross-process resource coordination. It should be used for all shared file-based resources to prevent race conditions and data corruption.

