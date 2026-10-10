package scanners

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/verification/matrix"
)

// CommandScanner executes an external shell command, linter, or CLI (e.g. eslint, ruff, clippy, golangci-lint, shellcheck).
type CommandScanner struct {
	checkID       string
	command       string
	timeout       time.Duration
	targetClasses []matrix.FileClass
}

// NewCommandScanner instantiates a new command check runner.
func NewCommandScanner(checkID string, command string, targetClasses []matrix.FileClass) *CommandScanner {
	return &CommandScanner{
		checkID:       checkID,
		command:       command,
		timeout:       30 * time.Second,
		targetClasses: targetClasses,
	}
}

// WithTimeout sets a custom execution timeout for the command.
func (s *CommandScanner) WithTimeout(d time.Duration) *CommandScanner {
	s.timeout = d
	return s
}

// Run executes the command substitution against the target file.
func (s *CommandScanner) Run(ctx context.Context, repoRoot string, entry *matrix.FileEntry) (matrix.CheckResult, error) {
	if s.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}

	// Substitute {{file}} or {{path}} with entry.Path
	cmdStr := strings.ReplaceAll(s.command, "{{file}}", entry.Path)
	cmdStr = strings.ReplaceAll(cmdStr, "{{path}}", entry.Path)

	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return matrix.CheckResult{
			CheckID:     s.checkID,
			Status:      matrix.CheckStatusSkipped,
			Evaluator:   "runner:command",
			EvaluatedAt: time.Now().UTC(),
			Feedback:    "Empty command definition",
		}, nil
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	evaluatedAt := time.Now().UTC()

	if err != nil {
		combinedOutput := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
		return matrix.CheckResult{
			CheckID:      s.checkID,
			Status:       matrix.CheckStatusFailed,
			DiamondScore: matrix.ScoreFailing,
			Evaluator:    "runner:command:" + parts[0],
			EvaluatedAt:  evaluatedAt,
			Feedback:     fmt.Sprintf("Command %q failed: %v\nOutput: %s", cmdStr, err, combinedOutput),
			Findings: []matrix.Finding{
				{
					RuleID:   s.checkID,
					Message:  combinedOutput,
					Severity: "error",
				},
			},
		}, nil
	}

	return matrix.CheckResult{
		CheckID:      s.checkID,
		Status:       matrix.CheckStatusPassed,
		DiamondScore: matrix.ScoreFlawless,
		Evaluator:    "runner:command:" + parts[0],
		EvaluatedAt:  evaluatedAt,
		Feedback:     fmt.Sprintf("Command %q completed successfully", cmdStr),
	}, nil
}
