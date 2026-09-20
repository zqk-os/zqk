package scheduler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

// convSugKey* are convergence suggested-field / after_state_snapshot map keys without objects.FieldKey* constants.
const (
	convSugKeyActivityAction                 = "action"
	convSugKeyActivityPhase                  = "phase"
	convSugKeyActivityTimestamp              = "timestamp"
	convSugKeyEvaluationSurface              = "evaluation_surface"
	convSugKeyFingerprintLatestOutcome       = "fingerprint_latest_outcome"
	convSugKeyFailingFingerprintsNow         = "failing_fingerprints_now"
	convSugKeyHealthWatermarkRFC3339         = "health_watermark_rfc3339"
	convSugKeyHeartbeat                      = "heartbeat"
	convSugKeyHeartbeatMeasurementAgeSeconds = "measurement_age_seconds"
	convSugKeyHeartbeatStale                 = "stale"
	convSugKeyHeartbeatStaleAfterSeconds     = "stale_after_seconds"
	convSugKeyLinesInWindow                  = "lines_in_window"
	convSugKeyMeasureTestBundleHealth        = "measure_test_bundle_health"
	// convSugKeyActivityActionNoNewWatermark is activity_log.action when measure ran but health.jsonl tail watermark unchanged.
	convSugKeyActivityActionNoNewWatermark    = "measure_no_new_health_watermark"
	convSugKeyMeasurementOutcomeSchemaVersion = "measurement_outcome_schema_version"
	convSugKeyObjectUpdateBody                = "object_update_body"
	convSugKeyPhaseRouter                     = "phase_router"
	convSugKeyPredictionDebrief               = "prediction_debrief"
	convSugKeySessionCompletionBlockedReasons = "session_completion_blocked_reasons"
	convSugKeySessionRoutingContext           = "session_routing_context"
	convSugKeyStampTombstone                  = "stamp_tombstone"
	convSugKeySuggestedRerunByFingerprint     = "suggested_rerun_by_fingerprint"
	convSugKeyTombstoneDisparity              = "tombstone_disparity"
	convSugKeyTriggerQueuePending             = "trigger_queue_pending"
	// convSugKeyPerTestFailureLedger nests under after_state_snapshot: fingerprint → test name → last failure meta.
	convSugKeyPerTestFailureLedger = "per_test_failure_ledger_v1"
)

func mergeCompletionGateObservabilityIntoPhaseRouter(pm map[string]any, sessionThresholds map[string]any) {
	note := CompletionGateObservabilityNote(sessionThresholds)
	if note == "" || pm == nil {
		return
	}
	raw, ok := pm[objects.FieldKeyNotes]
	if !ok || raw == nil {
		pm[objects.FieldKeyNotes] = []any{note}
		return
	}
	switch n := raw.(type) {
	case []any:
		out := make([]any, 0, len(n)+1)
		out = append(out, note)
		out = append(out, n...)
		pm[objects.FieldKeyNotes] = out
	default:
		pm[objects.FieldKeyNotes] = append([]any{note}, raw)
	}
}

