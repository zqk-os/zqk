package storage

import (
	"fmt"
	"slices"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
)

// matchesFiltersParsed checks if a parsed object matches the given filters
// Uses typed fields for faster list operations, falls back to raw map for simple fields
func (f *FileObjectStorage) matchesFiltersParsed(parsed *objects.ParsedObject, filters map[string]any) bool {
	if len(filters) == 0 {
		return true
	}

	for field, filterValue := range filters {
		// Try to get field from parsed object first (typed access)
		actualValue, fieldExists := parsed.GetField(field)
		if !fieldExists {
			// Fall back to raw map
			actualValue, fieldExists = parsed.Raw[field]
		}

		if field == objects.FieldKeyNamespaceID && parsed.Kind == objects.KindAuditEvent {
			continue
		}

		// Handle filter with operator (map syntax)
		if filterMap, ok := filterValue.(map[string]any); ok {
			// Check each operator in the map
			for opStr, opValue := range filterMap {
				operator := FilterOperator(opStr)
				// Get kind from parsed object for semantic type lookup
				kind, hasKind := parsed.GetField(objects.FieldKeyKind)
				_ = hasKind
				kindStr, hasKindStr := kind.(string)
				_ = hasKindStr
				if !f.matchesFilterOperator(actualValue, fieldExists, operator, opValue, kindStr, field) {
					return false
				}
			}
		} else {
			// Simple equality (backward compatible)
			if !fieldExists {
				return false
			}
			if !f.compareEqual(actualValue, filterValue) {
				return false
			}
		}
	}

	return true
}

// matchesFilters checks if an object matches the given filters
// Supports all filter operators for any property on the object
// This is the legacy version that works with raw maps
//
//nolint:unused // Legacy function - reserved for backward compatibility
func (f *FileObjectStorage) matchesFilters(obj, filters map[string]any) bool {
	if len(filters) == 0 {
		return true
	}

	for field, filterValue := range filters {
		actualValue, fieldExists := obj[field]

		// Handle filter with operator (map syntax)
		if filterMap, ok := filterValue.(map[string]any); ok {
			// Check each operator in the map
			for opStr, opValue := range filterMap {
				operator := FilterOperator(opStr)
				// Get kind from object for semantic type lookup
				kind, _ := obj[objects.FieldKeyKind].(string)
				if !f.matchesFilterOperator(actualValue, fieldExists, operator, opValue, kind, field) {
					return false
				}
			}
		} else {
			// Simple equality (backward compatible)
			if !fieldExists {
				return false
			}
			if !f.compareEqual(actualValue, filterValue) {
				return false
			}
		}
	}

	return true
}

// getFieldSemanticType gets the semantic type for a field from the object spec
// Returns empty string if not found or if spec can't be loaded
func (f *FileObjectStorage) getFieldSemanticType(kind, fieldName string) string {
	if f.specLoader == nil {
		return ""
	}

	// Try to load spec for this kind
	config := GetStorageConfig()
	specFile := fmt.Sprintf("%s%s", kind, config.YAMLExtension)
	spec, err := f.specLoader.LoadSpecWithInheritance(specFile)
	if err != nil {
		return ""
	}

	// Check resolved fields for semantic_type
	if spec.ResolvedFields != nil {
		if fieldDef, ok := spec.ResolvedFields[fieldName]; ok {
			if fieldDefMap, ok := fieldDef.(map[string]any); ok {
				if semanticType := objects.GetString(fieldDefMap, "semantic_type"); semanticType != "" {
					return semanticType
				}
			}
		}
	}

	return ""
}

