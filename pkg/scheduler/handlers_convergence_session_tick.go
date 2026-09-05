package scheduler

import (
	"context"
	"fmt"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// ConvergenceSessionTickHandler applies test-bundle health measurement to a convergence_session (CVS-*)
// using the same payload as `zqk scheduler convergence measure --session-id`. Configure the job via
// environment_variables: CONVERGENCE_SESSION_ID (required — must be a canonical CVS-* id),
// HEALTH_LIMIT (optional, default 500),
// HEALTH_FILE_MISSING (fail | skip | empty_baseline; default fail when health.jsonl is absent),
// HEALTH_FILE_MISSING_SKIP_BUDGET (optional; skip mode only — consecutive skips before failing, 0 = unlimited),
// CURRENT_PHASE, FLOW_VARIANT, SKIP_SESSION_CONTEXT (true/false),
// CVS_MEASUREMENT_EVENTS (optional; set 0/false to skip appending cvs_measurement_events.jsonl after a successful persist).
// CONVERGENCE_TICK_ROLLUP (optional; set 1/true to run cvs_outcome_rollup.py --apply after a successful tick via cvs_convergence_orchestrate.sh with CVS_ORCH_SKIP_PERSIST=1 — including duplicate-watermark audit-only ticks). Optional CVS_ORCH_ROLLUP_OUT on the job must match the script if you override rollup JSON location (see ResolveCVSRollupLatestJSONPath). When set, appends job events JSONL outcome lines (WriteJobOutcome) with rollup timing and rollup JSON summary when parent_convergence_session_id matches.
// Respects thresholds.max_ticks_per_hour
// on the session object. When there is no new health watermark vs persisted last_measurement_at,
// appends an activity_log audit line (same as measure --persist-session) instead of rewriting snapshot fields.
//
// When the session status is terminal (completed / abandoned / archived) or halted
// (escalated / paused / error), the handler does not persist measurement fields; it logs whether
// follow-up work is recommended from health.jsonl so operators can open a new active session.
// Optional: CONVERGENCE_TICK_SPAWN_FOLLOWUP_DRAFT=true creates a draft follow-up CVS (see convergence_terminal_tick.go).
//
// Execution respects ctx cancellation (scheduler binds execCtx to job max_runtime_seconds). Overlapping ticks for the same CVS id are serialized.
type ConvergenceSessionTickHandler struct {
	storage     storagepkg.ObjectStorageProvider
	projectRoot string
	logger      logging.Logger
}

// NewConvergenceSessionTickHandler creates a tick handler. projectRoot is used for health.jsonl paths.
// NewConvergenceSessionTickHandler creates a new convergence session tick handler
func NewConvergenceSessionTickHandler(storage storagepkg.ObjectStorageProvider, projectRoot string) ConvergenceSessionTickHandlerInterface {
	if projectRoot == emptyValue {
		if fileStorage, ok := storage.(*storagepkg.FileObjectStorage); ok {
			projectRoot = fileStorage.GetProjectRoot()
		}
	}
	return &ConvergenceSessionTickHandler{
		storage:     storage,
		projectRoot: projectRoot,
		logger:      logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// Execute runs one measure-and-update cycle for the configured CVS id.
func (h *ConvergenceSessionTickHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("convergence_session_tick: handler and job required")
	}
	sessionID := strings.TrimSpace(envLookup(job, EnvKeyConvergenceSessionID))
	if sessionID == emptyValue {
		return errfmt.Errorf("convergence_session_tick: set environment_variables %s on job %s", EnvKeyConvergenceSessionID, job.ID)
	}
	if err := validateConvergenceSessionTickTargetID(sessionID); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if h.projectRoot == emptyValue {
		return errfmt.Errorf("convergence_session_tick: project root is required for health.jsonl")
	}

	unlock := acquireConvergenceSessionTickLock(sessionID)
	defer unlock()

	secCtx := pkgctx.NewSystemSecurityContext()

	obj, readErr := h.storage.Read(ctx, secCtx, sessionID)
	if readErr != nil {
		return errfmt.Errorf("convergence_session_tick: read session %s: %w", sessionID, readErr)
	}

	surfaceID := EvaluationSurfaceIDFromSessionObject(obj)
	adapter := getEvaluationSurfaceAdapter(surfaceID)
	if adapter == nil {
		return errfmt.Errorf("convergence_session_tick: unhandled evaluation_surface: %s", surfaceID)
	}

	res, err := adapter.Measure(ctx, job, sessionID, obj, h)
	if err != nil {
		return err
	}
	if res.Skip {
		return nil
	}

	if res.IsDuplicateWatermark {
		if err := ctx.Err(); err != nil {
			return err
		}
		if res.AuditUpdateBody != nil {
			if err := h.storage.Update(ctx, secCtx, sessionID, res.AuditUpdateBody); err != nil {
				return errfmt.Newf("convergence_session_tick: duplicate-watermark audit append").Wrap(err)
			}
		}
		h.maybeRunOrchestrateRollupAfterTick(ctx, job, sessionID)
		if cvsMeasurementEventsEnabled(job) {
			AppendCVSMeasurementEvent(h.projectRoot, map[string]any{
				KeyCVSEventType:           CVSEventTypeMeasureNoNewWatermark,
				objects.FieldKeySessionID: sessionID,
				objects.FieldKeyTickJobID: job.ID,
				objects.FieldKeyWatermark: res.HealthWatermarkRFC3339,
			})
		}
		ConvergenceSessionTickLog(h.logger).Info(LogEventConvergenceSessionTickAuditNoNewWatermark).
			JobID(job.ID).
			SessionID(sessionID).
			Watermark(res.HealthWatermarkRFC3339).
			Log()
		return nil
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	res.ObjectUpdateBody[objects.FieldKeyID] = sessionID
	if err := h.storage.Update(ctx, secCtx, sessionID, res.ObjectUpdateBody); err != nil {
		return errfmt.Newf("convergence_session_tick: update session").Wrap(err)
	}

	h.maybeRunOrchestrateRollupAfterTick(ctx, job, sessionID)

	if cvsMeasurementEventsEnabled(job) {
		AppendCVSMeasurementEvent(h.projectRoot, map[string]any{
			KeyCVSEventType:                           CVSEventTypeMeasureApplied,
			objects.FieldKeySessionID:                 sessionID,
			objects.FieldKeyTickJobID:                 job.ID,
			objects.FieldKeyWatermark:                 res.HealthWatermarkRFC3339,
			objects.FieldKeyDeltaAssessment:           res.DeltaAssessment,
			objects.FieldKeyReadyForSessionCompletion: res.ReadyForSessionCompletion,
			objects.FieldKeyPrimaryMeasurementOutcome: res.PrimaryMeasurementOutcome,
			objects.FieldKeyNextActionHint:            truncateCVSHint(res.NextActionHint, cvsMeasurementHintMaxRunes),
			objects.FieldKeyAgentPromptCli: fmt.Sprintf(
				"zqk scheduler convergence measure --format agent-prompt --session-id %s",
				sessionID,
			),
		})
	}

	ConvergenceSessionTickLog(h.logger).Info(LogEventConvergenceSessionTickAppliedMeasure).
		JobID(job.ID).
		SessionID(sessionID).
		Watermark(res.HealthWatermarkRFC3339).
		Log()
	return nil
}

func envLookup(job *ScheduledJob, key string) string {
	if scheduledJobEnvMissing(job) {
		return ""
	}
	return job.EnvironmentVariables[key]
}

// maybeRunOrchestrateRollupAfterTick runs rollup only (orchestrate script with skip-persist; see EnvKeyCVSOrchestrateSkipPersist).
// Nested/outcome fields update without duplicating the measure step. Logs and returns on failure; does not fail the tick.
// When rollup is enabled, appends a WriteJobOutcome line to the job events JSONL with rollup timing and drift/cvs_rollup_latest.json summary when matched.
// MaybeRunOrchestrateRollupAfterTick runs rollup only (orchestrate script with skip-persist; see EnvKeyCVSOrchestrateSkipPersist).
func (h *ConvergenceSessionTickHandler) MaybeRunOrchestrateRollupAfterTick(ctx context.Context, job *ScheduledJob, sessionID string) {
	h.maybeRunOrchestrateRollupAfterTick(ctx, job, sessionID)
}

func (h *ConvergenceSessionTickHandler) maybeRunOrchestrateRollupAfterTick(ctx context.Context, job *ScheduledJob, sessionID string) {
	if h == nil || job == nil || !jobEnvAffirmative(job, EnvKeyConvergenceTickRollup) {
		return
	}
	root := strings.TrimSpace(envLookup(job, zqkenv.ProjectRoot()))
	if root == emptyValue {
		root = h.projectRoot
	}
	if root == emptyValue {
		ConvergenceSessionTickLog(h.logger).Warn(LogEventConvergenceSessionTickRollupSkippedNoProjectRoot).
			JobID(job.ID).
			SessionID(sessionID).
			Log()
		WriteJobOutcome(h.projectRoot, job.ID, JobTypeConvergenceSessionTick, map[string]any{
			OutcomeKeyConvergenceTickRollupAttempted:  true,
			OutcomeKeyConvergenceTickRollupOK:         false,
			OutcomeKeyConvergenceTickRollupSkipReason: convergenceTickRollupSkipReasonNoProjectRoot,
			OutcomeKeyConvergenceTickRollupDurationMs: 0,
		})
		return
	}
	script := convergenceOrchestrateScriptPath(root)
	rollupOut := strings.TrimSpace(envLookup(job, EnvKeyCVSOrchestrateRollupOut))
	started := time.Now()
	out, err := runCVSOrchestrateRollupCmd(ctx, root, sessionID, rollupOut)
	durationMs := time.Since(started).Milliseconds()

	baseOutcome := map[string]any{
		OutcomeKeyConvergenceTickRollupAttempted:  true,
		OutcomeKeyConvergenceTickRollupDurationMs: durationMs,
	}

	if err != nil {
		WriteJobOutcome(h.projectRoot, job.ID, JobTypeConvergenceSessionTick, mergeStringAnyMaps(baseOutcome, map[string]any{
			OutcomeKeyConvergenceTickRollupOK:                false,
			OutcomeKeyConvergenceTickRollupOutputByteCount:   len(out),
			OutcomeKeyConvergenceTickRollupLatestFileMatched: false,
		}))
		ConvergenceSessionTickLog(h.logger).Warn(LogEventConvergenceSessionTickRollupFailed).
			JobID(job.ID).
			SessionID(sessionID).
			String("script", script).
			Int("rollup_duration_ms", int(durationMs)).
			Int(OutcomeKeyConvergenceTickRollupOutputByteCount, len(out)).
			String("rollup_output", strings.TrimSpace(truncateOrchestrateOutputPreview(string(out), convergenceTickRollupOrchestrateOutputPreviewMaxBytes))).
			WithError(err).
			Log()
		return
	}

	rollupJSONPath := ResolveCVSRollupLatestJSONPath(root, rollupOut)
	rollupStatus, readyParent, matched, readErr := ReadCVSRollupLatestSummary(sessionID, rollupJSONPath)
	if readErr != nil {
		ConvergenceSessionTickLog(h.logger).Warn(LogEventConvergenceSessionTickRollupReadSummaryFailed).
			JobID(job.ID).
			SessionID(sessionID).
			WithError(readErr).
			Log()
	}

	successOutcome := mergeStringAnyMaps(baseOutcome, map[string]any{
		OutcomeKeyConvergenceTickRollupOK:                true,
		OutcomeKeyConvergenceTickRollupLatestFileMatched: matched,
	})
	if matched {
		successOutcome[OutcomeKeyConvergenceTickRollupStatus] = rollupStatus
		successOutcome[OutcomeKeyConvergenceTickRollupReadyForParentCompletion] = readyParent
	}
	WriteJobOutcome(h.projectRoot, job.ID, JobTypeConvergenceSessionTick, successOutcome)

	infoFields := []logging.Field{
		logging.JobIDField(job.ID),
		logging.String("session_id", sessionID),
		logging.ScriptField(script),
		logging.Int("rollup_duration_ms", int(durationMs)),
		logging.Bool(OutcomeKeyConvergenceTickRollupLatestFileMatched, matched),
	}
	if matched {
		infoFields = append(infoFields,
			logging.String(OutcomeKeyConvergenceTickRollupStatus, rollupStatus),
			logging.Bool(OutcomeKeyConvergenceTickRollupReadyForParentCompletion, readyParent),
		)
	}
	ConvergenceSessionTickLog(h.logger).Info(LogEventConvergenceSessionTickRollupCompleted).
		WithFields(infoFields...).
		Log()
}

func mergeStringAnyMaps(a, b map[string]any) map[string]any {
	out := make(map[string]any, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// truncateOrchestrateOutputPreview caps subprocess combined output for logs (ASCII-oriented; byte cap).
func truncateOrchestrateOutputPreview(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	return s[:maxBytes] + "…"
}

func thresholdMaxTicksPerHour(thresholds map[string]any) int {
	if thresholds == nil {
		return 4
	}
	v, ok := thresholds["max_ticks_per_hour"]
	if !ok {
		return 4
	}
	switch n := v.(type) {
	case int:
		if n == 0 {
			return 0 // unlimited
		}
		if n > 0 {
			return n
		}
	case int64:
		if n == 0 {
			return 0
		}
		if n > 0 {
			return int(n)
		}
	case float64:
		if n == 0 {
			return 0
		}
		if n > 0 {
			return int(n)
		}
	}
	return 4
}

func countMeasureTicksInLastHour(activityLog []any, now time.Time) int {
	if len(activityLog) == 0 {
		return 0
	}
	cutoff := now.Add(-1 * time.Hour)
	n := 0
	for _, e := range activityLog {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		if a, _ := m["action"].(string); a != "measure_test_bundle_health" {
			continue
		}
		ts, _ := m["timestamp"].(string)
		if ts == emptyValue {
			continue
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			continue
		}
		if !t.Before(cutoff) {
			n++
		}
	}
	return n
}
