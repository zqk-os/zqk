package llm

import (
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// SanitizeUntrustedText detects and escapes or rejects common prompt injection payloads.
// It returns an error if a blatant injection attempt is detected.
func SanitizeUntrustedText(input string) (string, error) {
	lower := strings.ToLower(input)

	// Fast fail on blatant classic override payloads
	if strings.Contains(lower, "ignore all previous instructions") ||
		strings.Contains(lower, "system override") ||
		strings.Contains(lower, "you are now") ||
		strings.Contains(lower, "bypass the following instructions") ||
		strings.Contains(lower, "forget previous instructions") {
		return "", errfmt.Errorf("security: prompt injection detected")
	}

	// Escape known model special tokens to prevent breaking out of context
	escaped := strings.ReplaceAll(input, "<|im_start|>", "")
	escaped = strings.ReplaceAll(escaped, "<|im_end|>", "")
	escaped = strings.ReplaceAll(escaped, "<|system|>", "")
	escaped = strings.ReplaceAll(escaped, "<|user|>", "")

	return escaped, nil
}
