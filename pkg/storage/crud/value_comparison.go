package crud

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// CompareValues compares two values and returns:
// -1 if a < b
//
//	0 if a == b
//	1 if a > b
//
// This is a shared utility for value comparison used in filtering and sorting.
// It handles common types (string, int, int64, float64, time.Time) and
// falls back to string comparison for other types.
func CompareValues(a, b any) int {
	// Handle nil values
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	// Type-specific comparison
	switch vA := a.(type) {
	case string:
		vB, ok := b.(string)
		if !ok {
			return 0 // Different types, consider equal for comparison purposes
		}
		if vA < vB {
			return -1
		} else if vA > vB {
			return 1
		}
		return 0

	case int:
		vB, ok := b.(int)
		if !ok {
			return 0
		}
		if vA < vB {
			return -1
		} else if vA > vB {
			return 1
		}
		return 0

	case int64:
		vB, ok := b.(int64)
		if !ok {
			return 0
		}
		if vA < vB {
			return -1
		} else if vA > vB {
			return 1
		}
		return 0

	case float64:
		vB, ok := b.(float64)
		if !ok {
			return 0
		}
		if vA < vB {
			return -1
		} else if vA > vB {
			return 1
		}
		return 0

	case time.Time:
		vB, ok := b.(time.Time)
		if !ok {
			return 0
		}
		if vA.Before(vB) {
			return -1
		} else if vA.After(vB) {
			return 1
		}
		return 0

	default:
		// For other types, convert to string and compare
		strA := fmt.Sprintf("%v", vA)
		strB := fmt.Sprintf("%v", b)
		if strA < strB {
			return -1
		} else if strA > strB {
			return 1
		}
		return 0
	}
}

// CompareEqual checks if two values are equal.
// This is a shared utility for equality comparison used in filtering.
// It handles slices/arrays (not directly comparable with ==) and
// falls back to direct comparison for other types.
func CompareEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	switch

	// Handle slices/arrays (not directly comparable with ==)
	v := a.(type) {
	case []string:
		bVal, ok := b.([]string)
		if !ok {
			return false
		}
		return slices.Equal(v, bVal)
	}

	// Handle []any slices

	// For other types, use direct comparison
	return a == b
}

// StringContains checks if a string value contains a substring.
// This is a shared utility for string filtering operations.
func StringContains(actualValue, filterValue any) bool {
	actualStr, ok := actualValue.(string)
	if !ok {
		return false
	}
	filterStr, ok := filterValue.(string)
	if !ok {
		return false
	}
	return strings.Contains(actualStr, filterStr)
}

// StringStartsWith checks if a string value starts with a prefix.
// This is a shared utility for string filtering operations.
func StringStartsWith(actualValue, filterValue any) bool {
	actualStr, ok := actualValue.(string)
	if !ok {
		return false
	}
	filterStr, ok := filterValue.(string)
	if !ok {
		return false
	}
	return strings.HasPrefix(actualStr, filterStr)
}

// StringEndsWith checks if a string value ends with a suffix.
// This is a shared utility for string filtering operations.
func StringEndsWith(actualValue, filterValue any) bool {
	actualStr, ok := actualValue.(string)
	if !ok {
		return false
	}
	filterStr, ok := filterValue.(string)
	if !ok {
		return false
	}
	return strings.HasSuffix(actualStr, filterStr)
}
