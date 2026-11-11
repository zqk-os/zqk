package community

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/acronyms"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/vet"
)

// TestDocsQualityGate_StaticFloorInvariant verifies that the documentation base
// adheres strictly to character budgeting, canonical acronym definitions, and
// zero leaked project-specific kernel IDs.
func TestDocsQualityGate_StaticFloorInvariant(t *testing.T) {
	repoRoot := "../.."
	cfg, err := vet.LoadConfig(repoRoot, "")
	if err != nil {
		t.Fatalf("failed to load vet gates config: %v", err)
	}

	findings, err := vet.CheckDocumentationQuality(repoRoot, cfg)
	if err != nil {
		t.Fatalf("CheckDocumentationQuality returned error: %v", err)
	}

	// Verify zero fatal error findings across public documentation
	errorCount := 0
	for _, f := range findings {
		if f.Severity == vet.SeverityError {
			errorCount++
			t.Errorf("Static Floor Invariant violated: %s", f.String())
		}
	}

	if errorCount > 0 {
		t.Fatalf("expected 0 documentation quality errors, found %d", errorCount)
	}

	// Verify all canonical acronyms have non-empty definitions
	terms := acronyms.CanonicalSeedTerms()
	if len(terms) == 0 {
		t.Fatal("expected at least 15 canonical seed acronym terms, got 0")
	}
	for _, term := range terms {
		if term.Code == "" || term.FullName == "" || term.Definition == "" {
			t.Errorf("canonical acronym %s has incomplete specification: %+v", term.Code, term)
		}
	}
}

// TestDocsQualityGate_OperationalDynamicProof executes the zqk-vet doc scrutiny
// suite end-to-end and validates fail-closed gate execution.
func TestDocsQualityGate_OperationalDynamicProof(t *testing.T) {
	repoRoot := "../.."
	cfg, err := vet.LoadConfig(repoRoot, "")
	if err != nil {
		t.Fatalf("failed to load gates config: %v", err)
	}

	runner := vet.NewRunner(repoRoot, cfg)
	report, err := runner.Run(vet.RunOptions{
		Suites:   []string{"docs"},
		FailFast: false,
	})
	if err != nil {
		t.Fatalf("runner.Run(docs) failed: %v", err)
	}

	if !report.Passed {
		t.Fatalf("operational proof failed: docs suite reported failure with %d errors", report.TotalErrors)
	}

	docsResult, ok := report.SuiteResults["docs"]
	if !ok {
		t.Fatal("operational proof failed: suite results missing 'docs' entry")
	}

	if !docsResult.Passed {
		t.Fatalf("docs suite result marked not passed: %+v", docsResult)
	}
}

// TestDocsQualityGate_NegativeAdversarialBoundary ensures that any injected
// contradictory acronym, leaked kernel ID, or cycle sprint narrative fails closed.
func TestDocsQualityGate_NegativeAdversarialBoundary(t *testing.T) {
	tempDir := t.TempDir()
	docsGuidesDir := filepath.Join(tempDir, "docs", "guides")
	if err := fileutil.MkdirAll(docsGuidesDir, 0755); err != nil {
		t.Fatalf("failed to create mock docs dir: %v", err)
	}

	// 1. Injected contradictory acronym file
	badAcronymContent := `# Subsystem Guide
## Overview
The ATK (Adversarial Test Kit) provides test automation for swarms.
`
	if err := fileutil.WriteFile(filepath.Join(docsGuidesDir, "BAD_ACRONYM.md"), []byte(badAcronymContent), 0644); err != nil {
		t.Fatalf("failed to write bad acronym file: %v", err)
	}

	// 2. Injected leaked project-specific kernel ID file
	leakedKernelIDContent := `# Execution Guide
## Implementation Details
Work is tracked under BLI-1793550036532835000-d003a18d for verification.
`
	if err := fileutil.WriteFile(filepath.Join(docsGuidesDir, "LEAKED_ID.md"), []byte(leakedKernelIDContent), 0644); err != nil {
		t.Fatalf("failed to write leaked ID file: %v", err)
	}

	// 3. Injected internal cycle sprint narrative file
	cycleNarrativeContent := `# Verification Guide
## Target Achievements
- Cycle 105 Target Verified: 100.0% statement coverage achieved.
`
	if err := fileutil.WriteFile(filepath.Join(docsGuidesDir, "CYCLE_NARRATIVE.md"), []byte(cycleNarrativeContent), 0644); err != nil {
		t.Fatalf("failed to write cycle narrative file: %v", err)
	}

	cfg := vet.DefaultConfig()
	findings, err := vet.CheckDocumentationQuality(tempDir, cfg)
	if err != nil {
		t.Fatalf("CheckDocumentationQuality returned unexpected error: %v", err)
	}

	var hasAcronymError, hasKernelIDError, hasCycleError bool
	for _, f := range findings {
		if f.CheckID == "doc/contradictory_acronym" {
			hasAcronymError = true
		}
		if f.CheckID == "doc/kernel_id_leak" {
			hasKernelIDError = true
		}
		if f.CheckID == "doc/cycle_narrative_leak" {
			hasCycleError = true
		}
	}

	if !hasAcronymError {
		t.Errorf("Negative Boundary failed: expected doc/contradictory_acronym error on bad ATK definition")
	}
	if !hasKernelIDError {
		t.Errorf("Negative Boundary failed: expected doc/kernel_id_leak error on leaked BLI ID")
	}
	if !hasCycleError {
		t.Errorf("Negative Boundary failed: expected doc/cycle_narrative_leak error on cycle sprint narrative")
	}
}
