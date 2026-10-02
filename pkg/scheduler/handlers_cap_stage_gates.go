package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/convergence"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

const (
	capStagePendingFile   = "cap_stage_pending.json"
	capStageReceiptFile   = "cap_stage_receipt.json"
	capReviewResultFile   = "cap_review_result.json"
	capHoldWakeFile       = "cap_hold_wake.json"
	capQuarantineFile     = "cap_quarantine.json"
	capQuarantineStatus   = "quarantined"
	capReviewMaxAge       = time.Hour
	capStateDefaultMaxAge = time.Hour
	capStageMaxAttempts   = 6
	// CRIT-CEF-R26-REMAINING-KINDS-001: cap_stage_grooming + planned=0 must
	// stop dispatching no-ops after N ticks (default 3), not 37 silent cycles.
	capGroomingPlannedZeroMaxTicks = 3
	capGroomingPlannedZeroFile     = "cap_grooming_planned_zero.json"
	// Quarantine bounds repeated stage work without becoming a permanent dead
	// latch. External evidence (system health, test bundles, peer delivery) can
	// recover without a process restart, so CAP opens a fresh bounded attempt
	// window after one normal scheduler interval.
	capStageQuarantineRetryAfter = 30 * time.Minute
	// Hold longer than two CAP ticks (15m cron) before re-waking TPM so stuck
	// orchestrating/grooming is visible in chat — not only in scheduler events.
	// TRACK: / F-001 CAP silent hold
	capHoldWakeAfter  = 30 * time.Minute
	capHoldWakePeriod = 30 * time.Minute
)

// capStagePending records when the current CAP stage began so delivery
// evidence can be measured against entry time (not against AGI create alone).
type capStagePending struct {
	Stage           string `json:"stage"`
	EnteredAt       string `json:"entered_at"`
	PlanID          string `json:"plan_id,omitempty"`
	CvsID           string `json:"cvs_id,omitempty"`
	FocusChildCvsID string `json:"focus_child_cvs_id,omitempty"`
	Attempts        int    `json:"attempts"`
	FailureAttempts int    `json:"failure_attempts,omitempty"`
	LastAttemptAt   string `json:"last_attempt_at"`
}

// capStateDir is the canonical CAP artifact directory (.zqk/state).
func (h *CapOrchestratorHandler) capStateDir() string {
	return filepath.Join(h.projectRoot, paths.ProjectDataDir, paths.StateDir)
}

func (h *CapOrchestratorHandler) capStatePath(name string) string {
	return filepath.Join(h.capStateDir(), name)
}

func (h *CapOrchestratorHandler) legacyCapStatePath(name string) string {
	return filepath.Join(h.projectRoot, paths.ProjectDataDir, paths.SchedulerDir, paths.StateDir, name)
}

func (h *CapOrchestratorHandler) markStageEntered(stage, planID string) {
	now := time.Now().UTC()
	pending := capStagePending{
		Stage:         stage,
		EnteredAt:     now.Format(time.RFC3339),
		PlanID:        planID,
		Attempts:      1,
		LastAttemptAt: now.Format(time.RFC3339),
	}
	existing, existingErr := h.readPendingStage()
	if existingErr == nil && existing.Stage == stage && existing.EnteredAt != "" {
		pending.EnteredAt = existing.EnteredAt
		pending.Attempts = existing.Attempts + 1
		if pending.PlanID == "" {
			pending.PlanID = existing.PlanID
		}
		pending.CvsID = existing.CvsID
		pending.FocusChildCvsID = existing.FocusChildCvsID
	} else if existingErr == nil && existing.Stage != stage {
		h.clearCAPStageQuarantine()
	}
	ctx := context.Background() // Background: request-or-shutdown derived
	_ = h.refreshPendingBoundCVS(ctx, &pending)
	if pending.CvsID == "" {
		if cvsID, focus := h.resolveBoundCVS(ctx, planID); cvsID != "" {
			pending.CvsID = cvsID
			pending.FocusChildCvsID = focus
		}
	}
	if h.capStageQuarantineRetryReady(pending, now) {
		pending.EnteredAt = now.Format(time.RFC3339)
		pending.Attempts = 1
		pending.LastAttemptAt = pending.EnteredAt
		h.clearCAPStageQuarantine()
	}
	h.writeStateFile(capStagePendingFile, pending)
}

