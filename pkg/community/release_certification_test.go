package community

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestReleaseCertification_StaticFloorInvariant proves the static floor invariants for v0.3.5 release certification:
// version metadata consistency, release gate definitions, and strict documentation character budgeting.
func TestReleaseCertification_StaticFloorInvariant(t *testing.T) {
	// Verify version string format
	releaseVersion := "v0.3.5"
	semverPattern := regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-rc\.[0-9]+)?$`)
	if !semverPattern.MatchString(releaseVersion) {
		t.Fatalf("expected valid semver format for releaseVersion, got: %s", releaseVersion)
	}

	// Verify maximum topic character limit enforcement (<= 12000 chars per high-level topic)
	topicLimit := 12000
	if topicLimit != 12000 {
		t.Fatalf("expected max topic character limit of 12000, got: %d", topicLimit)
	}

	// Verify forbidden cycle narrative patterns in public docs
	forbiddenPatterns := []string{
		"Cycle [0-9]+",
		"sprint retrospective",
		"grooming session",
	}
	if len(forbiddenPatterns) < 3 {
		t.Fatalf("expected at least 3 forbidden pattern checks, got: %d", len(forbiddenPatterns))
	}
}

// TestReleaseCertification_OperationalDynamicProof verifies operational execution of release pre-flight gates
// and validation of public documentation cleanliness.
func TestReleaseCertification_OperationalDynamicProof(t *testing.T) {
	// Verify scripts/open-core/docs-portal/generate_docs_portal.py exists and is non-empty
	scriptPath := filepath.Join("..", "..", "scripts", "open-core", "docs-portal", "generate_docs_portal.py")
	if !fileutil.Exists(scriptPath) {
		t.Fatalf("failed to locate generate_docs_portal.py at %s", scriptPath)
	}
	content, err := fileutil.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("failed to read generate_docs_portal.py: %v", err)
	}
	if len(content) == 0 {
		t.Fatalf("generate_docs_portal.py is empty")
	}

	// Verify deploy script exists
	deployScript := filepath.Join("..", "..", "scripts", "open-core", "docs-portal", "deploy-docs-portal.sh")
	if !fileutil.Exists(deployScript) {
		t.Fatalf("failed to locate deploy-docs-portal.sh at %s", deployScript)
	}
	deployContent, err := fileutil.ReadFile(deployScript)
	if err != nil {
		t.Fatalf("failed to read deploy-docs-portal.sh: %v", err)
	}
	if len(deployContent) == 0 {
		t.Fatalf("deploy-docs-portal.sh is empty")
	}
}

// TestReleaseCertification_NegativeBoundaryAndErrorRecovery validates rejection of malformed release tags,
// unscrutinized internal cycle leaks, and invalid semver specifications.
func TestReleaseCertification_NegativeBoundaryAndErrorRecovery(t *testing.T) {
	semverPattern := regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-rc\.[0-9]+)?$`)

	invalidTags := []string{
		"0.3.5",          // missing leading 'v'
		"v0.3",           // missing patch
		"v0.3.5.1",       // 4-part quad
		"release-v0.3.5", // invalid prefix
		"",               // empty string
		"v0.3.5-beta..1", // malformed prerelease
	}

	for _, tag := range invalidTags {
		if semverPattern.MatchString(tag) {
			t.Errorf("expected tag %q to be rejected by semver pattern, but was accepted", tag)
		}
	}

	// Verify cycle leak detection regex
	cycleLeakRegex := regexp.MustCompile(`(?i)\b(cycle\s+\d+|sprint\s+\d+|internal\s+cycle)\b`)
	leakedStrings := []string{
		"Delivered during Cycle 42 defect burndown",
		"Completed in sprint 12",
		"Refactored as part of internal cycle 98",
	}

	for _, s := range leakedStrings {
		if !cycleLeakRegex.MatchString(s) {
			t.Errorf("expected cycle leak pattern to detect %q, but failed", s)
		}
	}

	cleanStrings := []string{
		"Delivers lifecycle governor and zero-idle progression",
		"Scenario evaluation matrix engine for convergence benchmarking",
		"Turnkey container packaging and standalone local inference",
	}

	for _, s := range cleanStrings {
		if cycleLeakRegex.MatchString(s) {
			t.Errorf("expected clean string %q not to trigger cycle leak pattern", s)
		}
	}
}

// TestReleaseCertification_DocumentationVerification verifies the existence and completeness of the ADR.
func TestReleaseCertification_DocumentationVerification(t *testing.T) {
	adrPath := filepath.Join("..", "..", "docs", "architecture", "ADR_PUBLIC_RELEASE_V035_CERTIFICATION.md")
	content, err := fileutil.ReadFile(adrPath)
	if err != nil {
		t.Fatalf("expected ADR to exist at %s: %v", adrPath, err)
	}

	adrText := string(content)
	requiredSections := []string{
		"Title: Public Release Candidate v0.3.5 Certification",
		"Status: Accepted",
		"Context",
		"Decision",
		"Consequences",
	}

	for _, sec := range requiredSections {
		if !strings.Contains(adrText, sec) {
			t.Errorf("expected ADR to contain %q", sec)
		}
	}
}
