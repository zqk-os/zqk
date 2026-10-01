package quality

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type cycle3Scorecard struct {
	CEFVersion         string                `json:"cef_version"`
	RunID              string                `json:"run_id"`
	AssessedAt         string                `json:"assessed_at"`
	FreezeSHA          string                `json:"freeze_sha"`
	RepoFingerprint    string                `json:"repo_fingerprint"`
	Integrator         string                `json:"integrator"`
	Notes              string                `json:"notes"`
	OverallGrade       float64               `json:"overall_grade"`
	EnvelopeMin        float64               `json:"envelope_min"`
	OverallEnvelopeMin float64               `json:"overall_envelope_min"`
	Axes               map[string]cycle3Axis `json:"axes"`
	Lenses             map[string]cycle3Lens `json:"lenses"`
	Summary            cycle3Summary         `json:"summary"`
}

type cycle3Axis struct {
	Grade                 float64        `json:"grade"`
	Confidence            float64        `json:"confidence"`
	Drivers               []string       `json:"drivers"`
	Notes                 string         `json:"notes"`
	StandingFindingsCount map[string]int `json:"standing_findings_count"`
}

type cycle3Lens struct {
	Grade                 float64        `json:"grade"`
	Confidence            float64        `json:"confidence"`
	Drivers               []string       `json:"drivers"`
	Notes                 string         `json:"notes"`
	StandingFindingsCount map[string]int `json:"standing_findings_count"`
}

type cycle3Summary struct {
	TotalFindings     int            `json:"total_findings"`
	SeverityCounts    map[string]int `json:"severity_counts"`
	RemediatedInCycle int            `json:"remediated_in_cycle"`
	RemainingOpen     int            `json:"remaining_open"`
}

// TestCEFCycle3RegressionAndScorecardIntegrity verifies the CEF matrix registry,
// scorecard integrity for all evaluation cycles, Diamond Scale envelope floors,
// and the Usability score elevation achieved in Cycle 3.
func TestCEFCycle3RegressionAndScorecardIntegrity(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "../.."))

	// 1. Validate the live repository matrix registry
	res, err := ValidateMatrixRegistry(root, "cef_diamond_scorecard", "docs/quality/matrix_registry.yaml")
	if err != nil {
		t.Fatalf("ValidateMatrixRegistry: %v", err)
	}
	if !res.AllOK() {
		t.Fatalf("matrix registry not OK: %#v", res.Entries)
	}

	mRes, err := ResolveMatrixForCLI(root, "cef_diamond_scorecard", "docs/quality/matrix_registry.yaml", "", "")
	if err != nil {
		t.Fatalf("ResolveMatrixForCLI: %v", err)
	}

	prof, doneVals, err := LoadMatrixProfileYAML(mRes.ProfilePath)
	if err != nil {
		t.Fatalf("LoadMatrixProfileYAML: %v", err)
	}

	summary, err := SummarizeMatrixCSV(mRes.CSVPath, prof, doneVals, false, mRes.SessionRefColumn)
	if err != nil {
		t.Fatalf("SummarizeMatrixCSV: %v", err)
	}

	if summary.RowTotal < 22 {
		t.Errorf("expected at least 22 rows in CEF matrix after Cycle 3 registration, got %d", summary.RowTotal)
	}
	if summary.FullyDoneRows != summary.RowTotal {
		t.Errorf("expected all rows fully done, got %d/%d", summary.FullyDoneRows, summary.RowTotal)
	}

	// 2. Validate historical and current scorecards
	scorecardPaths := []string{
		"docs/quality/cef-runs/2026-09-24-CORE-STANDALONE/scorecard.json",
		"docs/quality/cef-runs/2026-09-25-CYCLE_2_EXPLORATORY/scorecard.json",
		"docs/quality/cef-runs/2026-09-28-CYCLE_3_EXPLORATORY/scorecard.json",
	}

	requiredAxes := []string{"RDB", "MNT", "TST", "REL", "OBS", "RCV", "SEC", "ROB"}

	if _, err := os.Stat(filepath.Join(root, "docs/quality/cef-runs")); os.IsNotExist(err) {
		t.Skip("skipping CEF scorecard checks: docs/quality/cef-runs not present (project-specific)")
	}

	for _, scRelPath := range scorecardPaths {
		fullPath := filepath.Join(root, scRelPath)
		data, err := fileutil.ReadFile(fullPath)
		if err != nil {
			t.Fatalf("failed to read scorecard %s: %v", scRelPath, err)
		}

		var sc cycle3Scorecard
		if err := json.Unmarshal(data, &sc); err != nil {
			t.Fatalf("failed to unmarshal scorecard %s: %v", scRelPath, err)
		}

		if sc.CEFVersion != "0.1.0" {
			t.Errorf("[%s] expected CEF version 0.1.0, got %q", scRelPath, sc.CEFVersion)
		}

		envMin := sc.EnvelopeMin
		if envMin == 0 {
			envMin = sc.OverallEnvelopeMin
		}
		if envMin < 4.0 {
			t.Errorf("[%s] expected envelope_min >= 4.0, got %f", scRelPath, envMin)
		}

		for _, axis := range requiredAxes {
			entry, ok := sc.Axes[axis]
			if !ok {
				t.Errorf("[%s] missing required diamond axis %s", scRelPath, axis)
				continue
			}
			if entry.Grade < 4.0 {
				t.Errorf("[%s] axis %s grade %f is below floor 4.0", scRelPath, axis, entry.Grade)
			}
			if entry.Confidence <= 0 {
				t.Errorf("[%s] axis %s confidence must be positive, got %f", scRelPath, axis, entry.Confidence)
			}
			if entry.StandingFindingsCount != nil {
				if crit := entry.StandingFindingsCount["critical"]; crit > 0 {
					t.Errorf("[%s] axis %s has %d standing critical findings", scRelPath, axis, crit)
				}
				if high := entry.StandingFindingsCount["high"]; high > 0 {
					t.Errorf("[%s] axis %s has %d standing high findings", scRelPath, axis, high)
				}
			}
		}
	}

	// 3. Specific Cycle 3 Invariant Checks
	c3Path := filepath.Join(root, "docs/quality/cef-runs/2026-09-28-CYCLE_3_EXPLORATORY/scorecard.json")
	c3Data, err := fileutil.ReadFile(c3Path)
	if err != nil {
		t.Fatalf("read Cycle 3 scorecard: %v", err)
	}

	var c3 cycle3Scorecard
	if err := json.Unmarshal(c3Data, &c3); err != nil {
		t.Fatalf("unmarshal Cycle 3 scorecard: %v", err)
	}

	if c3.EnvelopeMin < 4.5 {
		t.Errorf("expected Cycle 3 envelope_min >= 4.5, got %f", c3.EnvelopeMin)
	}

	// Usability Lens verification (answering user request: "have we improved our usability scores")
	usabilityLens, ok := c3.Lenses["L-USABILITY"]
	if !ok {
		t.Fatalf("missing L-USABILITY lens in Cycle 3 scorecard")
	}
	if usabilityLens.Grade < 4.8 {
		t.Errorf("expected L-USABILITY grade >= 4.8, got %f", usabilityLens.Grade)
	}
	if usabilityLens.StandingFindingsCount != nil {
		if crit := usabilityLens.StandingFindingsCount["critical"]; crit > 0 {
			t.Errorf("L-USABILITY has %d standing critical findings", crit)
		}
	}

	// Summary checks
	if c3.Summary.SeverityCounts["critical"] != 0 {
		t.Errorf("expected 0 critical findings in Cycle 3, got %d", c3.Summary.SeverityCounts["critical"])
	}
	if c3.Summary.SeverityCounts["high"] != 0 {
		t.Errorf("expected 0 high findings in Cycle 3, got %d", c3.Summary.SeverityCounts["high"])
	}
	if c3.Summary.RemainingOpen != 0 {
		t.Errorf("expected 0 remaining open findings in Cycle 3, got %d", c3.Summary.RemainingOpen)
	}
}

