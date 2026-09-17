# List Operations v1.0 - Common Query Patterns

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Status**: Design  
**Date**: 2025-12-26  
**Related**: Object Parsing Optimization, Filter Operators

## Common List Operations

When querying objects with list/array fields, users typically want to:

### 1. Membership Checks

| Operation | Description | Example | Filter Operator |
|-----------|-------------|---------|----------------|
| **Contains** | Does the list contain element X? | `goal_refs` contains `GOAL-001` | `$has` |
| **Contains All** | Does the list contain all of [X, Y, Z]? | `milestone_refs` contains all of `[MIL-001, MIL-002]` | `$hasAll` |
| **Contains Any** | Does the list contain any of [X, Y, Z]? | `requirement_refs` contains any of `[REQ-001, REQ-002]` | `$hasAny` |
| **Not Contains** | Does the list NOT contain element X? | `goal_refs` does not contain `GOAL-999` | `$has` with negation |

### 2. Exact Matching

| Operation | Description | Example | Filter Operator |
|-----------|-------------|---------|----------------|
| **Exact Match** | Is the list exactly [X, Y, Z]? | `goal_refs` equals `[GOAL-001, GOAL-002]` | `$eq` |
| **Not Equal** | Is the list not equal to [X, Y, Z]? | `goal_refs` not equals `[GOAL-001]` | `$ne` |

### 3. Size/Count Operations

| Operation | Description | Example | Filter Operator |
|-----------|-------------|---------|----------------|
| **Empty** | Is the list empty? | `goal_refs` is empty | `$size` = 0 or `$exists` = false |
| **Not Empty** | Does the list have at least one element? | `goal_refs` is not empty | `$size` > 0 or `$exists` = true |
| **Size Equals** | Does the list have exactly N elements? | `goal_refs` has exactly 3 elements | `$size` = 3 |
| **Size Greater Than** | Does the list have more than N elements? | `goal_refs` has more than 5 elements | `$size` > 5 |
| **Size Less Than** | Does the list have fewer than N elements? | `goal_refs` has fewer than 2 elements | `$size` < 2 |

### 4. Subset/Superset Operations

| Operation | Description | Example | Filter Operator |
|-----------|-------------|---------|----------------|
| **Is Subset** | Are all elements of list A in list B? | `goal_refs` is subset of `[GOAL-001, GOAL-002, GOAL-003]` | `$subsetOf` (future) |
| **Is Superset** | Does list A contain all elements of list B? | `goal_refs` contains all of `[GOAL-001, GOAL-002]` | `$hasAll` |
| **Intersection** | Do lists A and B share any elements? | `goal_refs` intersects with `[GOAL-001, GOAL-002]` | `$hasAny` |

## Current Implementation

### Supported Operators

✅ **Implemented:**
- `$has` - Array contains value
- `$hasAll` - Array contains all values
- `$hasAny` - Array contains any value
- `$eq` - Exact match (works for lists)
- `$ne` - Not equal (works for lists)
- `$in` - Value is in array (for scalar fields)
- `$nin` - Value is not in array (for scalar fields)

❌ **Not Yet Implemented:**
- `$size` - List size comparison (e.g., `{"$size": {"$gt": 5}}`)
- `$subsetOf` - Is list a subset of another list
- `$supersetOf` - Does list contain all elements of another list
- `$intersects` - Do lists share any elements (alias for `$hasAny`)

## Usage Examples

### Contains Element

```go
filter := ListFilter{
    Kind: "backlog_item",
    Filters: map[string]any{
        "goal_refs": map[string]any{
            "$has": "GOAL-001",
        },
    },
}
```

### Contains All

```go
filter := ListFilter{
    Kind: "backlog_item",
    Filters: map[string]any{
        "milestone_refs": map[string]any{
            "$hasAll": []string{"MIL-001", "MIL-002"},
        },
    },
}
```

### Contains Any

```go
filter := ListFilter{
    Kind: "backlog_item",
    Filters: map[string]any{
        "requirement_refs": map[string]any{
            "$hasAny": []string{"REQ-001", "REQ-002"},
        },
    },
}
```

### Size Check (Future)

```go
filter := ListFilter{
    Kind: "backlog_item",
    Filters: map[string]any{
        "goal_refs": map[string]any{
            "$size": map[string]any{
                "$gt": 5,
            },
        },
    },
}
```

### Empty Check

```go
filter := ListFilter{
    Kind: "backlog_item",
    Filters: map[string]any{
        "goal_refs": map[string]any{
            "$size": 0,
        },
    },
}
```

## Performance Considerations

### Typed List Operations

With the "parse once" optimization, list operations benefit from typed `[]string` slices:

- **Before**: `[]interface{}` → convert each item → check membership (O(n) with conversion overhead)
- **After**: `[]string` → direct comparison (O(n) but faster)

### Parallelization Opportunities

1. **Read Phase**: Read files in parallel with goroutines
2. **Parse Phase**: Parse objects in parallel (already typed, safe for concurrent access)
3. **Filter Phase**: Filter in parallel (read-only operations)
4. **Sort Phase**: Sequential (required for stable sort)

## Test Scenarios

### Basic List Operations

1. ✅ Contains single element
2. ✅ Contains all elements
3. ✅ Contains any element
4. ✅ Exact match
5. ✅ Not equal
6. ✅ Empty list
7. ✅ Non-empty list

### Complex Scenarios

1. **Multiple List Filters**: Filter by `goal_refs` AND `milestone_refs`
2. **Nested Conditions**: `goal_refs` contains X AND `milestone_refs` contains Y
3. **Mixed Types**: Filter by list field AND scalar field
4. **Empty vs Missing**: Distinguish between empty list `[]` and missing field
5. **Large Lists**: Performance with lists containing 100+ elements
6. **Concurrent Reads**: Parallel file reading and parsing

### Edge Cases

1. **Null Values**: List field is `null` vs missing
2. **Type Mismatches**: List contains non-string values
3. **Duplicate Values**: List contains duplicate elements
4. **Empty Strings**: List contains empty string `""`
5. **Whitespace**: List contains strings with leading/trailing whitespace

## Future Enhancements

1. **$size Operator**: Support size comparisons (`$size: {"$gt": 5}`)
2. **$subsetOf Operator**: Check if list is subset of another list
3. **$supersetOf Operator**: Check if list contains all elements of another list
4. **$intersects Operator**: Check if lists share any elements
5. **Indexed Access**: Access list elements by index (`goal_refs[0]`)
6. **Slice Operations**: Filter by list slice (`goal_refs[0:3]`)

