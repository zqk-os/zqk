package community

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestSecuritySLA_FunctionalAcceptance verifies that SECURITY.md exists,
// defines the CVSS-based triage and remediation SLA matrix, and provides explicit advisory classifications
// (CRIT-1789707075577756000-971188ef).
func TestSecuritySLA_FunctionalAcceptance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	secPath := filepath.Join(root, "SECURITY.md")
	if !fileutil.Exists(secPath) {
		t.Fatalf("SECURITY.md missing at %s", secPath)
	}

	contentBytes, err := os.ReadFile(secPath)
	if err != nil {
		t.Fatalf("failed reading SECURITY.md: %v", err)
	}
	content := string(contentBytes)

	// Check for SLA Matrix headings and definitions
	if !strings.Contains(content, "Vulnerability Triage and Response SLA Matrix") {
		t.Errorf("expected Vulnerability Triage and Response SLA Matrix in SECURITY.md")
	}
	for _, severity := range []string{"Critical", "High", "Medium", "Low"} {
		if !strings.Contains(content, severity) {
			t.Errorf("expected severity level %s in SLA matrix", severity)
		}
	}
	if !strings.Contains(content, "≤ 24 hours") {
		t.Errorf("expected critical response SLA ≤ 24 hours in SECURITY.md")
	}
	if !strings.Contains(content, "Security Advisory Classifications") {
		t.Errorf("expected Security Advisory Classifications in SECURITY.md")
	}
}

// TestSecuritySLA_BoundaryAndErrorHandling verifies advisory classification validation
// and coordinated disclosure workflow structure (CRIT-1789707075577757000-7a46c2a1).
func TestSecuritySLA_BoundaryAndErrorHandling(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	secPath := filepath.Join(root, "SECURITY.md")
	contentBytes, err := os.ReadFile(secPath)
	if err != nil {
		t.Fatalf("failed reading SECURITY.md: %v", err)
	}
	content := string(contentBytes)

	// Verify required disclosure steps
	for _, step := range []string{"Report", "Acknowledge", "Assessment", "Coordination"} {
		if !strings.Contains(content, step) {
			t.Errorf("expected disclosure process step %s in SECURITY.md", step)
		}
	}

	// Verify CVSS range definitions are properly bounded
	if !strings.Contains(content, "9.0 – 10.0") || !strings.Contains(content, "7.0 – 8.9") {
		t.Errorf("expected standard CVSS v3.1 score ranges in SECURITY.md")
	}
}

// TestSecuritySLA_IntegrationAndConformance verifies supply hygiene policies,
// secret scanning, and SBOM linkage in the security policy (CRIT-1789707075577758000-d3678d26).
func TestSecuritySLA_IntegrationAndConformance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	secPath := filepath.Join(root, "SECURITY.md")
	contentBytes, err := os.ReadFile(secPath)
	if err != nil {
		t.Fatalf("failed reading SECURITY.md: %v", err)
	}
	content := string(contentBytes)

	if !strings.Contains(content, "Supply hygiene") {
		t.Errorf("expected Supply hygiene section in SECURITY.md")
	}
	if !strings.Contains(content, "secret-scan.yml") || !strings.Contains(content, "sbom.yml") {
		t.Errorf("expected secret-scan and sbom workflow references in SECURITY.md")
	}
}
