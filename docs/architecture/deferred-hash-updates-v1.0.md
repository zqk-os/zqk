# Deferred Integrity Hash Updates

**Version**: 1.0.0  
**Created**: 2026-01-01  
**Status**: Active  
**Purpose**: Ensure integrity hashes are the final update for any system object, computed only after all pending operations are complete

## Overview

Integrity hashes are now computed and updated **only after all pending operations on an object are complete**. This ensures:
- Hashes reflect the final state of objects
- No hash updates occur while operations are still in progress
- Hash registry and ID cache are updated atomically
- System objects are marked "clean" only when truly ready

## Key Principles

### 1. Hash Updates Are Final
- Integrity hashes are the **last** update for any system object
- Hashes are computed only when we're certain no pending operations will affect the object
- This ensures hash integrity and prevents hash mismatches

### 2. Operation Tracking
- All operations that modify objects are tracked
- Operations register with the deferred hash manager before execution
- Operations complete registration after execution
- Hash update occurs only when operation count reaches zero

### 3. Atomic Updates
- Hash registry and ID cache are updated together
- Both updates succeed or both fail (best effort for ID cache)
- Object is marked "clean" only after both updates complete

## Architecture

### DeferredHashManager

The `DeferredHashManager` tracks pending operations and triggers hash updates:

```go
type DeferredHashManager struct {
    storage         ObjectStorageProvider
    pendingOps      map[string]*PendingObjectOps
    mu              sync.RWMutex
    logger          *logging.EventLogger
    updateInterval  time.Duration
    cleanupInterval time.Duration
}
```

### Operation Lifecycle

1. **Register Operation**: Before any create/update/delete operation
   ```go
   hashManager.RegisterOperation(objectID, objectKind, filePath, operationID)
   ```

2. **Execute Operation**: Perform the actual storage operation

3. **Complete Operation**: After operation succeeds
   ```go
   hashManager.CompleteOperation(objectID, operationID)
   ```

4. **Hash Update**: When last pending operation completes
   - Compute hash from file content
   - Update hash registry
   - Update ID cache
   - Mark object as "clean"

## Integration Points

### 1. Enhanced Operation Executor

Operations register and complete with the deferred hash manager:

```go
// Before operation
hashManager.RegisterOperation(op.ObjectID, op.ObjectKind, filePath, operationID)

// Execute operation
if err := e.storage.Create(ctx, op.SecCtx, op.Data); err != nil {
    hashManager.CompleteOperation(op.ObjectID, operationID) // Remove on failure
    return err
}

// After operation
hashManager.CompleteOperation(op.ObjectID, operationID) // Triggers hash update if last
```

### 2. Storage Operations

Storage operations (Create/Update/Delete) skip immediate hash updates:

```go
// Hash update is now deferred until all pending operations complete
if hashManager := GetDeferredHashManager(f); hashManager != nil {
    // Hash will be updated by deferred hash manager after all operations complete
} else {
    // Fallback: update hash immediately if deferred manager not available
}
```

### 3. Background Processor

Background goroutine periodically checks for ready objects:

```go
func (dhm *DeferredHashManager) StartBackgroundProcessor(ctx context.Context) {
    // Check every 5 seconds for objects ready for hash update
    // Cleanup old operations every 30 seconds
}
```

## Benefits

### 1. Integrity Guarantee
- Hashes always reflect final object state
- No hash mismatches from concurrent operations
- Hash registry is always consistent

### 2. Performance
- Hash computation happens once per object (not per operation)
- Reduces I/O for hash registry updates
- Batch processing of ready objects

### 3. Consistency
- Hash and ID cache updated together
- Object marked "clean" only when truly ready
- System state is always accurate

### 4. Reliability
- Operations can complete even if hash update fails (best effort)
- Background processor handles stale operations
- Automatic cleanup of old pending operations

## Usage Example

```go
// Create deferred hash manager
hashManager := GetDeferredHashManager(storage)

// Start background processor
ctx := context.Background()
hashManager.StartBackgroundProcessor(ctx)

// Register operation before create
operationID := fmt.Sprintf("create-%s-%d", objectID, time.Now().UnixNano())
hashManager.RegisterOperation(objectID, objectKind, filePath, operationID)

// Execute create operation
err := storage.Create(ctx, secCtx, data)
if err != nil {
    hashManager.CompleteOperation(objectID, operationID) // Remove on failure
    return err
}

// Complete operation (triggers hash update if last pending)
hashManager.CompleteOperation(objectID, operationID)
```

## Implementation Details

### Hash Update Process

1. **Check Pending Operations**: Verify no pending operations for object
2. **Read File Content**: Load file from disk
3. **Calculate Hash**: Compute SHA256 hash of content
4. **Update Hash Registry**: Save hash to registry with retry
5. **Update ID Cache**: Update object ID cache (best effort)
6. **Mark Clean**: Object is now marked as "clean"

### Error Handling

- **Hash Update Failure**: Logs warning but doesn't fail operation
- **ID Cache Failure**: Logs warning but doesn't fail operation
- **Stale Operations**: Background processor forces hash update after 1 hour

### Background Processing

- **Update Interval**: 5 seconds (check for ready objects)
- **Cleanup Interval**: 30 seconds (remove old operations)
- **Stale Threshold**: 1 hour (force hash update for old operations)

## Next Steps

1. **Testing**: Test deferred hash updates with concurrent operations
2. **Monitoring**: Add metrics for hash update performance
3. **Optimization**: Batch hash updates for multiple objects
4. **Documentation**: Add usage examples and best practices

