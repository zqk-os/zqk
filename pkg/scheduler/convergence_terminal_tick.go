package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"path/filepath"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/convergerollup"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	enumv "github.com/lanceman/zqk/pkg/specbuilder/bldr_enum_v1/convergence_session"
	"github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// convergenceTerminalFollowUpNeeded reports whether test-bundle health still shows work worth tracking
// after a convergence_session has stopped persisting measurements (lifecycle-terminal completed/abandoned,
// or Option A halted statuses such as escalated/paused/error).
// TRACK: REDACTED — escalated is not lifecycle-terminal.
func convergenceTerminalFollowUpNeeded(snap *TestBundleConvergenceSnapshot) (bool, []string) {
	if snap == nil {
		return false, nil
	}
	var reasons []string
	if snap.HadFailureInWindow {
		reasons = append(reasons, "had_failure_in_window")
	}
	if len(snap.FailingFingerprintsNow) > 0 {
		reasons = append(reasons, "failing_fingerprints_now")
	}
	if snap.TriggerQueuePending > 0 {
		reasons = append(reasons, "trigger_queue_pending")
	}
	if !snap.ReadyForSessionCompletion && (len(snap.SessionCompletionBlockedReasons) > 0 || strings.TrimSpace(snap.SessionCompletionNote) != "") {
		reasons = append(reasons, "session_completion_not_ready")
	}
	if strings.TrimSpace(snap.DeltaAssessment) == "trending_away" {
		reasons = append(reasons, "delta_trending_away")
	}
	pm := strings.TrimSpace(snap.PrimaryMeasurementOutcome)
	if pm != "" && pm != string(convergerollup.MeasurementYieldsConvergence) {
		reasons = append(reasons, "primary_measurement:"+pm)
	}
	return len(reasons) > 0, reasons
}

func (h *ConvergenceSessionTickHandler) executeTerminalConvergenceSessionTick(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	job *ScheduledJob,
	sessionID string,
	obj map[string]any,
	snap *TestBundleConvergenceSnapshot,
) error {
	if h == nil || job == nil {
		return nil
	}
	if ShouldSkipConvergencePersistForDuplicateWatermark(obj, snap) {
		ConvergenceSessionTickLog(h.logger).Info(LogEventConvergenceSessionTickSkippedTerminalDuplicateWatermark).
			JobID(job.ID).
			SessionID(sessionID).
			Watermark(snap.HealthWatermarkRFC3339).
			Log()
		return nil
	}

	followUp, reasons := convergenceTerminalFollowUpNeeded(snap)
	var spawnedID string
	if followUp && strings.EqualFold(strings.TrimSpace(envLookup(job, EnvKeyConvergenceTickSpawnFollowupDraft)), "true") {
		id, err := h.maybeSpawnTerminalFollowupDraft(ctx, secCtx, job, sessionID, obj, snap)
		if err != nil {
			ConvergenceSessionTickLog(h.logger).Warn(LogEventConvergenceSessionTickFollowupSpawnFailed).
				JobID(job.ID).
				SessionID(sessionID).
				WithError(err).
				Log()
		} else {
			spawnedID = id
		}
	}

	fields := []logging.Field{
		logging.JobIDField(job.ID),
		logging.String("session_id", sessionID),
		logging.String("session_status", FieldAsString(obj[objects.FieldKeyStatus])),
		logging.String("watermark", snap.HealthWatermarkRFC3339),
		logging.Bool("follow_up_recommended", followUp),
	}
	if spawnedID != "" {
		fields = append(fields, logging.String("spawned_followup_session_id", spawnedID))
	}
	if len(reasons) > 0 {
		fields = append(fields, logging.String("follow_up_reasons", strings.Join(reasons, ",")))
	}
	if followUp {
		if snap.NextActionHint != "" {
			fields = append(fields, logging.String("next_action_hint", snap.NextActionHint))
		}
		if len(snap.SuggestedRerunByFingerprint) > 0 {
			fields = append(fields, logging.Int("suggested_rerun_fingerprint_count", len(snap.SuggestedRerunByFingerprint)))
			fields = append(fields, logging.String("suggested_rerun_review", "see health.jsonl lines for suggested_rerun_commands per failing fingerprint; re-run bundles or fix then scan-tests"))
		}
		ConvergenceSessionTickLog(h.logger).Warn(LogEventConvergenceSessionTickTerminalMeasurementFollowupWarn).
			WithFields(fields...).
			Log()
	} else {
		ConvergenceSessionTickLog(h.logger).Info(LogEventConvergenceSessionTickTerminalMeasurementQuiet).
			WithFields(fields...).
			Log()
	}
	return nil
}

type terminalFollowupMarker struct {
	PriorSessionID  string `json:"prior_session_id"`
	NewSessionID    string `json:"new_session_id"`
	HealthWatermark string `json:"health_watermark_rfc3339"`
	SpawnJobID      string `json:"spawn_job_id,omitempty"`
}

func terminalFollowupSpawnDir(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir, "terminal_followup_spawn")
}

