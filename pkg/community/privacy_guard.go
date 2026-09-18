package community

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/telemetry"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

var (
	// piiEmailRegex matches standard email addresses.
	piiEmailRegex = regexp.MustCompile(`(?i)\b[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}\b`)

	// piiIPv4Regex matches standard IPv4 addresses.
	piiIPv4Regex = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)

	// piiMacRegex matches standard MAC addresses.
	piiMacRegex = regexp.MustCompile(`(?i)\b(?:[0-9a-f]{2}[:-]){5}[0-9a-f]{2}\b`)

	// piiUserPathRegex matches Unix and Windows user home directory paths.
	piiUserPathRegex = regexp.MustCompile(`(?i)(?:/(?:Users|home)/[a-zA-Z0-9._\-]+|[a-zA-Z]:\\Users\\[a-zA-Z0-9._\-]+)`)

	// piiTokenRegex matches popular API keys, bearer tokens, JWTs, and secret keys.
	piiTokenRegex = regexp.MustCompile(`(?i)(?:bearer\s+[a-zA-Z0-9_\-\.]{10,}|(?:ghp|gho|ghu|ghs|ghr|glpat|slack_token)_[a-zA-Z0-9]{20,}|(?:sk|pk)_(?:live|test)_[a-zA-Z0-9]{20,}|AKIA[0-9A-Z]{16}|eyJ[a-zA-Z0-9_\-]{10,}\.[a-zA-Z0-9_\-]{10,}\.[a-zA-Z0-9_\-]{10,})`)

	// piiPrivateKeyRegex matches PEM private key blocks.
	piiPrivateKeyRegex = regexp.MustCompile(`(?s)-----BEGIN[ A-Z0-9_-]*PRIVATE KEY-----.*?-----END[ A-Z0-9_-]*PRIVATE KEY-----`)

	// piiSensitiveKeyAssignment matches key=val or key: val where key contains sensitive keywords.
	piiSensitiveKeyAssignment = regexp.MustCompile(`(?i)\b([a-z0-9_\-]*(?:api[_-]?key|secret|token|password|auth|credential|bearer|private[_-]?key)[a-z0-9_\-]*)\s*([=:])\s*([^\s,;&"\']+)`)
)

const (
	// RedactedTokenPlaceholder is used to replace secret tokens.
	RedactedTokenPlaceholder = "[REDACTED_TOKEN]"
	// RedactedEmailPlaceholder is used to replace email addresses.
	RedactedEmailPlaceholder = "[REDACTED_EMAIL]"
	// RedactedIPPlaceholder is used to replace IPv4 addresses.
	RedactedIPPlaceholder = "[REDACTED_IP]"
	// RedactedUserHomePlaceholder is used to replace user directory paths.
	RedactedUserHomePlaceholder = "[USER_HOME]"
	// RedactedSecretPlaceholder is used to replace sensitive assignment values.
	RedactedSecretPlaceholder = "[REDACTED_SECRET]"
	// RedactedPrivateKeyPlaceholder is used to replace private key blocks.
	RedactedPrivateKeyPlaceholder = "[REDACTED_PRIVATE_KEY]"
	// RedactedURLCredsPlaceholder is used to replace basic auth in URLs.
	RedactedURLCredsPlaceholder = "[REDACTED_CREDS]@"
)

// PrivacyAuditRecord tracks an audit entry for sanitized diagnostics.
type PrivacyAuditRecord struct {
	RecordID           string    `json:"record_id"`
	Timestamp          time.Time `json:"timestamp"`
	OriginalByteCount  int       `json:"original_bytes"`
	SanitizedByteCount int       `json:"sanitized_bytes"`
	RedactionsApplied  int       `json:"redactions_applied"`
	ClusterHash        string    `json:"cluster_hash"`
	OptInVerified      bool      `json:"opt_in_verified"`
}

// PrivacyGuard coordinates PII sanitization, local token masking, and explicit opt-in verification.
type PrivacyGuard struct {
	mu           sync.RWMutex
	optedIn      bool
	clusterSeed  string
	auditHistory []PrivacyAuditRecord
}

// NewPrivacyGuard creates a new PrivacyGuard instance. Defaults strictly to opt-out.
func NewPrivacyGuard(clusterSeed string) *PrivacyGuard {
	if clusterSeed == "" {
		clusterSeed = "community-default"
	}
	return &PrivacyGuard{
		optedIn:      false,
		clusterSeed:  clusterSeed,
		auditHistory: make([]PrivacyAuditRecord, 0),
	}
}

// SetOptIn configures explicit opt-in status.
func (g *PrivacyGuard) SetOptIn(enabled bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.optedIn = enabled
	telemetry.SetOptIn(enabled)
}

// IsOptedIn reports whether opt-in permission has been granted.
func (g *PrivacyGuard) IsOptedIn() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.optedIn
}

// EvaluateEnvironment checks environment variables for explicit opt-in settings.
func (g *PrivacyGuard) EvaluateEnvironment() bool {
	envVal := strings.ToLower(strings.TrimSpace(zqkenv.TelemetryOptIn().Get()))
	if envVal == "1" || envVal == "true" || envVal == "yes" || envVal == "on" {
		g.SetOptIn(true)
		return true
	}
	g.SetOptIn(false)
	return false
}

