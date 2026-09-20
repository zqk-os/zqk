package storage

import (
	"fmt"

	"github.com/zqk-os/zqk/pkg/storage/crud"

	"github.com/zqk-os/zqk/pkg/objects"
)

// matchesFiltersParsed checks if a parsed object matches the given filters
// Uses typed fields for faster list operations, falls back to raw map for simple fields
func (f *FileObjectStorage) matchesFiltersParsed(parsed *objects.ParsedObject, filters map[string]any) bool {
	if len(filters) == 0 {
		return true
	}

	for field, filterValue := range filters {
		if objects.IsKernelObjectRefField(field) {
			if !kernelRefFilterMatches(parsed, field, filterValue, f) {
				return false
			}
			continue
		}

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
			if !crud.CompareEqual(actualValue, filterValue) {
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
		if objects.IsKernelObjectRefField(field) {
			parsed := &objects.ParsedObject{Raw: obj}
			if !kernelRefFilterMatches(parsed, field, filterValue, f) {
				return false
			}
			continue
		}

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
			if !crud.CompareEqual(actualValue, filterValue) {
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

// crud.ParseTimestampForFilter parses a timestamp string into time.Time for filter operations
// Supports RFC3339, ISO8601, and common date formats
// Returns error if parsing fails (unlike the parseTimestamp in change_journal_reconstruction.go)

// crud.IsDateSemanticType checks if a semantic type is a date/time type

// matchesFilterOperator checks if a value matches a filter operator
// kind and fieldName are used for semantic type detection
func (f *FileObjectStorage) matchesFilterOperator(actualValue any, fieldExists bool, operator FilterOperator, filterValue any, kind, fieldName string) bool {
	// Handle timestamp field comparisons for $gte/$lte/$gt/$lt on created_at/updated_at fields
	// These fields are commonly timestamps even if not explicitly marked with semantic type
	if fieldExists && (fieldName == "created_at" || fieldName == "updated_at") {
		if operator == OpGreaterThanOrEqual || operator == OpGreaterThan ||
			operator == OpLessThanOrEqual || operator == OpLessThan {
			// Try to parse as timestamps
			actualTime, err1 := crud.ParseTimestampForFilter(actualValue)
			filterTime, err2 := crud.ParseTimestampForFilter(filterValue)
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
			if crud.IsDateSemanticType(semanticType) {
				// Parse both values as timestamps
				actualTime, err1 := crud.ParseTimestampForFilter(actualValue)
				filterTime, err2 := crud.ParseTimestampForFilter(filterValue)
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
		actualTime, err1 := crud.ParseTimestampForFilter(actualValue)
		filterTime, err2 := crud.ParseTimestampForFilter(filterValue)
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

		actualTime, err1 := crud.ParseTimestampForFilter(actualValue)
		startTime, err2 := crud.ParseTimestampForFilter(rangeSlice[0])
		endTime, err3 := crud.ParseTimestampForFilter(rangeSlice[1])
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
		return crud.CompareEqual(actualValue, filterValue)

	case OpNotEqual:
		if !fieldExists {
			return true // Field doesn't exist, so it's not equal
		}
		return !crud.CompareEqual(actualValue, filterValue)

	case OpGreaterThan:
		if !fieldExists {
			return false
		}
		return crud.CompareValues(actualValue, filterValue) == 1 // actualValue > filterValue

	case OpGreaterThanOrEqual:
		if !fieldExists {
			return false
		}
		cmp := crud.CompareValues(actualValue, filterValue)
		return cmp == 1 || cmp == 0 // actualValue >= filterValue

	case OpLessThan:
		if !fieldExists {
			return false
		}
		return crud.CompareValues(actualValue, filterValue) == -1 // actualValue < filterValue

	case OpLessThanOrEqual:
		if !fieldExists {
			return false
		}
		cmp := crud.CompareValues(actualValue, filterValue)
		return cmp == -1 || cmp == 0 // actualValue <= filterValue

	case OpContains:
		if !fieldExists {
			return false
		}
		return crud.StringContains(actualValue, filterValue)

	case OpStartsWith:
		if !fieldExists {
			return false
		}
		return crud.StringStartsWith(actualValue, filterValue)

	case OpEndsWith:
		if !fieldExists {
			return false
		}
		return crud.StringEndsWith(actualValue, filterValue)

	case OpIn:
		if !fieldExists {
			return false
		}
		return crud.ValueInList(actualValue, filterValue)

	case OpNotIn:
		if !fieldExists {
			return true // Field doesn't exist, so it's not in the list
		}
		return !crud.ValueInList(actualValue, filterValue)

	case OpHas:
		if !fieldExists {
			return false
		}
		return crud.ArrayContains(actualValue, filterValue)

	case OpHasAll:
		if !fieldExists {
			return false
		}
		return crud.ArrayContainsAll(actualValue, filterValue)

	case OpHasAny:
		if !fieldExists {
			return false
		}
		return crud.ArrayContainsAny(actualValue, filterValue)

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
		return crud.CompareEqual(actualValue, filterValue)
	}
}

// kernelRefFilterMatches treats singular *_ref and plural *_refs as one
// membership set so --filter workstream_ref=WS-X hits objects that only store the list.
func kernelRefFilterMatches(parsed *objects.ParsedObject, field string, filterValue any, f *FileObjectStorage) bool {
	ids := parsed.KernelObjectRefIDs(field)
	fieldExists := len(ids) > 0
	kindStr := ""
	if parsed != nil {
		kindStr = parsed.Kind
	}
	if filterMap, ok := filterValue.(map[string]any); ok {
		for opStr, opValue := range filterMap {
			operator := FilterOperator(opStr)
			if operator == OpEqual {
				switch sliceVal := opValue.(type) {
				case []string:
					if !crud.CompareEqual(ids, sliceVal) {
						return false
					}
					continue
				case []any:
					if !crud.CompareEqual(ids, sliceVal) {
						return false
					}
					continue
				default:
					if !crud.ArrayContains(ids, opValue) {
						return false
					}
					continue
				}
			}
			if !f.matchesFilterOperator(ids, fieldExists, operator, opValue, kindStr, field) {
				return false
			}
		}
		return true
	}
	if !fieldExists {
		return false
	}
	switch sliceVal := filterValue.(type) {
	case []string:
		return crud.CompareEqual(ids, sliceVal)
	case []any:
		return crud.CompareEqual(ids, sliceVal)
	default:
		return crud.ArrayContains(ids, filterValue)
	}
}
