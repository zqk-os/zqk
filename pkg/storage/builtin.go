package storage

import (
	"regexp"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
)

// Built-in object ID patterns
var (
	// Built-in component types (COMP-TYPE-*)
	builtInComponentTypePattern = regexp.MustCompile(`^COMP-TYPE-\d+$`)

	// Add more built-in patterns as needed
	// builtInPatterns = []*regexp.Regexp{...}
)

// IsBuiltIn checks if an object is a built-in instance
// Built-in objects are identified by:
// 1. ID patterns (e.g., COMP-TYPE-* for component types)
// 2. source_type field set to "internal" (for system objects)
// 3. Special metadata indicating built-in status
func IsBuiltIn(obj map[string]any) bool {
	// Check ID pattern
	if id := objects.GetString(obj, objects.FieldKeyID); id != "" {
		if builtInComponentTypePattern.MatchString(id) {
			return true
		}
	}

	// Check source_type
	if sourceType := objects.GetString(obj, objects.FieldKeySourceType); sourceType != "" {
		if sourceType == "internal" {
			// Additional check: internal objects with specific ID patterns are built-in
			if id := objects.GetString(obj, objects.FieldKeyID); id != "" {
				// Check for known built-in prefixes
				builtInPrefixes := []string{"COMP-TYPE-", "TYPE-", "BUILTIN-"}
				for _, prefix := range builtInPrefixes {
					if strings.HasPrefix(id, prefix) {
						return true
					}
				}
			}
		}
	}

	// Check for built-in metadata
	if metadata, ok := obj[objects.FieldKeyMetadata].(map[string]any); ok {
		if builtIn, ok := metadata["built_in"].(bool); ok && builtIn {
			return true
		}
	}

	return false
}

// IsBuiltInByID checks if an ID matches a built-in pattern
func IsBuiltInByID(id string) bool {
	return builtInComponentTypePattern.MatchString(id) ||
		strings.HasPrefix(id, "TYPE-") ||
		strings.HasPrefix(id, "BUILTIN-")
}
