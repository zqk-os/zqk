# Storage Orchestration for Multi-Backend Support

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2026-01-01  
**Status**: Active  
**Purpose**: Coordinate operations across multiple storage backends and prevent undesirable actions on objects with pending updates

## Overview

The storage orchestrator coordinates operations across multiple storage backends (file, graph, neo4j, AllegroGraph, JSON-LD, etc.) and tracks pending operations to prevent undesirable actions on objects that have pending updates.

## Key Principles

### 1. Multi-Backend Coordination
- Objects may be managed by multiple backends simultaneously
- Operations must be coordinated across all backends
- Hash updates occur only after all backends complete their operations

### 2. Operation Prevention
- Operations are prevented on objects with pending updates
- Prevents race conditions and data corruption
- Ensures consistency across all backends

### 3. Backend Abstraction
- Unified interface for all storage backends
- Backend-specific logic encapsulated
- Easy to add new backends

## Architecture

### StorageOrchestrator

The `StorageOrchestrator` coordinates operations across backends:

```go
type StorageOrchestrator struct {
    backends    map[string]ObjectStorageProvider
    pendingOps  map[string]*PendingStorageOps
    mu          sync.RWMutex
    logger      *logging.EventLogger
    hashManager *DeferredHashManager
}
```

### Backend Registration

Backends are registered with the orchestrator:

```go
orchestrator.RegisterBackend("file", fileStorage)
orchestrator.RegisterBackend("graph", graphStorage)
orchestrator.RegisterBackend("neo4j", neo4jStorage)
orchestrator.RegisterBackend("allegro", allegroGraphStorage)
```

### Operation Lifecycle

1. **Register Operation**: Before any operation
   ```go
   orchestrator.RegisterOperation(backendName, objectID, operationID)
   ```

2. **Check Pending**: Before performing operation
   ```go
   if err := orchestrator.PreventOperation(objectID, "update"); err != nil {
       return err // Operation prevented
   }
   ```

3. **Execute Operation**: Perform the actual storage operation

4. **Complete Operation**: After operation succeeds
   ```go
   orchestrator.CompleteOperation(backendName, objectID, operationID)
   ```

5. **Hash Update**: When all backends complete (if file storage)
   - Hash update triggered automatically
   - Object marked as "clean"

## Integration Points

### 1. Enhanced Operation Executor

Operations register with orchestrator:

```go
// Get orchestrator
orchestrator := GetStorageOrchestrator()

// Get backend for object
backends := orchestrator.GetBackendForObject(objectID)

// Register operation
for _, backendName := range backends {
    orchestrator.RegisterOperation(backendName, objectID, operationID)
}

// Check if operation should be prevented
if err := orchestrator.PreventOperation(objectID, "update"); err != nil {
    return err
}

// Execute operation
err := storage.Update(ctx, secCtx, objectID, updates)

// Complete operation
for _, backendName := range backends {
    orchestrator.CompleteOperation(backendName, objectID, operationID)
}
```

### 2. Storage Backend Interface

All backends implement `ObjectStorageProvider`:

```go
type ObjectStorageProvider interface {
    Create(ctx context.Context, secCtx *pkgctx.SecurityContext, data map[string]any) error
    Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error)
    Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error
    Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error
    List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error)
}
```

### 3. Backend-Specific Logic

Each backend can implement backend-specific behavior:

```go
// File storage: Uses deferred hash manager
if fileStorage, ok := backend.(*FileObjectStorage); ok {
    // Hash updates deferred until all operations complete
}

// Graph storage: May have different consistency requirements
if graphStorage, ok := backend.(*GraphObjectStorage); ok {
    // Graph-specific operation coordination
}
```

## Benefits

### 1. Consistency
- Operations coordinated across all backends
- No race conditions between backends
- Hash updates reflect final state across all backends

### 2. Safety
- Operations prevented on objects with pending updates
- Prevents data corruption
- Ensures data integrity

### 3. Flexibility
- Easy to add new backends
- Backend-specific logic encapsulated
- Unified interface for all backends

### 4. Performance
- Operations can proceed in parallel across backends
- Hash updates batched when possible
- Efficient operation tracking

## Usage Example

```go
// Create orchestrator
orchestrator := GetStorageOrchestrator()

// Register backends
fileStorage, _ := NewFileObjectStorage(projectRoot)
graphStorage, _ := NewGraphObjectStorage(connectionString)
orchestrator.RegisterBackend("file", fileStorage)
orchestrator.RegisterBackend("graph", graphStorage)

// Get default backend
backend, _ := orchestrator.GetDefaultBackend()

// Register operation
objectID := "GOAL-001"
operationID := "update-001"
orchestrator.RegisterOperation("file", objectID, operationID)
orchestrator.RegisterOperation("graph", objectID, operationID)

// Check if operation should be prevented
if err := orchestrator.PreventOperation(objectID, "update"); err != nil {
    return err // Operation prevented
}

// Execute operation on all backends
for backendName, backend := range orchestrator.backends {
    err := backend.Update(ctx, secCtx, objectID, updates)
    if err != nil {
        // Handle error
    }
}

// Complete operations
orchestrator.CompleteOperation("file", objectID, operationID)
orchestrator.CompleteOperation("graph", objectID, operationID)
```

## Implementation Details

### Pending Operation Tracking

Operations are tracked per object and per backend:

```go
type PendingStorageOps struct {
    ObjectID   string
    BackendOps map[string][]string // backend -> operation IDs
    LastUpdate time.Time
    mu         sync.RWMutex
}
```

### Operation Prevention

Operations are prevented if any backend has pending operations:

```go
func (so *StorageOrchestrator) PreventOperation(objectID, operationType string) error {
    if so.HasPendingOperations(objectID) {
        pending := so.GetPendingOperations(objectID)
        return fmt.Errorf("cannot perform %s: pending operations: %v", operationType, pending)
    }
    return nil
}
```

### Hash Update Coordination

Hash updates occur only after all backends complete:

```go
func (so *StorageOrchestrator) CompleteOperation(backendName, objectID, operationID string) error {
    // Remove operation from backend's list
    // Check if all backends have completed
    if allComplete {
        // Trigger hash update (if file storage)
        return so.updateHashIfReady(ctx, objectID)
    }
    return nil
}
```

## Next Steps

1. **Backend Implementation**: Implement graph, neo4j, AllegroGraph backends
2. **Testing**: Test orchestration with multiple backends
3. **Performance**: Optimize operation tracking and coordination
4. **Monitoring**: Add metrics for orchestration performance

