# Object Parsing Optimization v1.0 - "Parse Once" Strategy

**Version**: 1.0.0  
**Status**: Implemented  
**Date**: 2025-12-26  
**Related**: Object Type Generation, List Performance

## Problem Statement

When filtering and sorting objects during `List` operations, the system was:
1. Reading objects from disk (YAML parsing)
2. Iterating over maps multiple times for filtering
3. Re-parsing complex nested structures (status_history, change_log) on each filter check
4. Converting list fields from `[]interface{}` to `[]string` repeatedly during filtering/sorting
5. No optimization for list operations (e.g., checking if a value is in `goal_refs`)

This resulted in:
- Multiple iterations over the same data
- Repeated type conversions
- Slower filtering/sorting operations
- Unnecessary memory allocations

## Solution: Parse Once, Use Typed Fields

Parse each object **once** when read, extracting:
1. **Complex nested structures** → Typed structs (StatusHistoryEntry, ChangeLogEntry)
2. **List fields** → Typed `[]string` slices (goal_refs, milestone_refs, etc.)
3. **Common simple fields** → Cached for quick access (id, status, title, etc.)

Extracted fields are **removed from the map** to avoid duplication. The original map is preserved for output.

### Benefits

1. **Single Parse**: Each object parsed once, not repeatedly
2. **Faster List Operations**: Typed `[]string` slices for `$has`, `$hasAll`, `$hasAny` operations
3. **Faster Sorting**: Direct typed field access instead of map lookups
4. **Memory Efficient**: Extracted fields removed from map (no duplication)
5. **Backward Compatible**: Original map preserved for output

## Implementation

### ParsedObject Type

```go
type ParsedObject struct {
    // Raw map with simple fields (extracted fields removed)
    Raw map[string]any

    // Extracted complex nested structures
    StatusHistory []StatusHistoryEntry
    ChangeLog     []ChangeLogEntry

    // Typed list fields (extracted from Raw)
    GoalRefs         []string
    MilestoneRefs    []string
    RequirementRefs []string
    // ... etc

    // Cached simple fields (also in Raw)
    ID            string
    Kind          string
    Status        string
    // ... etc
}
```

### ParseObject Function

```go
func ParseObject(obj map[string]any) (*ParsedObject, error) {
    // 1. Extract complex nested structures
    // 2. Convert list fields to typed slices
    // 3. Cache common simple fields
    // 4. Remove extracted fields from map (to avoid duplication)
}
```

### List Operation Flow

```go
// 1. Read object from disk
obj := readObjectFile(path)

// 2. Make copy for parsing (ParseObject modifies the map)
objCopy := copyMap(obj)

// 3. Parse once - extracts types, converts lists
parsed := ParseObject(objCopy)

// 4. Filter using parsed object (faster list operations)
if matchesFiltersParsed(parsed, filters) {
    // Use typed fields for $has, $hasAll, $hasAny operations
}

// 5. Sort using parsed object (faster field access)
sortParsedObjects(parsedObjects, rawObjects, sortBy, sortAsc)

// 6. Return original objects (with all fields intact)
return rawObjects
```

## Performance Impact

### Before (Multiple Parses)

```
Read object → Parse YAML
For each filter:
  - Access field from map
  - If list: convert []interface{} → []string
  - Check if value in list
For sorting:
  - Access field from map
  - Compare values
```

### After (Parse Once)

```
Read object → Parse YAML → ParseObject (once)
For each filter:
  - Access typed field directly ([]string)
  - Check if value in list (O(n) but no conversion)
For sorting:
  - Access typed field directly
  - Compare values
```

### Optimizations

1. **List Operations**: `$has`, `$hasAll`, `$hasAny` use typed `[]string` slices directly
2. **Field Access**: Common fields (id, status, title) cached in struct fields
3. **No Duplication**: Extracted fields removed from map, original preserved separately
4. **Single Iteration**: Parse once, use typed fields for all operations

## What Gets Extracted

| Field Type | Extracted? | Reason |
|------------|------------|--------|
| Simple strings/bools | ❌ No | Stay in map, no performance gain |
| `status_history` | ✅ Yes | Complex nested structure, benefits from typing |
| `change_log` | ✅ Yes | Complex nested structure, benefits from typing |
| `goal_refs`, `milestone_refs`, etc. | ✅ Yes | List operations benefit from typed `[]string` |
| `artifacts`, `dependencies`, etc. | ✅ Yes | List operations benefit from typed `[]string` |

## Usage Example

```go
// In List operation
for each object:
    obj := readObjectFile(path)
    objCopy := copyMap(obj)  // Preserve original
    parsed := ParseObject(objCopy)  // Parse once
    
    // Filter using typed fields
    if parsed.GoalRefs contains filterValue {
        // Fast typed []string comparison
    }
    
    // Sort using typed fields
    if parsed.Status < other.Status {
        // Fast direct comparison
    }
    
    // Return original object (all fields intact)
    results.append(obj)
```

## Backward Compatibility

- Original objects preserved (not modified)
- `ToMap()` method restores extracted fields if needed
- Legacy `matchesFilters()` and `sortObjects()` still work with raw maps
- New optimized versions (`matchesFiltersParsed`, `sortParsedObjects`) used internally

## Future Optimizations

1. **Parallel Parsing**: Parse multiple objects concurrently
2. **Lazy Parsing**: Only parse if filtering/sorting requires it
3. **Field-Specific Parsing**: Only extract fields needed for current operation
4. **Caching**: Cache parsed objects for frequently accessed objects

