package system

import (
	"testing"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

func newFocusFixture() *clipkg.AnalysisResult {
	return &clipkg.AnalysisResult{
		HighFailureRateCommands: []clipkg.CommandIssue{
			{Command: "zqk object get", NormalizedCmd: "object get", Issue: "High error rate", Severity: "high"},
			{Command: "zqk object list", NormalizedCmd: "object list", Issue: "High error rate", Severity: "medium"},
		},
		FrequentTimeouts: []clipkg.CommandIssue{
			{Command: "zqk system aggregate-audit", NormalizedCmd: "system aggregate-audit", Issue: "Frequent timeouts", Severity: "high"},
		},
		SlowCommands: []clipkg.CommandIssue{
			{Command: "zqk build", NormalizedCmd: "build", Issue: "Slow execution", Severity: "low"},
		},
		ImprovementSuggestions: []clipkg.Suggestion{
			{Type: "error_handling", Command: "object get"},
			{Type: "timeout", Command: "system aggregate-audit"},
			{Type: "optimization", Command: "build"},
			{Type: "documentation", Command: "object list"},
		},
		ChurnIndicators: []clipkg.ChurnIndicator{
			{Pattern: "multiple_variations", Commands: []string{"object get", "object list"}, Severity: "medium"},
		},
	}
}

func TestApplyFocusFilter_FailuresKeepsOnlyErrorRate(t *testing.T) {
	result := newFocusFixture()
	focused := ApplyFocusFilter(result, "failures")

	if len(focused.HighFailureRateCommands) != 2 {
		t.Fatalf("expected 2 high failure rate commands, got %d", len(focused.HighFailureRateCommands))
	}
	if len(focused.FrequentTimeouts) != 0 {
		t.Fatalf("expected timeouts cleared for failures focus, got %d", len(focused.FrequentTimeouts))
	}
	if len(focused.SlowCommands) != 0 {
		t.Fatalf("expected slow commands cleared for failures focus, got %d", len(focused.SlowCommands))
	}
	if len(focused.ChurnIndicators) != 0 {
		t.Fatalf("expected churn indicators cleared for failures focus, got %d", len(focused.ChurnIndicators))
	}
}

func TestApplyFocusFilter_TimeoutsKeepsOnlyTimeouts(t *testing.T) {
	result := newFocusFixture()
	focused := ApplyFocusFilter(result, "timeouts")

	if len(focused.FrequentTimeouts) != 1 {
		t.Fatalf("expected 1 frequent timeout, got %d", len(focused.FrequentTimeouts))
	}
	if len(focused.HighFailureRateCommands) != 0 || len(focused.SlowCommands) != 0 || len(focused.ChurnIndicators) != 0 {
		t.Fatalf("expected other sections cleared for timeouts focus: %+v", focused)
	}
}

func TestApplyFocusFilter_SlowKeepsOnlySlow(t *testing.T) {
	result := newFocusFixture()
	focused := ApplyFocusFilter(result, "slow")

	if len(focused.SlowCommands) != 1 {
		t.Fatalf("expected 1 slow command, got %d", len(focused.SlowCommands))
	}
	if len(focused.HighFailureRateCommands) != 0 || len(focused.FrequentTimeouts) != 0 || len(focused.ChurnIndicators) != 0 {
		t.Fatalf("expected other sections cleared for slow focus: %+v", focused)
	}
}

func TestApplyFocusFilter_ChurnKeepsOnlyChurn(t *testing.T) {
	result := newFocusFixture()
	focused := ApplyFocusFilter(result, "churn")

	if len(focused.ChurnIndicators) != 1 {
		t.Fatalf("expected 1 churn indicator, got %d", len(focused.ChurnIndicators))
	}
	if len(focused.HighFailureRateCommands) != 0 || len(focused.FrequentTimeouts) != 0 || len(focused.SlowCommands) != 0 {
		t.Fatalf("expected other sections cleared for churn focus: %+v", focused)
	}
}

func TestApplyFocusFilter_AllKeepsEverything(t *testing.T) {
	result := newFocusFixture()
	focused := ApplyFocusFilter(result, "all")

	if focused != result {
		t.Fatalf("expected focus 'all' to return the same result pointer")
	}
	if len(focused.HighFailureRateCommands) != 2 || len(focused.FrequentTimeouts) != 1 || len(focused.SlowCommands) != 1 {
		t.Fatalf("expected all sections preserved, got %+v", focused)
	}
}

func TestApplyFocusFilter_EmptyIsIdentityAndNilSafe(t *testing.T) {
	result := newFocusFixture()
	if got := ApplyFocusFilter(result, ""); got != result {
		t.Fatalf("expected empty focus to return the same result pointer, got %+v", got)
	}
	if got := ApplyFocusFilter(nil, "failures"); got != nil {
		t.Fatalf("expected nil input to return nil, got %+v", got)
	}
}