func (h *CapOrchestratorHandler) clearGroomingPlannedZeroLatch() {
	_ = os.Remove(h.capStatePath(capGroomingPlannedZeroFile))
	_ = os.Remove(h.legacyCapStatePath(capGroomingPlannedZeroFile))
}

// groomingPlannedZeroExhausted is true after N cap_stage_grooming ticks with
// planned=0 (CRIT-CEF-R26-REMAINING-KINDS-001). Callers must skip Create+wake.
func (h *CapOrchestratorHandler) groomingPlannedZeroExhausted(planned int) (capStagePending, bool) {
	if planned > 0 {
		h.clearGroomingPlannedZeroLatch()
		return capStagePending{}, false
	}
	pending, err := h.readPendingStage()
	if err != nil || pending.Stage != "cap_stage_grooming" {
		return capStagePending{}, false
	}
	if pending.Attempts <= capGroomingPlannedZeroMaxTicks {
		return pending, false
	}
	return pending, true
}

func capStageAttemptExhausted(pending capStagePending) bool {
	return pending.FailureAttempts > capStageMaxAttempts
}

func (h *CapOrchestratorHandler) quarantineCAPStage(pending capStagePending) {
	h.writeStateFile(capQuarantineFile, map[string]any{
		"stage":                           pending.Stage,
		capFieldPlanID:                    pending.PlanID,
		"cvs_id":                          pending.CvsID,
		"attempts":                        pending.Attempts,
		"failure_attempts":                pending.FailureAttempts,
		objects.FieldKeyReason:            "CAP stage attempt ceiling exhausted",
		objects.FieldKeyCreatedAt:         time.Now().UTC().Format(time.RFC3339),
		objects.FieldKeyStatus:            capQuarantineStatus,
		objects.FieldKeyRelatedObjectRefs: []string{pending.PlanID},
	})
}

func (h *CapOrchestratorHandler) readQuarantineMatch(pending capStagePending) (quarantine map[string]any, fileFound bool, matches bool) {
	raw, err := h.readStateFile(capQuarantineFile)
	if err != nil {
		return nil, false, false
	}
	q, ok := raw.(map[string]any)
	if !ok {
		return nil, true, false
	}
	stage, _ := q["stage"].(string)
	planID, _ := q[capFieldPlanID].(string)
	cvsID, _ := q["cvs_id"].(string)
	return q, true, stage == pending.Stage && planID == pending.PlanID && cvsID == pending.CvsID
}

// capStageQuarantineRetryReady reports whether a quarantined stage may begin a
// fresh bounded attempt window. Scope changes reset immediately; otherwise the
// retry window opens after capStageQuarantineRetryAfter. Malformed legacy state
// is reset rather than wedging CAP forever.
func (h *CapOrchestratorHandler) capStageQuarantineRetryReady(pending capStagePending, now time.Time) bool {
	quarantine, fileFound, matches := h.readQuarantineMatch(pending)
	if !fileFound {
		return false
	}
	if !matches {
		return true
	}
	createdAt, _ := quarantine[objects.FieldKeyCreatedAt].(string)
	created, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return true
	}
	return !now.Before(created.Add(capStageQuarantineRetryAfter))
}

func (h *CapOrchestratorHandler) clearCAPStageQuarantine() {
	_ = fileutil.RemoveFile(h.capStatePath(capQuarantineFile))
	_ = fileutil.RemoveFile(h.legacyCapStatePath(capQuarantineFile))
}

func (h *CapOrchestratorHandler) capStageQuarantineActive(pending capStagePending) bool {
	_, fileFound, matches := h.readQuarantineMatch(pending)
	return fileFound && matches
}

// clearCAPStageFailureAttempts keeps the stage entry watermark but resets the
// hard-failure streak after the handler succeeds. Waiting for asynchronous
// delivery or bundle evidence is a healthy hold, not another failed attempt.
func (h *CapOrchestratorHandler) clearCAPStageFailureAttempts(stage string) {
	pending, err := h.readPendingStage()
	if err != nil || pending.Stage != stage || pending.FailureAttempts == 0 {
		return
	}
	pending.FailureAttempts = 0
	h.writeStateFile(capStagePendingFile, pending)
}

func (h *CapOrchestratorHandler) recordCAPStageFailureAttempt(stage string) {
	pending, err := h.readPendingStage()
	if err != nil || pending.Stage != stage {
		return
	}
	pending.FailureAttempts++
	h.writeStateFile(capStagePendingFile, pending)
}

