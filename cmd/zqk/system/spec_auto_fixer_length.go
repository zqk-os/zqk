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

func extractStringLengthConstraint(currentValue any, fieldMap map[string]any, constraintName string) (string, int, bool) {
	strValue, ok := currentValue.(string)
	if !ok {
		return "", 0, false
	}
	validation, ok := getValidationMap(fieldMap)
	if !ok {
		return "", 0, false
	}
	length, ok := extractLengthConstraint(validation, constraintName)
	if !ok {
		return "", 0, false
	}
	return strValue, length, true
}

func extractArrayLengthConstraint(currentValue any, fieldMap map[string]any, constraintName string) (reflect.Value, int, bool) {
	val := reflect.ValueOf(currentValue)
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		return reflect.Value{}, 0, false
	}
	validation, ok := getValidationMap(fieldMap)
	if !ok {
		return reflect.Value{}, 0, false
	}
	length, ok := extractLengthConstraint(validation, constraintName)
	if !ok {
		return reflect.Value{}, 0, false
	}
	return val, length, true
}

// padToMinLength pads a string value to meet the minimum length constraint
func padToMinLength(fieldName string, currentValue any, fieldMap map[string]any, logger logging.Logger) any {
	strValue, minLength, ok := extractStringLengthConstraint(currentValue, fieldMap, "min_length")
	if !ok || len(strValue) >= minLength {
		return nil
	}

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
	strValue, maxLength, ok := extractStringLengthConstraint(currentValue, fieldMap, "max_length")
	if !ok || len(strValue) <= maxLength {
		return nil
	}

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
	val, minLength, ok := extractArrayLengthConstraint(currentValue, fieldMap, "min_length")
	if !ok {
		return nil
	}

	currentLen := val.Len()
	if currentLen >= minLength {
		return nil
	}

	slice := make([]any, currentLen)
	for i := 0; i < currentLen; i++ {
		slice[i] = val.Index(i).Interface()
	}

	fieldType, ok := fieldMap[objects.FieldKeyType].(string)
	var defaultValue any
	if ok && (fieldType == "list" || fieldType == "array") {
		if items, hasItems := fieldMap["items"].(map[string]any); hasItems {
			if itemDefault := extractDefaultValue(items); itemDefault != nil {
				defaultValue = itemDefault
			}
		}
		if defaultValue == nil {
			defaultValue = ""
		}
	}

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
	val, maxLength, ok := extractArrayLengthConstraint(currentValue, fieldMap, "max_length")
	if !ok {
		return nil
	}

	currentLen := val.Len()
	if currentLen <= maxLength {
		return nil
	}

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
