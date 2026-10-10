package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// FileScorecard represents the authoritative evaluation findings and policy outcome for a single file.
type FileScorecard struct {
	SchemaVersion    string           `json:"schema_version"`
	FilePath         string           `json:"file_path"`
	ContentHash      string           `json:"content_hash"`
	FileClass        FileClass        `json:"file_class"`
	EvaluatedAt      time.Time        `json:"evaluated_at"`
	Evaluator        string           `json:"evaluator"`
	Dimension        DimensionCode    `json:"dimension"`
	PolicyID         string           `json:"policy_id"`
	Status           CheckStatus      `json:"status"`
	DiamondScore     DiamondScore     `json:"diamond_score"`
	DiamondLabel     string           `json:"diamond_label"`
	Findings         []Finding        `json:"findings"`
	PolicyEvaluation PolicyEvaluation `json:"policy_evaluation"`
}

// PolicyEvaluation captures the central policy engine's verdict against invisible thresholds.
type PolicyEvaluation struct {
	EvaluatedAt         time.Time         `json:"evaluated_at"`
	Passed              bool              `json:"passed"`
	ThresholdFailures   []string          `json:"threshold_failures,omitempty"`
	RemediationRequired bool              `json:"remediation_required"`
	Remediation         *RemediationBlock `json:"remediation,omitempty"`
}

// RemediationBlock tracks the automatically minted kernel objects assigned to the fixer persona.
type RemediationBlock struct {
	TechnicalDebtID string    `json:"technical_debt_id,omitempty"`
	BacklogItemID   string    `json:"backlog_item_id,omitempty"`
	AssignedPersona string    `json:"assigned_persona,omitempty"`
	MintedAt        time.Time `json:"minted_at,omitempty"`
	Status          string    `json:"status"` // "pending", "resolved"
}

// ScorecardPath returns the canonical path on disk for a file's evaluation scorecard.
func ScorecardPath(repoRoot string, relFilePath string) string {
	encoded := strings.ReplaceAll(relFilePath, "/", "__")
	return filepath.Join(repoRoot, paths.ProjectDataDir, "scorecards", encoded+".json")
}

// SaveScorecard persists the scorecard to disk atomically.
func (e *Engine) SaveScorecard(sc *FileScorecard) error {
	if sc == nil || sc.FilePath == "" {
		return errfmt.Errorf("invalid scorecard: file_path required")
	}

	scPath := ScorecardPath(e.repoRoot, sc.FilePath)
	if err := fileutil.MkdirAll(filepath.Dir(scPath), fileutil.StandardDirPerm); err != nil {
		return errfmt.Errorf("failed to create scorecards directory: %w", err)
	}

	data, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return errfmt.Errorf("failed to marshal scorecard: %w", err)
	}

	return fileutil.WriteSecureFile(scPath, data)
}

// LoadScorecard loads a stored scorecard for a given file.
func (e *Engine) LoadScorecard(relFilePath string) (*FileScorecard, error) {
	scPath := ScorecardPath(e.repoRoot, relFilePath)
	data, err := fileutil.ReadFile(scPath)
	if err != nil {
		return nil, err
	}

	var sc FileScorecard
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, errfmt.Errorf("corrupt scorecard %s: %w", scPath, err)
	}

	return &sc, nil
}