// parseTimestampForFilter parses a timestamp string into time.Time for filter operations
// Supports RFC3339, ISO8601, and common date formats
// Returns error if parsing fails (unlike the parseTimestamp in change_journal_reconstruction.go)
func parseTimestampForFilter(value any) (time.Time, error) {
	var timeStr string
	switch v := value.(type) {
	case string:
		timeStr = v
	case time.Time:
		return v, nil
	case float64:
		// Unix timestamp in seconds
		return time.Unix(int64(v), 0).UTC(), nil
	case int64:
		return time.Unix(v, 0).UTC(), nil
	case int:
		return time.Unix(int64(v), 0).UTC(), nil
	default:
		return time.Time{}, errfmt.Errorf(ConstStreamCannotParseTimestampFromTypeType, value)
	}

	// Try RFC3339 first (most common)
	if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
		return t, nil
	}

	// Try RFC3339Nano
	if t, err := time.Parse(time.RFC3339Nano, timeStr); err == nil {
		return t, nil
	}

	// Try ISO8601 (without timezone)
	if t, err := time.Parse(ConstStream20060102t150405, timeStr); err == nil {
		return t, nil
	}

	// Try date only (YYYY-MM-DD)
	if t, err := time.Parse("2006-01-02", timeStr); err == nil {
		return t, nil
	}

	return time.Time{}, errfmt.Errorf(ConstStreamUnableToParseTimestampStr, timeStr)
}

// isDateSemanticType checks if a semantic type is a date/time type
func isDateSemanticType(semanticType string) bool {
	return semanticType == "timestamp" || semanticType == "date" || semanticType == "datetime"
}

// matchesFilterOperator checks if a value matches a filter operator
// kind and fieldName are used for semantic type detection
func (f *FileObjectStorage) matchesFilterOperator(actualValue any, fieldExists bool, operator FilterOperator, filterValue any, kind, fieldName string) bool {
	// Handle timestamp field comparisons for $gte/$lte/$gt/$lt on created_at/updated_at fields
	// These fields are commonly timestamps even if not explicitly marked with semantic type
	if fieldExists && (fieldName == "created_at" || fieldName == "updated_at") {
		if operator == OpGreaterThanOrEqual || operator == OpGreaterThan ||
			operator == OpLessThanOrEqual || operator == OpLessThan {
			// Try to parse as timestamps
			actualTime, err1 := parseTimestampForFilter(actualValue)
			filterTime, err2 := parseTimestampForFilter(filterValue)
			if err1 == nil && err2 == nil {
				// Both parsed successfully, use timestamp comparison
				switch operator {
				case OpGreaterThan:
					return actualTime.After(filterTime)
				case OpGreaterThanOrEqual:
					return actualTime.After(filterTime) || actualTime.Equal(filterTime)
				case OpLessThan:
					return actualTime.Before(filterTime)
				case OpLessThanOrEqual:
					return actualTime.Before(filterTime) || actualTime.Equal(filterTime)
				}
			}
		}
	}

	// Handle semantic date/time operators first
	switch operator {
	case OpBefore, OpAfter, OpOn, OpOnOrBefore, OpOnOrAfter:
		if !fieldExists {
			return false
		}

		// Check semantic type if kind and fieldName are available
		if kind != emptyValue && fieldName != emptyValue {
			semanticType := f.getFieldSemanticType(kind, fieldName)
			if isDateSemanticType(semanticType) {
				// Parse both values as timestamps
				actualTime, err1 := parseTimestampForFilter(actualValue)
				filterTime, err2 := parseTimestampForFilter(filterValue)
				if err1 == nil && err2 == nil {
					// Both parsed successfully, use semantic comparison
					switch operator {
					case OpBefore:
						return actualTime.Before(filterTime)
					case OpAfter:
						return actualTime.After(filterTime)
					case OpOn:
						return actualTime.Equal(filterTime)
					case OpOnOrBefore:
						return actualTime.Before(filterTime) || actualTime.Equal(filterTime)
					case OpOnOrAfter:
						return actualTime.After(filterTime) || actualTime.Equal(filterTime)
					}
				}
			}
		}

		// Fallback: try to parse as timestamps even without semantic type
		actualTime, err1 := parseTimestampForFilter(actualValue)
		filterTime, err2 := parseTimestampForFilter(filterValue)
		if err1 == nil && err2 == nil {
			// Both parsed successfully, use timestamp comparison
			switch operator {
			case OpBefore:
				return actualTime.Before(filterTime)
			case OpAfter:
				return actualTime.After(filterTime)
			case OpOn:
				return actualTime.Equal(filterTime)
			case OpOnOrBefore:
				return actualTime.Before(filterTime) || actualTime.Equal(filterTime)
			case OpOnOrAfter:
				return actualTime.After(filterTime) || actualTime.Equal(filterTime)
			}
		}
		// If parsing fails, fall through to default case (generic operators)

	case OpBetween:
		if !fieldExists {
			return false
		}

		// Filter value should be a slice of two timestamps
		rangeSlice, ok := filterValue.([]any)
		if !ok {
			// Try []string
			if strSlice, ok := filterValue.([]string); ok {
				rangeSlice = make([]any, len(strSlice))
				for i, s := range strSlice {
					rangeSlice[i] = s
				}
			} else {
				return false
			}
		}
		if len(rangeSlice) != 2 {
			return false
		}

		actualTime, err1 := parseTimestampForFilter(actualValue)
		startTime, err2 := parseTimestampForFilter(rangeSlice[0])
		endTime, err3 := parseTimestampForFilter(rangeSlice[1])
		if err1 != nil || err2 != nil || err3 != nil {
			return false
		}

		// Between is inclusive on both ends
		return (actualTime.Equal(startTime) || actualTime.After(startTime)) &&
			(actualTime.Equal(endTime) || actualTime.Before(endTime))

	case OpWithin:
		// OpWithin is more complex - would need range specification
		// For now, fall back to generic or return false
		// TODO: Implement $within operator with range specification
		return false

	default:
		// Fall through to generic operators
		return f.matchesFilterOperatorGeneric(actualValue, fieldExists, operator, filterValue)
	}

	return false
}

