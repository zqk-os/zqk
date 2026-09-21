package scheduler

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/zqk-os/zqk/pkg/agentclaim"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/primaryorch"
	riskblockerenum "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/risk_blocker"
	"github.com/zqk-os/zqk/pkg/specbuilder/bldr_instance_v1"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// missedDeadlineEscalationTitlePrefix is the stable title prefix for hourglass
// deadline escalations (used for dedupe and ops grepping).
const missedDeadlineEscalationTitlePrefix = "Missed Deadline Escalation: "

// expiredTimerAction is what an expired hourglass file warrants.
//
// Naming the fallback is the point: the watcher's default for an unrecognized type is to
// SIGKILL the recorded pid, so any new timer type that forgets to register here becomes a
// process killer. Routing decisions live in one pure function so that is testable.
type expiredTimerAction int

const (
	// actionKillStuckProcess terminates the recorded pid and errors the task. This is the
	// fallback for unrecognized types, which is why new types must be added here.
	actionKillStuckProcess expiredTimerAction = iota
	// actionEscalateDeadline files a risk_blocker and defers the object.
	actionEscalateDeadline
	// actionWakeOrchestrator asks a human-or-orchestrator to triage, changing nothing.
	actionWakeOrchestrator
)

// timerTypeDeadline is the dispatch-deadline timer written by the CAP orchestrator. It is
// a timer type, not an object field, so it is a domain constant rather than a FieldKey.
const timerTypeDeadline = "deadline"

func actionForExpiredTimer(timerType string) expiredTimerAction {
	switch timerType {
	case agentclaim.TimerTypeCheckin:
		return actionWakeOrchestrator
	case timerTypeDeadline:
		return actionEscalateDeadline
	default:
		return actionKillStuckProcess
	}
}

// startHourglassWatcher starts a background goroutine to clean up expired subagent hourglass timers.
func (s *Scheduler) startHourglassWatcher(ctx context.Context, bud *goroutinelabels.Budget) {
	if s.projectRoot == "" {
		return
	}
	watcherBuilder := goroutinelabels.NewGoroutine("scheduler_hourglass_watcher", "watching subagent hourglass timers")
	if bud != nil {
		watcherBuilder = watcherBuilder.WithBudget(bud)
	}
	watcherBuilder.StartSimple(func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		sweepTicker := time.NewTicker(2 * time.Minute)
		defer sweepTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.checkHourglassTimers(ctx)
			case <-sweepTicker.C:
				s.sweepStaleAgentTasks(ctx)
			}
		}
	})
}

