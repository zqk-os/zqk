package crud

import (
	"github.com/lanceman/zqk/pkg/errfmt"
	"slices"
	"time"
)

func ParseTimestampForFilter(value any) (time.Time, error) {
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
		return time.Time{}, errfmt.Errorf("cannot parse timestamp from type %T", value)
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
	if t, err := time.Parse("20060102T150405", timeStr); err == nil {
		return t, nil
	}

	// Try date only (YYYY-MM-DD)
	if t, err := time.Parse("2006-01-02", timeStr); err == nil {
		return t, nil
	}

	return time.Time{}, errfmt.Errorf("unable to parse timestamp: %s", timeStr)
}

func IsDateSemanticType(semanticType string) bool {
	return semanticType == "timestamp" || semanticType == "date" || semanticType == "datetime"
}

// valueInList checks if a value is in a list
func ValueInList(actualValue, filterValue any) bool {
	// filterValue should be a slice/array
	switch v := filterValue.(type) {
	case []any:
		for _, item := range v {
			if CompareEqual(actualValue, item) {
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
func ArrayContains(actualValue, filterValue any) bool {
	// actualValue should be a slice/array
	switch v := actualValue.(type) {
	case []any:
		for _, item := range v {
			if CompareEqual(item, filterValue) {
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
func ArrayContainsAll(actualValue, filterValue any) bool {
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
			if CompareEqual(arrayValue, checkValue) {
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
func ArrayContainsAny(actualValue, filterValue any) bool {
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
			if CompareEqual(arrayValue, checkValue) {
				return true
			}
		}
	}
	return false
}
