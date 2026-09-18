package validation

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

// ResolveStringForPatternValidation returns a string suitable for pattern (e.g. datetime regex) validation.
// Accepts string, time.Time (YAML often unmarshals datetime as time.Time), or int64/int (Unix seconds).
// Other types return ("", false) so callers skip pattern validation.
func ResolveStringForPatternValidation(fieldValue any) (str string, ok bool) {
	return resolveStringForPatternValidationWithField("", fieldValue)
}

// ResolveStringForPatternValidationWithField returns a string for pattern validation, with field-specific normalization.
// Use this when validating so that schema_version (float64 from YAML) and datetime (Unix numbers) are normalized.
func ResolveStringForPatternValidationWithField(fieldName string, fieldValue any) (str string, ok bool) {
	return resolveStringForPatternValidationWithField(fieldName, fieldValue)
}

// resolveStringForPatternValidationWithField normalizes field values for pattern validation.
// When fieldName is "schema_version", float64 from YAML (e.g. 2.0) is formatted as "X.Y.Z".
// When the value is int64/float64 (Unix seconds), it is converted to RFC3339 for datetime patterns.
func resolveStringForPatternValidationWithField(fieldName string, fieldValue any) (str string, ok bool) {
	switch v := fieldValue.(type) {
	case string:
		// Field-specific normalization for common YAML/storage encodings.
		if fieldName != emptyValue {
			if dt, ok := normalizeUnixTimestampStringForDateTimeField(fieldName, v); ok {
				return dt, true
			}
			if fieldName == ConstMagicExtracted_0 {
				if sv, ok := normalizeSchemaVersionString(v); ok {
					return sv, true
				}
			}
		}
		return v, true
	case time.Time:
		return v.Format(time.RFC3339), true
	case int64:
		// YAML or stream may produce Unix seconds for datetime fields (plausible range only)
		if v >= 1e9 && v <= 1e12 {
			return time.Unix(v, 0).UTC().Format(time.RFC3339), true
		}
		return "", false
	case int:
		if v >= 1e9 && v <= 1e12 {
			return time.Unix(int64(v), 0).UTC().Format(time.RFC3339), true
		}
		return "", false
	case float64:
		// schema_version in YAML is often unmarshaled as float64 (e.g. 2.0.0 -> 2.0)
		if fieldName == ConstMagicExtracted_0 {
			return formatSchemaVersionFromFloat(v), true
		}
		// Unix seconds for datetime fields (plausible range: 2001–~33658)
		sec := int64(v)
		if float64(sec) == v && v >= 1e9 && v <= 1e12 {
			return time.Unix(sec, 0).UTC().Format(time.RFC3339), true
		}
		return "", false
	default:
		return "", false
	}
}

// formatSchemaVersionFromFloat formats a float64 from YAML (e.g. 2.0) as "X.Y.Z" for pattern ^\d+\.\d+\.\d+$.
func formatSchemaVersionFromFloat(f float64) string {
	if f < 0 {
		return "0.0.0"
	}
	major := int(f)
	if major > 999 {
		major = 999
	}
	rest := f - float64(major)
	minor := int(rest*10 + 0.5)
	if minor > 99 {
		minor = 99
	}
	return fmt.Sprintf("%d.%d.0", major, minor)
}

func isDateTimePatternField(fieldName string) bool {
	switch fieldName {
	case "created_at", "updated_at", "last_seen", "first_seen", ConstMagicExtracted_1, ConstMagicExtracted_2, ConstMagicExtracted_3:
		return true
	default:
		return false
	}
}

// normalizeUnixTimestampStringForDateTimeField converts numeric timestamp strings into RFC3339 for known datetime fields.
// Supports seconds (10-12 digits) and milliseconds (13 digits). Returns ok=false when input is not a plausible Unix timestamp.
func normalizeUnixTimestampStringForDateTimeField(fieldName, raw string) (string, bool) {
	if !isDateTimePatternField(fieldName) {
		return "", false
	}
	s := strings.TrimSpace(raw)
	if s == emptyValue {
		return "", false
	}
	// Allow fractional seconds like "1735689600.0" by truncating at '.' when suffix is all zeros.
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		intPart := s[:dot]
		frac := strings.TrimRight(s[dot+1:], "0")
		if frac != emptyValue {
			return "", false
		}
		s = intPart
	}
	if len(s) < 10 || len(s) > 13 {
		return "", false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return "", false
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return "", false
	}
	sec := n
	if len(s) == 13 {
		sec = n / 1000
	}
	if sec < 1e9 || sec > 1e12 {
		return "", false
	}
	return time.Unix(sec, 0).UTC().Format(time.RFC3339), true
}

