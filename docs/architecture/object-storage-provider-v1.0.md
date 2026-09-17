# Object Storage Provider Interface v1.0

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Status**: Implemented  
**Date**: 2025-12-25  
**Last Updated**: 2025-12-26  
**Related**: BLI-634, HashRegistryProvider pattern, pkg/context refactor

## Problem Statement

All object kinds (backlog_item, goal, milestone, requirement, criteria, etc.) need consistent CRUD operations through a unified interface that works with both file-based and graph-based backends. Currently:

- Objects are created/updated by directly writing YAML files
- No unified interface for CRUD operations
- No security context or permission enforcement
- No validation hooks before persistence
- No transaction support for multi-object operations
- No optimistic locking for concurrent updates

## Solution: ObjectStorageProvider Interface

Following the `HashRegistryProvider` pattern, we define a unified interface that abstracts object storage operations.

**Topology (2026-07-29):** Implementations include file/CAS (`FileObjectStorage`), graph (`GraphObjectStorage`), and legacy hybrid dual-write. Product rule: **file is durability SSOT**; graph is optional projection — [ADR-STORAGE-FILE-SSOT-GRAPH-PROJECTION-v1.0.md](./decisions/ADR-STORAGE-FILE-SSOT-GRAPH-PROJECTION-v1.0.md).

## Interface Design

```go
package storage

import (
    "context"
    pkgctx "github.com/lanceman/zqk/pkg/context"
)

// Re-export context types for convenience
type SecurityContext = pkgctx.SecurityContext
type StorageContext = pkgctx.StorageContext

// ObjectStorageProvider defines the interface for object storage implementations
// This allows consistent behavior across file-based and graph-based backends
type ObjectStorageProvider interface {
    // Create creates a new object
    // - Validates object before persistence
    // - Enforces permissions (write permission for object kind)
    // - Generates ID if not provided
    // - Sets created_at, created_by, updated_at, updated_by
    // - Returns error if object already exists
    Create(ctx context.Context, secCtx *SecurityContext, obj map[string]any) error

    // Read retrieves an object by ID
    // - Enforces permissions (read permission for object kind)
    // - Returns error if object not found
    Read(ctx context.Context, secCtx *SecurityContext, id string) (map[string]any, error)

    // Update updates an existing object
    // - Validates object before persistence
    // - Enforces permissions (write permission for object kind)
    // - Uses optimistic locking (checks updated_at timestamp)
    // - Sets updated_at, updated_by
    // - Returns error if object not found or version conflict
    Update(ctx context.Context, secCtx *SecurityContext, id string, updates map[string]any) error

    // Delete deletes an object
    // - Enforces permissions (delete permission for object kind)
    // - Handles cascade deletes (deletes dependent objects)
    // - Returns error if object not found or has dependencies
    Delete(ctx context.Context, secCtx *SecurityContext, id string, cascade bool) error

    // List lists objects with filtering, sorting, pagination, and grouping
    // - Enforces permissions (read permission for object kind)
    // - Supports filtering by any field
    // - Supports sorting by any field
    // - Supports pagination (offset, limit) - respects StorageContext.MaxPageSize
    // - Supports grouping by any field (returns grouped results)
    List(ctx context.Context, secCtx *SecurityContext, storageCtx *StorageContext, filter ListFilter) (*QueryResult, error)

    // Query executes a custom query (backend-specific)
    // - File backend: supports basic filtering/sorting
    // - Graph backend: supports Cypher queries
    // - Respects StorageContext parameters (pagination, grouping)
    Query(ctx context.Context, secCtx *SecurityContext, storageCtx *StorageContext, query Query) (*QueryResult, error)

    // Batch operations for atomic multi-object operations
    BeginTransaction(ctx context.Context) (ObjectTransaction, error)
}

// ObjectTransaction provides transaction support for multi-object operations
type ObjectTransaction interface {
    Create(ctx context.Context, secCtx *SecurityContext, obj map[string]any) error
    Read(ctx context.Context, secCtx *SecurityContext, id string) (map[string]any, error)
    Update(ctx context.Context, secCtx *SecurityContext, id string, updates map[string]any) error
    Delete(ctx context.Context, secCtx *SecurityContext, id string, cascade bool) error
    Commit(ctx context.Context) error
    Rollback(ctx context.Context) error
}

// ListFilter defines filtering, sorting, and pagination options
type ListFilter struct {
    Kind     string                 // Object kind (required)
    Filters  map[string]any         // Field -> value filters
    SortBy   string                 // Field to sort by
    SortAsc  bool                   // Sort direction
    Offset   int                    // Pagination offset
    Limit    int                    // Pagination limit
}

// Query represents a backend-specific query
type Query struct {
    Kind        string                 // Object kind
    Type        QueryType              // Query type
    Expression  string                 // Query expression (Cypher for graph, filter for file)
    Parameters  map[string]any         // Query parameters
}

type QueryType string

const (
    QueryTypeFilter  QueryType = "filter"  // File backend: field filters
    QueryTypeCypher  QueryType = "cypher"  // Graph backend: Cypher query
    QueryTypeVector  QueryType = "vector"  // Graph backend: vector similarity
)

// QueryResult contains query execution results
type QueryResult struct {
    Objects     []map[string]any              // Flat list of objects
    Groups      map[string][]map[string]any    // Grouped objects (if GroupBy is set)
    Grouped     bool                           // True if results are grouped
    TotalGroups int                            // Total number of groups
    Meta        map[string]any
}
```

