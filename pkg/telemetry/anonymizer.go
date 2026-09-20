package telemetry

import (
	"regexp"
	"strings"
	"sync"
)

var (
	// emailPattern matches typical email addresses.
	emailPattern = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)

	// ipv4Pattern matches IPv4 addresses (excluding 127.0.0.1 or 0.0.0.0 if desired, or all IPv4).
	ipv4Pattern = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)

	// tokenPattern matches common API tokens, bearer tokens, JWTs, and secret keys.
	tokenPattern = regexp.MustCompile(`(?i)(?:bearer\s+[a-zA-Z0-9_\-\.]{10,}|(?:ghp|gho|ghu|ghs|ghr)_[a-zA-Z0-9]{36,}|(?:sk|pk)_(?:live|test)_[a-zA-Z0-9]{20,}|eyJ[a-zA-Z0-9_\-]{10,}\.[a-zA-Z0-9_\-]{10,}\.[a-zA-Z0-9_\-]{10,})`)

	// userPathPattern matches local user home directories (e.g., /Users/username/... or /home/username/...).
	userPathPattern = regexp.MustCompile(`(/(?:Users|home)/[a-zA-Z0-9._\-]+)`)
)

var (
	optInMu    sync.RWMutex
	optInState bool
)

// SetOptIn sets whether telemetry collection is explicitly opted-in.
// Community edition default is opt-out (false).
func SetOptIn(enabled bool) {
	optInMu.Lock()
	defer optInMu.Unlock()
	optInState = enabled
}

// IsOptedIn reports whether telemetry collection is active.
func IsOptedIn() bool {
	optInMu.RLock()
	defer optInMu.RUnlock()
	return optInState
}

// SanitizePayload scrubs all PII, IP addresses, email addresses, tokens, and home directory paths.
func SanitizePayload(raw string) string {
	if raw == "" {
		return ""
	}

	// 1. Scrub tokens and bearer auth
	scrubbed := tokenPattern.ReplaceAllString(raw, "[REDACTED_TOKEN]")

	// 2. Scrub emails
	scrubbed = emailPattern.ReplaceAllString(scrubbed, "[REDACTED_EMAIL]")

	// 3. Scrub IPv4 addresses
	scrubbed = ipv4Pattern.ReplaceAllString(scrubbed, "[REDACTED_IP]")

	// 4. Scrub user home directories
	scrubbed = userPathPattern.ReplaceAllString(scrubbed, "/[USER_HOME]")

	// 5. Check for key=value sensitive assignments
	lines := strings.Split(scrubbed, "\n")
	for i, line := range lines {
		if strings.Contains(line, "=") || strings.Contains(line, ":") {
			lines[i] = sanitizeLine(line)
		}
	}

	return strings.Join(lines, "\n")
}

var sensitiveKeyPattern = regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password|auth|credential|bearer|private[_-]?key)`)

func sanitizeLine(line string) string {
	// Look for key=value or key: value pairs
	var sep string
	if strings.Contains(line, "=") {
		sep = "="
	} else if strings.Contains(line, ": ") {
		sep = ": "
	} else {
		return line
	}

	parts := strings.SplitN(line, sep, 2)
	if len(parts) == 2 {
		keyPart := strings.TrimSpace(parts[0])
		if sensitiveKeyPattern.MatchString(keyPart) {
			return parts[0] + sep + "[REDACTED_SECRET]"
		}
	}
	return line
}
