# Storage Package

This package provides a unified storage abstraction layer for zqk, supporting both file-based and graph-based storage backends through a common interface.

## Architecture Overview

The storage package follows a provider pattern with pluggable backends:

```
ObjectStorageProvider (Interface)
├── FileObjectStorage (File-based backend)
└── GraphObjectStorage (Graph-based backend)
```

### Backend Selection

Storage backends are selected automatically via `StorageFactory`:
1. Checks if graph backend is enabled (`ZQK_GRAPH_ENABLED=true`)
2. Attempts to connect to graph database
3. Falls back to file backend if graph is unavailable or disabled
4. Only one backend is active at a time

### Intentional differences (file vs graph)

| Aspect | File backend | Graph backend |
|--------|--------------|---------------|
| **Storage** | YAML files in `docs/architecture/{kind}/`; bucketing; optional CAS | Nodes and edges in MemGraph/Neo4j; kind-specific labels |
| **Validation** | Full spec + lifecycle + reference + blocking-issue detection | Spec + lifecycle; graph enforces relationships |
| **Query** | List/filter/sort over files; full-text search | List/filter/sort; Cypher; vector similarity; traversal (GetRelated, GetPath, GetNeighbors) |
| **Use when** | Default; no graph DB; traceable YAML on disk | Graph DB enabled; relationship-heavy queries; vector search |

Shared behavior: same `ObjectStorageProvider` interface, permission checks, metadata (created_at, updated_at, namespace_id), and error types. See [REMAINING_REFACTOR_WORK.md](../../docs/architecture/README.md) for optional “extract storage backend commonalities” follow-up.

## Core Components

### ObjectStorageProvider Interface

The `ObjectStorageProvider` interface defines the contract for all storage backends:

- **CRUD Operations**: `Create`, `Read`, `Update`, `Delete`, `Move`
- **Query Operations**: `List`, `Query`, `Count`, `Exists`, `Aggregate`
- **Search**: `Search` (full-text search with fuzzy matching)
- **Bulk Operations**: `BulkCreate`, `BulkUpdate`, `BulkGet`, `BulkDelete`
- **Graph Operations**: `GetRelated`, `GetPath`, `GetNeighbors` (relationship traversal)
- **Transactions**: `BeginTransaction` (atomic multi-object operations)

### Shared Utilities

Common functionality extracted to `storage_common.go`:

- **Permission Checking**: `CheckPermission`, `CheckPermissionWithKindSpecialCases`
- **Metadata Management**: `EnsureObjectMetadata` (sets created_at, updated_at, namespace_id)
- **Built-in Detection**: `IsBuiltIn` (identifies system objects)

### File Storage Backend

`FileObjectStorage` implements file-based storage:

- **Location**: Objects stored as YAML files in `docs/architecture/{kind}/` directories
- **Bucketing**: Supports configurable bucketing strategies (date-based, hash-based, etc.)
- **Content-Addressable Storage (CAS)**: Optional CAS for large files
- **Hash Registry**: Tracks file hashes for integrity checking
- **Validation**: Comprehensive validation with blocking issue detection
- **Transactions**: File-based transaction support

**Key Files:**
- `object_storage_file.go` - Core infrastructure
- `object_storage_file_crud.go` - Create, Read, Update, Delete operations
- `object_storage_file_list.go` - List, query, filtering, sorting
- `object_storage_file_validation.go` - Validation logic
- `object_storage_file_helpers.go` - Helper functions

### Graph Storage Backend

`GraphObjectStorage` implements graph-based storage:

- **Database**: Uses MemGraph/Neo4j via `pkg/graph/provider`
- **Labels**: Objects stored as nodes with kind-specific labels
- **Relationships**: Reference fields create graph edges
- **Cypher Queries**: Supports custom Cypher queries
- **Vector Search**: Supports vector similarity search
- **Validation**: Simplified validation (graph handles relationships)

**Key Files:**
- `object_storage_graph.go` - Core infrastructure
- `object_storage_graph_crud.go` - Create, Read, Update, Delete operations
- `object_storage_graph_list.go` - List, query, filtering
- `object_storage_graph_traversal.go` - Graph traversal operations
- `object_storage_graph_validation.go` - Validation logic

## Common Patterns

### Permission Checking

All operations enforce permissions via `SecurityContext`:

```go
// Check permission before operation
if err := CheckPermission(secCtx, "write", kind); err != nil {
    return err
}
```

### Metadata Management

Objects automatically get metadata fields set:

```go
// Ensures created_at, updated_at, created_by, updated_by, namespace_id
EnsureObjectMetadata(obj, secCtx, isCreate, idValidator)
```

### Validation

Both backends validate objects before persistence:

- **Spec Validation**: Validates against object specs
- **Lifecycle Validation**: Validates state transitions
- **Reference Validation**: Ensures referenced objects exist
- **File Backend**: Additional blocking issue detection

