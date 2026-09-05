package cli

import (
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

const (
	displayFieldPrefix = "d_"
)

// AddDisplayFieldsForTable adds display fields (d_*) to objects for table display contexts.
// This preserves original field values while providing truncated versions for display.
// IMPORTANT (Lesson 10): Only d_* keys are added; we never replace or overwrite description,
// context, body, or content. Callers must never persist obj after this if they would use
// truncated values—persisted object YAML must contain full field values only.
func AddDisplayFieldsForTable(obj map[string]any, spec *objects.Spec, cmd *cobra.Command) {
	if spec == nil {
		return
	}

	// Iterate through all fields in the spec
	for fieldName, fieldDef := range spec.ResolvedFields {
		_, ok := fieldDef.(map[string]any)
		if !ok {
			continue
		}

		// Get display_length from spec
		displayLength := objects.GetDisplayLength(spec, fieldName, 0)
		if displayLength <= 0 {
			continue // No display_length constraint, skip
		}

		// Get original field value
		originalValue, exists := obj[fieldName]
		if !exists {
			continue
		}

		// Convert to string for truncation
		strValue, ok := originalValue.(string)
		if !ok {
			continue // Only truncate strings
		}

		// Check if value exceeds display_length
		if len(strValue) <= displayLength {
			continue // No truncation needed
		}

		// Create display field name (d_ prefix)
		displayFieldName := displayFieldPrefix + fieldName

		// Truncate value for display
		truncated := TruncateString(strValue, displayLength)

		// Add display field (only if different from original)
		// This allows table formatters to use d_* fields while preserving originals
		if truncated != strValue {
			obj[displayFieldName] = truncated
		}
	}
}

// GetDisplayValue gets the value to use for display, preferring display fields (d_*) when available
// Falls back to original field if display field doesn't exist
// For detail views (isDetailView=true), always returns original value for never-truncate fields
func GetDisplayValue(obj map[string]any, fieldName string, isDetailView bool) string {
	// Fields that should never be truncated in detail views
	neverTruncateFields := map[string]bool{
		objects.FieldKeyContext:     true,
		objects.FieldKeyDescription: true,
		objects.FieldKeyBody:        true,
		objects.FieldKeyContent:     true,
	}

	// In detail view, always return original for never-truncate fields
	if isDetailView && neverTruncateFields[fieldName] {
		if val, ok := obj[fieldName]; ok {
			return formatValue(val)
		}
		return emptyValue
	}

	// For table views, prefer display field if available
	displayFieldName := displayFieldPrefix + fieldName
	if displayVal, ok := obj[displayFieldName]; ok {
		return formatValue(displayVal)
	}

	// Fall back to original field
	if val, ok := obj[fieldName]; ok {
		return formatValue(val)
	}

	return emptyValue
}

// formatValue formats a field value to a display string, with special handling for times
func formatValue(val any) string {
	switch v := val.(type) {
	case nil:
		return emptyValue
	case string:
		return v
	case time.Time:
		return v.Format(time.RFC3339)
	case *time.Time:
		if v != nil {
			return v.Format(time.RFC3339)
		}
		return emptyValue
	default:
		return fmt.Sprintf("%v", v)
	}
}
