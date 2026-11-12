package strutil

import (
	"errors"
	"fmt"
	"strings"
)

// ErrorOnBlank checks whether the provided string value is blank (empty or whitespace-only).
// If blank, it returns an error formatted using format and optional args.
// If not blank, it returns nil.
// This enables declarative, functional error handling in lieu of procedural
// 'if strings.TrimSpace(s) == "" { return fmt.Errorf(...) }'.
//
// Example:
//
//	if err := strutil.ErrorOnBlank(name, "agent name cannot be blank"); err != nil {
//	    return err
//	}
func ErrorOnBlank(val, format string, args ...any) error {
	if IsBlank(val) {
		if len(args) == 0 {
			if format == "" {
				return errors.New("value cannot be blank")
			}
			return errors.New(format)
		}
		return fmt.Errorf(format, args...)
	}
	return nil
}

// RequireNonBlank returns an error if trimmed val is empty, formatted as "<name> cannot be blank".
func RequireNonBlank(val, name string) error {
	if IsBlank(val) {
		name = strings.TrimSpace(name)
		if name == "" {
			return errors.New("value cannot be blank")
		}
		return fmt.Errorf("%s cannot be blank", name)
	}
	return nil
}

// IsBlank returns true if s is empty or consists solely of whitespace characters.
func IsBlank(s string) bool {
	return strings.TrimSpace(s) == ""
}

// IsNonBlank returns true if s contains at least one non-whitespace character.
func IsNonBlank(s string) bool {
	return strings.TrimSpace(s) != ""
}