## Implementation Strategy

### Phase 1: File-Based Implementation

**FileObjectStorage** wraps existing YAML file operations:

- `Create`: Write YAML file, update hash registry, update object ID cache
- `Read`: Read YAML file, parse, return as map
- `Update`: Read file, merge updates, validate, write back, update hash registry
- `Delete`: Delete YAML file, remove from hash registry, update object ID cache
- `List`: Scan directory, filter, sort, paginate
- `Query`: Basic filtering/sorting (no Cypher support)
- `BeginTransaction`: File-based transactions using temporary files and atomic moves

### Phase 2: Graph-Based Implementation

**GraphObjectStorage** uses existing `GraphConnection`:

- `Create`: Create graph node with object properties
- `Read`: Query graph node by ID
- `Update`: Update graph node properties
- `Delete`: Delete graph node and relationships (cascade)
- `List`: Execute Cypher query with filters
- `Query`: Execute Cypher queries directly
- `BeginTransaction`: Use `GraphConnection.BeginTransaction()`

## Security Context Integration

All operations require a `SecurityContext`:

```go
// Example: Create a backlog item
// Use constructor from pkg/context package
secCtx := pkgctx.NewSecurityContext(
    "account:lanceettl",
    []string{"admin", "developer"},
    []string{"read:*", "write:backlog_item"},
)

// Or use system context for admin operations
secCtx := pkgctx.NewSystemSecurityContext()

err := storage.Create(ctx, secCtx, map[string]any{
    "id": "BLI-999",
    "kind": "backlog_item",
    "title": "New Feature",
    // ... other fields
})
```

## Validation Hooks

Before persistence, objects are validated:

1. **Spec Validation**: Object matches its spec (required fields, types, patterns)
2. **Lifecycle Validation**: Status transitions are valid
3. **Reference Validation**: Referenced objects exist
4. **Permission Validation**: User has permission to perform operation

## Optimistic Locking

Update operations check `updated_at` timestamp:

```go
// If object was modified since last read, update fails
if obj["updated_at"] != lastRead["updated_at"] {
    return ErrVersionConflict
}
```

## Cascade Deletes

Delete operations can cascade to dependent objects:

```go
// Delete backlog item and all its relationships
err := storage.Delete(ctx, secCtx, "BLI-999", cascade: true)
```

## Benefits

1. **Unified Interface**: Same operations work for file and graph backends
2. **Security**: Built-in permission enforcement
3. **Validation**: Automatic validation before persistence
4. **Transactions**: Support for atomic multi-object operations
5. **Concurrency**: Optimistic locking prevents conflicts
6. **Extensibility**: Easy to add new backends (e.g., database)

## Related Documents

- [Hash Registry Design](./hash-registry-design-v1.0.md) - Similar provider pattern
- [Pluggable Graph Backend](./pluggable-graph-backend-interface-v1.0.md) - Graph backend interface
- [Spec Persistence Gap Analysis](./spec-persistence-gap-analysis.md) - Similar pattern for specs

## Next Steps

1. Define `ObjectStorageProvider` interface in `pkg/storage/object_storage_interface.go`
2. Implement `FileObjectStorage` in `pkg/storage/object_storage_file.go`
3. Implement `GraphObjectStorage` in `pkg/storage/object_storage_graph.go`
4. Add validation hooks integration
5. Add security context and permission enforcement
6. Add transaction support
7. Add tests for both implementations

