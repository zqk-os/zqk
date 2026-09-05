package cli

import (
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"gopkg.in/yaml.v3"
)

const (
	filterOpNotEqual     = "!="
	filterOpEqual        = "="
	filterOpColon        = ":"
	filterValueSeparator = "|"
	filterMongoOpNotIn   = "$nin"
	filterMongoOpNotEq   = "$ne"
	filterMongoOpIn      = "$in"
)

// ParseFilterString parses a filter string into field name and filter value
// This is a shared utility used by object, internal, and other commands
// Supports:
//   - field=value (equality)
//   - field!=value (not equal, converts to {"$ne": value})
//   - field:value (equality, alternative syntax)
//   - field!="value1|value2" (not in list, converts to {"$nin": ["value1", "value2"]})
//   - field="value1|value2" (in list, converts to {"$in": ["value1", "value2"]})
//   - field={"$op": value} (explicit operator syntax)
func ParseFilterString(filterStr string) (field string, value any, err error) {
	// Try != operator first (not equal)
	if strings.Contains(filterStr, filterOpNotEqual) {
		f, v, e := parseNotEqualFilter(filterStr)
		return f, ResolveTimeTokens(v), e
	}

	// Try = operator (equality)
	if strings.Contains(filterStr, filterOpEqual) {
		f, v, e := parseEqualFilter(filterStr)
		return f, ResolveTimeTokens(v), e
	}

	// Try colon separator (equality, alternative syntax)
	if strings.Contains(filterStr, filterOpColon) {
		f, v, e := parseColonFilter(filterStr)
		return f, ResolveTimeTokens(v), e
	}

	return "", nil, errfmt.Errorf("invalid filter format: %s (expected field=value, field!=value, field:value, or field!=\"value1|value2\")", filterStr)
}

// parseNotEqualFilter parses a filter with != operator
func parseNotEqualFilter(filterStr string) (field string, value any, err error) {
	fieldPart, valuePart, ok := strings.Cut(filterStr, filterOpNotEqual)
	if !ok {
		return "", nil, errfmt.Errorf("invalid != filter format: %s", filterStr)
	}

	fieldName := strings.TrimSpace(fieldPart)
	fieldValue := strings.TrimSpace(valuePart)
	fieldValue = strings.Trim(fieldValue, `"'`)

	// Check for pipe-separated values (OR condition)
	if strings.Contains(fieldValue, filterValueSeparator) {
		valueList := parsePipeSeparatedValues(fieldValue)
		return fieldName, map[string]any{filterMongoOpNotIn: valueList}, nil
	}

	// Single value: convert to $ne (not equal)
	parsedValue := parseYAMLValue(fieldValue)
	return fieldName, map[string]any{filterMongoOpNotEq: parsedValue}, nil
}

// parseEqualFilter parses a filter with = operator
func parseEqualFilter(filterStr string) (field string, value any, err error) {
	fieldPart, valuePart, ok := strings.Cut(filterStr, filterOpEqual)
	if !ok {
		return "", nil, errfmt.Errorf("invalid = filter format: %s", filterStr)
	}

	fieldName := strings.TrimSpace(fieldPart)
	fieldValue := strings.TrimSpace(valuePart)
	fieldValue = strings.Trim(fieldValue, `"'`)

	// Check for pipe-separated values (OR condition)
	if strings.Contains(fieldValue, filterValueSeparator) {
		valueList := parsePipeSeparatedValues(fieldValue)
		return fieldName, map[string]any{filterMongoOpIn: valueList}, nil
	}

	// Try to parse as YAML value (supports JSON-like maps for operators)
	parsedValue := parseYAMLValue(fieldValue)

	// If parsed value is a map, it might be an explicit operator (e.g., {"$gt": 10})
	if filterMap, ok := parsedValue.(map[string]any); ok {
		return fieldName, filterMap, nil
	}

	return fieldName, parsedValue, nil
}

// parseColonFilter parses a filter with : separator
func parseColonFilter(filterStr string) (field string, value any, err error) {
	fieldPart, valuePart, ok := strings.Cut(filterStr, filterOpColon)
	if !ok {
		return "", nil, errfmt.Errorf("invalid : filter format: %s", filterStr)
	}

	fieldName := strings.TrimSpace(fieldPart)
	fieldValue := strings.TrimSpace(valuePart)

	parsedValue := parseYAMLValue(fieldValue)
	return fieldName, parsedValue, nil
}

// parsePipeSeparatedValues parses pipe-separated values into a list
func parsePipeSeparatedValues(fieldValue string) []string {
	valueList := make([]string, 0)
	for v := range strings.SplitSeq(fieldValue, filterValueSeparator) {
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		if v != emptyValue {
			valueList = append(valueList, v)
		}
	}
	return valueList
}

// parseYAMLValue parses a value as YAML, falling back to string if parsing fails
func parseYAMLValue(fieldValue string) any {
	var parsedValue any
	if err := yaml.Unmarshal([]byte(fieldValue), &parsedValue); err != nil {
		// If YAML parsing fails, treat as string
		return fieldValue
	}
	return parsedValue
}
