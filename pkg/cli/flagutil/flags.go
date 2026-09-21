package flagutil

import (
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// Common flag extraction and lookup utility to eliminate boilerplate,
// inconsistent whitespace handling, and swallowed error patterns.

// String returns the raw string value of a flag, or empty string if absent/error.
func String(cmd *cobra.Command, name string) string {
	if cmd == nil || cmd.Flags() == nil {
		return ""
	}
	v, err := cmd.Flags().GetString(name)
	if err != nil {
		return ""
	}
	return v
}

// StringTrimmed returns the string flag value with whitespace stripped.
func StringTrimmed(cmd *cobra.Command, name string) string {
	return strings.TrimSpace(String(cmd, name))
}

// StringTrimmedOrDefault returns the trimmed string flag value, or def if empty.
func StringTrimmedOrDefault(cmd *cobra.Command, name, def string) string {
	v := StringTrimmed(cmd, name)
	if v == "" {
		return def
	}
	return v
}

// RequireStringTrimmed returns the trimmed string flag value or an error if missing or empty.
func RequireStringTrimmed(cmd *cobra.Command, name string) (string, error) {
	v := StringTrimmed(cmd, name)
	if v == "" {
		return "", errfmt.Errorf("--%s is required", name)
	}
	return v, nil
}

// Bool returns the boolean value of a flag, or false if absent/error.
func Bool(cmd *cobra.Command, name string) bool {
	if cmd == nil || cmd.Flags() == nil {
		return false
	}
	v, err := cmd.Flags().GetBool(name)
	if err != nil {
		return false
	}
	return v
}

// Int returns the int value of a flag, or 0 if absent/error.
func Int(cmd *cobra.Command, name string) int {
	if cmd == nil || cmd.Flags() == nil {
		return 0
	}
	v, err := cmd.Flags().GetInt(name)
	if err != nil {
		return 0
	}
	return v
}

// IntOrDefault returns the int value of a flag, or def if <= 0 or on error.
func IntOrDefault(cmd *cobra.Command, name string, def int) int {
	v := Int(cmd, name)
	if v <= 0 {
		return def
	}
	return v
}

// Duration returns the duration value of a flag, or 0 if absent/error.
func Duration(cmd *cobra.Command, name string) time.Duration {
	if cmd == nil || cmd.Flags() == nil {
		return 0
	}
	v, err := cmd.Flags().GetDuration(name)
	if err != nil {
		return 0
	}
	return v
}

// DurationOrDefault returns the duration value of a flag, or def if <= 0 or on error.
func DurationOrDefault(cmd *cobra.Command, name string, def time.Duration) time.Duration {
	v := Duration(cmd, name)
	if v <= 0 {
		return def
	}
	return v
}

// StringSlice returns the string slice value of a flag, or nil if absent/error.
func StringSlice(cmd *cobra.Command, name string) []string {
	if cmd == nil || cmd.Flags() == nil {
		return nil
	}
	v, err := cmd.Flags().GetStringSlice(name)
	if err != nil {
		return nil
	}
	return v
}

// StringSliceTrimmed returns non-empty, trimmed elements of a string slice flag.
func StringSliceTrimmed(cmd *cobra.Command, name string) []string {
	raw := StringSlice(cmd, name)
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		trimmed := strings.TrimSpace(s)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// Float64 returns the float64 value of a flag, or 0 if absent/error.
func Float64(cmd *cobra.Command, name string) float64 {
	if cmd == nil || cmd.Flags() == nil {
		return 0
	}
	v, err := cmd.Flags().GetFloat64(name)
	if err != nil {
		return 0
	}
	return v
}

// IsChanged reports whether the flag was explicitly provided by the caller.
func IsChanged(cmd *cobra.Command, name string) bool {
	if cmd == nil || cmd.Flags() == nil {
		return false
	}
	f := cmd.Flags().Lookup(name)
	if f == nil {
		return false
	}
	return f.Changed
}

// IsTrue reports whether a parsed flag value is a valid true boolean.
func IsTrue(f *pflag.Flag) bool {
	if f == nil {
		return false
	}
	v, err := strconv.ParseBool(f.Value.String())
	return err == nil && v
}
