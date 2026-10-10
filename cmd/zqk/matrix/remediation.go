package matrix

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/verification/matrix"
)

// ZQKRemediationHook implements matrix.RemediationHook by minting ZQK Knowledge Kernel objects
// (technical_debt and backlog_item) when policy thresholds fail in ZQK CLI runs.
type ZQKRemediationHook struct {
	repoRoot string
}

// NewZQKRemediationHook creates a new ZQK remediation hook.
func NewZQKRemediationHook(repoRoot string) *ZQKRemediationHook {
	return &ZQKRemediationHook{repoRoot: repoRoot}
}

// OnPolicyFailure materializes technical_debt and backlog_item into the ZQK kernel.
func (h *ZQKRemediationHook) OnPolicyFailure(ctx context.Context, sc *matrix.FileScorecard, failures []string) (*matrix.RemediationBlock, error) {
	dimCode := string(sc.Dimension)
	if dimCode == "" {
		dimCode = "HCODE"
	}

	execCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// Resolve zqk executable
	exe := filepath.Join(h.repoRoot, "bin", "zqk")
	if _, err := fileutil.Stat(exe); err != nil {
		var execErr error
		exe, execErr = fileutil.Executable()
		if execErr != nil || exe == "" {
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

	tmpTdePath := filepath.Join(h.repoRoot, paths.ProjectDataDir, fmt.Sprintf("tmp-tde-%d.yaml", time.Now().UnixNano()))
	if err := fileutil.WriteSecureFile(tmpTdePath, []byte(tdeYaml)); err != nil {
		return nil, fmt.Errorf("failed to write temp tde yaml: %w", err)
	}
	defer func() { _ = fileutil.Remove(tmpTdePath) }()

	tdeCmd := exec.CommandContext(execCtx, exe, "object", "create", "technical_debt", "--file", tmpTdePath, "--promote")
	tdeCmd.Dir = h.repoRoot
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

	tmpBliPath := filepath.Join(h.repoRoot, paths.ProjectDataDir, fmt.Sprintf("tmp-bli-%d.yaml", time.Now().UnixNano()))
	if err := fileutil.WriteSecureFile(tmpBliPath, []byte(bliYaml)); err != nil {
		return nil, fmt.Errorf("failed to write temp bli yaml: %w", err)
	}
	defer func() { _ = fileutil.Remove(tmpBliPath) }()

	bliCmd := exec.CommandContext(execCtx, exe, "object", "create", "backlog_item", "--file", tmpBliPath, "--promote")
	bliCmd.Dir = h.repoRoot
	var bliOut, bliErr bytes.Buffer
	bliCmd.Stdout = &bliOut
	bliCmd.Stderr = &bliErr
	if err := bliCmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to create backlog_item: %w, out: %s, err: %s", err, bliOut.String(), bliErr.String())
	}

	bliID := extractObjectIDFromOutput(bliOut.String())

	return &matrix.RemediationBlock{
		TechnicalDebtID: tdeID,
		BacklogItemID:   bliID,
		AssignedPersona: "PER-COMMUNITY-SOFTWARE-ENGINEER",
		MintedAt:        time.Now().UTC(),
		Status:          "pending",
	}, nil
}

func extractObjectIDFromOutput(out string) string {
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
