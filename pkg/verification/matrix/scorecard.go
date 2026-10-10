package matrix

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
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
	PolicyID         string           `json:"policy_id,omitempty"`
	Status           CheckStatus      `json:"status"`
	DiamondScore     DiamondScore     `json:"diamond_score"`
	DiamondLabel     string           `json:"diamond_label"`
	Findings         []Finding        `json:"findings"`
	PolicyEvaluation PolicyEvaluation `json:"policy_evaluation"`
}

// PolicyEvaluation captures the evaluation verdict against quality thresholds.
type PolicyEvaluation struct {
	EvaluatedAt         time.Time         `json:"evaluated_at"`
	Passed              bool              `json:"passed"`
	ThresholdFailures   []string          `json:"threshold_failures,omitempty"`
	RemediationRequired bool              `json:"remediation_required"`
	Remediation         *RemediationBlock `json:"remediation,omitempty"`
}

// RemediationBlock tracks remediation tasks or issues assigned to address failures.
type RemediationBlock struct {
	TechnicalDebtID string    `json:"technical_debt_id,omitempty"`
	BacklogItemID   string    `json:"backlog_item_id,omitempty"`
	IssueID         string    `json:"issue_id,omitempty"`
	AssignedPersona string    `json:"assigned_persona,omitempty"`
	MintedAt        time.Time `json:"minted_at,omitempty"`
	Status          string    `json:"status"` // "pending", "resolved"
}

// ScorecardPath returns the canonical path on disk for a file's evaluation scorecard.
func ScorecardPath(storageDir string, relFilePath string) string {
	encoded := strings.ReplaceAll(relFilePath, "/", "__")
	return filepath.Join(storageDir, "scorecards", encoded+".json")
}

// SaveScorecard persists the scorecard to disk atomically.
func (e *Engine) SaveScorecard(sc *FileScorecard) error {
	if sc == nil || sc.FilePath == "" {
		return errfmt.Errorf("invalid scorecard: file_path required")
	}

	scPath := ScorecardPath(e.storageDir, sc.FilePath)
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
	scPath := ScorecardPath(e.storageDir, relFilePath)
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

// EvaluateScorecard evaluates a file's findings against dimension rules and quality thresholds.
// If the check fails and autoMint is true, it invokes the pluggable RemediationHook (if registered).
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

	// 2. Threshold Evaluation
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

	// 4. Auto-Mint Remediation if policy check failed
	if !passed && autoMint && (eval.Remediation == nil || eval.Remediation.Status == "resolved") {
		if e.remediation != nil {
			remediationBlock, err := e.remediation.OnPolicyFailure(ctx, sc, thresholdFailures)
			if err != nil {
				return nil, errfmt.Errorf("failed to execute remediation hook: %w", err)
			}
			eval.Remediation = remediationBlock
		} else {
			eval.Remediation = &RemediationBlock{
				Status:   "pending",
				MintedAt: time.Now().UTC(),
			}
		}
	}

	sc.PolicyEvaluation = eval
	if err := e.SaveScorecard(sc); err != nil {
		return nil, fmt.Errorf("failed to save evaluated scorecard: %w", err)
	}

	return &eval, nil
}

// ToJSON serializes the scorecard to a formatted JSON string.
func (sc *FileScorecard) ToJSON() (string, error) {
	data, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return "", errfmt.Errorf("failed to marshal scorecard to json: %w", err)
	}
	return string(data), nil
}

// ToMarkdown formats the scorecard into a portable Markdown report.
func (sc *FileScorecard) ToMarkdown() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# File Verification Scorecard: `%s`\n\n", sc.FilePath))
	b.WriteString("| Property | Value |\n")
	b.WriteString("| :--- | :--- |\n")
	b.WriteString(fmt.Sprintf("| **File Class** | `%s` |\n", sc.FileClass))
	b.WriteString(fmt.Sprintf("| **Evidence SHA-256** | `%s` |\n", sc.ContentHash))
	b.WriteString(fmt.Sprintf("| **Evaluator** | `%s` |\n", sc.Evaluator))
	b.WriteString(fmt.Sprintf("| **Dimension** | `%s` |\n", sc.Dimension))
	if sc.PolicyID != "" {
		b.WriteString(fmt.Sprintf("| **Policy ID** | `%s` |\n", sc.PolicyID))
	}
	b.WriteString(fmt.Sprintf("| **Score** | %s |\n", sc.DiamondLabel))
	b.WriteString(fmt.Sprintf("| **Status** | `%s` |\n", sc.Status))
	b.WriteString(fmt.Sprintf("| **Evaluated At** | `%s` |\n", sc.EvaluatedAt.Format(time.RFC3339)))
	b.WriteString("\n")

	b.WriteString("## Findings\n\n")
	if len(sc.Findings) == 0 {
		b.WriteString("No findings reported. File is clean.\n\n")
	} else {
		b.WriteString("| Line | Severity | Message | Rule ID |\n")
		b.WriteString("| :--- | :--- | :--- | :--- |\n")
		for _, f := range sc.Findings {
			b.WriteString(fmt.Sprintf("| %d | `%s` | %s | `%s` |\n", f.Line, f.Severity, f.Message, f.RuleID))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Policy Evaluation\n\n")
	if sc.PolicyEvaluation.Passed {
		b.WriteString("**Result**: `PASSED` - File satisfies all invisible thresholds and rules.\n")
	} else {
		b.WriteString("**Result**: `FAILED` - Policy thresholds violated.\n\n")
		if len(sc.PolicyEvaluation.ThresholdFailures) > 0 {
			b.WriteString("### Threshold Failures\n")
			for _, failure := range sc.PolicyEvaluation.ThresholdFailures {
				b.WriteString(fmt.Sprintf("- ✖ %s\n", failure))
			}
			b.WriteString("\n")
		}
	}

	if sc.PolicyEvaluation.Remediation != nil {
		b.WriteString("### Remediation\n")
		b.WriteString(fmt.Sprintf("- Status: `%s`\n", sc.PolicyEvaluation.Remediation.Status))
		if sc.PolicyEvaluation.Remediation.TechnicalDebtID != "" {
			b.WriteString(fmt.Sprintf("- Technical Debt ID: `%s`\n", sc.PolicyEvaluation.Remediation.TechnicalDebtID))
		}
		if sc.PolicyEvaluation.Remediation.BacklogItemID != "" {
			b.WriteString(fmt.Sprintf("- Backlog Item ID: `%s`\n", sc.PolicyEvaluation.Remediation.BacklogItemID))
		}
		if sc.PolicyEvaluation.Remediation.IssueID != "" {
			b.WriteString(fmt.Sprintf("- Issue ID: `%s`\n", sc.PolicyEvaluation.Remediation.IssueID))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// FindingsCSV exports the scorecard findings in CSV format.
func (sc *FileScorecard) FindingsCSV() string {
	var b strings.Builder
	b.WriteString("file,line,severity,rule_id,message\n")
	for _, f := range sc.Findings {
		escapedMsg := strings.ReplaceAll(f.Message, `"`, `""`)
		b.WriteString(fmt.Sprintf("%q,%d,%q,%q,%q\n", sc.FilePath, f.Line, f.Severity, f.RuleID, escapedMsg))
	}
	return b.String()
}

