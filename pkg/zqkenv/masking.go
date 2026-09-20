package zqkenv

import (
	"regexp"
	"strings"
)

var sensitiveKeyPattern = regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password|auth|credential|bearer|private[_-]?key)`)

// MaskSensitiveValue returns a redacted string if the key or value is detected to be sensitive.
func MaskSensitiveValue(key, value string) string {
	if value == "" {
		return ""
	}
	if sensitiveKeyPattern.MatchString(key) {
		return "******"
	}
	// Also check if the value itself looks like a bearer token or secret string
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return "Bearer ******"
	}
	return value
}

// SanitizeEnvironment sanitizes KEY=VALUE environment pairs by masking sensitive variables.
func SanitizeEnvironment(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			out = append(out, kv)
			continue
		}
		masked := MaskSensitiveValue(k, v)
		out = append(out, k+"="+masked)
	}
	return out
}
