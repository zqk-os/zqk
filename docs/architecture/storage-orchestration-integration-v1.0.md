# Storage Orchestration Integration Guide

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2026-01-01  
**Status**: Active

## Overview

The storage orchestrator coordinates operations across multiple storage backends (file, graph, neo4j, AllegroGraph, JSON-LD, etc.) and prevents undesirable actions on objects with pending updates.

## Integration Steps

### 1. Initialize Storage Orchestrator

```go
// Get global orchestrator
orchestrator := GetStorageOrchestrator()

// Register backends
fileStorage, _ := NewFileObjectStorage(projectRoot)
graphStorage, _ := NewGraphObjectStorage(connectionString)
orchestrator.RegisterBackend("file", fileStorage)
orchestrator.RegisterBackend("graph", graphStorage)
```

### 2. Integrate with Enhanced Operation Executor

```go
// In executeCreateWithCache, executeUpdateWithCache, executeDeleteWithCache:

// Get orchestrator
orchestrator := GetStorageOrchestrator()

// Get backends for object
backends := orchestrator.GetBackendForObject(op.ObjectID)

// Register operation with all backends
operationID := fmt.Sprintf("%s-%s-%d", op.Type, op.ObjectID, time.Now().UnixNano())
for _, backendName := range backends {
    orchestrator.RegisterOperation(backendName, op.ObjectID, operationID)
}

// Check if operation should be prevented
if err := orchestrator.PreventOperation(op.ObjectID, string(op.Type)); err != nil {
    return err // Operation prevented
}

// Execute operation
err := e.storage.Create(ctx, op.SecCtx, op.Data)

// Complete operation with all backends
for _, backendName := range backends {
    orchestrator.CompleteOperation(backendName, op.ObjectID, operationID)
}
```

### 3. Backend-Specific Integration

#### File Storage
- Uses deferred hash manager
- Hash updates deferred until all operations complete
- ID cache updates coordinated

#### Graph Storage (Neo4j, AllegroGraph)
- Operations tracked in orchestrator
- Consistency maintained across graph nodes
- Transaction coordination

#### JSON-LD Storage
- Operations tracked in orchestrator
- RDF graph consistency
- Linked data integrity

## Operation Prevention

The orchestrator prevents operations on objects with pending updates:

```go
// Before any operation
if err := orchestrator.PreventOperation(objectID, "update"); err != nil {
    return fmt.Errorf("operation prevented: %w", err)
}
```

**Prevention Rules**:
- Update operations prevented if any backend has pending operations
- Delete operations prevented if any backend has pending operations
- Read operations allowed (no prevention needed)

## Multi-Backend Coordination

### Operation Tracking

Operations are tracked per object and per backend:

```go
// Register operation
orchestrator.RegisterOperation("file", objectID, "op-001")
orchestrator.RegisterOperation("graph", objectID, "op-001")

// Complete operation
orchestrator.CompleteOperation("file", objectID, "op-001")
orchestrator.CompleteOperation("graph", objectID, "op-001")

// Hash update triggered only when all backends complete
```

### Hash Update Coordination

Hash updates occur only after all backends complete:

```go
// All backends must complete before hash update
if allBackendsComplete {
    // Trigger hash update (file storage only)
    hashManager.CompleteOperation(objectID, operationID)
}
```

## Testing Checklist

### Unit Tests
- ✅ Storage orchestrator basic operations
- ✅ Multiple backends coordination
- ✅ Concurrent operations
- ✅ Operation prevention
- ✅ Hash update coordination

### Integration Tests
- ⏳ File + Graph backend coordination
- ⏳ File + Neo4j backend coordination
- ⏳ File + AllegroGraph backend coordination
- ⏳ Multiple backends with concurrent operations

### Performance Tests
- ⏳ Baseline: synchronous operations
- ⏳ Orchestrated: multi-backend operations
- ⏳ Comparison: performance impact

## Next Steps

1. Complete unit test fixes
2. Run all unit tests
3. Implement integration tests
4. Test with multiple backends
5. Performance benchmarking
6. Production integration

