package builders

import "strings"

// FormatPatternForGoCode unescapes a pattern string from YAML for use in generated Go code.
// Patterns from YAML may have double backslashes (\\d) which need to be unescaped to single backslashes (\d).
// This happens because YAML stores patterns with escaped backslashes, but Go regexp expects single backslashes.
//
// The returned string is ready to be used in a Go raw string literal (backticks) for Pattern() calls.
//
// Example:
//
//	Input:  "^\\\\d{4}-\\\\d{2}-\\\\d{2}T\\\\d{2}:\\\\d{2}:\\\\d{2}Z$"
//	Output: "^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}Z$"
//	Usage:  Pattern(`^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}Z$`)
func FormatPatternForGoCode(pattern string) string {
	// Unescape double backslashes to single backslashes
	// This converts YAML-escaped patterns (\\d) to Go regexp patterns (\d)
	return strings.ReplaceAll(pattern, `\\`, `\`)
}
