package scanners_test

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/verification/matrix"
	"github.com/zqk-os/zqk/pkg/verification/matrix/scanners"
)

func TestPatternScanner_DeclarativeRules(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	scanner := scanners.NewPatternScanner("custom-pattern",
		scanners.PatternRule{
			ID:            "forbidden-endpoint",
			Message:       "Legacy API endpoint detected",
			Severity:      "error",
			Pattern:       `https?://api\.legacy\.internal`,
			TargetClasses: []matrix.FileClass{matrix.ClassSource},
			Dimension:     matrix.DimensionHCODE,
		},
		scanners.PatternRule{
			ID:               "no-temp-dir",
			Message:          "Hardcoded tmp path",
			Severity:         "warning",
			Regex:            regexp.MustCompile(`/tmp/scratch`),
			IgnoreIfContains: []string{"test_mock"},
			Dimension:        matrix.DimensionHCODE,
		},
	)

	// Verify Rules copy
	rules := scanner.Rules()
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}

	// 1. Violating file
	file1 := filepath.Join(tmpDir, "api.py")
	content1 := "import requests\nENDPOINT = 'http://api.legacy.internal/v1'\n"
	if err := fileutil.WriteFile(file1, []byte(content1), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	res1, err := scanner.Run(ctx, tmpDir, &matrix.FileEntry{Path: "api.py", Class: matrix.ClassSource})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res1.Status != matrix.CheckStatusFailed {
		t.Errorf("expected failed status, got %v", res1.Status)
	}
	if len(res1.Findings) != 1 || res1.Findings[0].RuleID != "forbidden-endpoint" {
		t.Errorf("expected finding forbidden-endpoint, got %+v", res1.Findings)
	}

	// 2. Ignored phrase
	file2 := filepath.Join(tmpDir, "service.py")
	content2 := "path = '/tmp/scratch' # test_mock\n"
	if err := fileutil.WriteFile(file2, []byte(content2), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	res2, err := scanner.Run(ctx, tmpDir, &matrix.FileEntry{Path: "service.py", Class: matrix.ClassSource})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res2.Status != matrix.CheckStatusPassed || len(res2.Findings) != 0 {
		t.Errorf("expected ignored finding to pass, got %+v", res2.Findings)
	}
}

func TestCommandScanner_Execution(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	testFile := filepath.Join(tmpDir, "sample.sh")
	if err := fileutil.WriteFile(testFile, []byte("#!/bin/sh\necho 'hello'\n"), 0755); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// 1. Passing command check
	passRunner := scanners.NewCommandScanner("echo-check", "echo {{path}}", []matrix.FileClass{matrix.ClassScript})
	passRunner.WithTimeout(5 * time.Second)
	resPass, err := passRunner.Run(ctx, tmpDir, &matrix.FileEntry{Path: "sample.sh", Class: matrix.ClassScript})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resPass.Status != matrix.CheckStatusPassed {
		t.Errorf("expected command to pass, got %v", resPass.Status)
	}
	if resPass.DiamondScore != matrix.ScoreFlawless {
		t.Errorf("expected flawless score, got %v", resPass.DiamondScore)
	}

	// 2. Failing command check (exit non-zero)
	failRunner := scanners.NewCommandScanner("fail-check", "false", []matrix.FileClass{matrix.ClassScript})
	resFail, err := failRunner.Run(ctx, tmpDir, &matrix.FileEntry{Path: "sample.sh", Class: matrix.ClassScript})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resFail.Status != matrix.CheckStatusFailed {
		t.Errorf("expected command to fail, got %v", resFail.Status)
	}
	if resFail.DiamondScore != matrix.ScoreFailing {
		t.Errorf("expected failing diamond score, got %v", resFail.DiamondScore)
	}
}

func TestAgentScanner_RubricEvaluation(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	target := &matrix.FileEntry{Path: "src/calc.rs", Class: matrix.ClassSource}

	// 1. Pending when evaluator is nil
	scannerPending := scanners.NewAgentScanner("czar-rubric", "Verify zero magic constants", nil, []matrix.FileClass{matrix.ClassSource})
	resPending, err := scannerPending.Run(ctx, tmpDir, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resPending.Status != matrix.CheckStatusPending {
		t.Errorf("expected pending status, got %v", resPending.Status)
	}

	// 2. Blind evaluator returns 5 diamonds (Flawless)
	evaluatorFlawless := scanners.AgentEvaluatorFunc(func(ctx context.Context, repoRoot string, entry *matrix.FileEntry, rubric string) (matrix.DiamondScore, string, []matrix.Finding, error) {
		return matrix.ScoreFlawless, "Exemplary code, adheres to all rubric requirements", nil, nil
	})
	scannerFlawless := scanners.NewAgentScanner("czar-rubric", "Rubric text", evaluatorFlawless, []matrix.FileClass{matrix.ClassSource})
	resFlawless, err := scannerFlawless.Run(ctx, tmpDir, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resFlawless.Status != matrix.CheckStatusPassed {
		t.Errorf("expected passed status, got %v", resFlawless.Status)
	}
	if resFlawless.DiamondScore != matrix.ScoreFlawless {
		t.Errorf("expected flawless score 5, got %v", resFlawless.DiamondScore)
	}

	// 3. Blind evaluator returns 2 diamonds (Failing)
	evaluatorFailing := scanners.AgentEvaluatorFunc(func(ctx context.Context, repoRoot string, entry *matrix.FileEntry, rubric string) (matrix.DiamondScore, string, []matrix.Finding, error) {
		return matrix.ScoreFailing, "Critical security issue found", []matrix.Finding{
			{Line: 10, Severity: "error", Message: "Unsafe buffer cast", RuleID: "sec-rubric-unsafe"},
		}, nil
	})
	scannerFailing := scanners.NewAgentScanner("czar-rubric", "Rubric text", evaluatorFailing, []matrix.FileClass{matrix.ClassSource})
	resFailing, err := scannerFailing.Run(ctx, tmpDir, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resFailing.Status != matrix.CheckStatusFailed {
		t.Errorf("expected failed status, got %v", resFailing.Status)
	}
	if resFailing.DiamondScore != matrix.ScoreFailing {
		t.Errorf("expected failing score 2, got %v", resFailing.DiamondScore)
	}

	// 4. Target classes mismatch skips check
	scannerSkip := scanners.NewAgentScanner("czar-rubric", "Rubric text", evaluatorFlawless, []matrix.FileClass{matrix.ClassTest})
	resSkip, err := scannerSkip.Run(ctx, tmpDir, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resSkip.Status != matrix.CheckStatusSkipped {
		t.Errorf("expected skipped status, got %v", resSkip.Status)
	}

	// 5. Evaluator error handling
	evaluatorErr := scanners.AgentEvaluatorFunc(func(ctx context.Context, repoRoot string, entry *matrix.FileEntry, rubric string) (matrix.DiamondScore, string, []matrix.Finding, error) {
		return matrix.ScoreCritical, "", nil, fmt.Errorf("timeout contacting LLM")
	})
	scannerErr := scanners.NewAgentScanner("czar-rubric", "Rubric text", evaluatorErr, nil)
	resErr, err := scannerErr.Run(ctx, tmpDir, target)
	if err == nil {
		t.Errorf("expected error from runner, got nil")
	}
	if resErr.Status != matrix.CheckStatusFailed {
		t.Errorf("expected failed status, got %v", resErr.Status)
	}
}
