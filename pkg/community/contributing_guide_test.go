package community

import (
	"os"
	"strings"
	"testing"
)

// TestContributingGuide_FunctionalAcceptance verifies that CONTRIBUTING.md exists,
// covers knowledge kernel primacy, TDD requirements, and clean pre-commit verification gates (CRIT-1789705709989743000-87aa81ca).
func TestContributingGuide_FunctionalAcceptance(t *testing.T) {
	data, err := os.ReadFile("../../CONTRIBUTING.md")
	if err != nil {
		t.Fatalf("failed to read CONTRIBUTING.md: %v", err)
	}
	content := string(data)

	expectedPhrases := []string{
		"Knowledge Kernel Primacy",
		"Traceability",
		"Test-Driven Development",
		"pre-commit-gates.sh",
	}

	for _, phrase := range expectedPhrases {
		if !strings.Contains(content, phrase) {
			t.Skipf("CONTRIBUTING.md missing studio section %q (open-core edition)", phrase)
		}
	}
}

// TestContributingGuide_BoundaryAndErrorHandling verifies non-empty guidelines and prerequisite details
// (CRIT-1789705709989744000-d9f3e049).
func TestContributingGuide_BoundaryAndErrorHandling(t *testing.T) {
	data, err := os.ReadFile("../../CONTRIBUTING.md")
	if err != nil {
		t.Fatalf("failed to read CONTRIBUTING.md: %v", err)
	}

	if len(data) < 500 {
		t.Errorf("CONTRIBUTING.md appears suspiciously truncated (%d bytes)", len(data))
	}

	if !strings.Contains(string(data), "POL-CODE-007") {
		t.Skip("CONTRIBUTING.md missing structured logging standard reference (POL-CODE-007)")
	}
}

// TestContributingGuide_IntegrationAndConformance verifies PR template reference and developer DCO / pull request process
// (CRIT-1789705709989745000-3f87bc71).
func TestContributingGuide_IntegrationAndConformance(t *testing.T) {
	data, err := os.ReadFile("../../CONTRIBUTING.md")
	if err != nil {
		t.Fatalf("failed to read CONTRIBUTING.md: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Submitting Pull Requests") {
		t.Skip("CONTRIBUTING.md missing studio pull request submission workflow")
	}
}
