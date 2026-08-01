package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/when"
)

const (
	fieldAppendSeparator = "+="
	fieldEqualSeparator  = "="
	fieldColonSeparator  = ":"
)

// FieldParser provides unified field flag parsing
type FieldParser struct {
	logger *logging.EventLogger
}

// NewFieldParser creates a new field parser
func NewFieldParser(logger *logging.EventLogger) *FieldParser {
	return &FieldParser{logger: logger}
}

// ParseFieldFlag parses a field flag string (field=value, field:value, or field+=value for append)
// Returns field name, field value, whether it's an append operation, and error
func (fp *FieldParser) ParseFieldFlag(fieldStr string) (fieldName string, fieldValue string, isAppend bool, err error) {
	// Check for append operator (+=) first
	if strings.Contains(fieldStr, fieldAppendSeparator) {
		namePart, valuePart, ok := strings.Cut(fieldStr, fieldAppendSeparator)
		if !ok {
			fp.logger.LogError("Invalid field format", errfmt.Errorf("expected field+=value for append"),
				logging.String("field", fieldStr))
			return "", "", false, errfmt.Errorf("invalid field format: %s (expected field+=value for append)", fieldStr)
		}
		fieldName = strings.TrimSpace(namePart)
		fieldValue = strings.TrimSpace(valuePart)
		return fieldName, fieldValue, true, nil
	}

	// Parse field=value or field:value
	namePart, valuePart, ok := strings.Cut(fieldStr, fieldEqualSeparator)
	if !ok {
		// Try colon separator
		namePart, valuePart, ok = strings.Cut(fieldStr, fieldColonSeparator)
		if !ok {
			fp.logger.LogError("Invalid field format", errfmt.Errorf("expected field=value, field:value, or field+=value"),
				logging.String("field", fieldStr))
			return "", "", false, errfmt.Errorf("invalid field format: %s (expected field=value, field:value, or field+=value)", fieldStr)
		}
	}
	fieldName = strings.TrimSpace(namePart)
	fieldValue = strings.TrimSpace(valuePart)
	return fieldName, fieldValue, false, nil
}

// ParseFieldValue parses a field value: try JSON first, otherwise treat the whole value as a plain string (BLI-854).
// Valid JSON includes objects, arrays, quoted strings, numbers, booleans, and null.
// Values that are not valid JSON (e.g. status=validated, title=Hello: world) stay strings, so prose with colons
// is not misread as a map. Use JSON quoting for values that must embed colons or special characters, e.g.
// next_action="Done: shipped" or --data '{"next_action":"Done: shipped"}'.
func (fp *FieldParser) ParseFieldValue(fieldValue string) any {
	if fieldValue == emptyValue {
		return ""
	}
	trimmed := strings.TrimSpace(fieldValue)
	if trimmed == emptyValue {
		return ""
	}
	var parsedValue any
	if err := json.Unmarshal([]byte(trimmed), &parsedValue); err != nil {
		return fieldValue
	}
	return parsedValue
}

// ParseFieldFlags parses multiple field flags and returns a map
// Note: Append operations (field+=value) are not handled here - they require the current object value
// Use ParseFieldFlagsWithAppend for append support
func (fp *FieldParser) ParseFieldFlags(fieldStrs []string) (map[string]any, error) {
	fields := make(map[string]any)
	for _, fieldStr := range fieldStrs {
		fieldName, fieldValue, isAppend, err := fp.ParseFieldFlag(fieldStr)
		if err != nil {
			return nil, err
		}
		if isAppend {
			// Append operations need current object value - return error
			return nil, errfmt.Errorf("append operation (field+=value) requires current object value - use ParseFieldFlagsWithAppend instead")
		}
		fields[fieldName] = fp.ParseFieldValue(fieldValue)
	}
	return fields, nil
}

// ParseFieldFlagsWithAppend parses field flags and handles append operations
// Requires current object to append to existing field values
func (fp *FieldParser) ParseFieldFlagsWithAppend(fieldStrs []string, currentObj map[string]any) (map[string]any, error) {
	fields := make(map[string]any)
	for _, fieldStr := range fieldStrs {
		fieldName, fieldValue, isAppend, err := fp.ParseFieldFlag(fieldStr)
		if err != nil {
			return nil, err
		}
		if isAppend {
			// Get current value
			currentValue := ""
			if currentObj != nil {
				if existing, ok := currentObj[fieldName]; ok {
					if str, ok := existing.(string); ok {
						currentValue = str
					} else {
						// Try to convert to string
						currentValue = fmt.Sprintf("%v", existing)
					}
				}
			}
			// Append new value (add newline separator for multiline fields like description)
			parsedValue := fp.ParseFieldValue(fieldValue)
			var newValueStr string
			when.When(func() bool { _, ok := parsedValue.(string); return ok }).Then(func() {
				newValueStr = parsedValue.(string)
			}).OrElse(func() {
				newValueStr = fmt.Sprintf("%v", parsedValue)
			}).Run()
			when.When(func() bool { return currentValue != emptyValue }).Then(func() {
				fields[fieldName] = currentValue + "\n\n" + newValueStr
			}).OrElse(func() {
				fields[fieldName] = newValueStr
			}).Run()
		} else {
			fields[fieldName] = fp.ParseFieldValue(fieldValue)
		}
	}
	return fields, nil
}
