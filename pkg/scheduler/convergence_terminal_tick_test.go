package scheduler

import (
	"testing"

	"github.com/lanceman/zqk/pkg/convergerollup"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestConvergenceTerminalFollowUpNeeded(t *testing.T) {
	t.Parallel()
	if ok, r := convergenceTerminalFollowUpNeeded(nil); ok || len(r) != 0 {
		t.Fatalf("nil snap: got ok=%v reasons=%v", ok, r)
	}
	green := &TestBundleConvergenceSnapshot{
		DeltaAssessment:                 "neutral",
		PrimaryMeasurementOutcome:       string(convergerollup.MeasurementYieldsConvergence),
		ReadyForSessionCompletion:       true,
		HadFailureInWindow:              false,
		TriggerQueuePending:             0,
		FailingFingerprintsNow:          nil,
		SessionCompletionBlockedReasons: nil,
	}
	if ok, r := convergenceTerminalFollowUpNeeded(green); ok || len(r) != 0 {
		t.Fatalf("green snap: got ok=%v reasons=%v", ok, r)
	}
	bad := &TestBundleConvergenceSnapshot{
		HadFailureInWindow:        true,
		PrimaryMeasurementOutcome: string(convergerollup.MeasurementYieldsConvergence),
	}
	if ok, _ := convergenceTerminalFollowUpNeeded(bad); !ok {
		t.Fatal("expected follow-up when HadFailureInWindow")
	}
}

func TestBuildFollowupDraftConvergenceSessionObject(t *testing.T) {
	t.Parallel()
	prior := map[string]any{
		objects.FieldKeyTitle:       "Prior title",
		objects.FieldKeyHypothesis:  "hyp",
		objects.FieldKeyFlowVariant: "fv",
	}
	snap := &TestBundleConvergenceSnapshot{NextActionHint: "run tests"}
	out := buildFollowupDraftConvergenceSessionObject("CONV-prior-1", prior, snap)
	if got, _ := out[objects.FieldKeyKind].(string); got != objects.KindConvergenceSession {
		t.Fatalf("kind: %q", got)
	}
	if got, _ := out[objects.FieldKeyStatus].(string); got != "draft" {
		t.Fatalf("status: %q", got)
	}
	refs, _ := out[objects.FieldKeyRelatedObjectRefs].([]string)
	if len(refs) != 1 || refs[0] != "CONV-prior-1" {
		t.Fatalf("related_object_refs: %#v", out[objects.FieldKeyRelatedObjectRefs])
	}
	if got, _ := out[objects.FieldKeyNextAction].(string); got != "run tests" {
		t.Fatalf("next_action: %q", got)
	}
}
