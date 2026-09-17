package strutil

import "strings"

const emptyString = ""

// OrDefault returns value if it is non-empty, otherwise defaultVal.
// Use for "set or default" patterns where a desired value may be empty and a fallback is needed.
//
// Example:
//
//	profile := strutil.OrDefault(ctx.Profile, "system")
//	kind := strutil.OrDefault(kindFromConfig, baseName)
func OrDefault(value, defaultVal string) string {
	if value != emptyString {
		return value
	}
	return defaultVal
}

// DefaultWhenNonEmpty returns defaultVal when value is non-empty, otherwise value.
// Negated case of OrDefault: use the default when a value is present, otherwise keep the (empty) value.
//
// Example: prefer a fallback when the caller supplied something, else leave empty.
//
//	display := strutil.DefaultWhenNonEmpty(userOverride, computed)
func DefaultWhenNonEmpty(value, defaultVal string) string {
	if value != emptyString {
		return defaultVal
	}
	return value
}

// PtrOrDefault returns the dereferenced value when v is non-nil, otherwise defaultVal.
// Use for "set or default" with pointers (nil is treated like empty for strings).
//
// Example:
//
//	name := strutil.PtrOrDefault(opts.Name, "unnamed")
func PtrOrDefault[T any](v *T, defaultVal T) T {
	if v != nil {
		return *v
	}
	return defaultVal
}

// FirstNonNil returns the first non-nil pointer, or the last if all are nil.
// Pointer analogue of "first non-empty"; useful for optional config chain.
//
// Example:
//
//	cfg := strutil.FirstNonNil(override, fromEnv, fromFile)
func FirstNonNil[T any](ptrs ...*T) *T {
	for _, p := range ptrs {
		if p != nil {
			return p
		}
	}
	return nil
}

// SplitLines splits a string by line endings (\r\n or \n), handling Windows and Unix line endings correctly.
func SplitLines(s string) []string {
	normalized := strings.ReplaceAll(s, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return strings.Split(normalized, "\n")
}
