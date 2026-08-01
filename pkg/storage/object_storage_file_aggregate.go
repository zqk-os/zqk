package storage

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
)

// Aggregate performs aggregations on objects matching the filter
//
//nolint:gocritic // Interface requires value semantics for ListFilter
func (f *FileObjectStorage) Aggregate(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter, aggregations []Aggregation) (*AggregateResult, error) {
	// Check permission
	if err := f.checkPermission(secCtx, "read", filter.Kind); err != nil {
		return nil, err
	}

	// Validate aggregations
	if len(aggregations) == 0 {
		return nil, errfmt.Errorf(ConstStreamAtLeastOneAggregationIsRequired)
	}

	// Use List to get matching objects (with filters applied)
	// We'll apply aggregations in memory
	result, err := f.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, errfmt.Newf(ConstStreamFailedToListObjectsForAggregation).Wrap(err)
	}

	// If GroupBy is specified, aggregate per group
	if filter.GroupBy != emptyValue {
		return f.aggregateByGroup(result, aggregations, filter.GroupBy)
	}

	// Aggregate all objects
	return f.aggregateObjects(result.Objects, aggregations), nil
}

// aggregateByGroup performs aggregations grouped by a field
func (f *FileObjectStorage) aggregateByGroup(result *QueryResult, aggregations []Aggregation, groupByField string) (*AggregateResult, error) {
	groups := make(map[string]*AggregateResult)

	// If result already has groups, use them
	if len(result.Groups) > 0 {
		for groupKey, groupObjects := range result.Groups {
			groups[groupKey] = f.aggregateObjects(groupObjects, aggregations)
		}
	} else {
		// Group objects by the specified field
		grouped := make(map[string][]map[string]any)
		for _, obj := range result.Objects {
			groupKey := f.getGroupKey(obj, groupByField)
			grouped[groupKey] = append(grouped[groupKey], obj)
		}

		// Aggregate each group
		for groupKey, groupObjects := range grouped {
			groups[groupKey] = f.aggregateObjects(groupObjects, aggregations)
		}
	}

	return &AggregateResult{
		Aggregations: make(map[string]any),
		Groups:       groups,
		Meta: map[string]any{
			"group_count": len(groups),
		},
	}, nil
}

// aggregateObjects performs aggregations on a set of objects
func (f *FileObjectStorage) aggregateObjects(objs []map[string]any, aggregations []Aggregation) *AggregateResult {
	result := &AggregateResult{
		Aggregations: make(map[string]any),
		Groups:       nil,
		Meta: map[string]any{
			objects.FieldKeyObjectCount: len(objs),
		},
	}

	// Apply each aggregation
	for _, agg := range aggregations {
		alias := agg.Alias
		if alias == emptyValue {
			// Generate default alias
			if agg.Field != emptyValue {
				alias = fmt.Sprintf("%s_%s", agg.Function, agg.Field)
			} else {
				alias = string(agg.Function)
			}
		}

		var value any
		switch agg.Function {
		case AggregationCount:
			value = len(objs)
		case AggregationSum:
			value = f.aggregateSum(objs, agg.Field)
		case AggregationAvg:
			value = f.aggregateAvg(objs, agg.Field)
		case AggregationMin:
			value = f.aggregateMin(objs, agg.Field)
		case AggregationMax:
			value = f.aggregateMax(objs, agg.Field)
		default:
			// Unknown aggregation function
			continue
		}

		result.Aggregations[alias] = value
	}

	return result
}

// aggregateSum calculates the sum of a numeric field
func (f *FileObjectStorage) aggregateSum(objects []map[string]any, field string) float64 {
	var sum float64
	for _, obj := range objects {
		value := obj[field]
		if value == nil {
			continue
		}
		sum += f.toFloat64(value)
	}
	return sum
}

// aggregateAvg calculates the average of a numeric field
func (f *FileObjectStorage) aggregateAvg(objects []map[string]any, field string) float64 {
	if len(objects) == 0 {
		return 0
	}
	sum := f.aggregateSum(objects, field)
	return sum / float64(len(objects))
}

// aggregateMin finds the minimum value of a field
func (f *FileObjectStorage) aggregateMin(objects []map[string]any, field string) any {
	if len(objects) == 0 {
		return nil
	}

	var minVal any
	first := true
	for _, obj := range objects {
		value := obj[field]
		if value == nil {
			continue
		}
		if first {
			minVal = value
			first = false
		} else if CompareValues(value, minVal) < 0 {
			minVal = value
		}
	}
	return minVal
}

// aggregateMax finds the maximum value of a field
func (f *FileObjectStorage) aggregateMax(objects []map[string]any, field string) any {
	if len(objects) == 0 {
		return nil
	}

	var maxVal any
	first := true
	for _, obj := range objects {
		value := obj[field]
		if value == nil {
			continue
		}
		if first {
			maxVal = value
			first = false
		} else if CompareValues(value, maxVal) > 0 {
			maxVal = value
		}
	}
	return maxVal
}

// toFloat64 converts a value to float64 for numeric operations
func (f *FileObjectStorage) toFloat64(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case int32:
		return float64(v)
	case uint:
		return float64(v)
	case uint64:
		return float64(v)
	case uint32:
		return float64(v)
	case string:
		// Try to parse as number
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			return parsed
		}
		return 0
	default:
		// Try reflection for other numeric types
		rv := reflect.ValueOf(value)
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return float64(rv.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return float64(rv.Uint())
		case reflect.Float32, reflect.Float64:
			return rv.Float()
		}
		return 0
	}
}

// getGroupKey extracts the group key from an object for a given field
func (f *FileObjectStorage) getGroupKey(obj map[string]any, field string) string {
	value := obj[field]
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%v", value)
}

// CompareValues compares two values for aggregation operations
// Uses the existing compareValues method from FileObjectStorage
func (f *FileObjectStorage) CompareValues(a, b any) int {
	// Reuse the existing compareValues method
	// Note: This method exists in object_storage_file.go
	return CompareValues(a, b)
}
