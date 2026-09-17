package system

import (
	"fmt"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/validation"

	"github.com/lanceman/zqk/pkg/objects"
)

// formatToPattern attempts to format a value to match a regex pattern
func formatToPattern(value any, pattern string, logger logging.Logger) *string {
	// Handle *string pointers by dereferencing them first
	var strValue string
	switch v := value.(type) {
	case string:
		strValue = v
	case *string:
		if v != nil {
			strValue = *v
		} else {
			return nil
		}
	case time.Time:
		// YAML often unmarshals datetime fields as time.Time; format as RFC3339 for pattern match
		strValue = v.Format(time.RFC3339)
	default:
		strValue = fmt.Sprintf("%v", value)
	}

	// Common pattern fixes
	// ID pattern: ^[A-Z]+-\d{3,}$
	if strings.Contains(pattern, "^[A-Z]+-\\d{3,}$") || strings.Contains(pattern, "^[A-Z]+-\\d+$") {
		// Try to extract ID from string (e.g., "REQ-999" from "req-999" or "REQ999")
		if fixed := fixIDPattern(strValue); fixed != nil {
			return fixed
		}
	}

	// Namespace pattern: ^(zqk|domain|integration):[a-z0-9_]+(:[a-z0-9_]+)*$
	if strings.Contains(pattern, "zqk|domain|integration") || strings.Contains(pattern, objects.KindNamespace) {
		if fixed := fixNamespacePattern(strValue, pattern, logger); fixed != nil {
			return fixed
		}
	}

	// Datetime pattern: ISO-8601
	if strings.Contains(pattern, "\\d{4}-\\d{2}-\\d{2}T") || strings.Contains(pattern, "dateTime") {
		if dt := parseAndFormatDateTime(strValue, logger); dt != nil {
			return dt
		}
	}

	// Date pattern: YYYY-MM-DD
	if strings.Contains(pattern, "\\d{4}-\\d{2}-\\d{2}") && !strings.Contains(pattern, "T") {
		if d := parseAndFormatDate(strValue, logger); d != nil {
			return d
		}
	}

	return nil
}

// fixNamespacePattern attempts to fix namespace_id pattern violations
// Pattern: ^(zqk|domain|integration):[a-z0-9_]+(:[a-z0-9_]+)*$
func fixNamespacePattern(value string, pattern string, logger logging.Logger) *string {
	value = strings.TrimSpace(value)
	if value == emptyValue {
		// If empty, try to provide a default namespace
		defaultNS := "zqk:default"
		logging.Fluent(logger).Debug("Namespace pattern fix: empty value, using default").
			String("default", defaultNS).
			Log()
		return &defaultNS
	}

	// Check if it already matches (basic check) - return value so caller can store it (e.g. normalize to string)
	re, err := getCachedRegexp(pattern)
	if err == nil && re.MatchString(value) {
		return &value // Already valid; return so objMap gets set to string (not nil *string)
	}

	// Try to fix common issues:
	// 1. Missing prefix - add "zqk:" if no prefix
	if !strings.Contains(value, ":") {
		// If it's a valid identifier, add zqk: prefix
		if reSpecAutoFixerSimpleIdentifier.MatchString(value) {
			fixed := "zqk:" + value
			return &fixed
		}
	}

	// 2. Wrong case - normalize to lowercase after prefix
	// Also handle invalid prefixes by converting to zqk:
	parts := strings.Split(value, ":")
	if len(parts) >= 2 {
		prefix := strings.ToLower(parts[0])
		// Validate prefix - if invalid, convert to zqk:
		if prefix != "zqk" && prefix != "domain" && prefix != "integration" {
			// Invalid prefix - convert to zqk: and use the rest
			normalizedParts := []string{"zqk"}
			for i := 1; i < len(parts); i++ {
				// Sanitize and lowercase each part
				part := strings.ToLower(parts[i])
				part = reSpecAutoFixerNonIdentifierChars.ReplaceAllString(part, "_")
				normalizedParts = append(normalizedParts, part)
			}
			fixed := strings.Join(normalizedParts, ":")
			return &fixed
		}
		// Valid prefix - normalize remaining parts to lowercase
		normalizedParts := []string{prefix}
		for i := 1; i < len(parts); i++ {
			normalizedParts = append(normalizedParts, strings.ToLower(parts[i]))
		}
		fixed := strings.Join(normalizedParts, ":")
		if fixed != value {
			return &fixed
		}
	}

	// 3. Invalid characters - try to sanitize
	sanitized := reSpecAutoFixerInvalidNamespaceChar.ReplaceAllString(strings.ToLower(value), "_")
	if sanitized != value {
		// Ensure it starts with valid prefix
		if !strings.HasPrefix(sanitized, "zqk:") && !strings.HasPrefix(sanitized, "domain:") && !strings.HasPrefix(sanitized, "integration:") {
			sanitized = "zqk:" + sanitized
		}
		return &sanitized
	}

	return nil
}

// fixIDPattern attempts to fix ID format violations (e.g., "req-999" -> "REQ-999")
func fixIDPattern(value string) *string {
	// Remove whitespace
	value = strings.TrimSpace(value)
	if value == emptyValue {
		return nil
	}

	// Split by dash
	parts := strings.Split(value, "-")
	if len(parts) != 2 {
		// Try to extract ID pattern (e.g., "REQ999" -> "REQ-999")
		if matched := reSpecAutoFixerAlphaDigits.FindStringSubmatch(value); len(matched) == 3 {
			prefix := strings.ToUpper(matched[1])
			suffix := matched[2]
			fixed := fmt.Sprintf("%s-%s", prefix, suffix)
			return &fixed
		}
		return nil
	}

	// Normalize: uppercase prefix, ensure numeric suffix
	prefix := strings.ToUpper(strings.TrimSpace(parts[0]))
	suffix := strings.TrimSpace(parts[1])

	// Validate suffix is numeric
	if !reSpecAutoFixerDigitsOnly.MatchString(suffix) {
		return nil
	}

	fixed := fmt.Sprintf("%s-%s", prefix, suffix)
	if fixed != value {
		return &fixed
	}

	return nil
}

// fixPatternViolation attempts to fix pattern violations for common patterns.
// Uses validation.ResolveStringForPatternValidationWithField so schema_version (e.g. float 2.0)
// and datetime fields (e.g. int64 Unix) are normalized before pattern check; then persists the normalized value.
func fixPatternViolation(fieldName string, currentValue any, fieldMap map[string]any, logger logging.Logger) any {
	fieldValidation, ok := getValidationMap(fieldMap)
	if !ok {
		return nil
	}

	pattern, ok := fieldValidation["pattern"].(string)
	if !ok {
		return nil
	}

	// Normalize using same rules as validator (schema_version float→"X.Y.Z", datetime int64→RFC3339, etc.)
	normalized, ok := validation.ResolveStringForPatternValidationWithField(fieldName, currentValue)
	if ok && normalized != emptyValue {
		re, err := getCachedRegexp(pattern)
		if err == nil && re.MatchString(normalized) {
			return &normalized
		}
	}

	// Use formatToPattern helper for other patterns (ID, namespace, datetime string, etc.)
	return formatToPattern(currentValue, pattern, logger)
}
