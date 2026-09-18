package community

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// TestPrivacyGuard_FunctionalAcceptance tests that token masking, PII redaction, and secret
// masking operate exhaustively on raw inputs (CRIT-1789704994005580000-cebc9c83).
func TestPrivacyGuard_FunctionalAcceptance(t *testing.T) {
	guard := NewPrivacyGuard("test-cluster-seed-12345")
	guard.SetOptIn(true)

	fakeGhp := "gh" + "p_" + strings.Repeat("a", 36)
	fakeAkia := "AK" + "IA" + strings.Repeat("Z", 16)
	fakePem := "-----BEG" + "IN RSA PRIV" + "ATE KEY-----\nMIIEowIBAAKCAQEA0Y1u...FAKEKEYDATA...\n-----END RSA PRIV" + "ATE KEY-----"
	fakeSecretVal := "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"

	raw := `
[PANIC TRACE]
runtime error: invalid memory address or nil pointer dereference
goroutine 42 [running]:
main.runProcess(0x104820, 0x1400018a000)
	/Users/alice/repo/zqk/cmd/zqk/main.go:128 +0x240
github_auth_token=` + fakeGhp + `
aws_access_key=` + fakeAkia + `
aws_secret_key=` + fakeSecretVal + `
user_contact: engineer.support@company.org
client_ip: 10.200.45.12
hardware_mac: 00:1A:2B:3C:4D:5E
webhook_target: https://admin:SuperSecretPass123@api.partner.net/v1/event?secret_token=tok_987654321&version=2
private_cert:
` + fakePem + `
`

	sanitized, redactionCount := guard.SanitizeText(raw)

	if redactionCount < 8 {
		t.Errorf("expected at least 8 redactions, got %d", redactionCount)
	}

	// 1. Tokens and Keys
	if strings.Contains(sanitized, fakeGhp) {
		t.Errorf("GitHub token was not redacted")
	}
	if strings.Contains(sanitized, fakeAkia) {
		t.Errorf("AWS access key was not redacted")
	}
	if strings.Contains(sanitized, fakeSecretVal) {
		t.Errorf("AWS secret key was not redacted")
	}
	if strings.Contains(sanitized, "tok_987654321") {
		t.Errorf("Webhook token parameter was not redacted")
	}
	if strings.Contains(sanitized, "FAKEKEYDATA") {
		t.Errorf("Private key block was not redacted")
	}

	// 2. Personal Information
	if strings.Contains(sanitized, "engineer.support@company.org") {
		t.Errorf("Email address was not redacted")
	}
	if strings.Contains(sanitized, "10.200.45.12") {
		t.Errorf("IP address was not redacted")
	}
	if strings.Contains(sanitized, "00:1A:2B:3C:4D:5E") {
		t.Errorf("MAC address was not redacted")
	}

	// 3. File Paths
	if strings.Contains(sanitized, "/Users/alice") {
		t.Errorf("User home directory was not redacted")
	}

	// 4. URL Credentials
	if strings.Contains(sanitized, "SuperSecretPass123") {
		t.Errorf("URL credentials were not redacted")
	}

	// Verify placeholders
	if !strings.Contains(sanitized, RedactedTokenPlaceholder) {
		t.Errorf("missing %s placeholder", RedactedTokenPlaceholder)
	}
	if !strings.Contains(sanitized, RedactedEmailPlaceholder) {
		t.Errorf("missing %s placeholder", RedactedEmailPlaceholder)
	}
	if !strings.Contains(sanitized, RedactedIPPlaceholder) {
		t.Errorf("missing %s placeholder", RedactedIPPlaceholder)
	}
	if !strings.Contains(sanitized, RedactedUserHomePlaceholder) {
		t.Errorf("missing %s placeholder", RedactedUserHomePlaceholder)
	}
	if !strings.Contains(sanitized, RedactedPrivateKeyPlaceholder) {
		t.Errorf("missing %s placeholder", RedactedPrivateKeyPlaceholder)
	}
}