func (h *CapOrchestratorHandler) readPendingStage() (capStagePending, error) {
	raw, err := h.readStateFile(capStagePendingFile)
	if err != nil {
		return capStagePending{}, err
	}
	switch v := raw.(type) {
	case map[string]any:
		b, _ := json.Marshal(v)
		var p capStagePending
		if err := json.Unmarshal(b, &p); err != nil {
			return capStagePending{}, err
		}
		return p, nil
	case string:
		var p capStagePending
		if err := json.Unmarshal([]byte(v), &p); err != nil {
			return capStagePending{}, err
		}
		return p, nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return capStagePending{}, err
		}
		var p capStagePending
		if err := json.Unmarshal(b, &p); err != nil {
			return capStagePending{}, err
		}
		return p, nil
	}
}

// cvsStatusEligibleForCAP is true while the session can still receive CAP advances.
// Option A (DEC / ): escalated stays CAP-bound.
func cvsStatusEligibleForCAP(status string) bool {
	return convergence.SessionStatusEligibleForCAP(status)
}

// liveCVSOrEmpty returns cvsID if the object exists and is active/paused; else "".
func (h *CapOrchestratorHandler) liveCVSOrEmpty(ctx context.Context, cvsID string) string {
	cvsID = strings.TrimSpace(cvsID)
	if cvsID == "" || h.storage == nil {
		return ""
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	obj, err := h.storage.Read(ctx, secCtx, cvsID)
	if err != nil || obj == nil {
		return ""
	}
	st, _ := obj[objects.FieldKeyStatus].(string)
	if !cvsStatusEligibleForCAP(st) {
		return ""
	}
	return cvsID
}

// resolveBoundCVS finds parent CVS for the plan (CRIT-CAPH-001 / CRIT-CAPH-012).
// Never returns a terminal/completed CVS — plan related_object_refs are status-checked.
func (h *CapOrchestratorHandler) resolveBoundCVS(ctx context.Context, planID string) (cvsID, focusChild string) {
	secCtx := pkgctx.NewSystemSecurityContext()

	storageCtx := pkgctx.NewStorageContext()
	if planID != "" {
		if obj, err := h.storage.Read(ctx, secCtx, planID); err == nil {
			for _, ref := range relatedStringRefs(obj[objects.FieldKeyRelatedObjectRefs]) {
				if strings.HasPrefix(ref, "CVS-") && h.liveCVSOrEmpty(ctx, ref) != "" {
					return ref, ""
				}
			}
		}
	}
	res, err := h.storage.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{Kind: objects.KindConvergenceSession})
	if err != nil || res == nil {
		return "", ""
	}
	var active []string
	for _, obj := range res.Objects {
		st, _ := obj[objects.FieldKeyStatus].(string)
		id, _ := obj[objects.FieldKeyID].(string)
		if id == "" {
			continue
		}
		if !cvsStatusEligibleForCAP(st) {
			continue
		}
		if planID != "" {
			for _, ref := range relatedStringRefs(obj[objects.FieldKeyRelatedObjectRefs]) {
				if ref == planID {
					return id, ""
				}
			}
		}
		active = append(active, id)
	}
	if len(active) == 1 {
		return active[0], ""
	}
	return "", ""
}

// refreshPendingBoundCVS rebinds pending.CvsID when the stored id is missing or terminal
// (completed/abandoned/…); otherwise CAP forever journals against a dead session.
// TRACK: CAP ops / stale pending CVS.
func (h *CapOrchestratorHandler) refreshPendingBoundCVS(ctx context.Context, pending *capStagePending) (rewrote bool) {
	if pending == nil {
		return false
	}
	planID := pending.PlanID
	if h.liveCVSOrEmpty(ctx, pending.CvsID) != "" {
		return false
	}
	cvsID, focus := h.resolveBoundCVS(ctx, planID)
	if cvsID == "" {
		if pending.CvsID != "" {
			pending.CvsID = ""
			pending.FocusChildCvsID = ""
			return true
		}
		return false
	}
	if pending.CvsID == cvsID && pending.FocusChildCvsID == focus {
		return false
	}
	pending.CvsID = cvsID
	pending.FocusChildCvsID = focus
	return true
}

