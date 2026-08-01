package validation

import (
	"regexp"
	"strings"
)

// extractPrefixesFromPattern extracts possible prefixes from a regex pattern
func extractPrefixesFromPattern(pattern string) []string {
	// Pattern like "^PLAN-\\d{3,}$" or "^AGENT-ARCH-\\d{3,}$" or "^[A-Z]+-\\d{3,}$"
	// Try to extract literal prefixes (e.g., "PLAN-", "AGENT-ARCH-")

	// Look for patterns like "^PLAN-", "^AGENT-ARCH-", etc. (match until first \d or $)
	// This handles multi-word prefixes like AGENT-ARCH-
	prefixPattern := regexp.MustCompile(`\^([A-Z]+(?:-[A-Z]+)*)-`)
	matches := prefixPattern.FindStringSubmatch(pattern)
	if len(matches) > 1 {
		return []string{matches[1] + "-"}
	}

	// Look for alternation patterns like "(PRI|PRIO)-" or "(PLAN-|PRIO-)"
	altPattern := regexp.MustCompile(`\(([A-Z\-\|]+)\)`)
	matches = altPattern.FindStringSubmatch(pattern)
	if len(matches) > 1 {
		parts := strings.Split(matches[1], "|")
		prefixes := make([]string, 0, len(parts))
		for _, part := range parts {
			p := strings.TrimSpace(part)
			if !strings.HasSuffix(p, "-") {
				p += "-"
			}
			prefixes = append(prefixes, p)
		}
		return prefixes
	}

	// If pattern is generic like "^[A-Z]+-", return empty and let inferPrefixesFromKind handle it
	return []string{}
}

// inferPrefixesFromKind infers ID prefixes from object kind name
// Uses instance config if available (for test isolation), otherwise falls back to global
func (v *IDValidator) inferPrefixesFromKind(kind string) []string {
	// Use config-based prefix lookup first (instance or global)
	config := v.getIDPrefixesConfig()
	if config != nil {
		if prefixes := config.GetPrefixesForKind(kind); len(prefixes) > 0 {
			return prefixes
		}
	}

	// Fallback: try to infer from kind name (backward compatibility)
	parts := strings.Split(kind, "_")
	if len(parts) > 0 {
		// Take first 3-4 letters of first part, uppercase
		firstPart := strings.ToUpper(parts[0])
		if len(firstPart) >= 3 {
			prefix := firstPart[:3] + "-"
			return []string{prefix}
		}
	}

	return []string{}
}