// EvaluateScorecard evaluates a file's findings against the dimension policy rules and invisible thresholds.
// If the policy check fails and autoMint is true, it automatically mints remediation kernel objects
// (technical_debt and backlog_item) assigned to the fixer persona (PER-COMMUNITY-SOFTWARE-ENGINEER).
func (e *Engine) EvaluateScorecard(ctx context.Context, sc *FileScorecard, autoMint bool) (*PolicyEvaluation, error) {
	if sc == nil {
		return nil, errfmt.Errorf("nil scorecard provided")
	}

	// 1. Resolve Policy ID if empty
	if sc.PolicyID == "" && sc.Dimension != "" {
		for _, d := range DefaultDimensions() {
			if d.Code == sc.Dimension {
				sc.PolicyID = d.PolicyID
				break
			}
		}
	}

	// 2. Invisible Threshold Evaluation
	thresholdFailures := make([]string, 0)
	criticalCount := 0
	errorCount := 0
	warningCount := 0

	for _, f := range sc.Findings {
		sev := strings.ToLower(f.Severity)
		switch sev {
		case "critical":
			criticalCount++
			thresholdFailures = append(thresholdFailures, fmt.Sprintf("Critical violation at line %d: %s (%s)", f.Line, f.Message, f.RuleID))
		case "error", "high":
			errorCount++
			thresholdFailures = append(thresholdFailures, fmt.Sprintf("High violation at line %d: %s (%s)", f.Line, f.Message, f.RuleID))
		case "warning", "medium":
			warningCount++
		}

		// Check global string literal deduplication threshold
		if f.LiteralValue != "" && e.inventory != nil {
			count, isDup := e.inventory.Record(f.LiteralValue, sc.FilePath, f.Line)
			if isDup {
				thresholdFailures = append(thresholdFailures, fmt.Sprintf("Duplicate literal violation: string %q occurs %d times across codebase (max 1 tolerated)", f.LiteralValue, count))
			}
		}
	}

	if e.inventory != nil {
		_ = e.inventory.Save()
	}

	// 3. Compute Diamond Score
	sc.DiamondScore = ComputeDiamondScore(sc.Findings)
	sc.DiamondLabel = sc.DiamondScore.String()

	passed := len(thresholdFailures) == 0 && sc.DiamondScore >= ScoreMinor
	if !passed {
		sc.Status = CheckStatusFailed
	} else {
		sc.Status = CheckStatusPassed
	}

	eval := PolicyEvaluation{
		EvaluatedAt:         time.Now().UTC(),
		Passed:              passed,
		ThresholdFailures:   thresholdFailures,
		RemediationRequired: !passed,
	}

	// Preserve prior remediation block if present
	if sc.PolicyEvaluation.Remediation != nil {
		eval.Remediation = sc.PolicyEvaluation.Remediation
		if passed {
			eval.Remediation.Status = "resolved"
		}
	}

	// 4. Auto-Mint Remediation Objects if Policy Failed
	if !passed && autoMint && (eval.Remediation == nil || eval.Remediation.Status == "resolved") {
		remediationBlock, err := e.mintRemediationObjects(ctx, sc, thresholdFailures)
		if err != nil {
			return nil, errfmt.Errorf("failed to auto-mint remediation objects: %w", err)
		}
		eval.Remediation = remediationBlock
	}

	sc.PolicyEvaluation = eval
	if err := e.SaveScorecard(sc); err != nil {
		return nil, fmt.Errorf("failed to save evaluated scorecard: %w", err)
	}

	return &eval, nil
}

