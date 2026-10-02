package scheduler

import (
	"context"
	"strconv"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/convergence"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

func checkRateLimitTicksPerHour(h *ConvergenceSessionTickHandler, job *ScheduledJob, sessionID string, obj map[string]any, thresholds map[string]any) bool {
	maxPerHour := thresholdMaxTicksPerHour(thresholds)
	if activityLog, ok := obj[objects.FieldKeyActivityLog].([]any); ok && maxPerHour > 0 {
		if n := countMeasureTicksInLastHour(activityLog, time.Now().UTC()); n >= maxPerHour {
			ConvergenceSessionTickLog(h.logger).Info(LogEventConvergenceSessionTickSkippedMaxTicksPerHour).
				JobID(job.ID).
				SessionID(sessionID).
				Int("ticks_last_hour", n).
				Int("max_ticks_per_hour", maxPerHour).
				Log()
			return true
		}
	}
	return false
}

type testBundleEvaluationAdapter struct{}

func (testBundleEvaluationAdapter) Measure(ctx context.Context, job *ScheduledJob, sessionID string, obj map[string]any, h *ConvergenceSessionTickHandler) (*ConvergenceMeasureResult, error) {
	limit := 500
	if s := envLookup(job, EnvKeyHealthLimit); s != emptyValue {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	skipCtx := strings.EqualFold(envLookup(job, EnvKeySkipSessionContext), "true")
	flagCP := envLookup(job, EnvKeyCurrentPhase)
	flagFV := envLookup(job, EnvKeyFlowVariant)

	secCtx := pkgctx.NewSystemSecurityContext()

	lines, stop, err := h.resolveTestBundleHealthLinesForTick(ctx, job, limit)
	if err != nil {
		return nil, err
	}
	if stop {
		return &ConvergenceMeasureResult{Skip: true}, nil
	}
	snap := BuildTestBundleConvergenceSnapshot(h.projectRoot, lines)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	statusStr, _ := obj[objects.FieldKeyStatus].(string)
	if !convergence.SessionStatusPersistsMeasurement(statusStr) {
		err := h.executeTerminalConvergenceSessionTick(ctx, secCtx, job, sessionID, obj, snap)
		if err != nil {
			return nil, err
		}
		return &ConvergenceMeasureResult{Skip: true}, nil
	}

	thresholds, _ := obj[objects.FieldKeyThresholds].(map[string]any)
	if checkRateLimitTicksPerHour(h, job, sessionID, obj, thresholds) {
		return &ConvergenceMeasureResult{Skip: true}, nil
	}

	if ShouldSkipConvergencePersistForDuplicateWatermark(obj, snap) {
		audit := ConvergenceDuplicateWatermarkAuditUpdate(obj, snap, time.Now())
		return &ConvergenceMeasureResult{
			IsDuplicateWatermark:   true,
			AuditUpdateBody:        audit,
			HealthWatermarkRFC3339: snap.HealthWatermarkRFC3339,
		}, nil
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	effPhase, effFlow, meta, beforeSnap, predictions, _, err := ResolveConvergenceRoutingSession(ctx, h.storage, secCtx, sessionID, flagCP, flagFV, skipCtx)
	if err != nil {
		return nil, err
	}
	suggested, err := BuildSuggestedConvergenceSessionFields(snap, effPhase, effFlow, meta, beforeSnap, false, predictions, false, "", ThresholdsMapFromObject(obj))
	if err != nil {
		return nil, errfmt.Newf("convergence_session_tick: build suggested fields").Wrap(err)
	}
	body, _ := suggested["object_update_body"].(map[string]any)
	if len(body) == 0 {
		return nil, errfmt.Errorf("convergence_session_tick: empty object_update_body")
	}

	MergeConvergencePersistObjectUpdateBody(obj, body, snap)

	return &ConvergenceMeasureResult{
		ObjectUpdateBody:          body,
		HealthWatermarkRFC3339:    snap.HealthWatermarkRFC3339,
		DeltaAssessment:           snap.DeltaAssessment,
		ReadyForSessionCompletion: snap.ReadyForSessionCompletion,
		PrimaryMeasurementOutcome: snap.PrimaryMeasurementOutcome,
		NextActionHint:            snap.NextActionHint,
	}, nil
}

type cefDiamondEvaluationAdapter struct{}

func (cefDiamondEvaluationAdapter) Measure(ctx context.Context, job *ScheduledJob, sessionID string, obj map[string]any, h *ConvergenceSessionTickHandler) (*ConvergenceMeasureResult, error) {
	statusStr, _ := obj[objects.FieldKeyStatus].(string)
	if !convergence.SessionStatusPersistsMeasurement(statusStr) {
		return &ConvergenceMeasureResult{Skip: true}, nil
	}

	thresholds, _ := obj[objects.FieldKeyThresholds].(map[string]any)
	if checkRateLimitTicksPerHour(h, job, sessionID, obj, thresholds) {
		return &ConvergenceMeasureResult{Skip: true}, nil
	}

	res, err := BuildCEFDiamondMeasureResult(h.projectRoot, sessionID, thresholds)
	if err != nil {
		return nil, err
	}

	// CEF diamond has no HealthWatermarkRFC3339 equivalent for deduplication, we rely on matrix re-reads always writing.
	// We might consider using LastMeasurementAt for duplicate suppression if needed, but for now we write.

	// Add an activity_log audit entry
	activityEntry := map[string]any{
		convSugKeyActivityTimestamp:     time.Now().UTC().Format(time.RFC3339),
		convSugKeyActivityPhase:         "", // Could read from obj CurrentPhase
		convSugKeyActivityAction:        "measure_cef_diamond_scorecard",
		objects.FieldKeyDeltaAssessment: res.DeltaAssessment,
		objects.FieldKeyNotes:           "Automated measure from CEF diamond scorecard matrix.",
	}

	res.ObjectUpdateBody[objects.FieldKeyActivityLog] = []any{activityEntry}

	return &ConvergenceMeasureResult{
		ObjectUpdateBody:          res.ObjectUpdateBody,
		HealthWatermarkRFC3339:    res.LastMeasurementAt,
		DeltaAssessment:           res.DeltaAssessment,
		ReadyForSessionCompletion: res.ReadyForSessionCompletion,
		PrimaryMeasurementOutcome: res.PrimaryMeasurementOutcome,
		NextActionHint:            res.NextAction,
	}, nil
}

func init() {
	RegisterEvaluationSurfaceAdapter(EvaluationSurfaceSchedulerTestBundleHealthJSONL, testBundleEvaluationAdapter{})
	RegisterEvaluationSurfaceAdapter(EvaluationSurfaceCEFDiamondScorecard, cefDiamondEvaluationAdapter{})
}