// matchesFilterOperatorGeneric handles generic operators (non-semantic)
func (f *FileObjectStorage) matchesFilterOperatorGeneric(actualValue any, fieldExists bool, operator FilterOperator, filterValue any) bool {
	switch operator {
	case OpEqual: // Default equality
		if !fieldExists {
			return false
		}
		return f.compareEqual(actualValue, filterValue)

	case OpNotEqual:
		if !fieldExists {
			return true // Field doesn't exist, so it's not equal
		}
		return !f.compareEqual(actualValue, filterValue)

	case OpGreaterThan:
		if !fieldExists {
			return false
		}
		return CompareValues(actualValue, filterValue) == 1 // actualValue > filterValue

	case OpGreaterThanOrEqual:
		if !fieldExists {
			return false
		}
		cmp := CompareValues(actualValue, filterValue)
		return cmp == 1 || cmp == 0 // actualValue >= filterValue

	case OpLessThan:
		if !fieldExists {
			return false
		}
		return CompareValues(actualValue, filterValue) == -1 // actualValue < filterValue

	case OpLessThanOrEqual:
		if !fieldExists {
			return false
		}
		cmp := CompareValues(actualValue, filterValue)
		return cmp == -1 || cmp == 0 // actualValue <= filterValue

	case OpContains:
		if !fieldExists {
			return false
		}
		return f.stringContains(actualValue, filterValue)

	case OpStartsWith:
		if !fieldExists {
			return false
		}
		return f.stringStartsWith(actualValue, filterValue)

	case OpEndsWith:
		if !fieldExists {
			return false
		}
		return f.stringEndsWith(actualValue, filterValue)

	case OpIn:
		if !fieldExists {
			return false
		}
		return f.valueInList(actualValue, filterValue)

	case OpNotIn:
		if !fieldExists {
			return true // Field doesn't exist, so it's not in the list
		}
		return !f.valueInList(actualValue, filterValue)

	case OpHas:
		if !fieldExists {
			return false
		}
		return f.arrayContains(actualValue, filterValue)

	case OpHasAll:
		if !fieldExists {
			return false
		}
		return f.arrayContainsAll(actualValue, filterValue)

	case OpHasAny:
		if !fieldExists {
			return false
		}
		return f.arrayContainsAny(actualValue, filterValue)

	case OpExists:
		exists, ok := filterValue.(bool)
		if !ok {
			return false
		}
		return fieldExists == exists

	case OpIsNull:
		isNull, ok := filterValue.(bool)
		if !ok {
			return false
		}
		if isNull {
			return !fieldExists || actualValue == nil
		}
		return fieldExists && actualValue != nil

	default:
		// Unknown operator, treat as equality for backward compatibility
		if !fieldExists {
			return false
		}
		return f.compareEqual(actualValue, filterValue)
	}
}