// checkHourglassTimers scans the hourglass directory and kills/errors any expired tasks.
func (s *Scheduler) checkHourglassTimers(ctx context.Context) {
	schedulerRoot := paths.ResolvePathFromCacheOrConstant(s.projectRoot, "scheduler", filepath.Join(paths.ProjectDataDir, paths.SchedulerDir))
	hourglassDir := filepath.Join(schedulerRoot, "hourglass")
	files, err := fileutil.ReadDir(hourglassDir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return
		}
		SchedulerDaemonLog(s.logger).Error("Failed to read hourglass directory", err).Log()
		return
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		filePath := filepath.Join(hourglassDir, f.Name())
		data, err := fileutil.ReadFile(filePath)
		if err != nil {
			continue
		}
		var info struct {
			TaskID    string `json:"task_id"`
			PID       int    `json:"pid"`
			ExpiresAt string `json:"expires_at"`
			Type      string `json:"type,omitempty"` // e.g., "deadline"
			Kind      string `json:"kind,omitempty"`
		}
		if err := json.Unmarshal(data, &info); err != nil {
			continue
		}
		expiresAt, err := time.Parse(time.RFC3339, info.ExpiresAt)
		if err != nil {
			continue
		}

		if time.Now().After(expiresAt) {
			if actionForExpiredTimer(info.Type) == actionWakeOrchestrator {
				// A silent claim is not a stuck process: the seat may be gone, or merely
				// not reporting. Killing a pid or moving the task would pre-empt the
				// triage this wake exists to request.
				s.handleMissedCheckin(ctx, secCtx, filePath)
				continue
			}
			if info.Type == timerTypeDeadline {
				SchedulerDaemonLog(s.logger).Warn("Hourglass expired for deadline, executing escalation").
					String("object_id", info.TaskID).
					String("kind", info.Kind).
					Log()

				taskObj, err := s.storage.Read(ctx, secCtx, info.TaskID)
				if err == nil {
					status, _ := taskObj[objects.FieldKeyStatus].(string)
					if status == "planned" || status == "in_progress" || status == "active" || status == "not_started" || status == "draft" {
						title, _ := taskObj[objects.FieldKeyTitle].(string)
						if len(title) > 50 {
							title = title[:50]
						}
						s.escalateMissedDeadline(ctx, secCtx, info.TaskID, info.Kind, title)
					}
				}
				_ = fileutil.Remove(filePath)
			} else {
				SchedulerDaemonLog(s.logger).Warn("Hourglass expired for task, terminating stuck sync-loop process").
					String("task_id", info.TaskID).
					Int("pid", info.PID).
					Log()

				// 1. Terminate the process
				if info.PID > 0 && info.PID != os.Getpid() {
					// Send SIGKILL to terminate the subagent process immediately
					_ = syscall.Kill(info.PID, syscall.SIGKILL)
				}

				// 2. Update task status in database to error
				taskObj, err := s.storage.Read(ctx, secCtx, info.TaskID)
				if err == nil {
					var updatedSteps []any
					if stepsRaw, ok := taskObj[objects.FieldKeyTaskSteps]; ok {
						if stepsList, ok2 := stepsRaw.([]any); ok2 {
							for _, stepRaw := range stepsList {
								if stepMap, ok3 := stepRaw.(map[string]any); ok3 {
									st, _ := stepMap[objects.FieldKeyStatus].(string)
									if st == "pending_verification" || st == "pending_implementation" || st == "pending" {
										stepMap[objects.FieldKeyStatus] = "error"
										stepMap[objects.FieldKeyVerificationFeedback] = "Hourglass timer expired (process died or hung)"
									}
									updatedSteps = append(updatedSteps, stepMap)
								}
							}
						}
					}
					updates := map[string]any{
						objects.FieldKeyStatus: objects.ObjectStatusError,
					}
					if len(updatedSteps) > 0 {
						updates[objects.FieldKeyTaskSteps] = updatedSteps
					}
					_ = s.storage.Update(ctx, secCtx, info.TaskID, updates)
				}

				// 3. Publish process_errored event to the CoordinationChannel
				if s.coordinationChannel != nil {
					_ = s.coordinationChannel.PublishEvent(Event{
						Type:      "process_errored",
						JobID:     info.TaskID,
						Timestamp: time.Now().UTC(),
						Metadata: map[string]any{
							"error": "hourglass timer expired",
						},
					})
				}

				// 4. Delete the hourglass file
				_ = fileutil.Remove(filePath)
			}
		}
	}
}

// maxCheckinBackoff caps the re-arm interval so a task under triage stops waking the
// orchestrator every cadence while still being re-examined.
const maxCheckinBackoff = 4 * time.Hour

// checkinBlockerMissThreshold is how many consecutive missed windows produce a durable
// risk_blocker in addition to the wake. One miss is a nudge; repeated silence is a fact
// that should survive the orchestrator's session.
const checkinBlockerMissThreshold = 3

