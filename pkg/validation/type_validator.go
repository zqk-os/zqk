package validation

import (
	"reflect"
	"regexp"
	"time"
)

var (
	// Precompiled regexes for hot-path validation.
	// NOTE: These are intentionally a subset of ISO-8601, but include common variants observed in system data:
	// - 'T' or space separator
	// - optional fractional seconds
	// - 'Z' or numeric timezone offsets
	reISODate     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	reISODateTime = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$`)
)

// ValidateType checks if a value matches the expected type.
// This is a shared implementation used by both InstanceValidator and GoValidator
// to ensure consistent type validation across all validators.
//
// Supported types:
// - string, xsd:string
// - int, integer, xsd:integer (accepts int, int64, and float64 for JSON compatibility)
// - float, number, xsd:double, xsd:float
// - bool, boolean, xsd:boolean
// - list, array
// - object, map
// - date, xsd:date (ISO-8601 format: YYYY-MM-DD)
// - datetime, xsd:dateTime (ISO-8601 format or time.Time)
// - text (string without length validation)
// - enum (validated separately)
// ValidateType validates a value against an expected type
//
//nolint:gocyclo // Function handles many type cases; complexity reduced via helper functions
func ValidateType(value any, expectedType string) bool {
	switch expectedType {
	case "string", "xsd:string":
		return validateStringType(value)

	case "int", "integer", "xsd:integer":
		return validateIntegerType(value)

	case "float", "xsd:double", "xsd:float":
		return validateFloatType(value)
	case "number":
		return validateNumberType(value)

	case "bool", "boolean", "xsd:boolean":
		return validateBooleanType(value)

	case "list", "array":
		return validateListType(value)

	case "object", "map":
		return validateObjectType(value)

	case "date", "xsd:date":
		return validateDateType(value)

	case "datetime", "xsd:dateTime":
		return validateDateTimeType(value)

	case "text":
		return validateTextType(value)

	case "enum":
		return true // Enum values are validated separately

	default:
		return true // Unknown type - permissive
	}
}

// validateStringType validates string type
func validateStringType(value any) bool {
	_, ok := value.(string)
	return ok
}

// validateIntegerType validates integer type (accepts int, int64, and float64 from JSON)
func validateIntegerType(value any) bool {
	if value == nil {
		return false
	}

	// Accept all integer kinds (including uint) for YAML/Go compatibility.
	// Also accept float64/float32 since JSON numbers commonly decode to floats.
	// NOTE: This remains permissive (does not require float to be integral).
	val := reflect.ValueOf(value)
	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return true
	case reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

// validateFloatType validates float type (strict - only accepts float32/float64)
func validateFloatType(value any) bool {
	if value == nil {
		return false
	}

	val := reflect.ValueOf(value)
	switch val.Kind() {
	case reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

// validateNumberType validates number type (permissive - accepts all numeric types)
// "number" should accept all numeric types that can appear from YAML/JSON decoding
func validateNumberType(value any) bool {
	if value == nil {
		return false
	}

	val := reflect.ValueOf(value)
	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return true
	case reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

// validateBooleanType validates boolean type
func validateBooleanType(value any) bool {
	_, ok := value.(bool)
	return ok
}

// validateListType validates list/array type
func validateListType(value any) bool {
	val := reflect.ValueOf(value)
	return val.Kind() == reflect.Slice || val.Kind() == reflect.Array
}

// validateObjectType validates object/map type
func validateObjectType(value any) bool {
	val := reflect.ValueOf(value)
	return val.Kind() == reflect.Map
}

// validateDateType validates date type (ISO-8601 format: YYYY-MM-DD)
func validateDateType(value any) bool {
	str, ok := value.(string)
	if !ok {
		return false
	}
	return reISODate.MatchString(str)
}

// validateDateTimeType validates datetime type (handles both string and time.Time)
func validateDateTimeType(value any) bool {
	switch
	// Handle string format
	v := value.(type) {
	case string:
		return reISODateTime.MatchString(v)
	case

		// Check if it's a time.Time object (from YAML parsing)
		time.Time:
		return true
	}

	// Fallback to reflection check
	val := reflect.ValueOf(value)
	if val.Kind() == reflect.Struct {
		timeType := reflect.TypeOf(time.Time{})
		if val.Type() == timeType {
			return true
		}
	}

	return false
}

// validateTextType validates text type (string with no length validation)
func validateTextType(value any) bool {
	_, ok := value.(string)
	return ok
}
