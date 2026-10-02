// Extracted from object_storage_dynamic_test_helper.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

func extractMinMaxLengths(validation map[string]any) (int, int) {
	var minLength, maxLength int
	switch v := validation["min_length"].(type) {
	case int:
		minLength = v
	case float64:
		minLength = int(v)
	}
	switch v := validation["max_length"].(type) {
	case int:
		maxLength = v
	case float64:
		maxLength = int(v)
	}
	return minLength, maxLength
}

func toAnySlice[T any](values []T) []any {
	result := make([]any, len(values))
	for i, v := range values {
		result[i] = v
	}
	return result
}

func generateStringBoundaryValues(fieldName string, validation map[string]any, required bool) []any {
	_ = fieldName // Reserved for future use
	values := []any{}

	minLength, maxLength := extractMinMaxLengths(validation)

	// Check for enum first
	if enum, ok := validation["enum"].([]any); ok && len(enum) > 0 {
		for _, val := range enum {
			if str, ok := val.(string); ok {
				values = append(values, str)
			}
		}
		if len(values) > 0 {
			return values // Use enum values if available
		}
	}

	// Check for pattern
	hasPattern := false
	if pattern := objects.GetString(validation, "pattern"); pattern != emptyValue {
		hasPattern = true
		// Generate pattern-matching values
		if strings.Contains(pattern, ConstStreamD4D2D2) {
			values = append(values, testDateFixed)
		}
		if strings.Contains(pattern, ConstStreamTD2D2D2Z) {
			values = append(values, testDateTimeFixed)
		}
		if strings.Contains(pattern, ConstStreamDDD) {
			values = append(values, objects.DefaultSchemaVersion)
		}
		if strings.Contains(pattern, "^[A-Z]+-\\d") {
			values = append(values, "TEST-001")
		}
		if strings.Contains(pattern, "account:") {
			values = append(values, "ACC-TEST")
		}
	}

	// Generate boundary values for length-constrained strings
	if maxLength > 0 || minLength > 0 {
		// 1. Value at min length (if min specified)
		if minLength > 0 {
			values = append(values, strings.Repeat("a", minLength))
		}
		// 2. Value at max length (if max specified)
		if maxLength > 0 {
			values = append(values, strings.Repeat("a", maxLength))
		}
		// 3. Value in middle of range
		if minLength > 0 && maxLength > 0 {
			mid := (minLength + maxLength) / 2
			values = append(values, strings.Repeat("a", mid))
		}
		// 4. Alphanumeric value within bounds
		if maxLength > 0 {
			length := maxLength / 2
			if length < minLength {
				length = minLength
			}
			if length > 50 {
				length = 20 // Reasonable length
			}
			values = append(values, fmt.Sprintf(ConstStreamTestValueIntAbc, length))
		}
	} else {
		// No length constraints - test various values
		if !required {
			values = append(values, "") // Empty string (if not required)
		}
		values = append(values,
			"test",                         // Short
			testValueNumbered,              // Alphanumeric
			ConstStreamTestValueWithSpaces, // With spaces
			"123456789",                    // Numeric string
			ConstStreamTestvalue123Abc)     // Mixed case alphanumeric
	}

	// If no values generated yet, use defaults
	if len(values) == 0 {
		if !required && !hasPattern {
			values = append(values, "") // Empty string
		}
		values = append(values, testValueNumbered)
	}

	return values
}

// generateIntegerBoundaryValues generates integer values testing boundaries
func generateIntegerBoundaryValues(validation map[string]any, required bool) []any {
	values := []int{}

	var minVal, maxVal int
	switch v := validation["min"].(type) {
	case int:
		minVal = v
	case float64:
		minVal = int(v)
	}
	switch v := validation["max"].(type) {
	case int:
		maxVal = v
	case float64:
		maxVal = int(v)
	}

	if maxVal > 0 && minVal >= 0 {
		// Both bounds specified
		values = append(values, minVal, maxVal, (minVal+maxVal)/2) // At minimum, maximum, and middle
		if minVal < maxVal {
			values = append(values, minVal+1, maxVal-1) // Just above min, just below max
		}
	}
	if maxVal <= 0 && minVal > 0 {
		// Only min specified
		values = append(values, minVal, minVal+1, minVal+10)
	}
	if maxVal > 0 && minVal < 0 {
		// Only max specified
		values = append(values, maxVal, maxVal-1, maxVal/2)
	}
	if maxVal <= 0 && minVal < 0 {
		// No constraints
		if !required {
			values = append(values, 0, 1, 42, 100) // Zero, small positive, typical, larger
		} else {
			values = append(values, 1, 42, 100) // Small positive, typical, larger
		}
	}

	return toAnySlice(values)
}