### Error Handling

Consistent error types:

- `ErrObjectNotFound` - Object doesn't exist
- `ErrObjectExists` - Object already exists
- `ErrVersionConflict` - Optimistic locking conflict
- `ErrPermissionDenied` - Permission denied

## Advanced Features

### Transactions

Atomic multi-object operations:

```go
tx, _ := storage.BeginTransaction(ctx)
tx.Create(ctx, secCtx, obj1)
tx.Update(ctx, secCtx, id2, updates)
tx.Commit(ctx) // or tx.Rollback(ctx)
```

### Bulk Operations

Efficient batch processing:

```go
result, _ := storage.BulkCreate(ctx, secCtx, objects)
// result.SuccessCount, result.FailureCount, result.Errors
```

### Filtering and Sorting

Rich query capabilities:

```go
filter := ListFilter{
    Kind: "backlog_item",
    Filters: map[string]any{
        "status": "planned",
        "priority": map[string]any{"$gt": 5},
    },
    SortBy: "created_at",
    SortAsc: false,
    Limit: 100,
}
```

### Graph Traversal

Relationship queries (graph backend):

```go
// Get all related objects (depth 2)
related, _ := storage.GetRelated(ctx, secCtx, id, "", 2)

// Find path between objects
path, _ := storage.GetPath(ctx, secCtx, fromID, toID)

// Get immediate neighbors
neighbors, _ := storage.GetNeighbors(ctx, secCtx, id, "both")
```

## File Organization

The package is organized into focused modules:

### Core Infrastructure
- `object_storage_interface.go` - Interface definitions
- `storage_factory.go` - Backend selection
- `storage_common.go` - Shared utilities
- `storage_config.go` - Configuration

### File Backend
- `object_storage_file*.go` - File storage implementation
- `hash_registry*.go` - Hash tracking
- `content_addressable_storage*.go` - CAS implementation
- `bucketing_strategy*.go` - Bucketing strategies

### Graph Backend
- `object_storage_graph*.go` - Graph storage implementation

### Caches and invalidation

- **List cache** (`list_cache.go`): Process-wide cache for `List()` results (keyed by project root + filter + limit). It must be invalidated whenever the **object ID cache** is updated or invalidated, or whenever **storage is modified** (create/update/delete), so list results stay consistent with the ID cache and on-disk state. Use `InvalidateListCache()` for a full clear or `InvalidateListCacheForKind(kind)` when only one kind changed. Object ID cache invalidation (e.g. in `cmd/zqk/system/check_cache.go` and `root.go` after create/update/delete) typically triggers list cache invalidation.

### Supporting Systems
- `operation_executor*.go` - Async operation processing
- `waitgroup_manager.go` - WaitGroup lifecycle management
- `value_comparison.go` - Shared comparison utilities
- `audit_*.go` - Audit event handling
- `metrics_*.go` - Metrics collection

## Usage Example

```go
// Create storage factory (auto-selects backend)
factory, _ := storage.NewStorageFactory(ctx, projectRoot)
storage := factory.GetStorage()

// Create an object
obj := map[string]any{
    "id": "BLI-001",
    "kind": "backlog_item",
    "title": "Implement feature X",
    "status": "planned",
}
err := storage.Create(ctx, secCtx, obj)

// List objects with filtering
filter := storage.ListFilter{
    Kind: "backlog_item",
    Filters: map[string]any{"status": "planned"},
    SortBy: "created_at",
    Limit: 10,
}
result, _ := storage.List(ctx, secCtx, storageCtx, filter)
```

## Running tests

- **Faster feedback**: `go test -short ./pkg/storage/...` skips heavy tests: `TestAllKindsCRUD`, `TestComprehensiveFilteringSortingGrouping`, comprehensive CAS tests, deadlock/concurrent/integration tests. Use for quick iteration.
- **Full suite**: `go test ./pkg/storage/...` runs all tests; expect several minutes. Increase per-package parallelism with `-parallel N` (e.g. `-parallel 8`) if you have CPU headroom.
- **Single test**: `go test -run TestFoo ./pkg/storage/...` to run one test or pattern.
- **Quick + parallel**: `go test -short -parallel 8 ./pkg/storage/...` for faster short runs.

## Related Documentation

- [CRUD Standardization](./CRUD_STANDARDIZATION.md) - CRUD operation patterns
- [Storage Architecture](../../docs/architecture/architecture/) - System architecture
- [Graph Package](../graph/README.md) - Graph database provider

## Notes

- Both backends implement the same interface for consistency
- File backend has more complex validation (blocking issues, partial data detection)
- Graph backend has simpler validation (appropriate for graph database)
- Shared utilities ensure consistent permission and metadata handling
- All operations are permission-aware and context-aware
