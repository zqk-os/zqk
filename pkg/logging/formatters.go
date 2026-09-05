package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// JSONFormatter formats logs as JSON
type JSONFormatter struct {
	ctx context.Context // Context for cancellation and context-aware formatting
}

func NewJSONFormatter(ctx context.Context) Formatter {
	return &JSONFormatter{
		ctx: ctx,
	}
}

func (f *JSONFormatter) Format(entry *LogEntry) ([]byte, error) {
	data := map[string]any{
		"timestamp": zqktime.FormatRFC3339UTC(entry.Timestamp),
		"level":     entry.Level.String(),
		"message":   entry.Message,
	}

	// Merge fields - sanitize values that can't be JSON marshaled
	for k, v := range entry.Fields {
		// Sanitize values that can't be marshaled (channels, functions, etc.)
		data[k] = sanitizeForJSON(v)
	}

	// Add error if present
	if entry.Error != nil {
		data["error"] = entry.Error.Error()
	}

	// Attempt to marshal as compact JSON (no newlines, no indentation)
	// This ensures JSONL format where each line is a single compact JSON object
	result, err := json.Marshal(data)
	if err != nil {
		// If marshaling fails, try to identify which field is causing the issue
		// by attempting to marshal each field individually
		problematicFields := []string{}
		for k, v := range data {
			testData := map[string]any{k: v}
			if _, fieldErr := json.Marshal(testData); fieldErr != nil {
				problematicFields = append(problematicFields, fmt.Sprintf("%s (%T)", k, v))
				// Replace problematic field with its string representation
				data[k] = fmt.Sprintf("%v", v)
			}
		}

		// Try marshaling again with sanitized problematic fields
		result, err = json.Marshal(data)
		if err != nil {
			// If it still fails, return error with context about problematic fields
			return nil, errfmt.Errorf("JSON marshal failed: %w (problematic fields: %v)", err, problematicFields)
		}
	}

	return result, nil
}

// sanitizeForJSON converts values that can't be JSON marshaled to strings
// This prevents JSON formatter failures that would trigger text-format fallback
func sanitizeForJSON(v any) any {
	if v == nil {
		return nil
	}

	// Use reflection to check for unmarshalable types before attempting marshal
	valType := reflect.TypeOf(v)
	if valType != nil {
		switch valType.Kind() {
		case reflect.Func, reflect.Chan, reflect.UnsafePointer, reflect.Uintptr:
			return fmt.Sprintf("<%s>", valType.Kind().String())
		case reflect.Ptr:
			// Check if it's a pointer to an unmarshalable type
			elemType := valType.Elem()
			if elemType != nil {
				switch elemType.Kind() {
				case reflect.Func, reflect.Chan:
					return fmt.Sprintf("<*%s>", elemType.Kind().String())
				}
			}
		}
	}

	// Try to marshal a test value to see if it's JSON-compatible
	// If it fails, convert to string representation
	testData := map[string]any{"test": v}
	if _, err := json.Marshal(testData); err != nil {
		// Value can't be marshaled - convert to string
		return fmt.Sprintf("%v", v)
	}

	return v
}

// flattenSingleLineFieldValue replaces embedded newlines in string field values so a single log line stays one line.
func flattenSingleLineFieldValue(v any) any {
	if s, ok := v.(string); ok && (strings.Contains(s, "\n") || strings.Contains(s, "\r")) {
		return strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " ")
	}
	return v
}

// TextFormatter formats logs as human-readable text
type TextFormatter struct {
	ctx context.Context // Context for cancellation and context-aware formatting
}

func NewTextFormatter(ctx context.Context) Formatter {
	return &TextFormatter{
		ctx: ctx,
	}
}

func (f *TextFormatter) Format(entry *LogEntry) ([]byte, error) {
	// Format: TIMESTAMP [LEVEL] MESSAGE [key=value ...]
	line := fmt.Sprintf("%s [%s] %s",
		zqktime.FormatRFC3339UTC(entry.Timestamp),
		entry.Level.String(),
		entry.Message,
	)

	// Add fields in sorted order for consistent output
	fieldKeys := make([]string, 0, len(entry.Fields))
	for k := range entry.Fields {
		if k != "timestamp" && k != "level" && k != "message" {
			fieldKeys = append(fieldKeys, k)
		}
	}
	sort.Strings(fieldKeys)
	for _, k := range fieldKeys {
		v := flattenSingleLineFieldValue(entry.Fields[k])
		line += fmt.Sprintf(" %s=%v", k, v)
	}

	line += "\n"
	return []byte(line), nil
}

// CompactFormatter formats logs compactly (for debug)
type CompactFormatter struct {
	ctx context.Context // Context for cancellation and context-aware formatting
}

func NewCompactFormatter(ctx context.Context) Formatter {
	return &CompactFormatter{
		ctx: ctx,
	}
}

func (f *CompactFormatter) Format(entry *LogEntry) ([]byte, error) {
	// Compact format with stack trace capability
	line := fmt.Sprintf("%s [%s] %s",
		entry.Timestamp.Format("15:04:05.000"),
		entry.Level.String()[:1], // Single char: D, I, W, E, F
		entry.Message,
	)

	// Add fields
	for k, v := range entry.Fields {
		if k != "timestamp" && k != "level" && k != "message" {
			v = flattenSingleLineFieldValue(v)
			line += fmt.Sprintf(" %s=%v", k, v)
		}
	}

	line += "\n"
	return []byte(line), nil
}
