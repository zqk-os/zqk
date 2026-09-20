package scheduler

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestAgentPromptPreferredNextAction_prefersPersistedOperatorText(t *testing.T) {
	t.Parallel()
	cvs := map[string]any{
		objects.FieldKeyNextAction: "Next: triage .zqk/logs/drift/ then scan-tests --package ./pkg/foo.",
	}
	sug := map[string]any{
		objects.FieldKeyNextAction: "No failing outcomes in this window.",
	}
	if got := AgentPromptPreferredNextAction(cvs, sug); got != cvs[objects.FieldKeyNextAction] {
		t.Fatalf("got %q want persisted", got)
	}
}

func TestMergeObjectUpdateBodyPreservingOperatorNextAction_keepsHandoff(t *testing.T) {
	t.Parallel()
	existing := map[string]any{
		objects.FieldKeyNextAction: "Rollup satisfied — not drift done. Next: triage .zqk/logs/drift/ and scan-tests --package ./pkg/foo.",
	}
	body := map[string]any{
		objects.FieldKeyNextAction: "No failing outcomes in this window.",
	}
	MergeObjectUpdateBodyPreservingOperatorNextAction(existing, body)
	if got := body[objects.FieldKeyNextAction]; got != existing[objects.FieldKeyNextAction] {
		t.Fatalf("next_action: got %v want operator handoff preserved", got)
	}
}

func TestMergeObjectUpdateBodyPreservingOperatorNextAction_replacesWhenSuggestedHasReruns(t *testing.T) {
	t.Parallel()
	existing := map[string]any{
		objects.FieldKeyNextAction: "Some drift guidance with .zqk/logs/drift/ and more text here for length.",
	}
	sug := "No failing outcomes in this window.\n\nSuggested reruns (one command per failing fingerprint):\n- fp1: go test ./x\n"
	body := map[string]any{
		objects.FieldKeyNextAction: sug,
	}
	MergeObjectUpdateBodyPreservingOperatorNextAction(existing, body)
	// Measurement-only check strips reruns block; remainder is "No failing outcomes" → preserve operator
	if got := body[objects.FieldKeyNextAction].(string); got != existing[objects.FieldKeyNextAction] {
		t.Fatalf("expected preserve: got %q", got)
	}
}

func TestMergeObjectUpdateBodyPreservingOperatorNextAction_noPreserveShortExisting(t *testing.T) {
	t.Parallel()
	existing := map[string]any{
		objects.FieldKeyNextAction: "short",
	}
	body := map[string]any{
		objects.FieldKeyNextAction: "No failing outcomes in this window.",
	}
	MergeObjectUpdateBodyPreservingOperatorNextAction(existing, body)
	if got := body[objects.FieldKeyNextAction]; got != "No failing outcomes in this window." {
		t.Fatalf("expected suggested: got %v", got)
	}
}

func TestMergeObjectUpdateBodyPreservingCurrentPhaseOnDegradedSnapshot_keepsLaterPhase(t *testing.T) {
	t.Parallel()
	degraded := &TestBundleConvergenceSnapshot{LinesInWindow: 0, DeltaAssessment: "unknown"}
	existing := map[string]any{
		objects.FieldKeyCurrentPhase: string(PhaseC6Exit),
	}
	body := map[string]any{
		objects.FieldKeyCurrentPhase: string(PhaseC1Scope),
	}
	MergeObjectUpdateBodyPreservingCurrentPhaseOnDegradedSnapshot(existing, body, degraded)
	if got := body[objects.FieldKeyCurrentPhase]; got != string(PhaseC6Exit) {
		t.Fatalf("current_phase: got %v want c6 preserved on empty health window", got)
	}
}

func TestMergeObjectUpdateBodyPreservingCurrentPhaseOnDegradedSnapshot_patchesActivityLogPhase(t *testing.T) {
	t.Parallel()
	degraded := &TestBundleConvergenceSnapshot{LinesInWindow: 0, DeltaAssessment: "unknown"}
	existing := map[string]any{
		objects.FieldKeyCurrentPhase: string(PhaseC6Exit),
	}
	body := map[string]any{
		objects.FieldKeyCurrentPhase: string(PhaseC1Scope),
		objects.FieldKeyActivityLog: []any{
			map[string]any{convSugKeyActivityPhase: string(PhaseC1Scope)},
		},
	}
	MergeObjectUpdateBodyPreservingCurrentPhaseOnDegradedSnapshot(existing, body, degraded)
	al := body[objects.FieldKeyActivityLog].([]any)
	last := al[len(al)-1].(map[string]any)
	if got := last[convSugKeyActivityPhase]; got != string(PhaseC6Exit) {
		t.Fatalf("activity_log phase: got %v want c6", got)
	}
}

func TestMergeObjectUpdateBodyPreservingCurrentPhaseOnDegradedSnapshot_allowsMeasurementWhenHealthy(t *testing.T) {
	t.Parallel()
	healthy := &TestBundleConvergenceSnapshot{LinesInWindow: 2, DeltaAssessment: "neutral"}
	existing := map[string]any{
		objects.FieldKeyCurrentPhase: string(PhaseC6Exit),
	}
	body := map[string]any{
		objects.FieldKeyCurrentPhase: string(PhaseC5Verify),
	}
	MergeObjectUpdateBodyPreservingCurrentPhaseOnDegradedSnapshot(existing, body, healthy)
	if got := body[objects.FieldKeyCurrentPhase]; got != string(PhaseC5Verify) {
		t.Fatalf("current_phase: got %v want measurement suggestion when window has data", got)
	}
}