// TestCEFCycle3_HistoricalRemediationProof verifies that all 4 findings discovered in Cycle 2
// are attested as verified_remediated in Cycle 3 findings.jsonl.
func TestCEFCycle3_HistoricalRemediationProof(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "../.."))

	findingsPath := filepath.Join(root, "docs/quality/cef-runs/2026-09-28-CYCLE_3_EXPLORATORY/findings.jsonl")
	if _, err := os.Stat(findingsPath); os.IsNotExist(err) {
		t.Skip("skipping historical remediation proof: findings.jsonl not present (project-specific)")
	}
	file, err := os.Open(findingsPath)
	if err != nil {
		t.Fatalf("open findings.jsonl: %v", err)
	}
	defer file.Close()

	expectedRemediated := map[string]bool{
		"F-TREE-POLICE-LINT-GAP-001":           false,
		"F-LIFECYCLE-QA-COMPLETION-MAP-001":    false,
		"F-SECURITY-AUDITOR-KEY-FALLBACK-001":  false,
		"F-TEST-RUNNER-GO-BUILD-HEURISTIC-001": false,
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		var finding struct {
			FindingID         string `json:"finding_id"`
			RemediationStatus string `json:"remediation_status"`
			VerificationProof string `json:"verification_proof"`
		}

		if err := json.Unmarshal([]byte(line), &finding); err != nil {
			t.Fatalf("unmarshal finding JSON line %q: %v", line, err)
		}

		if _, exists := expectedRemediated[finding.FindingID]; exists {
			if finding.RemediationStatus != "verified_remediated" {
				t.Errorf("finding %s has unexpected remediation_status: %q (expected verified_remediated)",
					finding.FindingID, finding.RemediationStatus)
			}
			if finding.VerificationProof == "" {
				t.Errorf("finding %s has empty verification_proof", finding.FindingID)
			}
			expectedRemediated[finding.FindingID] = true
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}

	for id, found := range expectedRemediated {
		if !found {
			t.Errorf("expected finding %s to be attested in findings.jsonl, but was not found", id)
		}
	}
}

// TestCEFCycle3_PackBuilderPromptTemplates verifies that ConvertCEFPrompts generates all 25
// prompt templates cleanly without banned stubs.
func TestCEFCycle3_PackBuilderPromptTemplates(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "../.."))
	cefSourceDir := filepath.Join(root, "docs/quality/codebase_evaluation")

	if _, err := fileutil.Stat(cefSourceDir); err != nil {
		t.Skipf("CEF source dir not present: %v", err)
	}

	tempOut := t.TempDir()
	templates, err := ConvertCEFPrompts(cefSourceDir, tempOut)
	if err != nil {
		t.Fatalf("ConvertCEFPrompts failed: %v", err)
	}

	if len(templates) != 25 {
		t.Errorf("expected 25 converted prompt templates, got %d", len(templates))
	}

	for _, tpl := range templates {
		if len(tpl.Variables) == 0 {
			t.Errorf("template %s has zero variables", tpl.ID)
		}
	}
}
