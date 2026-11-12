package stringutil

import (
	"github.com/zqk-os/zqk/pkg/strutil"
)

// ErrorOnBlank checks whether the provided string value is blank (empty or whitespace-only).
// If blank, it returns an error formatted using format and optional args.
// If not blank, it returns nil.
// This enables declarative, functional error handling in lieu of procedural
// 'if strings.TrimSpace(s) == "" { return fmt.Errorf(...) }'.
func ErrorOnBlank(val, format string, args ...any) error {
	return strutil.ErrorOnBlank(val, format, args...)
}

// RequireNonBlank returns an error if trimmed val is empty, formatted as "<name> cannot be blank".
func RequireNonBlank(val, name string) error {
	return strutil.RequireNonBlank(val, name)
}

// IsBlank returns true if s is empty or consists solely of whitespace characters.
func IsBlank(s string) bool {
	return strutil.IsBlank(s)
}

// IsNonBlank returns true if s contains at least one non-whitespace character.
func IsNonBlank(s string) bool {
	return strutil.IsNonBlank(s)
}
