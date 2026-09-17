# Filter Operators Specification

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Status**: Design  
**Date**: 2026-01-05  
**Related**: Object Storage, Query System, Semantic Types

## Overview

This document defines the filter operators available for querying objects in the zqk system. Filter operators enable semantic, type-aware comparisons that respect field `semantic_type` definitions from object specs.

## Core Principles

1. **Semantic Awareness**: Operators should understand the semantic type of fields (timestamp, date, datetime, etc.) and perform appropriate comparisons
2. **Contextual Clarity**: Operators should be self-documenting and contextually clear (e.g., `$before` for timestamps vs `$lt` for generic comparison)
3. **Type Safety**: Operators should validate types and provide clear errors when used incorrectly
4. **Backward Compatibility**: Generic operators (`$lt`, `$gt`, etc.) remain available but semantic operators are preferred

## Operator Categories

### 1. Equality Operators

| Operator | Description | Example | Semantic Type Aware |
|----------|-------------|---------|---------------------|
| `$eq` | Equal to | `{"status": {"$eq": "active"}}` | No |
| `$ne` | Not equal to | `{"status": {"$ne": "archived"}}` | No |

### 2. Generic Comparison Operators

These operators work with any comparable type (numbers, strings, etc.) but do NOT perform semantic type parsing.

| Operator | Description | Example | Notes |
|----------|-------------|---------|-------|
| `$gt` | Greater than | `{"priority": {"$gt": 5}}` | Lexicographic for strings |
| `$gte` | Greater than or equal | `{"priority": {"$gte": 5}}` | Lexicographic for strings |
| `$lt` | Less than | `{"priority": {"$lt": 10}}` | Lexicographic for strings |
| `$lte` | Less than or equal | `{"priority": {"$lte": 10}}` | Lexicographic for strings |

**Note**: For timestamp/date fields, use semantic operators (`$before`, `$after`, etc.) instead of generic operators.

### 3. Semantic Date/Time Operators

These operators are **semantic type aware** and automatically parse timestamp/date strings before comparison. They should be used for fields with `semantic_type: timestamp`, `semantic_type: date`, or `semantic_type: datetime`.

| Operator | Description | Example | Semantic Types |
|----------|-------------|---------|----------------|
| `$before` | Before (exclusive) | `{"created_at": {"$before": "2026-01-01T00:00:00Z"}}` | timestamp, datetime |
| `$after` | After (exclusive) | `{"created_at": {"$after": "2026-01-01T00:00:00Z"}}` | timestamp, datetime |
| `$on` | Exactly on (equality) | `{"plan_date": {"$on": "2026-01-01"}}` | date, datetime |
| `$onOrBefore` | On or before (inclusive) | `{"created_at": {"$onOrBefore": "2026-01-01T00:00:00Z"}}` | timestamp, datetime |
| `$onOrAfter` | On or after (inclusive) | `{"created_at": {"$onOrAfter": "2026-01-01T00:00:00Z"}}` | timestamp, datetime |
| `$between` | Between two values (inclusive) | `{"created_at": {"$between": ["2026-01-01T00:00:00Z", "2026-01-31T23:59:59Z"]}}` | timestamp, datetime |
| `$within` | Within a time range | `{"created_at": {"$within": {"start": "...", "end": "...", "unit": "days", "value": 30}}}` | timestamp, datetime |

**Implementation Notes**:
- These operators check the field's `semantic_type` from the object spec
- If `semantic_type` is `timestamp`, `date`, or `datetime`, the operator parses both the field value and filter value as time.Time before comparison
- If `semantic_type` is not a date/time type, the operator returns an error or falls back to generic comparison with a warning
- Supports RFC3339, ISO8601, and common date formats

### 4. String Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `$contains` | String contains substring | `{"title": {"$contains": "bug"}}` |
| `$startsWith` | String starts with prefix | `{"id": {"$startsWith": "BLI-"}}` |
| `$endsWith` | String ends with suffix | `{"id": {"$endsWith": "-001"}}` |
| `$regex` | Regular expression match | `{"title": {"$regex": "^Critical.*"}}` |

### 5. Array/List Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `$in` | Value is in array | `{"status": {"$in": ["active", "pending"]}}` |
| `$nin` | Value is not in array | `{"status": {"$nin": ["archived", "deleted"]}}` |
| `$has` | Array contains value | `{"goal_refs": {"$has": "GOAL-001"}}` |
| `$hasAll` | Array contains all values | `{"milestone_refs": {"$hasAll": ["MIL-001", "MIL-002"]}}` |
| `$hasAny` | Array contains any value | `{"requirement_refs": {"$hasAny": ["REQ-001", "REQ-002"]}}` |

### 6. Existence Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `$exists` | Field exists | `{"description": {"$exists": true}}` |
| `$isNull` | Field is null or missing | `{"parent_ref": {"$isNull": true}}` |

## Usage Examples

### Semantic Date/Time Filtering

```go
// Find objects created before a specific date
filter := ListFilter{
    Kind: "audit_aggregation_metric",
    Filters: map[string]any{
        "created_at": map[string]any{
            "$before": "2026-01-01T00:00:00Z",
        },
    },
}

// Find objects created within the last 30 days
filter := ListFilter{
    Kind: "command_metric",
    Filters: map[string]any{
        "created_at": map[string]any{
            "$onOrAfter": time.Now().AddDate(0, 0, -30).Format(time.RFC3339),
        },
    },
}

// Find objects created between two dates
filter := ListFilter{
    Kind: "backlog_item",
    Filters: map[string]any{
        "created_at": map[string]any{
            "$between": []string{
                "2026-01-01T00:00:00Z",
                "2026-01-31T23:59:59Z",
            },
        },
    },
}
```

### Generic Comparison (Not Recommended for Dates)

```go
// This works but relies on lexicographic string comparison
// NOT recommended for timestamp/date fields
filter := ListFilter{
    Kind: "audit_aggregation_metric",
    Filters: map[string]any{
        "created_at": map[string]any{
            "$lt": "2026-01-01T00:00:00Z", // Lexicographic comparison
        },
    },
}
```

## Implementation Requirements

### 1. Semantic Type Detection

The filter system must:
- Load object specs to determine field `semantic_type`
- Cache semantic type information for performance
- Check `semantic_type` when semantic operators are used

### 2. Timestamp Parsing

When a semantic date/time operator is used:
- Parse the field value as a timestamp/date (support multiple formats)
- Parse the filter value as a timestamp/date
- Perform time.Time comparison (not string comparison)
- Handle timezone conversions appropriately

### 3. Error Handling

- If a semantic operator is used on a non-date/time field, return a clear error
- If timestamp parsing fails, return a clear error with the problematic value
- Provide warnings when generic operators (`$lt`, `$gt`) are used on date/time fields

### 4. Performance

- Cache parsed object specs to avoid repeated lookups
- Cache semantic type mappings per field
- Consider pre-parsing timestamps during object parsing

## Migration Path

1. **Phase 1**: Add semantic operators alongside existing generic operators
2. **Phase 2**: Update documentation and examples to use semantic operators
3. **Phase 3**: Add warnings when generic operators are used on date/time fields
4. **Phase 4**: (Future) Consider deprecating generic operators for date/time fields

## Related Documentation

- [Semantic Types Ontology](./semantic-types-ontology-v1.0.md)
- [List Operations](./list-operations-v1.0.md)
- [Advanced Query Capabilities](./advanced-query-capabilities-v1.0.md)

