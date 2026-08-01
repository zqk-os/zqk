package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/workflow/whatsnext"
)

const (
	capStagePendingFile   = "cap_stage_pending.json"
	capStageReceiptFile   = "cap_stage_receipt.json"
	capReviewResultFile   = "cap_review_result.json"
	capReviewMaxAge       = time.Hour
	capStateDefaultMaxAge = time.Hour
)

// capStagePending records when the current CAP stage began so delivery
// evidence can be measured against entry time (not against AGI create alone).
type capStagePending struct {
	Stage           string `json:"stage"`
	EnteredAt       string `json:"entered_at"`
	PlanID          string `json:"plan_id,omitempty"`
	CvsID           string `json:"cvs_id,omitempty"`
	FocusChildCvsID string `json:"focus_child_cvs_id,omitempty"`
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
	pending := capStagePending{
		Stage:     stage,
		EnteredAt: time.Now().UTC().Format(time.RFC3339),
		PlanID:    planID,
	}
	if existing, err := h.readPendingStage(); err == nil && existing.Stage == stage && existing.EnteredAt != "" {
		pending.EnteredAt = existing.EnteredAt
		if pending.PlanID == "" {
			pending.PlanID = existing.PlanID
		}
		pending.CvsID = existing.CvsID
		pending.FocusChildCvsID = existing.FocusChildCvsID
	}
	if pending.CvsID == "" {
		if cvsID, focus := h.resolveBoundCVS(context.Background(), planID); cvsID != "" {
			pending.CvsID = cvsID
			pending.FocusChildCvsID = focus
		}
	}
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

// resolveBoundCVS finds parent CVS for the plan (CRIT-CAPH-001 / CRIT-CAPH-012).
func (h *CapOrchestratorHandler) resolveBoundCVS(ctx context.Context, planID string) (cvsID, focusChild string) {
	if pending, err := h.readPendingStage(); err == nil {
		if pending.CvsID != "" {
			return pending.CvsID, pending.FocusChildCvsID
		}
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	if planID != "" {
		if obj, err := h.storage.Read(ctx, secCtx, planID); err == nil {
			for _, ref := range relatedStringRefs(obj[objects.FieldKeyRelatedObjectRefs]) {
				if strings.HasPrefix(ref, "CONV-") {
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
		ls := strings.ToLower(st)
		if ls != "active" && ls != "paused" {
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

	for _, kind := range []string{objects.KindPriorityPlan, objects.KindBacklogItem, objects.KindStrategicPlan} {
		res, err := h.storage.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{Kind: kind})
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
	res, err := h.storage.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{Kind: objects.KindAgentTask})
	if err != nil || res == nil {
		return false, "unable to list agent_task for dispatch evidence"
	}
	var since time.Time
	if pending.Stage == stage && pending.EnteredAt != "" {
		if t, err := time.Parse(time.RFC3339, pending.EnteredAt); err == nil {
			since = t
		}
	}
	// CRIT-CAPH-003: require completed/verified outcomes or BLI advances — not open ATKs alone.
	completedFresh := 0
	for _, obj := range res.Objects {
		st, _ := obj[objects.FieldKeyStatus].(string)
		ls := strings.ToLower(st)
		if ls != "completed" && ls != "complete" && ls != "verified" && ls != "done" {
			continue
		}
		if since.IsZero() || objectTouchedSince(obj, since) {
			completedFresh++
		}
	}
	if completedFresh > 0 {
		return true, ""
	}
	bliRes, err := h.storage.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{Kind: objects.KindBacklogItem})
	if err == nil && bliRes != nil && !since.IsZero() {
		advanced := 0
		for _, obj := range bliRes.Objects {
			if !objectTouchedSince(obj, since) {
				continue
			}
			st, _ := obj[objects.FieldKeyStatus].(string)
			ls := strings.ToLower(st)
			if ls == objects.ObjectStatusInProgress || ls == "complete" || ls == "completed" || ls == "verifying" {
				advanced++
			}
		}
		if advanced > 0 {
			return true, ""
		}
	}
	return false, fmt.Sprintf("%s incomplete: no completed agent_task or advanced backlog_item since stage entry (open ATK alone is insufficient)", stage)
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
	fi, err := os.Stat(path)
	if err != nil {
		if !h.stateFileExists(name) {
			return false
		}
		legacy := h.legacyCapStatePath(name)
		fi, err = os.Stat(legacy)
		if err != nil {
			return false
		}
	}
	return time.Since(fi.ModTime()) <= maxAge
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
	pending, _ := h.readPendingStage()
	cvsID := pending.CvsID
	focus := pending.FocusChildCvsID
	if cvsID == "" {
		cvsID, focus = h.resolveBoundCVS(ctx, pending.PlanID)
	}
	if cvsID == "" {
		h.logger.Info("cap_stage_held_unbound_cvs", logging.String("stage", stage))
		return fmt.Errorf("stage held: unbound parent convergence_session")
	}
	ok, detail := h.stageDeliveryComplete(ctx, stage)
	if !ok {
		h.logger.Info("cap_stage_held_awaiting_delivery",
			logging.String("stage", stage),
			logging.String("cvs_id", cvsID),
			logging.String("reason", detail))
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

func (h *CapOrchestratorHandler) writeStateFile(name string, data any) {
	stateDir := h.capStateDir()
	fileutil.EnsureDir(stateDir)
	filePath := filepath.Join(stateDir, name)

	var dataBytes []byte
	if str, ok := data.(string); ok {
		dataBytes = []byte(str)
	} else {
		var err error
		dataBytes, err = json.MarshalIndent(data, "", "  ")
		if err != nil {
			h.logger.Error("cap_state_serialize_failed", err, logging.String("file", name))
			return
		}
	}

	if err := fileutil.WriteSecureFile(filePath, dataBytes); err != nil {
		h.logger.Error("cap_state_write_failed", err, logging.String("file", name))
	}
}

func (h *CapOrchestratorHandler) readStateFile(name string) (any, error) {
	for _, filePath := range []string{h.capStatePath(name), h.legacyCapStatePath(name)} {
		dataBytes, err := os.ReadFile(filePath)
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
