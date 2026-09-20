package system

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

// coerceType normalizes a value to the field's declared wire type (string/object/…).
// This is persistence/validation shape — not inventing business categories or linked-object values.
func coerceType(fieldName string, currentValue any, fieldMap map[string]any, logger logging.Logger) any {
	// Resolved spec fields usually store "type" as a string, but some inheritance/merge
	// paths may represent it as other shapes. Normalize to a best-effort string.
	var fieldType string
	switch v := fieldMap[objects.FieldKeyType].(type) {
	case string:
		fieldType = v
	case []any:
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				fieldType = s
			}
		}
	case map[string]any:
		// Some loaders may nest the type under an inner key.
		if s, ok := v[objects.FieldKeyType].(string); ok {
			fieldType = s
		} else if s, ok := v[objects.FieldKeyName].(string); ok {
			fieldType = s
		}
	case map[any]any:
		if s, ok := v[objects.FieldKeyType].(string); ok {
			fieldType = s
		} else if s, ok := v[objects.FieldKeyName].(string); ok {
			fieldType = s
		}
	default:
		// Keep fieldType empty to indicate an unrecognized structure.
	}

	if fieldType == emptyValue {
		return nil
	}

	if currentValue == nil {
		// Some persistence paths may omit optional object/map fields entirely while the
		// validator still reports a datatype mismatch. For object/map fields, treat a
		// missing value as an empty object so auto-fix can make progress.
		switch fieldType {
		case "object", "map":
			return map[string]any{}
		default:
			return nil
		}
	}

	// Get validation rules for additional type hints
	validation, _ := getValidationMap(fieldMap)

	switch fieldType {
	case "string", "xsd:string", "text":
		// Coerce to string
		if strValue := coerceToString(currentValue); strValue != nil {
			coercedStr := *strValue
			logging.Fluent(logger).Debug("Type coercion: converting to string").
				String("field", fieldName).
				String("original_type", fmt.Sprintf("%T", currentValue)).
				Log()

			// After successful type coercion, check if pattern fixing is needed
			// This handles cases where we fix *string -> string AND pattern in one pass
			if validation != nil {
				if pattern, ok := validation["pattern"].(string); ok {
					if fixedPattern := formatToPattern(coercedStr, pattern, logger); fixedPattern != nil {
						logging.Fluent(logger).Debug("Type coercion: also fixed pattern after type coercion").
							String("field", fieldName).
							String("pattern", pattern).
							Log()
						return *fixedPattern
					}
				}
			}
			return coercedStr
		}

	case "int", "integer", "xsd:integer":
		// Coerce to int
		if intValue := coerceToInt(currentValue); intValue != nil {
			logging.Fluent(logger).Debug("Type coercion: converting to int").
				String("field", fieldName).
				String("original_type", fmt.Sprintf("%T", currentValue)).
				Log()
			return *intValue
		}

	case "float", "number", "xsd:double", "xsd:float":
		// Coerce to float
		if floatValue := coerceToFloat(currentValue); floatValue != nil {
			logging.Fluent(logger).Debug("Type coercion: converting to float").
				String("field", fieldName).
				String("original_type", fmt.Sprintf("%T", currentValue)).
				Log()
			return *floatValue
		}

	case "bool", "boolean", "xsd:boolean":
		// Coerce to bool
		if boolValue := coerceToBool(currentValue); boolValue != nil {
			logging.Fluent(logger).Debug("Type coercion: converting to bool").
				String("field", fieldName).
				String("original_type", fmt.Sprintf("%T", currentValue)).
				Log()
			return *boolValue
		}

	case "datetime", "xsd:dateTime":
		// Coerce to datetime string (ISO-8601 format)
		if datetimeValue := coerceToDateTime(currentValue, logger); datetimeValue != nil {
			logging.Fluent(logger).Debug("Type coercion: converting to datetime").
				String("field", fieldName).
				String("original_type", fmt.Sprintf("%T", currentValue)).
				Log()
			return *datetimeValue
		}

	case "date", "xsd:date":
		// Coerce to date string (ISO-8601 format: YYYY-MM-DD)
		if dateValue := coerceToDate(currentValue, logger); dateValue != nil {
			logging.Fluent(logger).Debug("Type coercion: converting to date").
				String("field", fieldName).
				String("original_type", fmt.Sprintf("%T", currentValue)).
				Log()
			return *dateValue
		}

	case "object", "map":
		// Coerce to object (e.g. event_type_counts stored as JSON string -> map)
		if objValue := coerceToObject(currentValue, logger); objValue != nil {
			logging.Fluent(logger).Debug("Type coercion: converting to object").
				String("field", fieldName).
				String("original_type", fmt.Sprintf("%T", currentValue)).
				Log()
			return objValue
		}
	}

	// Check if validation has a pattern that might help with coercion
	if validation != nil {
		if pattern, ok := validation["pattern"].(string); ok {
			// Try to extract/format value to match pattern
			if formattedValue := formatToPattern(currentValue, pattern, logger); formattedValue != nil {
				logging.Fluent(logger).Debug("Type coercion: formatting to match pattern").
					String("field", fieldName).
					String("pattern", pattern).
					Log()
				return *formattedValue
			}
		}
	}

	return nil
}