func relatedStringRefs(raw any) []string {
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func (h *CapOrchestratorHandler) stageDeliveryComplete(ctx context.Context, stage string) (bool, string) {
	switch stage {
	case "cap_stage_grooming":
		return h.groomingDeliveryComplete(ctx)
	case "cap_stage_metrics":
		return h.stateFileFresh(capMetricsLatestFile, ""), "metrics snapshot missing or stale (>1h)"
	case "cap_stage_review":
		ok, detail := h.reviewDeliveryComplete()
		return ok, detail
	case "cap_stage_self_improvement":
		return h.stateFileExists("cap_self_improvement_result.json"), "self-improvement report missing"
	case "cap_stage_sentinel":
		return h.stateFileNonEmpty("cap_sentinel_directive.txt"), "sentinel directive missing or empty"
	case "cap_stage_planning", "cap_stage_design", "cap_stage_orchestrating":
		return h.dispatchDeliveryComplete(ctx, stage)
	default:
		if ok, detail := h.verifiedStageReceipt(ctx, stage); ok {
			return true, ""
		} else if detail != "" {
			return false, detail
		}
		return false, fmt.Sprintf("no delivery evidence for %s", stage)
	}
}

const capMetricsLatestFile = "cap_metrics_latest.json"

func (h *CapOrchestratorHandler) reviewDeliveryComplete() (bool, string) {
	raw, err := h.readStateFile(capReviewResultFile)
	if err != nil {
		return false, "review result missing"
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return false, "review result not an object"
	}
	if reason := reviewResultStaleReason(m, time.Now().UTC(), capReviewMaxAge); reason != "" {
		return false, reason
	}
	if b, _ := m["system_check"].(bool); !b {
		return false, "review system_check not true"
	}
	if b, _ := m["scheduler_health"].(bool); !b {
		return false, "review scheduler_health not true"
	}
	if b, _ := m["tests_passed"].(bool); !b {
		return false, "review tests_passed not true"
	}
	return true, ""
}

func reviewResultStaleReason(m map[string]any, now time.Time, maxAge time.Duration) string {
	ts, _ := m["timestamp"].(string)
	if strings.TrimSpace(ts) == "" {
		return "review result missing timestamp"
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return "review timestamp unparseable"
	}
	if now.Sub(t) > maxAge {
		return fmt.Sprintf("review result stale (timestamp=%s max_age=%s)", ts, maxAge)
	}
	return ""
}

func (h *CapOrchestratorHandler) groomingDeliveryComplete(ctx context.Context) (bool, string) {
	if ok, detail := h.verifiedStageReceipt(ctx, "cap_stage_grooming"); ok {
		return true, ""
	} else if detail != "" && strings.Contains(detail, "unverified") {
		// Prior-cycle receipt still labeled grooming with stale artifact_ids must not
		// permanently hold the stage — fall through to fresh plan/BLI mutations.
		h.logger.Info("cap_grooming_stale_receipt_ignored", logging.String("reason", detail))
	} else if detail != "" && !strings.Contains(detail, "missing") {
		return false, detail
	}
	pending, err := h.readPendingStage()
	if err != nil || pending.Stage != "cap_stage_grooming" || pending.EnteredAt == "" {
		return false, "grooming pending watermark missing"
	}
	entered, perr := time.Parse(time.RFC3339, pending.EnteredAt)
	if perr != nil {
		return false, "grooming pending entered_at unparseable"
	}

	ids, found := h.listGroomingArtifactsSince(ctx, entered)
	if !found {
		return false, "grooming incomplete: no priority_plan/backlog_item artifacts since stage entry (TPM must deliver graph work; AGI create alone does not advance)"
	}
	h.writeStateFile(capStageReceiptFile, map[string]any{
		"stage":                     "cap_stage_grooming",
		objects.FieldKeyCompletedAt: time.Now().UTC().Format(time.RFC3339),
		"artifact_ids":              ids,
		objects.FieldKeySource:      "graph_mutation",
	})
	return true, ""
}

func (h *CapOrchestratorHandler) listGroomingArtifactsSince(ctx context.Context, since time.Time) ([]string, bool) {
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	var ids []string
	store := h.capEvidenceStorage()

	for _, kind := range []string{objects.KindPriorityPlan, objects.KindBacklogItem, objects.KindStrategicPlan} {
		res, err := store.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{Kind: kind})
		if err != nil || res == nil {
			continue
		}
		for _, obj := range res.Objects {
			if !objectTouchedSince(obj, since) {
				continue
			}
			id, _ := obj[objects.FieldKeyID].(string)
			if id == "" {
				continue
			}
			ids = append(ids, id)
		}
	}
	return ids, len(ids) > 0
}

