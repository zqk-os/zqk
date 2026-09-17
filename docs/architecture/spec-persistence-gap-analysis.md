# Spec Persistence Gap Analysis

**Last Verified:** 2026-08-31


**Created**: 2025-12-25  
**Status**: Gap Identified  
**Related**: User query about spec updates and graph backend consistency

## Problem Statement

The user wants to update a spec to make a field required, but there's no unified way to do this that works consistently across file-based and graph-based backends. The graph backend was supposed to make "all persistence behave the same," but specs are currently only file-based.

## Current State

### Spec Loading
- **File-based only**: `SpecLoader` reads from `.zqk/specs/objects/*.yaml`
- **No graph backend**: Specs cannot be stored or loaded from graph
- **No update API**: Specs are edited by directly modifying YAML files

### Spec Updates
- **Manual file editing**: Users must edit YAML files directly
- **No validation on update**: Changes aren't validated until next load
- **No versioning**: No tracking of spec changes
- **No migration path**: Can't migrate specs to graph backend

## Expected Behavior (Graph Backend Promise)

If the graph backend makes "all persistence behave the same," then:

1. **Unified Interface**: Specs should be updatable via the same interface whether file or graph
2. **Programmatic Updates**: Should be able to update specs via CLI/API
3. **Consistent Storage**: Specs should be stored in graph when using graph backend
4. **Same Operations**: Create, Read, Update, Delete should work the same way

## Gap Analysis

### Missing Components

1. **SpecWriter Interface**
   - No interface for writing/updating specs
   - `SpecLoader` only reads, never writes
   - No abstraction for spec persistence

2. **Graph Spec Storage**
   - Design doc exists (`graph-spec-storage-v1.0.md`) but only covers ID patterns
   - No full spec storage in graph
   - No `GraphSpecLoader` implementation

3. **Spec Update Command**
   - No `zqk spec update` command
   - No `zqk spec set-field-required` command
   - No programmatic way to modify specs

4. **Persistence Abstraction**
   - No `SpecStorageProvider` interface (like `HashRegistryProvider`)
   - File and graph backends use different code paths
   - No unified persistence layer

## Proposed Solution

### 1. Create SpecStorageProvider Interface

```go
type SpecStorageProvider interface {
    LoadSpec(ontology string) (*Spec, error)
    SaveSpec(spec *Spec) error
    UpdateSpec(ontology string, updates SpecUpdates) error
    DeleteSpec(ontology string) error
    ListSpecs() ([]string, error)
}
```

### 2. Implement File and Graph Backends

- **FileSpecStorage**: Writes to YAML files (current behavior)
- **GraphSpecStorage**: Stores specs as graph nodes (new)

### 3. Add Spec Update Command

```bash
zqk spec update criteria --field category --set required=true
zqk spec update criteria --field category.validation.required true
```

### 4. Unified SpecLoader

```go
type SpecLoader struct {
    storage SpecStorageProvider  // File or Graph
    // ... existing fields
}
```

## Example: Making a Field Required

### Current Way (File-based only)
```bash
# Edit file manually
vim .zqk/specs/objects/criteria.yaml
# Change: required: false → required: true
```

### Desired Way (Unified)
```bash
# Works with both file and graph backends
zqk spec update criteria \
  --field category \
  --set validation.required=true

# Or via graph query (if using graph backend)
MATCH (spec:ObjectSpec {ontology: "criteria"})
SET spec.fields.category.validation.required = true
```

## Benefits

1. **Consistency**: Same operations work for file and graph
2. **Programmatic**: Can update specs via CLI/API
3. **Validation**: Can validate changes before saving
4. **Versioning**: Can track spec changes
5. **Migration**: Easy to migrate specs to graph

## Related Documents

- [Graph Spec Storage Design](./graph-spec-storage-v1.0.md) - Partial implementation (ID patterns only)
- [Hash Registry Design](./hash-registry-design-v1.0.md) - Good example of unified persistence
- [Pluggable Graph Backend](./pluggable-graph-backend-interface-v1.0.md) - Backend abstraction

## Next Steps

1. Design `SpecStorageProvider` interface
2. Implement `FileSpecStorage` (wrap current file operations)
3. Implement `GraphSpecStorage` (store full specs in graph)
4. Refactor `SpecLoader` to use storage provider
5. Add `zqk spec` command group
6. Add `zqk spec update` command