func terminalFollowupSpawnMarkerFile(projectRoot, priorSessionID, watermark string) string {
	sum := sha256.Sum256([]byte(priorSessionID + "\n" + watermark))
	return filepath.Join(terminalFollowupSpawnDir(projectRoot), hex.EncodeToString(sum[:16])+".json")
}

func readTerminalFollowupMarker(path string) (*terminalFollowupMarker, error) {
	b, err := fileutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m terminalFollowupMarker
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func writeTerminalFollowupMarker(path string, m *terminalFollowupMarker) error {
	if err := fileutil.EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteSecureFile(path, b)
}

// buildFollowupDraftConvergenceSessionObject builds a create payload (no id) for a draft CVS linked to priorSessionID.
func buildFollowupDraftConvergenceSessionObject(priorSessionID string, prior map[string]any, snap *TestBundleConvergenceSnapshot) map[string]any {
	b := bldr_instance_v1.NewConvergenceSessionInstanceBuilder(objects.DefaultSchemaVersion)
	b.Status(enumv.StatusDraft)
	title := "Follow-up convergence session"
	if t, _ := prior[objects.FieldKeyTitle].(string); strings.TrimSpace(t) != "" {
		title = "Follow-up: " + strings.TrimSpace(t)
	}
	b.Title(title)
	b.CurrentPhase(enumv.CurrentPhaseC1Scope)
	b.DeltaAssessment(enumv.DeltaAssessmentUnknown)
	b.OutcomeCharacter(enumv.OutcomeCharacterPending)
	na := "Review test-bundle health and activate this session when ready."
	if snap != nil && strings.TrimSpace(snap.NextActionHint) != "" {
		na = strings.TrimSpace(snap.NextActionHint)
	}
	b.SetNextAction(na)
	b.RelatedObjectRefs([]string{priorSessionID})

	copyKeys := []string{
		objects.FieldKeyHypothesis,
		objects.FieldKeyDesiredEndState,
		objects.FieldKeyIterationProcess,
		objects.FieldKeyFlowVariant,
		objects.FieldKeyAutomationHooks,
		objects.FieldKeyRequirementRefs,
		objects.FieldKeyGlossaryTermRef,
		objects.FieldKeyConvergenceSessionProfile,
		objects.FieldKeyBacklogItemRefs,
		objects.FieldKeyStakeholders,
		objects.FieldKeyOriginProject,
		objects.FieldKeyOriginSystem,
		objects.FieldKeySourceType,
	}
	for _, k := range copyKeys {
		if v, ok := prior[k]; ok && v != nil {
			b.SetField(k, v)
		}
	}

	// Build() applies spec defaults (timestamps, origin, etc.); ID is stripped for storage Create.
	b.SetID("REDACTED")
	obj, err := b.Build()
	if err != nil {
		out := b.ToEventMap()
		merged := make(map[string]any, len(out)+4)
		maps.Copy(merged, out)
		merged[objects.FieldKeyKind] = objects.KindConvergenceSession
		merged[objects.FieldKeySchemaVersion] = objects.DefaultSchemaVersion
		delete(merged, objects.FieldKeyID)
		return merged
	}
	delete(obj, objects.FieldKeyID)
	return obj
}

func (h *ConvergenceSessionTickHandler) maybeSpawnTerminalFollowupDraft(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	job *ScheduledJob,
	priorSessionID string,
	prior map[string]any,
	snap *TestBundleConvergenceSnapshot,
) (newSessionID string, err error) {
	if h == nil || h.storage == nil || secCtx == nil || snap == nil {
		return "", nil
	}
	wm := strings.TrimSpace(snap.HealthWatermarkRFC3339)
	if h.projectRoot == "" || wm == "" {
		return "", nil
	}
	markerPath := terminalFollowupSpawnMarkerFile(h.projectRoot, priorSessionID, wm)
	if existing, rerr := readTerminalFollowupMarker(markerPath); rerr == nil && existing != nil && strings.TrimSpace(existing.NewSessionID) != "" {
		return existing.NewSessionID, nil
	}

	draft := buildFollowupDraftConvergenceSessionObject(priorSessionID, prior, snap)
	createCtx := storagepkg.WithSyncCreateForKind(ctx, objects.KindConvergenceSession)
	if err := h.storage.Create(createCtx, secCtx, draft); err != nil {
		return "", err
	}
	// ID was generated during Create; ensureObjectID mutates draft in place.
	id, _ := draft[objects.FieldKeyID].(string)
	if id == "" {
		return "", nil
	}
	m := &terminalFollowupMarker{
		PriorSessionID:  priorSessionID,
		NewSessionID:    id,
		HealthWatermark: wm,
	}
	if job != nil {
		m.SpawnJobID = job.ID
	}
	if werr := writeTerminalFollowupMarker(markerPath, m); werr != nil {
		ConvergenceSessionTickLog(h.logger).Warn(LogEventConvergenceSessionTickFollowupSpawnMarkerFailed).
			String("prior_session_id", priorSessionID).
			String("new_session_id", id).
			WithError(werr).
			Log()
	}
	return id, nil
}