func objectTouchedSince(obj map[string]any, since time.Time) bool {
	for _, key := range []string{objects.FieldKeyUpdatedAt, objects.FieldKeyCreatedAt} {
		raw, _ := obj[key].(string)
		if raw == "" {
			continue
		}
		ts, err := parseFlexibleTime(raw)
		if err != nil {
			continue
		}
		if !ts.Before(since) {
			return true
		}
	}
	return false
}

func parseFlexibleTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unparseable time %q", raw)
}

func (h *CapOrchestratorHandler) dispatchDeliveryComplete(ctx context.Context, stage string) (bool, string) {
	if ok, detail := h.verifiedStageReceipt(ctx, stage); ok {
		return true, ""
	} else if detail != "" && strings.Contains(detail, "unverified") {
		return false, detail
	}
	pending, _ := h.readPendingStage()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	// prefer CAS file evidence for CAP gates; graph List can lag /
	// omit status so design/orchestrating never advances despite implemented ATKs on disk.
	store := h.capEvidenceStorage()
	res, err := store.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{Kind: objects.KindAgentTask})
	if err != nil || res == nil {
		return false, "unable to list agent_task for dispatch evidence"
	}
	var since time.Time
	if pending.Stage == stage && pending.EnteredAt != "" {
		if t, err := time.Parse(time.RFC3339, pending.EnteredAt); err == nil {
			since = t
		}
	}
	// CRIT-CAPH-003: require terminal ATK outcomes or BLI advances — not open ATKs alone.
	// agent_task terminal is `implemented` (not `completed`); see objects lifecycle tests.
	completedFresh := 0
	for _, obj := range res.Objects {
		st, _ := obj[objects.FieldKeyStatus].(string)
		if !agentTaskDeliveryTerminal(st) {
			continue
		}
		if since.IsZero() || objectTouchedSince(obj, since) {
			completedFresh++
		}
	}
	if completedFresh > 0 {
		return true, ""
	}
	return false, fmt.Sprintf("%s incomplete: no implemented/terminal agent_task since stage entry (open ATK or in-progress backlog alone is insufficient)", stage)
}

// agentTaskDeliveryTerminal reports whether an agent_task status counts as dispatch delivery.
func agentTaskDeliveryTerminal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case objects.ObjectStatusImplemented, objects.ObjectStatusCompleted, objects.ObjectStatusComplete, "verified", "done":
		return true
	default:
		return false
	}
}

// capEvidenceStorage returns CAS file storage for stage-delivery scans when available.
// Scheduler handlers often hold a graph-backed provider whose List omits or lags status.
// Prefer the handler's existing file storage; otherwise open with SkipGlobalWiring so
// evidence scans do not OpenFile(object.wal) / start another write-behind (FD leak under
// CompactWAL rename).
func (h *CapOrchestratorHandler) capEvidenceStorage() storagepkg.ObjectStorageProvider {
	if h == nil {
		return nil
	}
	if fs := storagepkg.UnwrapToFileObjectStorage(h.storage); fs != nil {
		return fs
	}
	if h.storage != nil {
		return h.storage
	}
	if h.projectRoot != "" {
		if fs, err := storagepkg.NewFileObjectStorage(h.projectRoot, &storagepkg.FileObjectStorageOptions{SkipGlobalWiring: true}); err == nil && fs != nil {
			return fs
		}
	}
	return nil
}

