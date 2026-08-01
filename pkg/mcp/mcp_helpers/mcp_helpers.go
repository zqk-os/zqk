package mcp_helpers

import (
	"regexp"
	"strings"
)

// sanitizeToolName converts a command path to a valid MCP tool name
// e.g., "object list" -> "object_list"
// Removes flag placeholders and optional flags to keep names short
// Cursor has a 60-character limit for "server:tool_name", so we limit to 53 chars
func SanitizeToolName(path string) string {
	// Remove flag placeholders like <id1,id2,...>, <file>, etc.
	// Pattern: <...> or <...|...>
	path = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(path, "")

	// Remove optional flags like [--cascade]
	path = regexp.MustCompile(`\[[^\]]+\]`).ReplaceAllString(path, "")

	// Remove pipe separators like " | "
	path = strings.ReplaceAll(path, " | ", " ")

	// Remove multiple spaces
	path = regexp.MustCompile(`\s+`).ReplaceAllString(path, " ")

	// Trim spaces
	path = strings.TrimSpace(path)

	// Replace spaces with underscores
	name := strings.ReplaceAll(path, " ", "_")

	// Cursor limit: 60 chars total for "server:tool_name"
	// Tool name max length (accounting for server: prefix)
	const maxToolNameLength = 53
	if len(name) > maxToolNameLength {
		// Truncate but try to preserve meaningful parts
		// Keep the first part (usually the command group) and truncate the rest
		parts := strings.Split(name, "_")
		if len(parts) > 1 {
			// Keep first part and as much of the rest as fits
			result := parts[0]
			for i := 1; i < len(parts); i++ {
				candidate := result + "_" + parts[i]
				if len(candidate) <= maxToolNameLength {
					result = candidate
				} else {
					break
				}
			}
			name = result
		} else {
			// Single part, just truncate
			name = name[:maxToolNameLength]
		}
	}

	return name
}