// normalizeSchemaVersionString normalizes common abbreviated SemVer encodings (e.g. "2", "2.0", "major.minor.patch").
// Fully expanded values align with pkg/objects.DefaultSchemaVersion for the default instance schema.
// Returns ok=false when the input is not purely numeric dot-separated parts.
func normalizeSchemaVersionString(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == emptyValue {
		return "", false
	}
	parts := strings.Split(s, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return "", false
	}
	nums := make([]string, 0, 3)
	for _, p := range parts {
		if p == emptyValue {
			return "", false
		}
		for i := 0; i < len(p); i++ {
			if p[i] < '0' || p[i] > '9' {
				return "", false
			}
		}
		// Trim leading zeros for stability, but keep a single "0".
		p = strings.TrimLeft(p, "0")
		if p == emptyValue {
			p = "0"
		}
		nums = append(nums, p)
	}
	for len(nums) < 3 {
		nums = append(nums, "0")
	}
	return strings.Join(nums, "."), true
}

// ValidateRequiredField validates that a required field is present and not empty.
// This is a shared utility used by all validators to ensure consistent required field validation.
//
// An empty value is defined as:
// - nil
// - empty string ""
// - empty slice/array (length 0)
// - empty map (length 0)
//
// Returns a ValidationError if the field is required but empty, nil otherwise.
func ValidateRequiredField(fieldName string, fieldValue any, exists bool, checkEmptyCollections bool) *ValidationError {
	if !exists || fieldValue == nil {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf(ConstMagicf14fc058, fieldName),
			Rule:    "required",
		}
	}

	// Check for empty string
	if strValue, ok := fieldValue.(string); ok {
		trimmed := strings.TrimSpace(strValue)
		if trimmed == emptyValue {
			return &ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf(ConstMagicf14fc058, fieldName),
				Rule:    "required",
			}
		}
		if fieldName == objects.FieldKeyDescription {
			lower := strings.ToLower(trimmed)
			switch lower {
			case "required", "todo", "tbd", "none", "null", "n/a", "placeholder", "title":
				return &ValidationError{
					Field:   fieldName,
					Message: fmt.Sprintf("field '%s' cannot be a placeholder (%q)", fieldName, trimmed),
					Rule:    "placeholder",
				}
			}
			if len(trimmed) < 10 {
				return &ValidationError{
					Field:   fieldName,
					Message: fmt.Sprintf("field '%s' is too short (min 10 characters required)", fieldName),
					Rule:    "minLength",
				}
			}
		}
	}

	// Check for empty collections if requested (SHACL minCount behavior)
	if checkEmptyCollections {
		val := reflect.ValueOf(fieldValue)
		if (val.Kind() == reflect.Slice || val.Kind() == reflect.Array) && val.Len() == 0 {
			return &ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf(ConstMagicedfcb684, fieldName),
				Rule:    "minCount",
			}
		}
		if val.Kind() == reflect.Map && val.Len() == 0 {
			return &ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf(ConstMagicedfcb684, fieldName),
				Rule:    "minCount",
			}
		}
	}

	return nil
}

// ValidateEnumField validates that a field value is in the allowed enum values.
// This is a shared utility used by all validators to ensure consistent enum validation.
//
// Supports case-insensitive string comparison for better user experience.
// Returns a ValidationError if the value is not in the enum, nil otherwise.
func ValidateEnumField(fieldName string, fieldValue any, enumValues []any) *ValidationError {
	if len(enumValues) == 0 {
		return nil // No enum constraint
	}

	// Check for exact match first
	for _, allowed := range enumValues {
		if reflect.DeepEqual(fieldValue, allowed) {
			return nil
		}
	}

	// Handle string case-insensitive comparison
	if strValue, ok := fieldValue.(string); ok {
		for _, allowed := range enumValues {
			if enumStr, ok := allowed.(string); ok {
				if strings.EqualFold(strValue, enumStr) {
					return nil
				}
			}
		}
	}

	// Value not found in enum
	enumStr := formatEnumValues(enumValues)
	return &ValidationError{
		Field:   fieldName,
		Message: fmt.Sprintf(ConstMagic07f529da, fieldName, enumStr),
		Rule:    "enum",
	}
}

// formatEnumValues formats enum values for error messages
func formatEnumValues(values []any) string {
	if len(values) == 0 {
		return ""
	}
	if len(values) == 1 {
		return fmt.Sprintf("%v", values[0])
	}
	if len(values) <= 5 {
		strs := make([]string, len(values))
		for i, v := range values {
			strs[i] = fmt.Sprintf("%v", v)
		}
		return strings.Join(strs, ", ")
	}
	// Too many values - show first few
	strs := make([]string, 5)
	for i := 0; i < 5; i++ {
		strs[i] = fmt.Sprintf("%v", values[i])
	}
	return strings.Join(strs, ", ") + ", ..."
}

// IsEmptyValue checks if a value is considered empty for validation purposes.
// This is a shared utility for consistent empty value detection.
//
// Empty values are:
// - nil
// - empty string ""
// - empty slice/array (length 0)
// - empty map (length 0)
func IsEmptyValue(value any) bool {
	if value == nil {
		return true
	}

	if strValue, ok := value.(string); ok && strValue == emptyValue {
		return true
	}

	val := reflect.ValueOf(value)
	if (val.Kind() == reflect.Slice || val.Kind() == reflect.Array) && val.Len() == 0 {
		return true
	}
	if val.Kind() == reflect.Map && val.Len() == 0 {
		return true
	}

	return false
}