// ClusterHash returns a deterministic one-way SHA-256 hash prefix for cluster-level diagnostics.
func (g *PrivacyGuard) ClusterHash() string {
	h := sha256.New()
	h.Write([]byte(g.clusterSeed))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// SanitizeText applies zero-PII redaction and token masking to a string.
func (g *PrivacyGuard) SanitizeText(raw string) (string, int) {
	if raw == "" {
		return "", 0
	}

	redactionCount := 0

	// 1. Private keys
	if piiPrivateKeyRegex.MatchString(raw) {
		raw = piiPrivateKeyRegex.ReplaceAllStringFunc(raw, func(_ string) string {
			redactionCount++
			return RedactedPrivateKeyPlaceholder
		})
	}

	// 2. High-entropy API tokens and Bearer credentials
	if piiTokenRegex.MatchString(raw) {
		raw = piiTokenRegex.ReplaceAllStringFunc(raw, func(_ string) string {
			redactionCount++
			return RedactedTokenPlaceholder
		})
	}

	// 3. Email addresses
	if piiEmailRegex.MatchString(raw) {
		raw = piiEmailRegex.ReplaceAllStringFunc(raw, func(_ string) string {
			redactionCount++
			return RedactedEmailPlaceholder
		})
	}

	// 4. IPv4 addresses (ignoring localhost / loopback 127.0.0.1 and broadcast 0.0.0.0 if not considered PII, but here we redact all IPv4)
	if piiIPv4Regex.MatchString(raw) {
		raw = piiIPv4Regex.ReplaceAllStringFunc(raw, func(_ string) string {
			redactionCount++
			return RedactedIPPlaceholder
		})
	}

	// 5. MAC addresses
	if piiMacRegex.MatchString(raw) {
		raw = piiMacRegex.ReplaceAllStringFunc(raw, func(_ string) string {
			redactionCount++
			return "[REDACTED_MAC]"
		})
	}

	// 6. User home paths
	if piiUserPathRegex.MatchString(raw) {
		raw = piiUserPathRegex.ReplaceAllStringFunc(raw, func(_ string) string {
			redactionCount++
			return RedactedUserHomePlaceholder
		})
	}

	// 7. Embedded URL credentials
	raw = sanitizeURLCredentials(raw, &redactionCount)

	// 8. Key-value sensitive parameter assignments
	if piiSensitiveKeyAssignment.MatchString(raw) {
		raw = piiSensitiveKeyAssignment.ReplaceAllStringFunc(raw, func(match string) string {
			sub := piiSensitiveKeyAssignment.FindStringSubmatch(match)
			if len(sub) >= 4 {
				redactionCount++
				return fmt.Sprintf("%s%s%s", sub[1], sub[2], RedactedSecretPlaceholder)
			}
			return match
		})
	}

	return raw, redactionCount
}

// sanitizeURLCredentials removes user:password@ credentials from any URLs present in text.
func sanitizeURLCredentials(text string, count *int) string {
	urlRegex := regexp.MustCompile(`(https?://)([^/\s:@]+):([^/\s:@]+)@([^\s]+)`)
	return urlRegex.ReplaceAllStringFunc(text, func(m string) string {
		*count++
		return urlRegex.ReplaceAllString(m, fmt.Sprintf("${1}%s${4}", RedactedURLCredsPlaceholder))
	})
}

// SanitizeCrashDiagnostic takes a crash diagnostic log or panic stack trace and
// strips path usernames, env vars, and token traces while preserving relevant code frames.
func (g *PrivacyGuard) SanitizeCrashDiagnostic(stackTrace string) (string, PrivacyAuditRecord, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.optedIn {
		return "", PrivacyAuditRecord{}, errors.New("cannot process crash diagnostic: telemetry opt-in is false")
	}

	origLen := len(stackTrace)
	sanitized, redactions := g.SanitizeText(stackTrace)

	record := PrivacyAuditRecord{
		RecordID:           fmt.Sprintf("audit-%d", time.Now().UnixNano()),
		Timestamp:          time.Now().UTC(),
		OriginalByteCount:  origLen,
		SanitizedByteCount: len(sanitized),
		RedactionsApplied:  redactions,
		ClusterHash:        g.ClusterHash(),
		OptInVerified:      true,
	}

	g.auditHistory = append(g.auditHistory, record)
	return sanitized, record, nil
}

// SanitizeURLQuery scrubs sensitive parameters (key, token, secret, signature) from URL query strings.
func (g *PrivacyGuard) SanitizeURLQuery(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		sanitized, _ := g.SanitizeText(rawURL)
		return sanitized
	}

	// Scrub user info if present
	if parsed.User != nil {
		parsed.User = url.User("[REDACTED_USER]")
	}

	// Scrub query parameters
	values := parsed.Query()
	for paramKey := range values {
		lower := strings.ToLower(paramKey)
		if strings.Contains(lower, "token") || strings.Contains(lower, "key") ||
			strings.Contains(lower, "secret") || strings.Contains(lower, "auth") ||
			strings.Contains(lower, "sig") {
			values.Set(paramKey, RedactedSecretPlaceholder)
		}
	}
	parsed.RawQuery = values.Encode()
	return parsed.String()
}

// GetAuditHistory returns a copy of all audit records generated by this guard.
func (g *PrivacyGuard) GetAuditHistory() []PrivacyAuditRecord {
	g.mu.RLock()
	defer g.mu.RUnlock()

	history := make([]PrivacyAuditRecord, len(g.auditHistory))
	copy(history, g.auditHistory)
	return history
}