// verifiedStageReceipt accepts receipts only when artifact_ids Get and were touched since stage entry.
func (h *CapOrchestratorHandler) verifiedStageReceipt(ctx context.Context, stage string) (bool, string) {
	raw, err := h.readStateFile(capStageReceiptFile)
	if err != nil {
		return false, ""
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return false, "receipt not an object"
	}
	got, _ := m["stage"].(string)
	if got != stage {
		return false, ""
	}
	arts, ok := m["artifact_ids"].([]any)
	if !ok || len(arts) == 0 {
		return false, "receipt unverified: artifact_ids missing or empty"
	}
	pending, _ := h.readPendingStage()
	var since time.Time
	if pending.Stage == stage && pending.EnteredAt != "" {
		if t, err := time.Parse(time.RFC3339, pending.EnteredAt); err == nil {
			since = t
		}
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	for _, a := range arts {
		id, _ := a.(string)
		if strings.TrimSpace(id) == "" {
			return false, "receipt unverified: empty artifact id"
		}
		obj, err := h.storage.Read(ctx, secCtx, id)
		if err != nil {
			return false, fmt.Sprintf("receipt unverified: artifact %s not readable", id)
		}
		if !since.IsZero() && !objectTouchedSince(obj, since) {
			return false, fmt.Sprintf("receipt unverified: artifact %s not touched since stage entry", id)
		}
	}
	return true, ""
}

func (h *CapOrchestratorHandler) stateFileExists(name string) bool {
	_, err := h.readStateFile(name)
	return err == nil
}

func (h *CapOrchestratorHandler) stateFileNonEmpty(name string) bool {
	raw, err := h.readStateFile(name)
	if err != nil {
		return false
	}
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v) != ""
	default:
		return true
	}
}

func (h *CapOrchestratorHandler) stateFileFresh(name, maxAgeHint string) bool {
	maxAge := capStateDefaultMaxAge
	if strings.TrimSpace(maxAgeHint) != "" {
		if d, err := time.ParseDuration(maxAgeHint); err == nil && d > 0 {
			maxAge = d
		}
	}
	path := h.capStatePath(name)
	fi, err := fileutil.Stat(path)
	if err != nil {
		if !h.stateFileExists(name) {
			return false
		}
		legacy := h.legacyCapStatePath(name)
		fi, err = fileutil.Stat(legacy)
		if err != nil {
			return false
		}
	}
	return time.Since(fi.ModTime()) <= maxAge
}

func (h *CapOrchestratorHandler) readPendingAndResolveBoundCVS(ctx context.Context) (capStagePending, string, string) {
	pending, _ := h.readPendingStage()
	if h.refreshPendingBoundCVS(ctx, &pending) {
		h.writeStateFile(capStagePendingFile, pending)
	}
	cvsID := pending.CvsID
	focus := pending.FocusChildCvsID
	if cvsID == "" {
		cvsID, focus = h.resolveBoundCVS(ctx, pending.PlanID)
	}
	return pending, cvsID, focus
}

// maybeAdvanceCAPStage advances only when bound CVS + delivery evidence exist.
func (h *CapOrchestratorHandler) maybeAdvanceCAPStage(ctx context.Context, stage string) error {
	if !strings.HasPrefix(stage, "cap_stage_") {
		return nil
	}
	if whatsnext.CapCycleTamperDetected(h.projectRoot) {
		restored, ok := whatsnext.RestoreCAPStageFromJournal(h.projectRoot)
		h.logger.Warn("cap_cycle_tamper_detected",
			logging.String("restored", restored),
			logging.Bool("ok", ok))
		return fmt.Errorf("stage held: cap_cycle tamper detected (restored=%v)", restored)
	}
	_, cvsID, focus := h.readPendingAndResolveBoundCVS(ctx)
	if cvsID == "" {
		h.logger.Info("cap_stage_held_unbound_cvs", logging.StageField(stage))
		h.maybeWakeOnStageHold(stage, "unbound parent convergence_session")
		return fmt.Errorf("stage held: unbound parent convergence_session")
	}
	ok, detail := h.stageDeliveryComplete(ctx, stage)
	if !ok {
		h.logger.Info("cap_stage_held_awaiting_delivery",
			logging.StageField(stage),
			logging.String("cvs_id", cvsID),
			logging.String("reason", detail))
		h.maybeWakeOnStageHold(stage, detail)
		return fmt.Errorf("stage held: %s", detail)
	}
	var arts []string
	if raw, err := h.readStateFile(capStageReceiptFile); err == nil {
		if m, ok := raw.(map[string]any); ok {
			if ids, ok := m["artifact_ids"].([]any); ok {
				for _, a := range ids {
					if s, ok := a.(string); ok {
						arts = append(arts, s)
					}
				}
			}
		}
	}
	next := whatsnext.AdvanceCAPStageFromOrchestrator(h.projectRoot, stage, cvsID, focus, arts)
	h.logger.Info("cap_stage_advanced",
		logging.String("completed", stage),
		logging.String("next", next),
		logging.String("cvs_id", cvsID))
	return nil
}