// BuildSuggestedConvergenceSessionFields maps health snapshot → convergence_session field shapes (for zqk object update).
// sessionThresholds is the CVS thresholds map when known (same read as routing); nil skips completion_gate observability lines in phase_router.notes.
func BuildSuggestedConvergenceSessionFields(snap *TestBundleConvergenceSnapshot, effCurrentPhase, effFlowVariant string, sessionRoutingMeta map[string]any, beforeStateSnapshot map[string]any, stampTombstone bool, sessionPredictions map[string]any, finalizeDebrief bool, debriefNotes string, sessionThresholds map[string]any) (map[string]any, error) {
	next := BuildNextActionText(snap)
	afterMap := BuildAfterStateSnapshotMap(snap)
	out := map[string]any{
		objects.FieldKeyDeltaAssessment:    snap.DeltaAssessment,
		objects.FieldKeyLastMeasurementAt:  snap.HealthWatermarkRFC3339,
		objects.FieldKeyAfterStateSnapshot: afterMap,
		objects.FieldKeyNextAction:         next,
	}
	out[convSugKeyTombstoneDisparity] = ComputeTombstoneDisparity(beforeStateSnapshot, afterMap)
	if stampTombstone {
		out[convSugKeyStampTombstone] = true
	}
	if len(sessionRoutingMeta) > 0 {
		out[convSugKeySessionRoutingContext] = sessionRoutingMeta
	}
	if deb := BuildPredictionDebrief(sessionPredictions, snap, debriefNotes); deb != nil {
		out[convSugKeyPredictionDebrief] = deb
	}
	pr := RouteConvergencePhase(snap, effCurrentPhase, effFlowVariant)

	// Telemetry: Emit convergence evaluation trace via coordinator
	sessionID, _ := sessionRoutingMeta[convRouteMetaKeyConvergenceSessionID].(string)
	if sessionID != "" {
		EmitConvergenceEvaluationEvent(
			context.Background(), // Background: request-or-shutdown derived
			"",                   // Handled cleanly if empty
			sessionID,
			effFlowVariant,
			pr,
			out[convSugKeyTombstoneDisparity].(map[string]any),
		)
	}

	pm, err := PhaseRouterResultAsMap(pr)
	if err != nil {
		return nil, err
	}
	mergeCompletionGateObservabilityIntoPhaseRouter(pm, sessionThresholds)
	out[convSugKeyPhaseRouter] = pm
	out[objects.FieldKeyCurrentPhase] = string(pr.SuggestedCurrentPhase)

	out[convSugKeyObjectUpdateBody] = BuildConvergenceObjectUpdateBody(out, stampTombstone, snap, sessionPredictions, finalizeDebrief, debriefNotes)
	return out, nil
}

// BuildConvergenceObjectUpdateBody returns top-level convergence_session fields suitable for object update --file.
func BuildConvergenceObjectUpdateBody(suggested map[string]any, stampTombstone bool, snap *TestBundleConvergenceSnapshot, sessionPredictions map[string]any, finalizeDebrief bool, debriefNotes string) map[string]any {
	keys := []string{
		objects.FieldKeyDeltaAssessment,
		objects.FieldKeyLastMeasurementAt,
		objects.FieldKeyAfterStateSnapshot,
		objects.FieldKeyNextAction,
		objects.FieldKeyCurrentPhase,
	}
	body := make(map[string]any, len(keys)+3)
	for _, k := range keys {
		if v, ok := suggested[k]; ok {
			body[k] = v
		}
	}
	if stampTombstone && snap != nil {
		body[objects.FieldKeyBeforeStateSnapshot] = BuildTestBundleTombstoneFields(snap)
	}
	if finalizeDebrief && snap != nil {
		body[objects.FieldKeyPredictions] = MergePredictionsWithRetrospective(sessionPredictions, snap, debriefNotes)
	}
	if strings.TrimSpace(debriefNotes) != emptyValue {
		body[objects.FieldKeyDebriefNotes] = strings.TrimSpace(debriefNotes)
	}
	if snap != nil {
		body[objects.FieldKeyActivityLog] = []any{BuildConvergenceActivityLogEntry(suggested, snap)}
	}
	return body
}

// BuildConvergenceActivityLogEntryNoNewWatermark records a measurement attempt when the health tail watermark
// matches CVS last_measurement_at. Timestamp is wall-clock (when the operator or tick ran measure); health
// tail time remains in health_watermark_rfc3339 and CVS last_measurement_at.
func BuildConvergenceActivityLogEntryNoNewWatermark(phase string, snap *TestBundleConvergenceSnapshot, wallClock time.Time) map[string]any {
	wm := ""
	if snap != nil {
		wm = snap.HealthWatermarkRFC3339
	}
	notes := "Measurement run recorded; health.jsonl tail unchanged vs CVS last_measurement_at — no duplicate snapshot fields written."
	if wm != "" {
		notes = fmt.Sprintf("%s (health tail watermark %s)", notes, wm)
	}
	entry := map[string]any{
		convSugKeyActivityTimestamp: wallClock.UTC().Format(time.RFC3339),
		convSugKeyActivityPhase:     strings.TrimSpace(phase),
		convSugKeyActivityAction:    convSugKeyActivityActionNoNewWatermark,
		objects.FieldKeyNotes:       notes,
	}
	if wm != "" {
		entry[convSugKeyHealthWatermarkRFC3339] = wm
	}
	return entry
}

