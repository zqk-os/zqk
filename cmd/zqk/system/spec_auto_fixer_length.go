package system

import (
	"reflect"
	"strings"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// extractLengthConstraint extracts a length constraint (min_length or max_length) from validation
// Returns (value, found)
func extractLengthConstraint(validation map[string]any, constraintName string) (int, bool) {
	var length int
	switch val := validation[constraintName].(type) {
	case int:
		length = val
		return length, true
	case float64:
		length = int(val)
		return length, true
	default:
		return 0, false
	}
}

// truncateToDisplayLength truncates a string value to the display_length constraint
// NOTE: Currently unused - display_length auto-fix is skipped (display fields are created on the fly)
// Kept for potential future use or reference
//
//nolint:unused // Reserved for potential future use
func truncateToDisplayLength(fieldName string, currentValue any, fieldMap map[string]any) any {
	strValue, ok := currentValue.(string)
	if !ok {
		return nil
	}

	validation, ok := getValidationMap(fieldMap)
	if !ok {
		return nil
	}

	var displayLength int
	switch dl := validation["display_length"].(type) {
	case int:
		displayLength = dl
	case float64:
		displayLength = int(dl)
	default:
		// Default display length when not specified
		displayLength = 50
	}

	if len(strValue) <= displayLength {
		return nil // No truncation needed
	}

	// Truncate with ellipsis if needed
	truncated := strValue[:displayLength]
	if displayLength > 3 {
		truncated = strValue[:displayLength-3] + "..."
	}
	return truncated
}

// padToMinLength pads a string value to meet the minimum length constraint
func padToMinLength(fieldName string, currentValue any, fieldMap map[string]any, logger logging.Logger) any {
	strValue, ok := currentValue.(string)
	if !ok {
		return nil
	}

	validation, ok := getValidationMap(fieldMap)
	if !ok {
		return nil
	}

	minLength, ok := extractLengthConstraint(validation, "min_length")
	if !ok {
		return nil // No min_length constraint
	}

	if len(strValue) >= minLength {
		return nil // No padding needed
	}

	// Pad with spaces to meet minimum length
	padded := strValue + strings.Repeat(" ", minLength-len(strValue))
	logging.Fluent(logger).Debug("Padded string to meet min_length").
		String("field", fieldName).
		Int("original_length", len(strValue)).
		Int("min_length", minLength).
		Log()
	return padded
}

// truncateToMaxLength truncates a string value to the maximum length constraint
func truncateToMaxLength(fieldName string, currentValue any, fieldMap map[string]any, logger logging.Logger) any {
	strValue, ok := currentValue.(string)
	if !ok {
		return nil
	}

	validation, ok := getValidationMap(fieldMap)
	if !ok {
		return nil
	}

	maxLength, ok := extractLengthConstraint(validation, "max_length")
	if !ok {
		return nil // No max_length constraint
	}

	if len(strValue) <= maxLength {
		return nil // No truncation needed
	}

	// Truncate to max length
	truncated := strValue[:maxLength]
	logging.Fluent(logger).Debug("Truncated string to meet max_length").
		String("field", fieldName).
		Int("original_length", len(strValue)).
		Int("max_length", maxLength).
		Log()
	return truncated
}

// padArrayToMinLength pads an array to meet the minimum length constraint
func padArrayToMinLength(fieldName string, currentValue any, fieldMap map[string]any, logger logging.Logger) any {
	val := reflect.ValueOf(currentValue)
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		return nil
	}

	validation, ok := getValidationMap(fieldMap)
	if !ok {
		return nil
	}

	minLength, ok := extractLengthConstraint(validation, "min_length")
	if !ok {
		return nil // No min_length constraint
	}

	currentLen := val.Len()
	if currentLen >= minLength {
		return nil // No padding needed
	}

	// Convert to slice for modification
	slice := make([]any, currentLen)
	for i := 0; i < currentLen; i++ {
		slice[i] = val.Index(i).Interface()
	}

	// Get element type from spec to create default values
	fieldType, _ := fieldMap[objects.FieldKeyType].(string)
	var defaultValue any
	if fieldType == "list" || fieldType == "array" {
		// Try to get item type from spec
		if items, ok := fieldMap["items"].(map[string]any); ok {
			if itemDefault := extractDefaultValue(items); itemDefault != nil {
				defaultValue = itemDefault
			}
		}
		// If no default, use empty string for string lists, 0 for number lists, etc.
		if defaultValue == nil {
			defaultValue = "" // Default to empty string for list items
		}
	}

	// Pad array with default values
	for i := currentLen; i < minLength; i++ {
		slice = append(slice, defaultValue)
	}

	logging.Fluent(logger).Debug("Padded array to meet min_length").
		String("field", fieldName).
		Int("original_length", currentLen).
		Int("min_length", minLength).
		Log()
	return slice
}

// truncateArrayToMaxLength truncates an array to meet the maximum length constraint
func truncateArrayToMaxLength(fieldName string, currentValue any, fieldMap map[string]any, logger logging.Logger) any {
	val := reflect.ValueOf(currentValue)
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		return nil
	}

	validation, ok := getValidationMap(fieldMap)
	if !ok {
		return nil
	}

	maxLength, ok := extractLengthConstraint(validation, "max_length")
	if !ok {
		return nil // No max_length constraint
	}

	currentLen := val.Len()
	if currentLen <= maxLength {
		return nil // No truncation needed
	}

	// Convert to slice and truncate
	slice := make([]any, maxLength)
	for i := 0; i < maxLength; i++ {
		slice[i] = val.Index(i).Interface()
	}

	logging.Fluent(logger).Debug("Truncated array to meet max_length").
		String("field", fieldName).
		Int("original_length", currentLen).
		Int("max_length", maxLength).
		Log()
	return slice
}
