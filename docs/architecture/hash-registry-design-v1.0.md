# Hash Registry Design v1.0

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Created:** 2025-12-24  
**Status:** Design Complete  
**Related:** BLI-207, BLI-626, REQ-016

## Overview

The Hash Registry provides consistent integrity verification across both file-based and graph-based storage backends. It uses a **single registry per object kind** pattern for efficient bulk operations, ensuring consistent performance characteristics regardless of storage backend.

## Design Principles

1. **Consistency**: Same interface, same performance characteristics across backends
2. **Efficiency**: Single read/write operation per kind for bulk operations
3. **Simplicity**: One registry per kind, not per-instance
4. **Backend Agnostic**: Interface allows switching between file and graph storage

## Architecture

### Interface

```go
type HashRegistryProvider interface {
    Load() error
    Save() error
    GetHash(identifier string) string
    SetHash(identifier, hash string)
    DeleteHash(identifier string)
    HasHash(identifier string) bool
    GetAllHashes() map[string]string
}
```

### File-Based Implementation

**Storage**: Single JSON file per kind (e.g., `.backlog_item.hashes`)

**Structure**:
```json
{
  "hashes": {
    "BLI-001.yaml": "abc123...",
    "BLI-002.yaml": "def456...",
    ...
  }
}
```

**Performance**:
- Load: One file read per kind
- Save: One file write per kind
- GetHash: In-memory map lookup (O(1))
- Bulk operations: Single file operation

**Location**: `docs/process/{kind_dir}/.{kind}.hashes`

### Graph-Based Implementation

**Storage**: Single graph node per kind (e.g., `HashRegistry:backlog_item`)

**Structure**:
```cypher
CREATE (hr:HashRegistry {
  id: "HashRegistry:backlog_item",
  kind: "backlog_item",
  hashes: "{\"BLI-001\":\"abc123...\",\"BLI-002\":\"def456...\",...}"
})
```

**Performance**:
- Load: One node read per kind
- Save: One node update per kind
- GetHash: In-memory map lookup (O(1)) after load
- Bulk operations: Single node operation

**Node ID Format**: `HashRegistry:{kind}`

## Key Differences

| Aspect | File-Based | Graph-Based |
|--------|-----------|-------------|
| **Storage Unit** | Single file | Single node |
| **Identifier** | Filename (e.g., `BLI-001.yaml`) | Object ID (e.g., `BLI-001`) |
| **Data Format** | JSON file | JSON string in node property |
| **Load Operation** | `os.ReadFile()` | `GetNode()` |
| **Save Operation** | `os.WriteFile()` | `UpdateNode()` or `CreateNode()` |
| **Bulk Performance** | Single file I/O | Single node query |

## Usage Pattern

### File-Based Backend

```go
// Create registry for file-based storage
registry := storage.NewHashRegistry("backlog_item", "/path/to/backlog")

// Load all hashes for this kind
if err := registry.Load(); err != nil {
    return err
}

// Get hash for specific file
hash := registry.GetHash("BLI-001.yaml")

// Update hash
registry.SetHash("BLI-001.yaml", newHash)

// Save all changes
if err := registry.Save(); err != nil {
    return err
}
```

### Graph-Based Backend

```go
// Create registry for graph-based storage
registry := storage.NewGraphHashRegistry("backlog_item", graphConn)

// Load all hashes for this kind
if err := registry.Load(); err != nil {
    return err
}

// Get hash for specific object
hash := registry.GetHash("BLI-001")

// Update hash
registry.SetHash("BLI-001", newHash)

// Save all changes
if err := registry.Save(); err != nil {
    return err
}
```

## Performance Characteristics

Both implementations have **identical performance characteristics**:

1. **Bulk Load**: O(1) storage operations (one file/node read)
2. **Bulk Save**: O(1) storage operations (one file/node write)
3. **Hash Lookup**: O(1) in-memory map lookup after load
4. **Hash Update**: O(1) in-memory map update (requires save to persist)

**Key Insight**: The registry pattern ensures that checking integrity for 1000 objects of the same kind requires:
- **File-based**: 1 file read + 1000 in-memory lookups
- **Graph-based**: 1 node read + 1000 in-memory lookups

This is **dramatically more efficient** than per-instance storage, which would require:
- **Per-instance file**: 1000 file reads
- **Per-instance node**: 1000 node queries

## Migration Considerations

When migrating from file-based to graph-based storage:

1. **During Migration**: Both registries can coexist
   - File registry tracks source file integrity
   - Graph registry tracks graph node integrity

2. **Post-Migration**: 
   - File registry can be kept for source file verification
   - Graph registry used for graph-native object verification

3. **Hybrid Mode**: 
   - System can use both registries simultaneously
   - File registry for file-based objects
   - Graph registry for graph-native objects

## Integrity Verification Flow

### File-Based Objects

```
1. Load HashRegistry for object kind
2. Read object file
3. Calculate SHA-256 hash of file content
4. Compare with hash from registry
5. Report integrity status
```

### Graph-Based Objects

```
1. Load GraphHashRegistry for object kind
2. Get object node from graph
3. Serialize node properties to canonical format
4. Calculate SHA-256 hash of serialized data
5. Compare with hash from registry
6. Report integrity status
```

## Benefits of Consistent Design

1. **Code Reuse**: Same verification logic works for both backends
2. **Performance**: Bulk operations are efficient in both cases
3. **Maintainability**: Single interface, clear separation of concerns
4. **Testability**: Can test interface independently of backend
5. **Flexibility**: Easy to switch backends or use both simultaneously

## Future Enhancements

1. **Hash History**: Track hash changes over time
2. **Distributed Verification**: Verify hashes across multiple nodes
3. **Incremental Updates**: Only update changed hashes
4. **Compression**: Compress large hash maps for storage efficiency
5. **Indexing**: Add indexes for faster lookups in graph backend

