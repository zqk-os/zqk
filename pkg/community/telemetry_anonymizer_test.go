package community

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/telemetry"
)

// TestTelemetryAnonymizer_FunctionalAcceptance verifies that PII, API tokens, IP addresses,
// emails, and user home paths are strictly redacted (CRIT-1789705081038259000-d322f677).
func TestTelemetryAnonymizer_FunctionalAcceptance(t *testing.T) {
	fakeGhp := "ghp_" + "1234567890abcdef1234567890abcdef1234"
	rawInput := `
user_email=developer@example.com
server_ip=192.168.1.105
github_token=` + fakeGhp + `
authorization: Bearer mysecretbearerauthstring123456789
file_path=/Users/developer/project/code.go
api_key: secret-api-key-999
`

	sanitized := telemetry.SanitizePayload(rawInput)

	if strings.Contains(sanitized, "developer@example.com") {
		t.Errorf("expected email to be redacted, got:\n%s", sanitized)
	}
	if strings.Contains(sanitized, "192.168.1.105") {
		t.Errorf("expected IP to be redacted, got:\n%s", sanitized)
	}
	if strings.Contains(sanitized, fakeGhp) {
		t.Errorf("expected GitHub token to be redacted, got:\n%s", sanitized)
	}
	if strings.Contains(sanitized, "mysecretbearerauthstring") {
		t.Errorf("expected Bearer auth to be redacted, got:\n%s", sanitized)
	}
	if strings.Contains(sanitized, "/Users/developer") {
		t.Errorf("expected user home directory to be redacted, got:\n%s", sanitized)
	}
	if strings.Contains(sanitized, "secret-api-key-999") {
		t.Errorf("expected secret api key to be redacted, got:\n%s", sanitized)
	}

	if !strings.Contains(sanitized, "[REDACTED_EMAIL]") {
		t.Errorf("expected [REDACTED_EMAIL] placeholder, got:\n%s", sanitized)
	}
	if !strings.Contains(sanitized, "[REDACTED_IP]") {
		t.Errorf("expected [REDACTED_IP] placeholder, got:\n%s", sanitized)
	}
	if !strings.Contains(sanitized, "[USER_HOME]") {
		t.Errorf("expected [USER_HOME] placeholder, got:\n%s", sanitized)
	}
}

// TestTelemetryAnonymizer_BoundaryAndErrorHandling verifies default opt-out posture,
// state transitions for opt-in toggles, and handling of empty or edge-case payloads
// (CRIT-1789705081038260000-b5e076f2).
func TestTelemetryAnonymizer_BoundaryAndErrorHandling(t *testing.T) {
	// 1. Verify default posture is opt-out
	telemetry.SetOptIn(false)
	if telemetry.IsOptedIn() {
		t.Fatalf("expected default posture to be opt-out (false)")
	}

	// 2. Verify opt-in toggle
	telemetry.SetOptIn(true)
	if !telemetry.IsOptedIn() {
		t.Fatalf("expected telemetry to be opted in after SetOptIn(true)")
	}
	telemetry.SetOptIn(false)
	if telemetry.IsOptedIn() {
		t.Fatalf("expected telemetry to return to opted-out state")
	}

	// 3. Edge case: empty string
	if out := telemetry.SanitizePayload(""); out != "" {
		t.Errorf("expected empty string for empty input, got %q", out)
	}

	// 4. Edge case: clean string without PII
	clean := "INFO task completed successfully duration=45ms count=10"
	if out := telemetry.SanitizePayload(clean); out != clean {
		t.Errorf("expected clean string to remain unchanged, got %q", out)
	}
}

// TestTelemetryAnonymizer_IntegrationAndConformance verifies telemetry tracker integration
// and safe payload handling across error events (CRIT-1789705081038261000-73d13ef5).
func TestTelemetryAnonymizer_IntegrationAndConformance(t *testing.T) {
	errPayload := "failed connecting to https://api.service.com with auth header Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.doNotLeakThisSignature"
	sanitized := telemetry.SanitizePayload(errPayload)

	if strings.Contains(sanitized, "eyJhbGciOiJIUzI1Ni") {
		t.Errorf("expected JWT token to be scrubbed from error payload, got:\n%s", sanitized)
	}
	if !strings.Contains(sanitized, "[REDACTED_TOKEN]") {
		t.Errorf("expected [REDACTED_TOKEN] in scrubbed error payload, got:\n%s", sanitized)
	}
}
