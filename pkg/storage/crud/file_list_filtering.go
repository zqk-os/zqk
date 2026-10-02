package crud

import (
	"slices"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
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
func toAnySlice(val any) ([]any, bool) {
	switch v := val.(type) {
	case []any:
		return v, true
	case []string:
		res := make([]any, len(v))
		for i, s := range v {
			res[i] = s
		}
		return res, true
	default:
		return nil, false
	}
}

func toAnySlices(actualValue, filterValue any) ([]any, []any, bool) {
	valuesToCheck, ok := toAnySlice(filterValue)
	if !ok {
		return nil, nil, false
	}
	actualArray, ok := toAnySlice(actualValue)
	return valuesToCheck, actualArray, ok
}

// ArrayContainsAll checks if an array contains all specified values.
func ArrayContainsAll(actualValue, filterValue any) bool {
	valuesToCheck, actualArray, ok := toAnySlices(actualValue, filterValue)
	if !ok {
		return false
	}

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

// ArrayContainsAny checks if an array contains any of the specified values.
func ArrayContainsAny(actualValue, filterValue any) bool {
	valuesToCheck, actualArray, ok := toAnySlices(actualValue, filterValue)
	if !ok {
		return false
	}

	for _, checkValue := range valuesToCheck {
		for _, arrayValue := range actualArray {
			if CompareEqual(arrayValue, checkValue) {
				return true
			}
		}
	}
	return false
}
