# Advanced Query Capabilities v1.0

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-12-29  
**Status**: Implemented  
**Related**: BLI-647, Gap #18

## Overview

This document defines the advanced query capabilities for the zqk object storage system, enabling complex aggregations, joins, and analytical queries across object kinds.

## Requirements (from Gap #18)

1. **Aggregation Functions**: `Count`, `Sum`, `Avg`, `Min`, `Max`, `GroupBy`
2. **Aggregate Method**: `Aggregate(ctx, secCtx, filter ListFilter, aggregations []Aggregation) (*AggregateResult, error)`
3. **Joins**: Support joins across object kinds (via references)
4. **Subqueries**: Support nested queries
5. **Date/Time Range Queries**: With timezone support
6. **Geospatial Queries**: If applicable

## API Design

### Aggregate Method

```go
// Aggregate performs aggregations on objects matching the filter
// - Enforces permissions (read permission for object kind)
// - Supports multiple aggregations in a single call
// - Returns aggregated results grouped by aggregation function
Aggregate(ctx context.Context, secCtx *SecurityContext, filter ListFilter, aggregations []Aggregation) (*AggregateResult, error)
```

### Aggregation Types

```go
// AggregationFunction represents the type of aggregation
type AggregationFunction string

const (
	AggregationCount AggregationFunction = "count"
	AggregationSum   AggregationFunction = "sum"
	AggregationAvg   AggregationFunction = "avg"
	AggregationMin   AggregationFunction = "min"
	AggregationMax   AggregationFunction = "max"
	AggregationGroup AggregationFunction = "group"
)

// Aggregation defines a single aggregation operation
type Aggregation struct {
	Function AggregationFunction // count, sum, avg, min, max, group
	Field    string              // Field to aggregate (empty for count)
	Alias    string              // Result alias (e.g., "total_count", "avg_priority")
}

// AggregateResult contains aggregated query results
type AggregateResult struct {
	Aggregations map[string]any // Alias -> aggregated value
	Groups       map[string]*AggregateResult // GroupBy results (key = group value)
	Meta         map[string]any // Metadata (count, execution_time, etc.)
}
```

## Implementation Strategy

### File Backend

- Load objects matching filter
- Apply aggregations in memory
- Support GroupBy via map grouping
- Efficient for small to medium datasets

### Graph Backend

- Use Cypher aggregation functions (`COUNT`, `SUM`, `AVG`, `MIN`, `MAX`)
- Leverage `GROUP BY` in Cypher
- More efficient for large datasets
- Native support for complex aggregations

## Examples

### Count Aggregation

```go
aggregations := []Aggregation{
	{Function: AggregationCount, Alias: "total"},
}
result, err := storage.Aggregate(ctx, secCtx, filter, aggregations)
// result.Aggregations["total"] = 42
```

### Multiple Aggregations

```go
aggregations := []Aggregation{
	{Function: AggregationCount, Alias: "total"},
	{Function: AggregationAvg, Field: "priority", Alias: "avg_priority"},
	{Function: AggregationMax, Field: "priority", Alias: "max_priority"},
}
```

### GroupBy Aggregation

```go
filter := ListFilter{
	Kind: "backlog_item",
	GroupBy: "status", // Group by status
}
aggregations := []Aggregation{
	{Function: AggregationCount, Alias: "count"},
	{Function: AggregationAvg, Field: "priority", Alias: "avg_priority"},
}
result, err := storage.Aggregate(ctx, secCtx, filter, aggregations)
// result.Groups["planned"] = AggregateResult{Aggregations: {"count": 10, "avg_priority": 5.2}}
// result.Groups["in_progress"] = AggregateResult{Aggregations: {"count": 5, "avg_priority": 7.1}}
```

## Future Enhancements

1. **Joins**: Cross-kind joins via reference fields
2. **Subqueries**: Nested query support
3. **Date/Time Ranges**: Enhanced timezone-aware queries
4. **Geospatial**: Location-based queries (if needed)

