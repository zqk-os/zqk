
package scheduler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"github.com/lanceman/zqk/pkg/syscallutil"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
)

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
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.checkHourglassTimers(ctx)
			}
		}
	})
}

// checkHourglassTimers scans the hourglass directory and kills/errors any expired tasks.
func (s *Scheduler) checkHourglassTimers(ctx context.Context) {
	schedulerRoot := paths.ResolvePathFromCacheOrConstant(s.projectRoot, "scheduler", filepath.Join(paths.ProjectDataDir, paths.SchedulerDir))
	hourglassDir := filepath.Join(schedulerRoot, "hourglass")
	files, err := os.ReadDir(hourglassDir)
	if err != nil {
		if os.IsNotExist(err) {
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
		data, err := os.ReadFile(filePath)
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
			if info.Type == "deadline" {
				SchedulerDaemonLog(s.logger).Warn("Hourglass expired for deadline, executing escalation").
					String("object_id", info.TaskID).
					String("kind", info.Kind).
					Log()

				// Natively escalate deadline
				taskObj, err := s.storage.Read(ctx, secCtx, info.TaskID)
				if err == nil {
					status, _ := taskObj[objects.FieldKeyStatus].(string)
					if status == "planned" || status == "in_progress" || status == "active" || status == "not_started" || status == "draft" {
						title, _ := taskObj[objects.FieldKeyTitle].(string)
						if len(title) > 50 {
							title = title[:50]
						}

						// Create risk blocker
						blocker := map[string]any{
							objects.FieldKeyKind:              objects.KindRiskBlocker,
							objects.FieldKeyTitle:             "Missed Deadline Escalation: " + title,
							objects.FieldKeyDescription:       "The " + info.Kind + " " + info.TaskID + " missed its deadline. Automated wake signal triggered.",
							objects.FieldKeyStatus:            "active",
							objects.FieldKeyPriorityTier:      "P0",
							objects.FieldKeyRelatedObjectRefs: []string{info.TaskID},
						}
						_ = s.storage.Create(ctx, secCtx, blocker)

						// Defer the original item
						_ = s.storage.Update(ctx, secCtx, info.TaskID, map[string]any{
							objects.FieldKeyStatus: "deferred",
						})
					}
				}
				_ = os.Remove(filePath)
			} else {
				SchedulerDaemonLog(s.logger).Warn("Hourglass expired for task, terminating stuck sync-loop process").
					String("task_id", info.TaskID).
					Int("pid", info.PID).
					Log()

				// 1. Terminate the process
				if info.PID > 0 && info.PID != os.Getpid() {
					// Send SIGKILL to terminate the subagent process immediately
					_ = syscallutil.KillProcess(info.PID)
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
				_ = os.Remove(filePath)
			}
		}
	}
}
