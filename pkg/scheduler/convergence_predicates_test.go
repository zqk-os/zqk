package scheduler

import (
	"strings"
	"testing"
)

func TestEvaluateConvergencePredicateReadiness(t *testing.T) {
	t.Parallel()
	snap := &TestBundleConvergenceSnapshot{
		LinesInWindow:             3,
		ReadyForSessionCompletion: true,
	}
	start, comp := EvaluateConvergencePredicateReadiness("active", nil, snap)
	if !start || !comp {
		t.Fatalf("active session: start=%v completion=%v", start, comp)
	}

	start, comp = EvaluateConvergencePredicateReadiness("draft", map[string]any{
		"start_gate": map[string]any{"min_lines_in_window": 2},
	}, snap)
	if !start || !comp {
		t.Fatalf("draft with gates: start=%v completion=%v", start, comp)
	}

	start, _ = EvaluateConvergencePredicateReadiness("draft", map[string]any{
		"start_gate": map[string]any{"min_lines_in_window": 10},
	}, snap)
	if start {
		t.Fatal("expected start gate closed")
	}

	snap2 := &TestBundleConvergenceSnapshot{LinesInWindow: 1, ReadyForSessionCompletion: false}
	start, comp = EvaluateConvergencePredicateReadiness("draft", map[string]any{
		"start_gate":      map[string]any{"min_lines_in_window": 1},
		"completion_gate": map[string]any{"require_ready_for_session_completion": false},
	}, snap2)
	if !start || !comp {
		t.Fatalf("completion gate relaxed: start=%v completion=%v", start, comp)
	}

	start, comp = EvaluateConvergencePredicateReadiness("draft", map[string]any{
		"start_gate":      map[string]any{"min_lines_in_window": 1},
		"completion_gate": map[string]any{"require_ready_for_session_completion": "false"},
	}, snap2)
	if !start || !comp {
		t.Fatalf("completion gate relaxed (string false): start=%v completion=%v", start, comp)
	}

	start, comp = EvaluateConvergencePredicateReadiness("active", map[string]any{
		"completion_gate": map[string]any{"require_ready_for_session_completion": false},
	}, nil)
	if !start || !comp {
		t.Fatalf("nil snap + completion bypass: start=%v completion=%v", start, comp)
	}

	start, comp = EvaluateConvergencePredicateReadiness("active", nil, nil)
	if !start || comp {
		t.Fatalf("nil snap without bypass: start=%v completion=%v (want completion false)", start, comp)
	}
}

func TestCompletionGateObservabilityNote(t *testing.T) {
	t.Parallel()
	if s := CompletionGateObservabilityNote(nil); s != "" {
		t.Fatalf("nil thresholds: got %q", s)
	}
	if s := CompletionGateObservabilityNote(map[string]any{"start_gate": map[string]any{"min_lines_in_window": 1}}); s != "" {
		t.Fatalf("no completion_gate: got %q", s)
	}
	relaxed := CompletionGateObservabilityNote(map[string]any{
		"completion_gate": map[string]any{"require_ready_for_session_completion": false},
	})
	if relaxed == "" || !strings.Contains(relaxed, "false") {
		t.Fatalf("relaxed note: %q", relaxed)
	}
	strict := CompletionGateObservabilityNote(map[string]any{
		"completion_gate": map[string]any{"require_ready_for_session_completion": true},
	})
	if strict == "" || !strings.Contains(strict, "true") {
		t.Fatalf("strict note: %q", strict)
	}
}

func TestThresholdsCompletionGateRequiresReadyForSessionCompletion(t *testing.T) {
	t.Parallel()
	if !ThresholdsCompletionGateRequiresReadyForSessionCompletion(nil) {
		t.Fatal("nil thresholds: want default true (require RFS)")
	}
	if !ThresholdsCompletionGateRequiresReadyForSessionCompletion(map[string]any{}) {
		t.Fatal("empty thresholds: want default true")
	}
	if ThresholdsCompletionGateRequiresReadyForSessionCompletion(map[string]any{
		"completion_gate": map[string]any{"require_ready_for_session_completion": false},
	}) {
		t.Fatal("explicit false: want require RFS false")
	}
}
