package scanners

import (
	"context"
	"time"

	"github.com/zqk-os/zqk/pkg/verification/matrix"
)

// CommandScanner executes an external shell command, linter, or CLI (e.g. eslint, ruff, clippy, golangci-lint, shellcheck).
type CommandScanner struct {
	runner *matrix.CommandCheckRunner
}

// NewCommandScanner instantiates a new command check runner.
func NewCommandScanner(checkID string, command string, targetClasses []matrix.FileClass) *CommandScanner {
	return &CommandScanner{
		runner: matrix.NewCommandCheckRunner(checkID, command, targetClasses),
	}
}

// WithTimeout sets a custom execution timeout for the command.
func (s *CommandScanner) WithTimeout(d time.Duration) *CommandScanner {
	if s.runner != nil {
		s.runner.Timeout = d
	}
	return s
}

// Run executes the command substitution against the target file.
func (s *CommandScanner) Run(ctx context.Context, repoRoot string, entry *matrix.FileEntry) (matrix.CheckResult, error) {
	return s.runner.Run(ctx, repoRoot, entry)
}
