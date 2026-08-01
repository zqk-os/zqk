// Convergence lifecycle end-to-end tests (storage-backed CVS updates).
// Coverage: measure → BuildSuggestedConvergenceSessionFields → Update(object_update_body); phases c4–c6;
// tombstone stamp + disparity; finalize debrief; routing profile flag; skip-session-context.
// See docs/architecture/CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md. Complemented by
// cmd/zqk/scheduler TestMarshalConvergenceOutputJSON_* and pkg/scheduler unit tests.
package scenario

import (
	"bytes"
	stdcontext "context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/scheduler"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// setupAppliedConvergenceLifecycleBundle applies the convergence-lifecycle bundle into test storage
// and returns handles for follow-up measure/apply steps.
func setupAppliedConvergenceLifecycleBundle(t *testing.T) (
	ctx stdcontext.Context,
	secCtx *pkgctx.SecurityContext,
	provider storage.ObjectStorageProvider,
	cvsID, projectRoot string,
) {
	t.Helper()

	env := setupScenarioCompleteTestEnvironment(t)
	projectRoot = env.TestRoot
	ctx = stdcontext.Background()
	secCtx = env.SecurityContext

	var ok bool
	provider, ok = env.Storage.(storage.ObjectStorageProvider)
	if !ok {
		t.Fatal("env.Storage is not ObjectStorageProvider")
	}

	path := convergenceLifecycleBundlePath(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}

	summary, err := ApplyScenarioBundle(ctx, projectRoot, bytes.NewBuffer(data), ApplyObjectsOnly, &ApplyOptions{Storage: provider})
	if err != nil {
		t.Fatalf("ApplyScenarioBundle: %v", err)
	}
	if len(summary.CreatedConvergenceSessionIDs) != 1 {
		t.Fatalf("expected 1 convergence_session, got %v", summary.CreatedConvergenceSessionIDs)
	}
	cvsID = summary.CreatedConvergenceSessionIDs[0]
	return ctx, secCtx, provider, cvsID, projectRoot
}

// writeHealthJSONLines writes .zqk/logs/scheduler/cvs/test-bundles/health.jsonl (one JSON object per line).
func writeHealthJSONLines(t *testing.T, projectRoot string, lines []map[string]any) {
	t.Helper()
	p := scheduler.TestBundlesHealthFilePath(projectRoot)
	if err := fileutil.EnsureDir(filepath.Dir(p)); err != nil {
		t.Fatalf("mkdir health dir: %v", err)
	}
	var buf bytes.Buffer
	for _, ln := range lines {
		b, err := json.Marshal(ln)
		if err != nil {
			t.Fatalf("marshal health line: %v", err)
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	if err := fileutil.WriteSecureFile(p, buf.Bytes()); err != nil {
		t.Fatalf("write health.jsonl: %v", err)
	}
}

// TestConvergenceLifecycleBundle_MeasureAndApplySuggestedUpdate exercises the documented loop:
// bundle apply → measure from health.jsonl → BuildSuggestedConvergenceSessionFields →
// persist object_update_body via storage Update (same field contract as zqk object update --file).
func TestConvergenceLifecycleBundle_MeasureAndApplySuggestedUpdate(t *testing.T) {
	t.Parallel()

	ctx, secCtx, provider, cvsID, projectRoot := setupAppliedConvergenceLifecycleBundle(t)

	ts := zqktime.NowRFC3339UTC()
	writeHealthJSONLines(t, projectRoot, []map[string]any{
		{
			scheduler.KeyTimestamp:                ts,
			scheduler.KeyBundleCommandFingerprint: "e2e-fp-neutral",
			scheduler.KeyTestOutcome:              "pass",
		},
	})

	lines, err := scheduler.ReadTestBundleHealthTailLines(stdcontext.Background(), projectRoot, 50)
	if err != nil {
		t.Fatalf("ReadTestBundleHealthTailLines: %v", err)
	}
	snap := scheduler.BuildTestBundleConvergenceSnapshot(projectRoot, lines)
	if snap.DeltaAssessment != "neutral" {
		t.Fatalf("delta_assessment: got %q want neutral", snap.DeltaAssessment)
	}
	if !snap.ReadyForSessionCompletion {
		t.Fatalf("expected ready_for_session_completion, blocked: %v", snap.SessionCompletionBlockedReasons)
	}

	effCP, effFV, meta, beforeSnap, predictions, sessionTh, err := scheduler.ResolveConvergenceRoutingSession(
		ctx, provider, secCtx, cvsID, "", "", false,
	)
	if err != nil {
		t.Fatalf("ResolveConvergenceRoutingSession: %v", err)
	}
	sug, err := scheduler.BuildSuggestedConvergenceSessionFields(
		snap, effCP, effFV, meta, beforeSnap, false, predictions, false, "", sessionTh,
	)
	if err != nil {
		t.Fatalf("BuildSuggestedConvergenceSessionFields: %v", err)
	}
	ouRaw, ok := sug["object_update_body"]
	if !ok {
		t.Fatalf("object_update_body missing: %#v", sug["object_update_body"])
	}
	ouRaw, ok = nildecode.DecodeNonNilPayload[any](ouRaw)
	if !ok {
		t.Fatalf("object_update_body missing: %#v", sug["object_update_body"])
	}
	ou, ok := ouRaw.(map[string]any)
	if !ok {
		t.Fatalf("object_update_body missing: %#v", sug["object_update_body"])
	}

	if err := provider.Update(ctx, secCtx, cvsID, ou); err != nil {
		t.Fatalf("Update convergence_session: %v", err)
	}

	updated, err := provider.Read(ctx, secCtx, cvsID)
	if err != nil {
		t.Fatalf("Read CVS: %v", err)
	}
	if got, _ := updated[objects.FieldKeyDeltaAssessment].(string); got != "neutral" {
		t.Fatalf("persisted delta_assessment: %v", updated[objects.FieldKeyDeltaAssessment])
	}
	if got, _ := updated[objects.FieldKeyCurrentPhase].(string); got != "c6_exit" {
		t.Fatalf("persisted current_phase: got %q want c6_exit", got)
	}
	al, ok := updated[objects.FieldKeyActivityLog].([]any)
	if !ok || len(al) < 1 {
		t.Fatalf("activity_log: %#v", updated[objects.FieldKeyActivityLog])
	}
	last, ok := al[len(al)-1].(map[string]any)
	if !ok || last["action"] != "measure_test_bundle_health" {
		t.Fatalf("last activity_log entry: %#v", last)
	}
}

// TestConvergenceLifecycleBundle_RemediateThenGreenTwoIterations runs a failing health window,
// persists router output (trending_away → c4_act), then appends a passing line for the same
// fingerprint and verifies neutral + ready → c6_exit (superseded fail) with two measure entries.
func TestConvergenceLifecycleBundle_RemediateThenGreenTwoIterations(t *testing.T) {
	t.Parallel()

	ctx, secCtx, provider, cvsID, projectRoot := setupAppliedConvergenceLifecycleBundle(t)

	healthPath := scheduler.TestBundlesHealthFilePath(projectRoot)
	if err := fileutil.EnsureDir(filepath.Dir(healthPath)); err != nil {
		t.Fatalf("mkdir health dir: %v", err)
	}

	fp := "e2e-fp-remediate"
	ts1 := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)
	line1 := map[string]any{
		scheduler.KeyTimestamp:                ts1,
		scheduler.KeyBundleCommandFingerprint: fp,
		scheduler.KeyTestOutcome:              "test_fail",
		scheduler.KeySuggestedRerunCommands:   []any{"go test ./pkg/foo -run TestX -timeout 60s"},
	}
	b1, err := json.Marshal(line1)
	if err != nil {
		t.Fatalf("marshal health line: %v", err)
	}
	if err := fileutil.WriteSecureFile(healthPath, append(b1, '\n')); err != nil {
		t.Fatalf("write health.jsonl: %v", err)
	}

	lines, err := scheduler.ReadTestBundleHealthTailLines(stdcontext.Background(), projectRoot, 50)
	if err != nil {
		t.Fatalf("ReadTestBundleHealthTailLines: %v", err)
	}
	snap1 := scheduler.BuildTestBundleConvergenceSnapshot(projectRoot, lines)
	if snap1.DeltaAssessment != "trending_away" {
		t.Fatalf("iteration1 delta_assessment: got %q want trending_away", snap1.DeltaAssessment)
	}

	effCP, effFV, meta, beforeSnap, predictions, sessionTh1, err := scheduler.ResolveConvergenceRoutingSession(
		ctx, provider, secCtx, cvsID, "", "", false,
	)
	if err != nil {
		t.Fatalf("ResolveConvergenceRoutingSession: %v", err)
	}
	sug1, err := scheduler.BuildSuggestedConvergenceSessionFields(
		snap1, effCP, effFV, meta, beforeSnap, false, predictions, false, "", sessionTh1,
	)
	if err != nil {
		t.Fatalf("BuildSuggestedConvergenceSessionFields: %v", err)
	}
	ou1Raw, ok := sug1["object_update_body"]
	if !ok {
		t.Fatalf("object_update_body: %#v", sug1["object_update_body"])
	}
	ou1Raw, ok = nildecode.DecodeNonNilPayload[any](ou1Raw)
	if !ok {
		t.Fatalf("object_update_body: %#v", sug1["object_update_body"])
	}
	ou1, ok := ou1Raw.(map[string]any)
	if !ok {
		t.Fatalf("object_update_body: %#v", sug1["object_update_body"])
	}
	if err := provider.Update(ctx, secCtx, cvsID, ou1); err != nil {
		t.Fatalf("Update CVS: %v", err)
	}

	after1, err := provider.Read(ctx, secCtx, cvsID)
	if err != nil {
		t.Fatalf("Read CVS: %v", err)
	}
	if got, _ := after1[objects.FieldKeyDeltaAssessment].(string); got != "trending_away" {
		t.Fatalf("persisted delta_assessment: %v", after1[objects.FieldKeyDeltaAssessment])
	}
	if got, _ := after1[objects.FieldKeyCurrentPhase].(string); got != "c4_act" {
		t.Fatalf("persisted current_phase: got %q want c4_act", got)
	}

	// Second pass: append newer green line for same fingerprint → neutral (recovered).
	ts2 := zqktime.NowRFC3339UTC()
	line2 := map[string]any{
		scheduler.KeyTimestamp:                ts2,
		scheduler.KeyBundleCommandFingerprint: fp,
		scheduler.KeyTestOutcome:              "pass",
	}
	b2, err := json.Marshal(line2)
	if err != nil {
		t.Fatalf("marshal line2: %v", err)
	}
	f, err := os.OpenFile(healthPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open health append: %v", err)
	}
	if _, err := f.Write(append(b2, '\n')); err != nil {
		_ = f.Close()
		t.Fatalf("append health: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close health: %v", err)
	}

	lines2, err := scheduler.ReadTestBundleHealthTailLines(stdcontext.Background(), projectRoot, 50)
	if err != nil {
		t.Fatalf("ReadTestBundleHealthTailLines: %v", err)
	}
	snap2 := scheduler.BuildTestBundleConvergenceSnapshot(projectRoot, lines2)
	if snap2.DeltaAssessment != "neutral" {
		t.Fatalf("iteration2 delta_assessment: got %q want neutral", snap2.DeltaAssessment)
	}
	if !snap2.ReadyForSessionCompletion {
		t.Fatalf("iteration2 expected ready_for_session_completion, blocked: %v", snap2.SessionCompletionBlockedReasons)
	}

	effCP2, effFV2, meta2, beforeSnap2, predictions2, sessionTh2, err := scheduler.ResolveConvergenceRoutingSession(
		ctx, provider, secCtx, cvsID, "", "", false,
	)
	if err != nil {
		t.Fatalf("ResolveConvergenceRoutingSession: %v", err)
	}
	sug2, err := scheduler.BuildSuggestedConvergenceSessionFields(
		snap2, effCP2, effFV2, meta2, beforeSnap2, false, predictions2, false, "", sessionTh2,
	)
	if err != nil {
		t.Fatalf("BuildSuggestedConvergenceSessionFields: %v", err)
	}
	ou2Raw, ok := sug2["object_update_body"]
	if !ok {
		t.Fatalf("object_update_body: %#v", sug2["object_update_body"])
	}
	ou2Raw, ok = nildecode.DecodeNonNilPayload[any](ou2Raw)
	if !ok {
		t.Fatalf("object_update_body: %#v", sug2["object_update_body"])
	}
	ou2, ok := ou2Raw.(map[string]any)
	if !ok {
		t.Fatalf("object_update_body: %#v", sug2["object_update_body"])
	}
	if err := provider.Update(ctx, secCtx, cvsID, ou2); err != nil {
		t.Fatalf("Update CVS: %v", err)
	}

	final, err := provider.Read(ctx, secCtx, cvsID)
	if err != nil {
		t.Fatalf("Read CVS: %v", err)
	}
	if got, _ := final[objects.FieldKeyDeltaAssessment].(string); got != "neutral" {
		t.Fatalf("persisted delta_assessment: %v", final[objects.FieldKeyDeltaAssessment])
	}
	if got, _ := final[objects.FieldKeyCurrentPhase].(string); got != "c6_exit" {
		t.Fatalf("persisted current_phase: got %q want c6_exit", got)
	}
	al, ok := final[objects.FieldKeyActivityLog].([]any)
	if !ok || len(al) != 2 {
		t.Fatalf("activity_log len: got %d want 2: %#v", len(al), final[objects.FieldKeyActivityLog])
	}
}

// TestConvergenceLifecycleBundle_StampTombstoneAndDisparity covers --stamp-tombstone persistence
// and a follow-up measure where scope fingerprint drifts (second fingerprint appears).
func TestConvergenceLifecycleBundle_StampTombstoneAndDisparity(t *testing.T) {
	t.Parallel()

	ctx, secCtx, provider, cvsID, projectRoot := setupAppliedConvergenceLifecycleBundle(t)

	ts := time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339)
	writeHealthJSONLines(t, projectRoot, []map[string]any{
		{
			scheduler.KeyTimestamp:                ts,
			scheduler.KeyBundleCommandFingerprint: "e2e-tomb-a",
			scheduler.KeyTestOutcome:              "pass",
		},
	})

	lines, err := scheduler.ReadTestBundleHealthTailLines(stdcontext.Background(), projectRoot, 50)
	if err != nil {
		t.Fatalf("ReadTestBundleHealthTailLines: %v", err)
	}
	snap0 := scheduler.BuildTestBundleConvergenceSnapshot(projectRoot, lines)
	effCP, effFV, meta, beforeSnap, predictions, sessionTh, err := scheduler.ResolveConvergenceRoutingSession(
		ctx, provider, secCtx, cvsID, "", "", false,
	)
	if err != nil {
		t.Fatalf("ResolveConvergenceRoutingSession: %v", err)
	}
	sug0, err := scheduler.BuildSuggestedConvergenceSessionFields(
		snap0, effCP, effFV, meta, beforeSnap, true, predictions, false, "", sessionTh,
	)
	if err != nil {
		t.Fatalf("BuildSuggestedConvergenceSessionFields: %v", err)
	}
	ou0Raw, ok := sug0["object_update_body"]
	if !ok {
		t.Fatalf("expected before_state_snapshot in object_update_body when stamp tombstone: %#v", sug0["object_update_body"])
	}
	ou0Raw, ok = nildecode.DecodeNonNilPayload[any](ou0Raw)
	if !ok {
		t.Fatalf("expected before_state_snapshot in object_update_body when stamp tombstone: %#v", sug0["object_update_body"])
	}
	ou0, ok := ou0Raw.(map[string]any)
	if !ok || ou0[objects.FieldKeyBeforeStateSnapshot] == nil {
		t.Fatalf("expected before_state_snapshot in object_update_body when stamp tombstone: %#v", ou0)
	}
	if err := provider.Update(ctx, secCtx, cvsID, ou0); err != nil {
		t.Fatalf("Update CVS: %v", err)
	}

	ts2 := zqktime.NowRFC3339UTC()
	writeHealthJSONLines(t, projectRoot, []map[string]any{
		{
			scheduler.KeyTimestamp:                ts,
			scheduler.KeyBundleCommandFingerprint: "e2e-tomb-a",
			scheduler.KeyTestOutcome:              "pass",
		},
		{
			scheduler.KeyTimestamp:                ts2,
			scheduler.KeyBundleCommandFingerprint: "e2e-tomb-b",
			scheduler.KeyTestOutcome:              "test_fail",
		},
	})

	lines2, err := scheduler.ReadTestBundleHealthTailLines(stdcontext.Background(), projectRoot, 50)
	if err != nil {
		t.Fatalf("ReadTestBundleHealthTailLines: %v", err)
	}
	snap1 := scheduler.BuildTestBundleConvergenceSnapshot(projectRoot, lines2)
	if snap1.DeltaAssessment != "trending_away" {
		t.Fatalf("delta: got %q want trending_away", snap1.DeltaAssessment)
	}

	effCP2, effFV2, meta2, beforeSnap2, predictions2, sessionTh2b, err := scheduler.ResolveConvergenceRoutingSession(
		ctx, provider, secCtx, cvsID, "", "", false,
	)
	if err != nil {
		t.Fatalf("ResolveConvergenceRoutingSession: %v", err)
	}
	sug1, err := scheduler.BuildSuggestedConvergenceSessionFields(
		snap1, effCP2, effFV2, meta2, beforeSnap2, false, predictions2, false, "", sessionTh2b,
	)
	if err != nil {
		t.Fatalf("BuildSuggestedConvergenceSessionFields: %v", err)
	}
	td, ok := sug1["tombstone_disparity"].(map[string]any)
	if !ok {
		t.Fatalf("tombstone_disparity: %#v", sug1["tombstone_disparity"])
	}
	if td["active_tombstone"] != true {
		t.Fatalf("expected active tombstone, got %#v", td)
	}
	if td["scope_fingerprint_match"] != false {
		t.Fatalf("expected scope drift after second fingerprint: %#v", td)
	}
	summary, _ := td["disparity_summary"].(string)
	if summary == emptyValue || summary == "tombstone anchor fields match current measurement" {
		t.Fatalf("unexpected disparity_summary: %q", summary)
	}
}

// TestConvergenceLifecycleBundle_FinalizeDebriefAndDebriefNotes persists predictions.retrospective
// and top-level debrief_notes (same flags as CLI --finalize-debrief / debrief copy).
func TestConvergenceLifecycleBundle_FinalizeDebriefAndDebriefNotes(t *testing.T) {
	t.Parallel()

	ctx, secCtx, provider, cvsID, projectRoot := setupAppliedConvergenceLifecycleBundle(t)

	pred := map[string]any{
		"expected_signal":       "decreasing test failures",
		"hypothesis_confidence": "high",
	}
	if err := provider.Update(ctx, secCtx, cvsID, map[string]any{objects.FieldKeyPredictions: pred}); err != nil {
		t.Fatalf("seed predictions: %v", err)
	}

	ts := zqktime.NowRFC3339UTC()
	writeHealthJSONLines(t, projectRoot, []map[string]any{
		{
			scheduler.KeyTimestamp:                ts,
			scheduler.KeyBundleCommandFingerprint: "e2e-debrief-fp",
			scheduler.KeyTestOutcome:              "pass",
		},
	})

	lines, err := scheduler.ReadTestBundleHealthTailLines(stdcontext.Background(), projectRoot, 50)
	if err != nil {
		t.Fatalf("ReadTestBundleHealthTailLines: %v", err)
	}
	snap := scheduler.BuildTestBundleConvergenceSnapshot(projectRoot, lines)

	effCP, effFV, meta, beforeSnap, predictions, sessionTh, err := scheduler.ResolveConvergenceRoutingSession(
		ctx, provider, secCtx, cvsID, "", "", false,
	)
	if err != nil {
		t.Fatalf("ResolveConvergenceRoutingSession: %v", err)
	}
	if predictions == nil {
		t.Fatal("expected predictions loaded from CVS")
	}

	sug, err := scheduler.BuildSuggestedConvergenceSessionFields(
		snap, effCP, effFV, meta, beforeSnap, false, predictions, true, "handoff: confirm scheduler activity idle", sessionTh,
	)
	if err != nil {
		t.Fatalf("BuildSuggestedConvergenceSessionFields: %v", err)
	}
	if _, ok := sug["prediction_debrief"].(map[string]any); !ok {
		t.Fatalf("expected prediction_debrief: %#v", sug["prediction_debrief"])
	}
	ou, ok := sug["object_update_body"].(map[string]any)
	if !ok {
		t.Fatalf("object_update_body: %#v", sug["object_update_body"])
	}
	predRaw, ok := ou[objects.FieldKeyPredictions]
	if !ok {
		t.Fatalf("expected predictions.retrospective in object_update_body: %#v", ou[objects.FieldKeyPredictions])
	}
	predRaw, ok = nildecode.DecodeNonNilPayload[any](predRaw)
	if !ok {
		t.Fatalf("expected predictions.retrospective in object_update_body: %#v", ou[objects.FieldKeyPredictions])
	}
	predOut, ok := predRaw.(map[string]any)
	if !ok || predOut["retrospective"] == nil {
		t.Fatalf("expected predictions.retrospective in object_update_body: %#v", ou[objects.FieldKeyPredictions])
	}
	if dn, _ := ou[objects.FieldKeyDebriefNotes].(string); dn == emptyValue {
		t.Fatalf("expected debrief_notes: %#v", ou[objects.FieldKeyDebriefNotes])
	}

	if err := provider.Update(ctx, secCtx, cvsID, ou); err != nil {
		t.Fatalf("Update CVS: %v", err)
	}
	updated, err := provider.Read(ctx, secCtx, cvsID)
	if err != nil {
		t.Fatalf("Read CVS: %v", err)
	}
	ppRaw, ok := updated[objects.FieldKeyPredictions]
	if !ok {
		t.Fatalf("persisted predictions: %#v", updated[objects.FieldKeyPredictions])
	}
	ppRaw, ok = nildecode.DecodeNonNilPayload[any](ppRaw)
	if !ok {
		t.Fatalf("persisted predictions: %#v", updated[objects.FieldKeyPredictions])
	}
	persistPred, ok := ppRaw.(map[string]any)
	if !ok || persistPred["retrospective"] == nil {
		t.Fatalf("persisted predictions: %#v", updated[objects.FieldKeyPredictions])
	}
}

// TestConvergenceLifecycleBundle_SchedulerFastRoutingProfileViaFlag ensures flow_variant flag selects
// the scheduler_fast routing profile (CLI: --flow-variant scheduler_fast).
func TestConvergenceLifecycleBundle_SchedulerFastRoutingProfileViaFlag(t *testing.T) {
	t.Parallel()

	ctx, secCtx, provider, cvsID, projectRoot := setupAppliedConvergenceLifecycleBundle(t)

	ts := zqktime.NowRFC3339UTC()
	writeHealthJSONLines(t, projectRoot, []map[string]any{
		{
			scheduler.KeyTimestamp:                ts,
			scheduler.KeyBundleCommandFingerprint: "e2e-fast-fp",
			scheduler.KeyTestOutcome:              "pass",
		},
	})

	lines, err := scheduler.ReadTestBundleHealthTailLines(stdcontext.Background(), projectRoot, 50)
	if err != nil {
		t.Fatalf("ReadTestBundleHealthTailLines: %v", err)
	}
	snap := scheduler.BuildTestBundleConvergenceSnapshot(projectRoot, lines)

	effCP, effFV, meta, beforeSnap, predictions, sessionTh, err := scheduler.ResolveConvergenceRoutingSession(
		ctx, provider, secCtx, cvsID, "", "scheduler_fast", false,
	)
	if err != nil {
		t.Fatalf("ResolveConvergenceRoutingSession: %v", err)
	}
	if effFV != "scheduler_fast" {
		t.Fatalf("effective flow variant: got %q", effFV)
	}
	sug, err := scheduler.BuildSuggestedConvergenceSessionFields(
		snap, effCP, effFV, meta, beforeSnap, false, predictions, false, "", sessionTh,
	)
	if err != nil {
		t.Fatalf("BuildSuggestedConvergenceSessionFields: %v", err)
	}
	pr, ok := sug["phase_router"].(map[string]any)
	if !ok {
		t.Fatalf("phase_router: %#v", sug["phase_router"])
	}
	if pr["routing_profile"] != "scheduler_fast" {
		t.Fatalf("routing_profile: got %#v", pr["routing_profile"])
	}
}

// TestConvergenceLifecycleBundle_SkipSessionContextDoesNotReadCVS matches --skip-session-context:
// no before_state_snapshot from storage; tombstone_disparity notes missing tombstone.
func TestConvergenceLifecycleBundle_SkipSessionContextDoesNotReadCVS(t *testing.T) {
	t.Parallel()

	ctx, secCtx, provider, cvsID, projectRoot := setupAppliedConvergenceLifecycleBundle(t)

	ts := zqktime.NowRFC3339UTC()
	writeHealthJSONLines(t, projectRoot, []map[string]any{
		{
			scheduler.KeyTimestamp:                ts,
			scheduler.KeyBundleCommandFingerprint: "e2e-skip-fp",
			scheduler.KeyTestOutcome:              "pass",
		},
	})

	lines, err := scheduler.ReadTestBundleHealthTailLines(stdcontext.Background(), projectRoot, 50)
	if err != nil {
		t.Fatalf("ReadTestBundleHealthTailLines: %v", err)
	}
	snap := scheduler.BuildTestBundleConvergenceSnapshot(projectRoot, lines)

	effCP, effFV, meta, beforeSnap, predictions, sessionTh, err := scheduler.ResolveConvergenceRoutingSession(
		ctx, provider, secCtx, cvsID, "", "", true,
	)
	if err != nil {
		t.Fatalf("ResolveConvergenceRoutingSession: %v", err)
	}
	if beforeSnap != nil {
		t.Fatalf("expected no before_state_snapshot when skip_session_context: %#v", beforeSnap)
	}
	if meta["skip_session_context"] != true {
		t.Fatalf("meta: %#v", meta)
	}
	sug, err := scheduler.BuildSuggestedConvergenceSessionFields(
		snap, effCP, effFV, meta, beforeSnap, false, predictions, false, "", sessionTh,
	)
	if err != nil {
		t.Fatalf("BuildSuggestedConvergenceSessionFields: %v", err)
	}
	td, ok := sug["tombstone_disparity"].(map[string]any)
	if !ok || td["active_tombstone"] != false {
		t.Fatalf("tombstone_disparity: %#v", sug["tombstone_disparity"])
	}
}

// TestConvergenceLifecycleBundle_RemediateToExitThreeIterations runs fail → pass (neutral, ready) →
// clean single-fingerprint window (neutral, ready) ending at c6_exit with three activity entries.
func TestConvergenceLifecycleBundle_RemediateToExitThreeIterations(t *testing.T) {
	t.Parallel()

	ctx, secCtx, provider, cvsID, projectRoot := setupAppliedConvergenceLifecycleBundle(t)

	healthPath := scheduler.TestBundlesHealthFilePath(projectRoot)
	if err := fileutil.EnsureDir(filepath.Dir(healthPath)); err != nil {
		t.Fatalf("mkdir health dir: %v", err)
	}

	fp := "e2e-exit-chain"
	ts1 := time.Now().UTC().Add(-3 * time.Minute).Format(time.RFC3339)
	line1 := map[string]any{
		scheduler.KeyTimestamp:                ts1,
		scheduler.KeyBundleCommandFingerprint: fp,
		scheduler.KeyTestOutcome:              "test_fail",
		scheduler.KeySuggestedRerunCommands:   []any{"go test ./pkg/x -timeout 60s"},
	}
	b1, err := json.Marshal(line1)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := fileutil.WriteSecureFile(healthPath, append(b1, '\n')); err != nil {
		t.Fatalf("write health: %v", err)
	}

	lines1, err := scheduler.ReadTestBundleHealthTailLines(stdcontext.Background(), projectRoot, 50)
	if err != nil {
		t.Fatalf("ReadTestBundleHealthTailLines: %v", err)
	}
	s1 := scheduler.BuildTestBundleConvergenceSnapshot(projectRoot, lines1)
	eff1, effv1, m1, b1snap, p1, th1, err := scheduler.ResolveConvergenceRoutingSession(ctx, provider, secCtx, cvsID, "", "", false)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	sug1, err := scheduler.BuildSuggestedConvergenceSessionFields(s1, eff1, effv1, m1, b1snap, false, p1, false, "", th1)
	if err != nil {
		t.Fatalf("BuildSuggested: %v", err)
	}
	ou1, _ := sug1["object_update_body"].(map[string]any)
	if err := provider.Update(ctx, secCtx, cvsID, ou1); err != nil {
		t.Fatalf("Update: %v", err)
	}

	ts2 := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)
	line2 := map[string]any{
		scheduler.KeyTimestamp:                ts2,
		scheduler.KeyBundleCommandFingerprint: fp,
		scheduler.KeyTestOutcome:              "pass",
	}
	b2, err := json.Marshal(line2)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	f, err := os.OpenFile(healthPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open append: %v", err)
	}
	if _, err := f.Write(append(b2, '\n')); err != nil {
		_ = f.Close()
		t.Fatalf("append: %v", err)
	}
	_ = f.Close()

	lines2, err := scheduler.ReadTestBundleHealthTailLines(stdcontext.Background(), projectRoot, 50)
	if err != nil {
		t.Fatalf("ReadTestBundleHealthTailLines: %v", err)
	}
	s2 := scheduler.BuildTestBundleConvergenceSnapshot(projectRoot, lines2)
	eff2, effv2, m2, b2snap, p2, th2, err := scheduler.ResolveConvergenceRoutingSession(ctx, provider, secCtx, cvsID, "", "", false)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	sug2, err := scheduler.BuildSuggestedConvergenceSessionFields(s2, eff2, effv2, m2, b2snap, false, p2, false, "", th2)
	if err != nil {
		t.Fatalf("BuildSuggested: %v", err)
	}
	ou2, _ := sug2["object_update_body"].(map[string]any)
	if err := provider.Update(ctx, secCtx, cvsID, ou2); err != nil {
		t.Fatalf("Update: %v", err)
	}

	ts3 := zqktime.NowRFC3339UTC()
	writeHealthJSONLines(t, projectRoot, []map[string]any{
		{
			scheduler.KeyTimestamp:                ts3,
			scheduler.KeyBundleCommandFingerprint: "e2e-exit-clean-only",
			scheduler.KeyTestOutcome:              "pass",
		},
	})

	lines3, err := scheduler.ReadTestBundleHealthTailLines(stdcontext.Background(), projectRoot, 50)
	if err != nil {
		t.Fatalf("ReadTestBundleHealthTailLines: %v", err)
	}
	s3 := scheduler.BuildTestBundleConvergenceSnapshot(projectRoot, lines3)
	if s3.DeltaAssessment != "neutral" || !s3.ReadyForSessionCompletion {
		t.Fatalf("iter3 snap: delta=%q ready=%v blocked=%v", s3.DeltaAssessment, s3.ReadyForSessionCompletion, s3.SessionCompletionBlockedReasons)
	}
	eff3, effv3, m3, b3snap, p3, th3, err := scheduler.ResolveConvergenceRoutingSession(ctx, provider, secCtx, cvsID, "", "", false)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	sug3, err := scheduler.BuildSuggestedConvergenceSessionFields(s3, eff3, effv3, m3, b3snap, false, p3, false, "", th3)
	if err != nil {
		t.Fatalf("BuildSuggested: %v", err)
	}
	ou3, _ := sug3["object_update_body"].(map[string]any)
	if err := provider.Update(ctx, secCtx, cvsID, ou3); err != nil {
		t.Fatalf("Update: %v", err)
	}

	final, err := provider.Read(ctx, secCtx, cvsID)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got, _ := final[objects.FieldKeyCurrentPhase].(string); got != "c6_exit" {
		t.Fatalf("current_phase: %q", got)
	}
	al, ok := final[objects.FieldKeyActivityLog].([]any)
	if !ok || len(al) != 3 {
		t.Fatalf("activity_log len: got %d want 3: %#v", len(al), final[objects.FieldKeyActivityLog])
	}
}