// handleMissedCheckin wakes the primary orchestrator about a claim that has gone silent.
//
// It deliberately does not change the task's status or signal its process. The claim
// holder may be alive and merely quiet, and the recovery decision — probe, release, or
// reassign — belongs to the orchestrator. Auto-deferring here is what makes a stalled task
// look handled while nobody has looked at it.
func (s *Scheduler) handleMissedCheckin(ctx context.Context, secCtx *pkgctx.SecurityContext, filePath string) {
	timer, err := agentclaim.LoadCheckinFile(filePath)
	if err != nil || timer == nil {
		// Unreadable timer: remove it rather than wake on a file nobody can interpret.
		_ = fileutil.Remove(filePath)
		return
	}

	// A timer that outlived its reason is noise, not signal.
	if s.storage != nil {
		task, readErr := s.storage.Read(ctx, secCtx, timer.TaskID)
		if readErr != nil {
			_ = fileutil.Remove(filePath)
			return
		}
		status, _ := task[objects.FieldKeyStatus].(string)
		switch status {
		case objects.ObjectStatusImplemented, objects.ObjectStatusArchived,
			objects.ObjectStatusError, objects.ObjectStatusDeferred:
			_ = fileutil.Remove(filePath)
			return
		}
		if holder, _ := task[objects.FieldKeyClaimedBy].(string); strings.TrimSpace(holder) == "" {
			_ = fileutil.Remove(filePath)
			return
		}
	}

	timer.Misses++
	silentFor := "unknown"
	if last := timer.LastCheckinAt; last != "" {
		if parsed, perr := time.Parse(time.RFC3339, last); perr == nil {
			silentFor = time.Since(parsed).Round(time.Minute).String()
		}
	} else if timer.ArmedAt != "" {
		if parsed, perr := time.Parse(time.RFC3339, timer.ArmedAt); perr == nil {
			silentFor = time.Since(parsed).Round(time.Minute).String()
		}
	}

	SchedulerDaemonLog(s.logger).Warn("hourglass_checkin_missed").
		String("object_id", timer.TaskID).
		String("claimed_by", timer.ClaimedBy).
		Int("misses", timer.Misses).
		String("silent_for", silentFor).
		Log()

	msg := fmt.Sprintf(
		"CLAIM SILENT: %s held by %s has missed %d check-in window(s) (cadence %s, silent %s). "+
			"Probe the holder, then release or reassign — the kernel cannot tell dropped from busy.",
		timer.TaskID, timer.ClaimedBy, timer.Misses, timer.Cadence(), silentFor)

	if res, wakeErr := primaryorch.WakePrimary(ctx, s.projectRoot, primaryorch.WakeRequest{
		TaskID:  timer.TaskID,
		Persona: "tpm",
		Message: msg,
	}); wakeErr != nil {
		SchedulerDaemonLog(s.logger).Error("hourglass_checkin_wake_failed", wakeErr).
			String("object_id", timer.TaskID).
			Log()
	} else {
		SchedulerDaemonLog(s.logger).Info("hourglass_checkin_wake").
			String("object_id", timer.TaskID).
			String("adapter", res.Adapter).
			String("delivered_to", res.DeliveredTo).
			Log()
	}

	if timer.Misses >= checkinBlockerMissThreshold {
		s.recordSilentClaimBlocker(ctx, secCtx, timer.TaskID)
	}

	if rearmErr := agentclaim.Rearm(s.projectRoot, timer, maxCheckinBackoff); rearmErr != nil {
		SchedulerDaemonLog(s.logger).Error("hourglass_checkin_rearm_failed", rearmErr).
			String("object_id", timer.TaskID).
			Log()
	}
}

// recordSilentClaimBlocker files one durable risk_blocker for a persistently silent claim,
// reusing the missed-deadline dedupe so repeated wakes do not multiply objects. Unlike
// escalateMissedDeadline it leaves the task's status alone, because the orchestrator has
// been asked to triage rather than told the work is deferred.
func (s *Scheduler) recordSilentClaimBlocker(ctx context.Context, secCtx *pkgctx.SecurityContext, taskID string) {
	if s.storage == nil || taskID == "" {
		return
	}
	if !hourglassSourcePresent(ctx, s.storage, secCtx, taskID) {
		SchedulerDaemonLog(s.logger).Info("hourglass_checkin_blocker_skipped_missing_source").
			String("object_id", taskID).
			Log()
		return
	}
	if hasOpenMissedDeadlineEscalation(ctx, s.storage, secCtx, taskID) {
		return
	}
	blocker, err := buildMissedDeadlineRiskBlocker("", taskID, objects.KindAgentTask, taskID)
	if err != nil {
		SchedulerDaemonLog(s.logger).Error("hourglass_checkin_blocker_build_failed", err).
			String("object_id", taskID).
			Log()
		return
	}
	if err := s.storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, blocker); err != nil {
		SchedulerDaemonLog(s.logger).Error("hourglass_checkin_blocker_create_failed", err).
			String("object_id", taskID).
			Log()
	}
}