// coerceToString attempts to coerce a value to string
func coerceToString(value any) *string {
	switch v := value.(type) {
	case string:
		return &v
	case *string:
		// Handle pointer to string - dereference it
		if v != nil {
			return v
		}
		return nil
	case int, int64, int32:
		str := fmt.Sprintf("%d", v)
		return &str
	case float64, float32:
		str := fmt.Sprintf("%g", v)
		return &str
	case bool:
		str := fmt.Sprintf("%v", v)
		return &str
	default:
		// For other types, try to convert to string
		// This handles cases where the value might be wrapped in an any
		str := fmt.Sprintf("%v", v)
		return &str
	}
}

// coerceToInt attempts to coerce a value to int
func coerceToInt(value any) *int {
	switch v := value.(type) {
	case int:
		return &v
	case int64:
		i := int(v)
		return &i
	case int32:
		i := int(v)
		return &i
	case float64:
		i := int(v)
		return &i
	case float32:
		i := int(v)
		return &i
	case string:
		// Try to parse string as int
		var i int
		if _, err := fmt.Sscanf(v, "%d", &i); err == nil {
			return &i
		}
	}
	return nil
}

// coerceToFloat attempts to coerce a value to float64
func coerceToFloat(value any) *float64 {
	switch v := value.(type) {
	case float64:
		return &v
	case float32:
		f := float64(v)
		return &f
	case int:
		f := float64(v)
		return &f
	case int64:
		f := float64(v)
		return &f
	case string:
		// Try to parse string as float
		var f float64
		if _, err := fmt.Sscanf(v, "%g", &f); err == nil {
			return &f
		}
	}
	return nil
}

// coerceToBool attempts to coerce a value to bool
func coerceToBool(value any) *bool {
	switch v := value.(type) {
	case bool:
		return &v
	case string:
		lower := strings.ToLower(v)
		if lower == "true" || lower == "1" || lower == "yes" {
			b := true
			return &b
		}
		if lower == "false" || lower == "0" || lower == "no" {
			b := false
			return &b
		}
	case int, int64:
		// 0 = false, non-zero = true
		if v != 0 {
			b := true
			return &b
		}
		b := false
		return &b
	}
	return nil
}

// coerceToDateTime attempts to coerce a value to datetime string (ISO-8601)
func coerceToDateTime(value any, logger logging.Logger) *string {
	if v, ok := value.(string); ok {
		// Try to parse and reformat to ISO-8601
		if dt := parseAndFormatDateTime(v, logger); dt != nil {
			return dt
		}
		// If already looks like ISO-8601, return as-is
		if strings.Contains(v, "T") && len(v) >= 19 {
			return &v
		}
	}
	return nil
}

// coerceToDate attempts to coerce a value to date string (YYYY-MM-DD)
func coerceToDate(value any, logger logging.Logger) *string {
	if v, ok := value.(string); ok {
		// Try to parse and reformat to YYYY-MM-DD
		if d := parseAndFormatDate(v, logger); d != nil {
			return d
		}
		// If already looks like YYYY-MM-DD, return as-is
		if len(v) == 10 && strings.Count(v, "-") == 2 {
			return &v
		}
	}
	return nil
}

