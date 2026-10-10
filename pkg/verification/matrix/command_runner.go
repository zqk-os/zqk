package matrix

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// CommandCheckRunner executes an external command or linter (e.g. eslint, flake8, shellcheck) for a file.
type CommandCheckRunner struct {
	CheckID       string
	Command       string
	Timeout       time.Duration
	TargetClasses []FileClass
}

// NewCommandCheckRunner instantiates a new command check runner.
func NewCommandCheckRunner(checkID string, command string, targetClasses []FileClass) *CommandCheckRunner {
	return &CommandCheckRunner{
		CheckID:       checkID,
		Command:       command,
		Timeout:       30 * time.Second,
		TargetClasses: targetClasses,
	}
}

// Run executes the command substitution against the target file.
func (r *CommandCheckRunner) Run(ctx context.Context, repoRoot string, entry *FileEntry) (CheckResult, error) {
	if r.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}

	// Substitute {{file}} or {{path}} with entry.Path
	cmdStr := strings.ReplaceAll(r.Command, "{{file}}", entry.Path)
	cmdStr = strings.ReplaceAll(cmdStr, "{{path}}", entry.Path)

	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return CheckResult{
			CheckID:     r.CheckID,
			Status:      CheckStatusSkipped,
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
		return CheckResult{
			CheckID:      r.CheckID,
			Status:       CheckStatusFailed,
			DiamondScore: ScoreFailing,
			Evaluator:    "runner:command:" + parts[0],
			EvaluatedAt:  evaluatedAt,
			Feedback:     fmt.Sprintf("Command %q failed: %v\nOutput: %s", cmdStr, err, combinedOutput),
			Findings: []Finding{
				{
					RuleID:   r.CheckID,
					Message:  combinedOutput,
					Severity: "error",
				},
			},
		}, nil
	}

	return CheckResult{
		CheckID:      r.CheckID,
		Status:       CheckStatusPassed,
		DiamondScore: ScoreFlawless,
		Evaluator:    "runner:command:" + parts[0],
		EvaluatedAt:  evaluatedAt,
		Feedback:     fmt.Sprintf("Command %q completed successfully", cmdStr),
	}, nil
}
