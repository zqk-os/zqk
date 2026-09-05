// Extracted from object_storage_dynamic_test_helper.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
)

func generateDateTimeValue(validation map[string]any) string {
	// Check for pattern to determine format
	if pattern := objects.GetString(validation, "pattern"); pattern != "" {
		if strings.Contains(pattern, ConstStreamTD2D2D2Z) {
			// Full datetime
			return testDateTimeFixed
		}
		// Date only
		return testDateFixed
	}
	// Default: full datetime
	return testDateTimeFixed
}

// generateObjectValue generates an object value
func generateObjectValue(_ map[string]any) map[string]any {
	// Simple object with test data
	return map[string]any{
		"key": "value",
		"num": 42,
	}
}

// generateTestValuesWithBoundaries generates multiple test values including boundary conditions
// Returns a slice of values to try, ordered from most likely to succeed to edge cases
func generateTestValuesWithBoundaries(fieldName string, fieldDef any) []any {
	values := []any{}

	// Extract field definition map
	fieldMap, ok := fieldDef.(map[string]any)
	if !ok {
		return []any{testValueDefault} // Fallback
	}

	// Get field type
	fieldType, _ := fieldMap[objects.FieldKeyType].(string)

	// Extract validation constraints
	var validation map[string]any
	if val, ok := fieldMap["validation"].(map[string]any); ok {
		validation = val
	}

	// Check if field is required
	required := false
	if req, ok := validation["required"].(bool); ok {
		required = req
	}

	switch fieldType {
	case "string", "text":
		// Generate string values with boundaries
		values = generateStringBoundaryValues(fieldName, validation, required)
	case "integer":
		values = generateIntegerBoundaryValues(validation, required)
	case "number", "float":
		values = generateNumberBoundaryValues(validation, required)
	case "boolean":
		values = []any{true, false} // Test both boolean values
	case "list", "array":
		values = generateListBoundaryValues(validation, required)
	case "enum":
		// Use enum values from spec
		if enum, ok := validation["enum"].([]any); ok && len(enum) > 0 {
			values = append(values, enum...)
		}
		if len(values) == 0 {
			values = []any{"test"} // Fallback
		}
	case "datetime", "date":
		values = []any{generateDateTimeValue(validation)} // Datetime is usually constrained by pattern
	case "object":
		values = []any{generateObjectValue(validation)}
	default:
		values = []any{testValueDefault} // Fallback
	}

	// If no values generated, use a default
	if len(values) == 0 {
		values = []any{testValueDefault}
	}

	return values
}

// generateStringBoundaryValues generates string values testing boundaries