// BuildConvergenceActivityLogEntry produces one activity_log entry matching convergence_session conventions.
func BuildConvergenceActivityLogEntry(suggested map[string]any, snap *TestBundleConvergenceSnapshot) map[string]any {
	phase := ""
	if p, ok := suggested[objects.FieldKeyCurrentPhase].(string); ok {
		phase = p
	}
	notes := paths.RewriteCanonicalCLIInvocations("Automated measure from test-bundle health.jsonl (zqk scheduler convergence measure).")
	if snap != nil && len(snap.FailingFingerprintsNow) > 0 {
		notes = fmt.Sprintf("Failing fingerprints in window: %d. %s", len(snap.FailingFingerprintsNow), notes)
	}
	ts := ""
	da := ""
	if snap != nil {
		ts = snap.HealthWatermarkRFC3339
		da = snap.DeltaAssessment
	}
	return map[string]any{
		convSugKeyActivityTimestamp:     ts,
		convSugKeyActivityPhase:         phase,
		convSugKeyActivityAction:        convSugKeyMeasureTestBundleHealth,
		objects.FieldKeyDeltaAssessment: da,
		objects.FieldKeyNotes:           notes,
	}
}

// BuildAfterStateSnapshotMap maps snapshot fields for after_state_snapshot persistence.
func BuildAfterStateSnapshotMap(snap *TestBundleConvergenceSnapshot) map[string]any {
	m := map[string]any{
		convSugKeyEvaluationSurface:               EvaluationSurfaceSchedulerTestBundleHealthJSONL,
		convSugKeyHealthWatermarkRFC3339:          snap.HealthWatermarkRFC3339,
		convSugKeyLinesInWindow:                   snap.LinesInWindow,
		convSugKeyFailingFingerprintsNow:          snap.FailingFingerprintsNow,
		convSugKeyFingerprintLatestOutcome:        snap.FingerprintLatestOutcome,
		objects.FieldKeyReadyForSessionCompletion: snap.ReadyForSessionCompletion,
		convSugKeyTriggerQueuePending:             snap.TriggerQueuePending,
	}
	for k, v := range BuildTestBundleTombstoneFields(snap) {
		m[k] = v
	}
	// Always set session_completion_blocked_reasons. Storage update shallow-merges map-valued fields
	// (mergeMapPatchIntoExisting): omitting this key when empty would leave stale reasons from a
	// prior persist alongside ready_for_session_completion=true.
	blocked := snap.SessionCompletionBlockedReasons
	if blocked == nil {
		blocked = []string{}
	}
	m[convSugKeySessionCompletionBlockedReasons] = blocked
	if snap.Heartbeat != nil {
		m[convSugKeyHeartbeat] = map[string]any{
			convSugKeyHeartbeatMeasurementAgeSeconds: snap.Heartbeat.MeasurementAgeSeconds,
			convSugKeyHeartbeatStale:                 snap.Heartbeat.Stale,
			convSugKeyHeartbeatStaleAfterSeconds:     snap.Heartbeat.StaleAfterSeconds,
		}
	}
	if len(snap.SuggestedRerunByFingerprint) > 0 {
		m[convSugKeySuggestedRerunByFingerprint] = snap.SuggestedRerunByFingerprint
	}
	if snap.PrimaryMeasurementOutcome != "" {
		m[objects.FieldKeyPrimaryMeasurementOutcome] = snap.PrimaryMeasurementOutcome
	}
	if snap.PrimaryMeasurementOutcomeDetail != "" {
		m[objects.FieldKeyPrimaryMeasurementOutcomeDetail] = snap.PrimaryMeasurementOutcomeDetail
	}
	if snap.MeasurementOutcomeSchemaVersion != "" {
		m[convSugKeyMeasurementOutcomeSchemaVersion] = snap.MeasurementOutcomeSchemaVersion
	}
	return m
}

// BuildNextActionText formats next_action for persistence.
func BuildNextActionText(snap *TestBundleConvergenceSnapshot) string {
	var b strings.Builder
	b.WriteString(snap.NextActionHint)
	if len(snap.SuggestedRerunByFingerprint) == 0 {
		return b.String()
	}
	b.WriteString("\n\nSuggested reruns (one command per failing fingerprint):\n")
	fps := make([]string, 0, len(snap.SuggestedRerunByFingerprint))
	for fp := range snap.SuggestedRerunByFingerprint {
		fps = append(fps, fp)
	}
	sort.Strings(fps)
	for _, fp := range fps {
		cmds := snap.SuggestedRerunByFingerprint[fp]
		if len(cmds) == 0 {
			continue
		}
		fmt.Fprintf(&b, "- %s: %s\n", fp, cmds[0])
	}
	return b.String()
}
