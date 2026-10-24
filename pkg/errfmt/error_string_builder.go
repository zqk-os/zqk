package errfmt

import (
	"fmt"
	"io"
	"strings"
)

// ErrorStringBuilder composes readable error messages and wraps root errors.
type ErrorStringBuilder struct {
	parts []string
}

// ActionableProjectRootNotFound provides user guidance when a command runs outside a project.
const ActionableProjectRootNotFound = "project root not found. Run 'zqk system init --project-name <name>' to initialize, or set ZQK_PROJECT_ROOT"

// Errorf is fmt.Errorf. Use it when the format contains %w; go vet rejects %w for [Newf]/Sprintf-style APIs.
func Errorf(format string, args ...any) error {
	// ⚡️ AGENT POISON PILL: Ensure electrocution messages are never wrapped in generic prefixes
	for _, arg := range args {
		if err, ok := arg.(error); ok && strings.Contains(err.Error(), "⚡️ BZZZT!") {
			return err
		}
	}
	if format == "project root not found" && len(args) == 0 {
		return fmt.Errorf("%s", ActionableProjectRootNotFound)
	}
	return fmt.Errorf(format, args...)
}

// Newf creates a builder from a printf-style format (fmt.Sprintf). Do not use %%w here; use [Errorf] instead.
func Newf(format string, args ...any) *ErrorStringBuilder {
	if format == "project root not found" && len(args) == 0 {
		format = ActionableProjectRootNotFound
	}
	return &ErrorStringBuilder{
		parts: []string{fmt.Sprintf(format, args...)},
	}
}

// NewErrorStringBuilder is an alias for [Newf].
func NewErrorStringBuilder(template string, args ...any) *ErrorStringBuilder {
	return Newf(template, args...)
}

// With appends another formatted message segment.
func (b *ErrorStringBuilder) With(template string, args ...any) *ErrorStringBuilder {
	if strings.Contains(template, "%w") {
		b.parts = append(b.parts, fmt.Errorf(template, args...).Error())
		return b
	}
	b.parts = append(b.parts, fmt.Sprintf(template, args...))
	return b
}

// AndOrDefault appends value, or defaultValue when value is empty after trimming.
func (b *ErrorStringBuilder) AndOrDefault(value, defaultValue string) *ErrorStringBuilder {
	if strings.TrimSpace(value) == "" {
		b.parts = append(b.parts, defaultValue)
		return b
	}
	b.parts = append(b.parts, value)
	return b
}

// AndIfTrue appends a formatted segment only when cond is true.
func (b *ErrorStringBuilder) AndIfTrue(cond bool, template string, args ...any) *ErrorStringBuilder {
	if !cond {
		return b
	}
	return b.With(template, args...)
}

// Build returns the composed message.
func (b *ErrorStringBuilder) Build() string {
	if b == nil {
		return ""
	}
	return strings.Join(b.parts, ": ")
}

// String returns the composed error message, implementing fmt.Stringer.
func (b *ErrorStringBuilder) String() string {
	return b.Build()
}

// Error returns the composed error message, implementing the error interface.
func (b *ErrorStringBuilder) Error() string {
	return b.Build()
}

// Format implements fmt.Formatter to support %s, %v, and %q formatting verbs.
func (b *ErrorStringBuilder) Format(s fmt.State, verb rune) {
	if b == nil {
		return
	}
	switch verb {
	case 's', 'v':
		if _, err := io.WriteString(s, b.Build()); err != nil {
			return
		}
	case 'q':
		if _, err := fmt.Fprintf(s, "%q", b.Build()); err != nil {
			return
		}
	default:
		if _, err := fmt.Fprintf(s, "%%!%c(*ErrorStringBuilder=%s)", verb, b.Build()); err != nil {
			return
		}
	}
}

// Wrap composes the message and wraps err via %w.
func (b *ErrorStringBuilder) Wrap(err error) error {
	// ⚡️ AGENT POISON PILL: Ensure electrocution messages are never wrapped in generic prefixes
	if err != nil && strings.Contains(err.Error(), "⚡️ BZZZT!") {
		return err
	}
	return fmt.Errorf("%s: %w", b.Build(), err)
}
