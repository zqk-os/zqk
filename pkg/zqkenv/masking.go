package zqkenv

import (
	"regexp"
	"strings"
)

var sensitiveKeyPattern = regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password|auth[_-]?token|credential|bearer|private[_-]?key|authorization)`)

// IsSensitiveKey returns true if the key matches sensitive credential names.
func IsSensitiveKey(key string) bool {
	return sensitiveKeyPattern.MatchString(key)
}

// MaskSensitiveValue returns a redacted string if the key or value is detected to be sensitive.
func MaskSensitiveValue(key, value string) string {
	if value == "" {
		return ""
	}
	if IsSensitiveKey(key) {
		return "******"
	}
	// Also check if the value itself looks like a bearer token or secret string
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return "Bearer ******"
	}
	return value
}

// SanitizeFields recursively sanitizes map fields by masking sensitive keys and values.
func SanitizeFields(fields map[string]any) map[string]any {
	if len(fields) == 0 {
		return fields
	}
	out := make(map[string]any, len(fields))
	for k, v := range fields {
		if IsSensitiveKey(k) {
			out[k] = "******"
			continue
		}
		switch val := v.(type) {
		case string:
			out[k] = MaskSensitiveValue(k, val)
		case map[string]any:
			out[k] = SanitizeFields(val)
		default:
			out[k] = v
		}
	}
	return out
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