// mintRemediationObjects programmatically materializes technical_debt and backlog_item into the kernel.
func (e *Engine) mintRemediationObjects(ctx context.Context, sc *FileScorecard, failures []string) (*RemediationBlock, error) {
	dimCode := string(sc.Dimension)
	if dimCode == "" {
		dimCode = "HCODE"
	}

	execCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Resolve executable
	exe := filepath.Join(e.repoRoot, "bin", "zqk")
	if _, err := fileutil.Stat(exe); err != nil {
		exe, err = fileutil.Executable()
		if err != nil || exe == "" {
			exe = "zqk"
		}
	}

	// 1. Build Technical Debt Payload
	findingsMd := strings.Builder{}
	for _, fail := range failures {
		findingsMd.WriteString(fmt.Sprintf("- %s\n", fail))
	}
	for _, f := range sc.Findings {
		findingsMd.WriteString(fmt.Sprintf("- Line %d: [%s] %s (%s)\n", f.Line, f.Severity, f.Message, f.RuleID))
	}

	tdeTitle := fmt.Sprintf("Remediate %s policy violations in %s", dimCode, sc.FilePath)
	tdeDesc := fmt.Sprintf(`### Continuous Verification Policy Violation: %s
**File**: %s
**Evidence SHA**: %s
**Evaluator**: %s
**Policy ID**: %s
**Score**: %s

#### Observed Failures:
%s

#### Actionable Remediation Guidance:
1. Examine %s at the flagged line numbers.
2. Extract all hardcoded strings, release tags, and permissions into canonical constants or configuration.
3. Validate fixes by running:
   `+"`"+`%s`+"`"+`
`, dimCode, sc.FilePath, sc.ContentHash, sc.Evaluator, sc.PolicyID, sc.DiamondLabel, findingsMd.String(), sc.FilePath, paths.CLIInvocation("matrix verify --file "+sc.FilePath))

	targetDate := time.Now().AddDate(0, 0, 14).Format("2006-01-02")
	tdeYaml := fmt.Sprintf(`kind: technical_debt
title: %q
description: %q
debt_type: maintainability
impact_assessment: critical
target_resolution_date: %q
file_path: %q
goal_refs:
  - GOAL-LAUNCH-SURFACE-COMPLETENESS
tags:
  - matrix-failure
  - %s
  - auto-minted-remediation
`, tdeTitle, tdeDesc, targetDate, sc.FilePath, dimCode)

	tmpTdePath := filepath.Join(e.repoRoot, paths.ProjectDataDir, fmt.Sprintf("tmp-tde-%d.yaml", time.Now().UnixNano()))
	if err := fileutil.WriteSecureFile(tmpTdePath, []byte(tdeYaml)); err != nil {
		return nil, fmt.Errorf("failed to write temp tde yaml: %w", err)
	}
	defer func() { _ = fileutil.Remove(tmpTdePath) }()

	tdeCmd := exec.CommandContext(execCtx, exe, "object", "create", "technical_debt", "--file", tmpTdePath, "--promote")
	tdeCmd.Dir = e.repoRoot
	var tdeOut, tdeErr bytes.Buffer
	tdeCmd.Stdout = &tdeOut
	tdeCmd.Stderr = &tdeErr
	if err := tdeCmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to create technical_debt: %w, out: %s, err: %s", err, tdeOut.String(), tdeErr.String())
	}

	tdeID := extractObjectIDFromOutput(tdeOut.String())

	// 2. Build Backlog Item Payload
	bliTitle := fmt.Sprintf("[Fixer] Remediate %s violations in %s", dimCode, sc.FilePath)
	bliDesc := fmt.Sprintf("Remediate hardcoded literals and policy violations in %s as specced in %s.", sc.FilePath, tdeID)

	bliYaml := fmt.Sprintf(`kind: backlog_item
title: %q
description: %q
goal_refs:
  - GOAL-LAUNCH-SURFACE-COMPLETENESS
persona_refs:
  - PER-COMMUNITY-SOFTWARE-ENGINEER
`, bliTitle, bliDesc)

	tmpBliPath := filepath.Join(e.repoRoot, paths.ProjectDataDir, fmt.Sprintf("tmp-bli-%d.yaml", time.Now().UnixNano()))
	if err := fileutil.WriteSecureFile(tmpBliPath, []byte(bliYaml)); err != nil {
		return nil, fmt.Errorf("failed to write temp bli yaml: %w", err)
	}
	defer func() { _ = fileutil.Remove(tmpBliPath) }()

	bliCmd := exec.CommandContext(execCtx, exe, "object", "create", "backlog_item", "--file", tmpBliPath, "--promote")
	bliCmd.Dir = e.repoRoot
	var bliOut, bliErr bytes.Buffer
	bliCmd.Stdout = &bliOut
	bliCmd.Stderr = &bliErr
	if err := bliCmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to create backlog_item: %w, out: %s, err: %s", err, bliOut.String(), bliErr.String())
	}

	bliID := extractObjectIDFromOutput(bliOut.String())

	return &RemediationBlock{
		TechnicalDebtID: tdeID,
		BacklogItemID:   bliID,
		AssignedPersona: "PER-COMMUNITY-SOFTWARE-ENGINEER",
		MintedAt:        time.Now().UTC(),
		Status:          "pending",
	}, nil
}

func extractObjectIDFromOutput(out string) string {
	// e.g. "✓ Object TDE-001 created successfully"
	lines := strings.Split(out, "\n")
	for _, l := range lines {
		fields := strings.Fields(l)
		for _, f := range fields {
			if strings.HasPrefix(f, "TDE-") || strings.HasPrefix(f, "BLI-") {
				return strings.Trim(f, ",.:;\"'")
			}
		}
	}
	return ""
}
