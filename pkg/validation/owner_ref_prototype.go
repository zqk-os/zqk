package validation

import (
	"strings"
)

// containsString checks if haystack contains any of the needle substrings.
func containsString(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

// validatePrototypeAccountRef checks a raw owner_ref for prototype/test indicators.
// Returns true when the ref is valid (should NOT be rejected).
// Returns false when the ref indicates a prototype/test account (should be rejected).
func validatePrototypeAccountRef(ref string) bool {
	s := strings.TrimSpace(ref)
	if s == "" {
		return true // valid: empty means no owner_ref set, tolerate
	}
	lower := strings.ToLower(s)

	// known prefixes that indicate prototype/test accounts
	rejectedPrefixes := []string{"test", "pvt", "dev", "sbox", "mock", "prototyper"}
	for _, prefix := range rejectedPrefixes {
		if lower == prefix || strings.HasPrefix(lower, prefix+"-") {
			return false // invalid: rejected prefix detected
		}
	}
	return true
}
