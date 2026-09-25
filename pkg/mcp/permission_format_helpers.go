package mcp

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/zqk-os/zqk/pkg/objects"
)

// getPermissionOperations returns the list of valid permission operations (BLI-958)
// Loads from config if available, otherwise uses system defaults
func (s *Server) getPermissionOperations() []string {
	defaultOps := []string{"read", "write", "delete", "execute"}
	if s.config != nil && len(s.config.MCPServer.Security.PermissionOperations) > 0 {
		return s.config.MCPServer.Security.PermissionOperations
	}
	return defaultOps
}

// getObjectKindsForExamples returns a sample of object kinds for permission examples
// Uses system discovery to get actual object kinds, not hardcoded values
func (s *Server) getObjectKindsForExamples() []string {
	// Try to get from field registry (most reliable)
	registry := objects.GetGlobalFieldRegistry()
	if err := registry.LoadFields(); err == nil {
		if kinds, err := registry.GetAllKinds(); err == nil && len(kinds) > 0 {
			// Return first few common kinds for examples
			// Return first few kinds for examples (prioritize by discovery order)
			// TODO: Load priority/common kinds from config if available
			maxKinds := 5
			if len(kinds) > maxKinds {
				return kinds[:maxKinds]
			}
			return kinds
		}
	}

	// Fallback: try kind mapper
	mapper := objects.GetGlobalKindMapper()
	if kinds := mapper.GetAllKinds(); len(kinds) > 0 {
		// Return first 5 kinds
		if len(kinds) > 5 {
			return kinds[:5]
		}
		return kinds
	}

	// Last resort: return empty (will be handled by caller)
	return []string{}
}

// generatePermissionFormatExamples generates permission format examples dynamically
// Returns formatted text with operations and resources from system/config
func (s *Server) generatePermissionFormatExamples() string {
	operations := s.getPermissionOperations()
	objectKinds := s.getObjectKindsForExamples()

	var examples []string

	// Generate examples for each operation
	for _, op := range operations {
		// Capitalize first letter of operation for display
		opTitle := capitalizeFirst(op)

		// Wildcard example
		examples = append(examples, fmt.Sprintf("- %s:* - %s access to all object kinds", op, opTitle))

		// Specific kind example (use first available kind)
		if len(objectKinds) > 0 {
			kind := objectKinds[0]
			examples = append(examples, fmt.Sprintf("- %s:%s - %s access to %s only", op, kind, opTitle, kind))
		}
	}

	// Build operations list
	opsList := strings.Join(operations, ", ")

	// Build resources list
	var resourcesList string
	if len(objectKinds) > 0 {
		resourcesList = fmt.Sprintf("* (all), %s, etc.", strings.Join(objectKinds[:min(3, len(objectKinds))], ", "))
	} else {
		// Provide safe default resource examples if object kinds are not yet discoverable
		resourcesList = "* (all), backlog_item, requirement, test_case, etc."
	}

	return fmt.Sprintf("## Understanding Permission Format\n\n"+
		"Permissions use the format: operation:resource\n\n"+
		"- **Operations**: %s\n"+
		"- **Resources**: %s\n\n"+
		"Examples:\n%s\n",
		opsList, resourcesList, strings.Join(examples, "\n"))
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// capitalizeFirst capitalizes the first letter of a string
// Replaces deprecated strings.Title for single-word capitalization
func capitalizeFirst(s string) string {
	if s == emptyValue {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