// compareEqual checks if two values are equal
// Uses shared CompareEqual utility
func (f *FileObjectStorage) compareEqual(a, b any) bool {
	return CompareEqual(a, b)
}

// stringContains checks if a string value contains a substring
// Uses shared StringContains utility
func (f *FileObjectStorage) stringContains(actualValue, filterValue any) bool {
	return StringContains(actualValue, filterValue)
}

// stringStartsWith checks if a string value starts with a prefix
// Uses shared StringStartsWith utility
func (f *FileObjectStorage) stringStartsWith(actualValue, filterValue any) bool {
	return StringStartsWith(actualValue, filterValue)
}

// stringEndsWith checks if a string value ends with a suffix
// Uses shared StringEndsWith utility
func (f *FileObjectStorage) stringEndsWith(actualValue, filterValue any) bool {
	return StringEndsWith(actualValue, filterValue)
}

// valueInList checks if a value is in a list
func (f *FileObjectStorage) valueInList(actualValue, filterValue any) bool {
	// filterValue should be a slice/array
	switch v := filterValue.(type) {
	case []any:
		for _, item := range v {
			if f.compareEqual(actualValue, item) {
				return true
			}
		}
		return false
	case []string:
		actualStr, ok := actualValue.(string)
		if !ok {
			return false
		}
		return slices.Contains(v, actualStr)
	default:
		return false
	}
}

// arrayContains checks if an array contains a value
func (f *FileObjectStorage) arrayContains(actualValue, filterValue any) bool {
	// actualValue should be a slice/array
	switch v := actualValue.(type) {
	case []any:
		for _, item := range v {
			if f.compareEqual(item, filterValue) {
				return true
			}
		}
		return false
	case []string:
		filterStr, ok := filterValue.(string)
		if !ok {
			return false
		}
		return slices.Contains(v, filterStr)
	default:
		return false
	}
}

// arrayContainsAll checks if an array contains all values
func (f *FileObjectStorage) arrayContainsAll(actualValue, filterValue any) bool {
	// Get list of values to check
	var valuesToCheck []any
	switch v := filterValue.(type) {
	case []any:
		valuesToCheck = v
	case []string:
		for _, s := range v {
			valuesToCheck = append(valuesToCheck, s)
		}
	default:
		return false
	}

	// Check if actualValue is an array
	var actualArray []any
	switch v := actualValue.(type) {
	case []any:
		actualArray = v
	case []string:
		for _, s := range v {
			actualArray = append(actualArray, s)
		}
	default:
		return false
	}

	// Check if all values are in the array
	for _, checkValue := range valuesToCheck {
		found := false
		for _, arrayValue := range actualArray {
			if f.compareEqual(arrayValue, checkValue) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// arrayContainsAny checks if an array contains any value
func (f *FileObjectStorage) arrayContainsAny(actualValue, filterValue any) bool {
	// Get list of values to check
	var valuesToCheck []any
	switch v := filterValue.(type) {
	case []any:
		valuesToCheck = v
	case []string:
		for _, s := range v {
			valuesToCheck = append(valuesToCheck, s)
		}
	default:
		return false
	}

	// Check if actualValue is an array
	var actualArray []any
	switch v := actualValue.(type) {
	case []any:
		actualArray = v
	case []string:
		for _, s := range v {
			actualArray = append(actualArray, s)
		}
	default:
		return false
	}

	// Check if any value is in the array
	for _, checkValue := range valuesToCheck {
		for _, arrayValue := range actualArray {
			if f.compareEqual(arrayValue, checkValue) {
				return true
			}
		}
	}
	return false
}
