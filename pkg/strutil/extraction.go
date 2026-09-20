package strutil

import "strings"

// ExtractFirstPrefixedValue scans lines for the first occurrence of any of the given prefixes.
// It returns the trimmed value following the prefix.
// If no prefix is found, it returns an empty string.
func ExtractFirstPrefixedValue(lines []string, prefixes ...string) string {
	for _, line := range lines {
		line = strings.TrimSpace(line)
		for _, prefix := range prefixes {
			if strings.HasPrefix(line, prefix) {
				return strings.TrimSpace(strings.TrimPrefix(line, prefix))
			}
		}
	}
	return ""
}

// FindLineWithPrefix scans lines and returns the first entire line that starts with any of the prefixes.
func FindLineWithPrefix(lines []string, prefixes ...string) string {
	for _, line := range lines {
		line = strings.TrimSpace(line)
		for _, prefix := range prefixes {
			if strings.HasPrefix(line, prefix) {
				return line
			}
		}
	}
	return ""
}