type capHoldWakeState struct {
	Stage      string `json:"stage"`
	LastWakeAt string `json:"last_wake_at"`
	Reason     string `json:"reason,omitempty"`
}

// maybeWakeOnStageHold re-wakes TPM when CAP ticks succeed but the stage gate
// holds for longer than capHoldWakeAfter (rate-limited by capHoldWakePeriod).
// Without this, orchestrating reuses open AGIs and never interrupts chat — the
// 15m job looks "dead" even though it is completing successfully.
// TRACK: follow-up in kernel backlog
func (h *CapOrchestratorHandler) maybeWakeOnStageHold(stage, reason string) {
	if h == nil || h.projectRoot == "" {
		return
	}
	pending, err := h.readPendingStage()
	if err != nil || pending.Stage != stage || pending.EnteredAt == "" {
		return
	}
	entered, err := time.Parse(time.RFC3339, pending.EnteredAt)
	if err != nil {
		return
	}
	heldFor := time.Since(entered)
	if heldFor < capHoldWakeAfter {
		return
	}
	now := time.Now().UTC()
	if raw, err := h.readStateFile(capHoldWakeFile); err == nil {
		if m, ok := raw.(map[string]any); ok {
			if lastStage, _ := m["stage"].(string); lastStage == stage {
				if lastAt, _ := m["last_wake_at"].(string); lastAt != "" {
					if t, err := time.Parse(time.RFC3339, lastAt); err == nil && now.Sub(t) < capHoldWakePeriod {
						return
					}
				}
			}
		}
	}
	planID := pending.PlanID
	if planID == "" {
		planID = stage
	}
	msg := fmt.Sprintf(
		"CAP stage HOLD (%s) for %s — %s. Job %s is ticking; delivery evidence missing. Advance ATKs to implemented or write cap_stage_receipt.json.",
		stage, heldFor.Round(time.Minute), reason, CapOrchestratorJobID,
	)
	h.logger.Info("cap_stage_hold_wake",
		logging.StageField(stage),
		logging.PlanIDField(planID),
		logging.String("held_for", heldFor.String()),
		logging.String("reason", reason))
	// Reuse TPM wake path (chat channel + primaryorch adapters).
	h.wakeAgentAndScheduleHourglass(planID, "tpm")
	// Append hold-specific detail so operators see the gate reason, not only assignment text.
	eventPath := datacell.AgentChatChannelEventsJSONLPath(h.projectRoot)
	if err := fileutil.MkdirAll(filepath.Dir(eventPath), paths.DirPerm755); err == nil {
		if f, err := fileutil.OpenFile(eventPath, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm644); err == nil {
			_ = json.NewEncoder(f).Encode(map[string]any{
				"timestamp": now.Format(time.RFC3339),
				"sender":    "cap_orchestrator",
				"message":   msg,
				"stage":     stage,
				"plan_id":   planID,
			})
			_ = f.Close()
		}
	}
	h.writeStateFile(capHoldWakeFile, capHoldWakeState{
		Stage:      stage,
		LastWakeAt: now.Format(time.RFC3339),
		Reason:     reason,
	})
}

func (h *CapOrchestratorHandler) writeStateFile(name string, data any) {
	stateDir := h.capStateDir()
	_ = fileutil.EnsureDir(stateDir)
	filePath := filepath.Join(stateDir, name)

	var dataBytes []byte
	if str, ok := data.(string); ok {
		dataBytes = []byte(str)
	} else {
		var err error
		dataBytes, err = json.MarshalIndent(data, "", "  ")
		if err != nil {
			h.logger.Error("cap_state_serialize_failed", err, logging.FileField(name))
			return
		}
	}

	if err := fileutil.WriteSecureFile(filePath, dataBytes); err != nil {
		h.logger.Error("cap_state_write_failed", err, logging.FileField(name))
	}
}

func (h *CapOrchestratorHandler) readStateFile(name string) (any, error) {
	for _, filePath := range []string{h.capStatePath(name), h.legacyCapStatePath(name)} {
		dataBytes, err := fileutil.ReadFile(filePath)
		if err != nil {
			continue
		}
		var parsed any
		if err := json.Unmarshal(dataBytes, &parsed); err != nil {
			return string(dataBytes), nil
		}
		return parsed, nil
	}
	return nil, fmt.Errorf("cap state file %s not found", name)
}
