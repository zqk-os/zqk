package object

import (
	"maps"
	"strconv"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
)

// normalizeValueForField converts a value to the appropriate type based on field definition
// This handles cases where YAML parsing converts whole-number floats to ints,
// or where we need to ensure values match expected field types
func normalizeValueForField(value any, fieldInfo *objects.FieldInfo) (any, error) {
	if fieldInfo == nil {
		return value, nil
	}

	// Handle number type conversions
	// For "number" or "float" types, convert int to float64
	// This handles YAML parsing where whole-number floats become ints
	if fieldInfo.Type == "number" || fieldInfo.Type == "float" {
		// Convert int to float64 for number/float types
		switch v := value.(type) {
		case int:
			return float64(v), nil
		case int32:
			return float64(v), nil
		case int64:
			return float64(v), nil
		case float64:
			return v, nil
		case float32:
			return float64(v), nil
		case string:
			if floatVal, err := strconv.ParseFloat(v, 64); err == nil {
				return floatVal, nil
			}
		}
	}

	// Handle integer type - ensure it's an int
	if fieldInfo.Type == "integer" {
		switch v := value.(type) {
		case float64:
			// Check if it's a whole number
			if v == float64(int64(v)) {
				return int(v), nil
			}
		case int:
			return v, nil
		}
	}

	// Handle list/array types
	if fieldInfo.Type == "list" || fieldInfo.Type == "array" {
		if slice, ok := value.([]any); ok {
			// Recursively normalize each element if we have field info for items
			normalized := make([]any, len(slice))
			copy(normalized, slice) // For now, don't normalize items (would need item field info)
			return normalized, nil
		}
	}

	// For other types, return as-is
	return value, nil
}

// NormalizeObjectValues normalizes all values in an object map based on field definitions.
// If the field registry is not already loaded, returns the object unchanged to avoid
// LoadFields() on the hot path (object update/create), which would load all 83 specs
// and cause 30–90+ second latency and memory spikes (PRE_CHANGE_CHECKLIST §3).
func NormalizeObjectValues(obj map[string]any, kind string) (map[string]any, error) {
	normalizedObj := make(map[string]any)
	maps.Copy(normalizedObj, obj)

	fieldRegistry := objects.GetGlobalFieldRegistry()
	kindFields, ok := fieldRegistry.GetFieldsForKindIfLoaded(kind)
	if !ok {
		// Registry not loaded — skip normalization rather than triggering LoadFields()
		return obj, nil
	}

	return normalizedObj, normalizeObjectValues(normalizedObj, kindFields)
}

// normalizeObjectValues normalizes all values in an object based on field definitions
// This ensures that values match expected types, especially for number fields
func normalizeObjectValues(obj map[string]any, kindFields *objects.KindFields) error {
	if kindFields == nil {
		return nil
	}

	// Create a map of field name to field info for quick lookup
	fieldMap := make(map[string]*objects.FieldInfo)
	for i := range kindFields.AllFields {
		field := &kindFields.AllFields[i]
		fieldMap[field.Name] = field
	}

	// Normalize each field value
	for fieldName, value := range obj {
		if fieldInfo, ok := fieldMap[fieldName]; ok {
			normalized, err := normalizeValueForField(value, fieldInfo)
			if err != nil {
				return errfmt.Errorf("failed to normalize field %s: %w", fieldName, err)
			}
			obj[fieldName] = normalized
		}
	}

	return nil
}
