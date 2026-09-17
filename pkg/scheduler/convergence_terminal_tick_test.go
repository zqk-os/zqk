package scheduler

import (
	stdcontext "context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/convergerollup"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
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
	out := buildFollowupDraftConvergenceSessionObject("CVS-prior-1", prior, snap)
	if got, _ := out[objects.FieldKeyKind].(string); got != objects.KindConvergenceSession {
		t.Fatalf("kind: %q", got)
	}
	if got, _ := out[objects.FieldKeyStatus].(string); got != "draft" {
		t.Fatalf("status: %q", got)
	}
	refs, _ := out[objects.FieldKeyRelatedObjectRefs].([]string)
	if len(refs) != 1 || refs[0] != "CVS-prior-1" {
		t.Fatalf("related_object_refs: %#v", out[objects.FieldKeyRelatedObjectRefs])
	}
	if got, _ := out[objects.FieldKeyNextAction].(string); got != "run tests" {
		t.Fatalf("next_action: %q", got)
	}
}

func TestMaybeSpawnTerminalFollowupDraft_CreateIntegration(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	defer env.Cleanup()

	provider, ok := env.Storage.(storage.ObjectStorageProvider)
	if !ok {
		t.Fatal("storage is not ObjectStorageProvider")
	}
	ctx := stdcontext.Background()
	secCtx := env.SecurityContext
	priorID := "CVS-REDACTED"
	prior := map[string]any{
		objects.FieldKeyID:               priorID,
		objects.FieldKeyKind:             objects.KindConvergenceSession,
		objects.FieldKeySchemaVersion:    objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:           "abandoned",
		objects.FieldKeyTitle:            "Prior CVS",
		objects.FieldKeyCurrentPhase:     "c1_scope",
		objects.FieldKeyHypothesis:       "hypothesis text",
		objects.FieldKeyDesiredEndState:  "desired end state",
		objects.FieldKeyOutcomeCharacter: "pending",
		objects.FieldKeyDeltaAssessment:  "unknown",
	}
	if err := provider.Create(storage.WithSyncCreateForKind(ctx, objects.KindConvergenceSession), secCtx, prior); err != nil {
		t.Fatalf("create prior: %v", err)
	}

	h := &ConvergenceSessionTickHandler{
		storage:     provider,
		projectRoot: env.TestRoot,
		logger:      logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
	snap := &TestBundleConvergenceSnapshot{
		HealthWatermarkRFC3339: "2026-08-02T12:00:01Z",
		HadFailureInWindow:     true,
		FailingFingerprintsNow: []string{"tick-spawn-fp"},
		NextActionHint:         "run tests",
	}
	job := &ScheduledJob{
		ID: "SCH-spawn-int",
		EnvironmentVariables: map[string]string{
			EnvKeyConvergenceTickSpawnFollowupDraft: "true",
		},
	}
	spawnedID, err := h.maybeSpawnTerminalFollowupDraft(ctx, secCtx, job, priorID, prior, snap)
	if err != nil {
		t.Fatalf("maybeSpawnTerminalFollowupDraft: %v", err)
	}
	if spawnedID == "" {
		t.Fatal("expected spawned follow-up session id")
	}
	if _, err := provider.Read(ctx, secCtx, spawnedID); err != nil {
		t.Fatalf("read spawned %s: %v", spawnedID, err)
	}
	obj, err := provider.Read(ctx, secCtx, spawnedID)
	if err != nil {
		t.Fatalf("read spawned object: %v", err)
	}
	if st, _ := obj[objects.FieldKeyStatus].(string); st != "draft" {
		t.Fatalf("spawned status = %q want draft", st)
	}
	refs, _ := obj[objects.FieldKeyRelatedObjectRefs].([]any)
	if len(refs) == 0 {
		// []string path
		if rs, ok := obj[objects.FieldKeyRelatedObjectRefs].([]string); !ok || len(rs) != 1 || rs[0] != priorID {
			t.Fatalf("related_object_refs = %#v", obj[objects.FieldKeyRelatedObjectRefs])
		}
	}
}
