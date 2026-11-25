package eval

import (
	"strings"
	"testing"
	"time"
)

func TestFormatEvalResultMarkdown_PassAndFail(t *testing.T) {
	if got := FormatEvalResultMarkdown(nil); got != "" {
		t.Fatalf("expected empty string for nil EvalResult, got %q", got)
	}

	passRes := &EvalResult{
		TemplateID:     "public-artifact-10-eval-gate",
		Target:         "docs/gtm/LAUNCH_POST.md",
		AssessedAt:     time.Date(2026, 11, 25, 7, 0, 0, 0, time.UTC),
		Passed:         true,
		CompositeScore: 9.40,
		DimensionScores: map[string]float64{
			"technical_accuracy": 9.6,
			"clarity":            9.2,
		},
	}
	mdPass := FormatEvalResultMarkdown(passRes)
	for _, want := range []string{"✅", "PASS", "public-artifact-10-eval-gate", "9.40", "| `clarity` | 9.20 |", "| `technical_accuracy` | 9.60 |"} {
		if !strings.Contains(mdPass, want) {
			t.Errorf("pass markdown missing %q:\n%s", want, mdPass)
		}
	}

	// Verify deterministic sorted ordering of dimensions (clarity before technical_accuracy).
	if strings.Index(mdPass, "`clarity`") > strings.Index(mdPass, "`technical_accuracy`") {
		t.Errorf("expected dimensions sorted alphabetically:\n%s", mdPass)
	}

	failRes := &EvalResult{
		TemplateID:     "study3-drift-gate",
		Target:         "pkg/search/engine.go",
		Passed:         false,
		CompositeScore: 6.10,
		Violations: []Violation{
			{File: "pkg/search/engine.go", Rule: "no-raw-grep", Severity: "blocker", Message: "Raw grep fallback detected"},
		},
		Remediations: []string{"Replace shell grep with zqk grep --max-tokens 2000 -f json"},
	}
	mdFail := FormatEvalResultMarkdown(failRes)
	for _, want := range []string{"❌", "FAIL", "### Violations", "no-raw-grep", "### Remediations", "Replace shell grep"} {
		if !strings.Contains(mdFail, want) {
			t.Errorf("fail markdown missing %q:\n%s", want, mdFail)
		}
	}
}

func TestFormatMatrixResultMarkdown_DeterministicModelOrder(t *testing.T) {
	if got := FormatMatrixResultMarkdown(nil); got != "" {
		t.Fatalf("expected empty string for nil MatrixResult, got %q", got)
	}

	res := &MatrixResult{
		SessionID:             "SES-MATRIX-001",
		AssessedAt:            time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC),
		TotalScenarios:        4,
		PassedScenarios:       3,
		AverageCompositeScore: 4.50,
		ModelConvergence: map[string]*ModelScoreSummary{
			"qwen2.5-coder:32b": {Model: "qwen2.5-coder:32b", RunsCount: 2, PassedCount: 2, PassRate: 1.0, MeanScore: 4.80, ScoreVariance: 0.010},
			"claude-sonnet-4":   {Model: "claude-sonnet-4", RunsCount: 2, PassedCount: 1, PassRate: 0.5, MeanScore: 4.20, ScoreVariance: 0.120},
		},
		Remediations: []string{"Tighten boundary test coverage"},
	}

	md := FormatMatrixResultMarkdown(res)
	for _, want := range []string{
		"ZQK Evaluation Matrix Convergence Report (`SES-MATRIX-001`)",
		"`3 / 4`",
		"`4.50`",
		"| `claude-sonnet-4` | 2 | 1 | 50.0% | 4.20 | 0.120 |",
		"| `qwen2.5-coder:32b` | 2 | 2 | 100.0% | 4.80 | 0.010 |",
		"Tighten boundary test coverage",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("matrix markdown missing %q:\n%s", want, md)
		}
	}
	if strings.Index(md, "`claude-sonnet-4`") > strings.Index(md, "`qwen2.5-coder:32b`") {
		t.Errorf("expected models sorted alphabetically:\n%s", md)
	}
}