// escalateMissedDeadline creates one approved risk_blocker for a missed deadline
// (skips if an open escalation already references taskID) and defers the source object.
//
// Uses WithPromoteOnCreate (same intent as `zqk new object … --promote`): the payload is
// already shovel-ready, so Create keeps status=open and writes CAS instead of parking
// on the draft plane. List/dedupe then see the escalation. TRACK
func (s *Scheduler) escalateMissedDeadline(ctx context.Context, secCtx *pkgctx.SecurityContext, taskID, kind, title string) {
	if s.storage == nil || taskID == "" {
		return
	}
	if !hourglassSourcePresent(ctx, s.storage, secCtx, taskID) {
		SchedulerDaemonLog(s.logger).Info("hourglass_deadline_escalation_skipped_missing_source").
			String("object_id", taskID).
			Log()
		return
	}
	if hasOpenMissedDeadlineEscalation(ctx, s.storage, secCtx, taskID) {
		SchedulerDaemonLog(s.logger).Info("hourglass_deadline_escalation_skipped_duplicate").
			String("object_id", taskID).
			Log()
	} else if blocker, err := buildMissedDeadlineRiskBlocker("", taskID, kind, title); err != nil {
		SchedulerDaemonLog(s.logger).Error("hourglass_deadline_escalation_build_failed", err).
			String("object_id", taskID).
			Log()
	} else if err := s.storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, blocker); err != nil {
		SchedulerDaemonLog(s.logger).Error("hourglass_deadline_escalation_create_failed", err).
			String("object_id", taskID).
			Log()
	}

	_ = s.storage.Update(ctx, secCtx, taskID, map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusDeferred,
	})
}

// buildMissedDeadlineRiskBlocker builds a lifecycle-valid risk_blocker for hourglass escalation.
// Status is open (CASable), not document-approval vocabulary or preliminary "proposed".
// Empty id allocates a RIS-{nanos}-{hex8} id (Create also accepts caller-supplied ids).
func buildMissedDeadlineRiskBlocker(id, taskID, kind, title string) (map[string]any, error) {
	if id == "" {
		id = newMissedDeadlineRiskBlockerID()
	}
	desc := "The " + kind + " " + taskID + " missed its deadline. Automated wake signal triggered."
	b := bldr_instance_v1.NewRiskBlockerInstanceBuilder(objects.DefaultSchemaVersion)
	b.ID(id).
		Title(missedDeadlineEscalationTitlePrefix + title).
		Status(riskblockerenum.StatusOpen).
		PriorityTier(riskblockerenum.PriorityTierP0).
		RiskType(riskblockerenum.RiskTypeBlocker).
		Severity(riskblockerenum.SeverityHigh).
		DetectedBy("hourglass").
		RelatedObjectRefs([]string{taskID}).
		AffectedItems([]string{taskID})
	b.SetField(objects.FieldKeyDescription, desc)
	return b.Build()
}

func newMissedDeadlineRiskBlockerID() string {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return fmt.Sprintf("RIS-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("RIS-%d-%x", time.Now().UnixNano(), suffix)
}

// hourglassSourcePresent is the Exists gate for missed-deadline / silent-claim RIS mint.
// TRACK: BLI-CEF-R26-DEADLINE-RIS-CLOSE-001 — process heal landed; mint still wrote
// GhostRefs when the ATK was already gone (2026-08-31 ).
type hourglassExister interface {
	Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error)
}

func hourglassSourcePresent(ctx context.Context, store hourglassExister, secCtx *pkgctx.SecurityContext, taskID string) bool {
	if store == nil || taskID == "" {
		return false
	}
	ok, err := store.Exists(ctx, secCtx, taskID)
	return err == nil && ok
}

