package system

import (
	"regexp"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/when"
)

// parseQueryHintToMap converts a query hint string to a map
// Query hint format: "category=feature&tags=branding,white-label&status=planned&title_pattern=White-Label Branding System"
func parseQueryHintToMap(queryHint string) map[string]any {
	result := make(map[string]any)

	if when.IsEmpty(queryHint) {
		return result
	}

	// Split by & to get key-value pairs
	for pair := range strings.SplitSeq(queryHint, "&") {
		// Split by = to get key and value
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]

		switch key {
		case objects.FieldKeyCategory, objects.FieldKeyStatus, "title_pattern":
			result[key] = value
		case "tags":
			// Tags: comma-separated list
			var tags []string
			for tag := range strings.SplitSeq(value, ",") {
				tags = append(tags, tag)
			}
			if len(tags) > 0 {
				result[objects.FieldKeyTags] = tags
			}
		}
	}

	return result
}

// buildQueryHintFromMap converts a query hints map back to a query hint string
// Reverse of parseQueryHintToMap
func buildQueryHintFromMap(queryHints map[string]any) string {
	var parts []string

	if category, ok := queryHints[objects.FieldKeyCategory].(string); ok && category != emptyValue {
		parts = append(parts, "category="+category)
	}
	if tags, ok := queryHints[objects.FieldKeyTags]; ok {
		if tagsList, ok := tags.([]any); ok {
			tagStrings := make([]string, 0, len(tagsList))
			for _, tag := range tagsList {
				if tagStr, ok := tag.(string); ok {
					tagStrings = append(tagStrings, tagStr)
				}
			}
			if len(tagStrings) > 0 {
				parts = append(parts, "tags="+strings.Join(tagStrings, ","))
			}
		} else if tagsList, ok := tags.([]string); ok {
			if len(tagsList) > 0 {
				parts = append(parts, "tags="+strings.Join(tagsList, ","))
			}
		}
	}
	if status, ok := queryHints[objects.FieldKeyStatus].(string); ok && status != emptyValue {
		parts = append(parts, "status="+status)
	}
	if titlePattern, ok := queryHints["title_pattern"].(string); ok && titlePattern != emptyValue {
		parts = append(parts, "title_pattern="+titlePattern)
	}

	return strings.Join(parts, "&")
}

// extractFieldNameFromCommand extracts the field name from a fix command
// Example: "<CLI_COMMAND> object update BLI-917 --field milestone_refs+=<MILESTONE_ID:...>" → objects.FieldKeyMilestoneRefs
func extractFieldNameFromCommand(command string) string {
	// Match pattern: --field <field_name>+= or --field <field_name>=
	// Regex: --field\s+([\w_]+)(\+?)=
	re := regexp.MustCompile(`--field\s+([\w]+)(\+?)=`)
	matches := re.FindStringSubmatch(command)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}