// generateNumberBoundaryValues generates number/float values testing boundaries
func generateNumberBoundaryValues(validation map[string]any, required bool) []any {
	values := []float64{}

	var minVal, maxVal float64
	switch v := validation["min"].(type) {
	case float64:
		minVal = v
	case int:
		minVal = float64(v)
	}
	switch v := validation["max"].(type) {
	case float64:
		maxVal = v
	case int:
		maxVal = float64(v)
	}

	if maxVal > 0 && minVal >= 0 {
		// Both bounds specified
		values = append(values, minVal, maxVal, (minVal+maxVal)/2.0) // At minimum, maximum, and middle
		if minVal < maxVal {
			values = append(values, minVal+0.1, maxVal-0.1) // Just above min, just below max
		}
	}
	if maxVal <= 0 && minVal > 0 {
		// Only min specified
		values = append(values, minVal, minVal+1.0, minVal+10.0)
	}
	if maxVal > 0 && minVal < 0 {
		// Only max specified
		values = append(values, maxVal, maxVal-1.0, maxVal/2.0)
	}
	if maxVal <= 0 && minVal < 0 {
		// No constraints
		if !required {
			values = append(values, 0.0, 1.0, 42.0, 100.0) // Zero, small positive, typical, larger
		} else {
			values = append(values, 1.0, 42.0, 100.0) // Small positive, typical, larger
		}
	}

	return toAnySlice(values)
}

// generateListBoundaryValues generates list/array values testing boundaries
func generateListBoundaryValues(validation map[string]any, required bool) []any {
	values := []any{}
	minLength, maxLength := extractMinMaxLengths(validation)

	// Generate lists with different lengths
	if maxLength > 0 || minLength > 0 {
		// At minimum length
		if minLength >= 0 {
			minList := make([]string, minLength)
			for i := 0; i < minLength; i++ {
				minList[i] = fmt.Sprintf(testItemFmt, i+1)
			}
			values = append(values, minList)
		}
		// At maximum length
		if maxLength > 0 {
			maxList := make([]string, maxLength)
			for i := 0; i < maxLength; i++ {
				maxList[i] = fmt.Sprintf(testItemFmt, i+1)
			}
			values = append(values, maxList)
		}
		// Middle length
		if minLength > 0 && maxLength > 0 {
			mid := (minLength + maxLength) / 2
			midList := make([]string, mid)
			for i := 0; i < mid; i++ {
				midList[i] = fmt.Sprintf(testItemFmt, i+1)
			}
			values = append(values, midList)
		}
	} else {
		// No length constraints
		if !required {
			values = append(values,
				[]string{},                     // Empty list
				[]string{testItemPrefix + "1"}, // Single item
				[]string{testItemPrefix + "1", testItemPrefix + "2"},                       // Two items
				[]string{testItemPrefix + "1", testItemPrefix + "2", testItemPrefix + "3"}) // Three items
		} else {
			values = append(values,
				[]string{testItemPrefix + "1"},                                             // Single item
				[]string{testItemPrefix + "1", testItemPrefix + "2"},                       // Two items
				[]string{testItemPrefix + "1", testItemPrefix + "2", testItemPrefix + "3"}) // Three items
		}
	}

	// If no values generated, use default
	if len(values) == 0 {
		values = append(values, []string{testItemPrefix + "1", testItemPrefix + "2"})
	}

	return values
}

// setRequiredFieldsForTest sets required fields for test object creation