// hasOpenMissedDeadlineEscalation reports whether a non-terminal risk_blocker already
// references taskID (related_object_refs or affected_items).
func hasOpenMissedDeadlineEscalation(ctx context.Context, store storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, taskID string) bool {
	if store == nil || taskID == "" {
		return false
	}
	res, err := store.List(ctx, secCtx, nil, storage.ListFilter{
		Kind: objects.KindRiskBlocker,
		Fields: []string{
			objects.FieldKeyID,
			objects.FieldKeyStatus,
			objects.FieldKeyRelatedObjectRefs,
			objects.FieldKeyAffectedItems,
			objects.FieldKeyTitle,
		},
	})
	if err != nil || res == nil {
		return false
	}
	for _, obj := range res.Objects {
		st, _ := obj[objects.FieldKeyStatus].(string)
		if st == string(riskblockerenum.StatusArchived) || st == string(riskblockerenum.StatusImplemented) {
			continue
		}
		if refsContainID(obj[objects.FieldKeyRelatedObjectRefs], taskID) ||
			refsContainID(obj[objects.FieldKeyAffectedItems], taskID) {
			return true
		}
		if title, _ := obj[objects.FieldKeyTitle].(string); strings.HasPrefix(title, missedDeadlineEscalationTitlePrefix) {
			if desc, _ := obj[objects.FieldKeyDescription].(string); strings.Contains(desc, taskID) {
				return true
			}
		}
	}
	return false
}

func refsContainID(raw any, id string) bool {
	switch v := raw.(type) {
	case []string:
		for _, s := range v {
			if s == id {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == id {
				return true
			}
		}
	}
	return false
}

// sweepStaleAgentTasks scans for in_progress agent_task objects that are orphaned (parent plan terminal)
// or timed out without ambient activity (> 2h), auto-transitioning them out of in_progress.
func (s *Scheduler) sweepStaleAgentTasks(ctx context.Context) {
	if s.storage == nil {
		return
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	res, err := s.storage.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind: objects.KindAgentTask,
		Filters: map[string]any{
			objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		},
	})
	if err != nil || len(res.Objects) == 0 {
		return
	}

	staleThreshold := 2 * time.Hour
	now := time.Now().UTC()
	checker := objects.GetGlobalStatusChecker()

	for _, obj := range res.Objects {
		taskID, _ := obj[objects.FieldKeyID].(string)
		planRef, _ := obj[objects.FieldKeyPriorityPlanRef].(string)

		parentIsTerminal := false
		if planRef != "" {
			if parentObj, err := s.storage.Read(ctx, secCtx, planRef); err == nil && parentObj != nil {
				parentStatus, _ := parentObj[objects.FieldKeyStatus].(string)
				if checker.IsTerminal(objects.KindPriorityPlan, parentStatus) {
					parentIsTerminal = true
				}
			}
		}

		if parentIsTerminal {
			// Auto-archive orphaned task whose plan is already complete
			updateCtx := pkgctx.WithLifecycleBreakGlass(ctx, "hourglass sweep archive orphaned task")
			updates := map[string]any{
				objects.FieldKeyStatus: objects.ObjectStatusArchived,
			}
			if err := s.storage.Update(updateCtx, secCtx, taskID, updates); err == nil {
				SchedulerDaemonLog(s.logger).Info("Archived orphaned agent_task whose parent priority_plan is terminal").
					String("task_id", taskID).
					String("plan_id", planRef).
					Log()
			}
			continue
		}

		// Check if claim is stale
		updatedAtStr, _ := obj[objects.FieldKeyUpdatedAt].(string)
		if updatedAtStr == "" {
			updatedAtStr, _ = obj[objects.FieldKeyClaimedAt].(string)
		}
		if updatedAtStr != "" {
			if t, err := time.Parse(time.RFC3339, updatedAtStr); err == nil {
				if now.Sub(t) >= staleThreshold {
					// Timed out with no ambient activity; transition to error so it can be triaged/recovered
					updateCtx := pkgctx.WithLifecycleBreakGlass(ctx, "hourglass sweep stale task timeout")
					updates := map[string]any{
						objects.FieldKeyStatus: objects.ObjectStatusError,
					}
					if err := s.storage.Update(updateCtx, secCtx, taskID, updates); err == nil {
						SchedulerDaemonLog(s.logger).Warn("Stale agent_task in_progress timed out without ambient activity").
							String("task_id", taskID).
							Log()
					}
				}
			}
		}
	}
}