// coerceToObject attempts to coerce a value to object (map[string]any).
// Handles event_type_counts and similar fields stored as JSON strings (expected object, got string).
func coerceToObject(value any, logger logging.Logger) map[string]any {
	if value == nil {
		return nil
	}
	switch
	// Already a map - return as map[string]any for spec consistency
	v := value.(type) {
	case map[string]any:
		return v
	case

		// String: try JSON unmarshal (common when field was serialized as string)
		string:
		v = strings.TrimSpace(v)
		if v == emptyValue || v == "{}" {
			return map[string]any{}
		}

		// Some persistence paths may store the JSON payload as an embedded/escaped string
		// (e.g. "\"{\\\"create\\\":1}\"") or wrap it in quotes. Normalize to the inner
		// JSON/YAML-ish representation before coercion.
		trimmed := v
		if len(trimmed) >= 2 {
			if (trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"') || (trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'') {
				if unq, err := strconv.Unquote(trimmed); err == nil {
					trimmed = unq
				} else if trimmed[0] == '\'' {
					// strconv.Unquote expects a Go string literal; accept YAML single quotes.
					trimmed = strings.TrimPrefix(strings.TrimSuffix(trimmed, "'"), "'")
				}
			}
		}
		trimmed = strings.TrimSpace(trimmed)
		switch strings.ToLower(trimmed) {
		case "",
			"{}", // explicit empty-object literal
			"null", "none", "~", "nil",
			"undefined", "unknown", "n/a", "na",
			"<nil>", "<none>", "<missing>",
			"(none)", "(null)",
			"map[]", "[]":
			return map[string]any{}
		}

		// 1) Direct JSON object -> map
		var out map[string]any
		if err := json.Unmarshal([]byte(trimmed), &out); err == nil && out != nil {
			return out
		}

		// 2) JSON string containing JSON object -> unwrap and decode again
		var inner any
		if err := json.Unmarshal([]byte(trimmed), &inner); err == nil {
			if innerStr, ok := inner.(string); ok {
				innerStr = strings.TrimSpace(innerStr)
				if innerStr == emptyValue || innerStr == "{}" {
					return map[string]any{}
				}
				var out2 map[string]any
				if err2 := json.Unmarshal([]byte(innerStr), &out2); err2 == nil && out2 != nil {
					return out2
				}
			}
		}

		// 3) YAML inline map/object inside string -> map
		var yamlOut map[string]any
		if err := yaml.Unmarshal([]byte(trimmed), &yamlOut); err == nil && yamlOut != nil {
			return yamlOut
		}

		logging.Fluent(logger).Debug("Coerce to object: failed to coerce string to map").
			String("value_preview", truncateForLog(v, 80)).
			Log()
		return nil
	}

	return nil
}

// truncateForLog returns s truncated to maxLen for logging.
func truncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// parseAndFormatDateTime parses various datetime formats and returns ISO-8601 string
func parseAndFormatDateTime(value string, logger logging.Logger) *string {
	value = strings.TrimSpace(value)
	if value == emptyValue {
		return nil
	}
	// Common datetime formats to try (including Go default time.String() / fmt %v output).
	// Space-separated wall times with numeric offsets (e.g. "2026-07-29 00:03:00+00:00")
	// show up from YAML/Python dumps and fail the auditable created_at pattern until normalized.
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05",
		"2006/01/02 15:04:05",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05 -0700 MST",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, value); err == nil {
			formatted := t.Format(time.RFC3339)
			return &formatted
		}
	}

	return nil
}

// parseAndFormatDate parses various date formats and returns YYYY-MM-DD string
func parseAndFormatDate(value string, logger logging.Logger) *string {
	// Common date formats to try
	formats := []string{
		"2006-01-02",
		"2006/01/02",
		"01/02/2006",
		"2006-1-2",
		time.RFC3339, // Extract date part from datetime
	}

	for _, format := range formats {
		if t, err := time.Parse(format, value); err == nil {
			formatted := t.Format("2006-01-02")
			return &formatted
		}
	}

	// Try extracting date from datetime string
	if strings.Contains(value, "T") {
		parts := strings.Split(value, "T")
		if len(parts) > 0 && len(parts[0]) == 10 {
			return &parts[0]
		}
	}

	return nil
}