// TestPrivacyGuard_BoundaryAndErrorHandling tests default opt-out posture, fail-closed
// gates, empty string handling, and environment toggle evaluation (CRIT-1789704994005581000-85589d66).
func TestPrivacyGuard_BoundaryAndErrorHandling(t *testing.T) {
	guard := NewPrivacyGuard("boundary-test")

	// 1. Fail-closed: default must be opt-out
	if guard.IsOptedIn() {
		t.Fatalf("expected NewPrivacyGuard to default to opt-out (false)")
	}

	// 2. Reject crash diagnostic when opted-out
	_, _, err := guard.SanitizeCrashDiagnostic("panic: runtime fault")
	if err == nil {
		t.Fatalf("expected error when processing diagnostic in opted-out posture")
	}

	// 3. Toggle opt-in and confirm acceptance
	guard.SetOptIn(true)
	if !guard.IsOptedIn() {
		t.Fatalf("expected guard to be opted in after SetOptIn(true)")
	}
	cleanText, record, err := guard.SanitizeCrashDiagnostic("panic: sample error\n/home/bob/app.go:10")
	if err != nil {
		t.Fatalf("unexpected error when opted in: %v", err)
	}
	if strings.Contains(cleanText, "/home/bob") {
		t.Errorf("expected /home/bob to be sanitized")
	}
	if record.RedactionsApplied != 1 {
		t.Errorf("expected 1 redaction, got %d", record.RedactionsApplied)
	}

	// 4. Edge case: Empty input
	emptyClean, count := guard.SanitizeText("")
	if emptyClean != "" || count != 0 {
		t.Errorf("expected empty string and 0 redactions for empty input")
	}

	// 5. Edge case: Clean non-PII diagnostic text
	safeText := "status=ready worker_id=4 duration_ms=120"
	sanitizedSafe, safeCount := guard.SanitizeText(safeText)
	if sanitizedSafe != safeText || safeCount != 0 {
		t.Errorf("expected safe text to remain unmodified, got %q (count %d)", sanitizedSafe, safeCount)
	}

	// 6. Environment variable evaluation via zqkenv
	zqkenv.TelemetryOptIn().Set("1")
	defer zqkenv.TelemetryOptIn().Unset()
	if !guard.EvaluateEnvironment() {
		t.Errorf("expected EvaluateEnvironment to return true when TelemetryOptIn=1")
	}
	if !guard.IsOptedIn() {
		t.Errorf("expected IsOptedIn to be true")
	}

	zqkenv.TelemetryOptIn().Set("0")
	if guard.EvaluateEnvironment() {
		t.Errorf("expected EvaluateEnvironment to return false when TelemetryOptIn=0")
	}
	if guard.IsOptedIn() {
		t.Errorf("expected IsOptedIn to be false")
	}
}

// TestPrivacyGuard_IntegrationAndConformance verifies audit trail recording, cluster
// hashing non-reversibility, and URL query parameter scrubbing (CRIT-1789704994005582000-c669d751).
func TestPrivacyGuard_IntegrationAndConformance(t *testing.T) {
	guard := NewPrivacyGuard("production-cluster-enterprise-node")
	guard.SetOptIn(true)

	// 1. Test Cluster Hash
	hash := guard.ClusterHash()
	if len(hash) != 16 {
		t.Errorf("expected 16-character cluster hash, got %q (len %d)", hash, len(hash))
	}
	if strings.Contains(hash, "production") || strings.Contains(hash, "node") {
		t.Errorf("cluster hash leaked seed plaintext")
	}

	// 2. Audit Trail Tracking
	fakeToken2 := "gh" + "p_" + strings.Repeat("1", 36)
	stack1 := "panic: user /Users/charlie/dev/file.go:50 with email test@charlie.org"
	stack2 := "error: token=" + fakeToken2

	_, _, err := guard.SanitizeCrashDiagnostic(stack1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, _, err = guard.SanitizeCrashDiagnostic(stack2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	history := guard.GetAuditHistory()
	if len(history) != 2 {
		t.Fatalf("expected 2 audit records, got %d", len(history))
	}

	if history[0].RedactionsApplied != 2 { // /Users/charlie and email
		t.Errorf("record 1 expected 2 redactions, got %d", history[0].RedactionsApplied)
	}
	if history[1].RedactionsApplied < 1 { // ghp token
		t.Errorf("record 2 expected at least 1 redaction, got %d", history[1].RedactionsApplied)
	}
	if !history[0].OptInVerified || !history[1].OptInVerified {
		t.Errorf("expected OptInVerified to be true on all records")
	}

	// 3. URL Query Parameter Sanitization
	rawURL := "https://telemetry.zqk.io/api/v2/ingest?api_key=secretKey123&session_id=sess456&auth_token=jwtBearer789"
	cleanedURL := guard.SanitizeURLQuery(rawURL)

	if strings.Contains(cleanedURL, "secretKey123") {
		t.Errorf("expected api_key value to be sanitized in URL, got %s", cleanedURL)
	}
	if strings.Contains(cleanedURL, "jwtBearer789") {
		t.Errorf("expected auth_token value to be sanitized in URL, got %s", cleanedURL)
	}
	if !strings.Contains(cleanedURL, "session_id=sess456") {
		t.Errorf("expected benign session_id parameter to be preserved in URL, got %s", cleanedURL)
	}
}
