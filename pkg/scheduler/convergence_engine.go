package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/convergence"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"

	"github.com/zqk-os/zqk/pkg/objects"
)

// ConvergenceEngine loop interval

// ConvergenceStorageProvider defines the storage operations required by the convergence engine.
type ConvergenceStorageProvider interface {
	ListConvergenceSessions(ctx context.Context) ([]map[string]any, error)
	UpdateSessionStatus(ctx context.Context, id, status string) error
	CreatePriorityPlanForTimeout(ctx context.Context, sessionID string) error
}

const (
	convergenceEngineInterval        = 30 * time.Second
	convergenceEngineShutdownTimeout = 15 * time.Second
)

// EventKindSynthesisRequired is emitted when the Convergence Engine detects a test failure.
const EventKindSynthesisRequired = "synthesis_required"

// StartConvergenceEngine starts a background daemon that watches for active convergence_session
// objects and autonomously evaluates them using the cvs_convergence_orchestrate.sh script as a stepping stone.
func (s *Scheduler) StartConvergenceEngine(ctx context.Context) (stop func(context.Context)) {
	if s.projectRoot == emptyValue {
		SchedulerDaemonLog(s.logger).Debug("convergence_engine: skipping start, no project root").Log()
		return func(context.Context) {}
	}

	bud := goroutinelabels.DefaultBudget()
	engineBuilder := goroutinelabels.NewGoroutine("scheduler_convergence_engine", "watching for active convergence_sessions to evaluate")
	if bud != nil {
		engineBuilder = engineBuilder.WithBudget(bud)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	engineBuilder.StartWithContext(ctx, func(ctx context.Context) error {
		defer wg.Done()
		ticker := time.NewTicker(convergenceEngineInterval)
		defer ticker.Stop()

		SchedulerDaemonLog(s.logger).Info("convergence_engine: started").Log()

		// Run once immediately on start
		s.evaluateActiveConvergenceSessions(ctx)
		s.evaluateStaleConvergenceSessions(ctx)

		for {
			select {
			case <-ctx.Done():
				SchedulerDaemonLog(s.logger).Info("convergence_engine: stopped").Log()
				return nil
			case <-ticker.C:
				s.evaluateActiveConvergenceSessions(ctx)
				s.evaluateStaleConvergenceSessions(ctx)
			}
		}
	})

	return func(ctx context.Context) {
		done := make(chan struct{})
		goroutinelabels.NewGoroutine("refactored_worker", "Refactored raw goroutine").
			StartSimple(func() {
				func() {
					wg.Wait()
					close(done)
				}()
			})
		select {
		case <-done:
		case <-ctx.Done():
			SchedulerDaemonLog(s.logger).Warn("convergence_engine: shutdown canceled by context").Log()
		case <-time.After(convergenceEngineShutdownTimeout):
			SchedulerDaemonLog(s.logger).Warn("convergence_engine: shutdown timed out; forcing exit").Log()
		}
	}
}

// EvaluateActiveConvergenceSessions evaluates all active convergence sessions
func (s *Scheduler) EvaluateActiveConvergenceSessions(ctx context.Context) {
	s.evaluateActiveConvergenceSessions(ctx)
}

func (s *Scheduler) evaluateActiveConvergenceSessions(ctx context.Context) {
	if s.storage == nil {
		return
	}

	// 1. Query for convergence_session objects
	engineStorage := &ConvergenceStorageAdapter{storage: s.storage}
	sessions, err := engineStorage.ListConvergenceSessions(ctx)
	if err != nil {
		SchedulerDaemonLog(s.logger).Warn("convergence_engine: failed to list sessions").WithError(err).Log()
		return
	}

	for _, session := range sessions {
		if err := ctx.Err(); err != nil {
			return
		}

		status, _ := session[objects.FieldKeyStatus].(string)
		if status != "active" {
			continue
		}

		id, _ := session[objects.FieldKeyID].(string)
		if id == emptyValue {
			continue
		}

		// Throttle ticks: if the session has been tick-evaluated in the last X seconds, skip it here.
		// For the PoC, we will look at the activity_log or last_updated. To avoid an infinite fast loop,
		// we should rate-limit execution per active session.
		// Wait, cvs_convergence_orchestrate.sh creates/updates cvs_rollup_latest.json.
		// Let's rely on the script's internal logic, but we must not spawn a new process if one is already running for this ID!
		// Let's acquire a lock.
		unlock := acquireConvergenceSessionTickLock(id)

		SchedulerDaemonLog(s.logger).Debug("convergence_engine: autonomously evaluating active session").String("session_id", id).Log()

		cmd := s.executor.CommandContext(ctx, "bash", "scripts/cvs_convergence_orchestrate.sh", id, "--no-fail-on-gates")
		cmd.SetDir(s.projectRoot)
		// Inherit env, set ZQK_PROJECT_ROOT
		env := append(os.Environ(), "ZQK_PROJECT_ROOT="+s.projectRoot)
		cmd.SetEnv(env)

		var buf bytes.Buffer
		cmd.SetStdout(&buf)
		cmd.SetStderr(&buf)
		err = cmd.Run()
		out := buf.Bytes()

		if err != nil {
			SchedulerDaemonLog(s.logger).Warn("convergence_engine: evaluation failed").
				String("session_id", id).
				WithError(err).
				String("output", truncateOrchestrateOutputPreview(string(out), 2000)).
				Log()
		} else {
			SchedulerDaemonLog(s.logger).Info("convergence_engine: evaluation succeeded").
				String("session_id", id).
				Log()
		}

		// Extract rollup_status to determine if we need synthesis
		var rollupStatus string
		var result map[string]any
		if err := json.Unmarshal(out, &result); err == nil {
			if rs, ok := result["rollup_status"].(string); ok {
				rollupStatus = rs
			}
		}

		if rollupStatus == "" {
			// Fallback: read the file produced by the script
			rollupPath := filepath.Join(s.projectRoot, paths.ProjectDataDir, paths.LogsDir, "drift", "cvs_rollup_latest.json")
			if data, err := fileutil.ReadFile(rollupPath); err == nil {
				if err := json.Unmarshal(data, &result); err == nil {
					if rs, ok := result["rollup_status"].(string); ok {
						rollupStatus = rs
					}
				}
			}
		}

		if rollupStatus != "" && rollupStatus != "success" && rollupStatus != "ready_for_parent" {
			// Emit synthesis request telemetry
			payload := map[string]any{
				"event_kind":              EventKindSynthesisRequired,
				objects.FieldKeySessionID: id,
				"rollup_status":           rollupStatus,
			}

			SchedulerDaemonLog(s.logger).Info("convergence_engine: emitting synthesis requirement").
				String("session_id", id).
				String("rollup_status", rollupStatus).
				Log()

			if s.samplingPipeline != nil {
				_, _ = s.samplingPipeline.Sample(payload)
			} else {
				// s.logger does not satisfy *logging.EventLogger directly
				SchedulerDaemonLog(s.logger).Info("synthesis required").
					String("event_kind", EventKindSynthesisRequired).
					String("session_id", id).
					String("rollup_status", rollupStatus).
					Log()
			}

			// Debounce/Reconciliation fix: update the session status so we don't spam telemetry.
			// Partial status-only update (full session maps from List can fail instance validation).
			updCtx := pkgctx.WithLifecycleBreakGlass(ctx, "scheduler convergence_engine")
			if err := engineStorage.UpdateSessionStatus(updCtx, id, objects.ObjectStatusPaused); err != nil {
				SchedulerDaemonLog(s.logger).Warn("convergence_engine: failed to update session status to paused").
					String("session_id", id).
					WithError(err).
					Log()
			}
		}

		unlock()
	}
}

// EvaluateStaleConvergenceSessions evaluates all stale convergence sessions
func (s *Scheduler) EvaluateStaleConvergenceSessions(ctx context.Context) {
	s.evaluateStaleConvergenceSessions(ctx)
}

func (s *Scheduler) evaluateStaleConvergenceSessions(ctx context.Context) {
	if s.storage == nil {
		return
	}

	engineStorage := &ConvergenceStorageAdapter{storage: s.storage}
	sessions, err := engineStorage.ListConvergenceSessions(ctx)
	if err != nil {
		SchedulerDaemonLog(s.logger).Warn("convergence_engine: failed to list sessions for stale evaluation").WithError(err).Log()
		return
	}

	now := time.Now()
	for _, session := range sessions {
		if err := ctx.Err(); err != nil {
			return
		}

		status, _ := session[objects.FieldKeyStatus].(string)
		if !convergence.SessionStatusEligibleForAutoStaleEscalate(status) {
			continue
		}

		updatedAtStr, _ := session[objects.FieldKeyUpdatedAt].(string)
		if updatedAtStr == "" {
			continue
		}

		updatedAt, err := time.Parse(time.RFC3339, updatedAtStr)
		if err != nil {
			continue
		}

		if now.Sub(updatedAt) > time.Hour {
			id, _ := session[objects.FieldKeyID].(string)

			SchedulerDaemonLog(s.logger).Info("convergence_engine: session timed out").String("session_id", id).Log()

			// Status-only update; avoid re-saving the List payload.
			updCtx := pkgctx.WithLifecycleBreakGlass(ctx, "scheduler convergence_engine")
			if err := engineStorage.UpdateSessionStatus(updCtx, id, objects.ObjectStatusEscalated); err != nil {
				SchedulerDaemonLog(s.logger).Warn("convergence_engine: failed to update stale session status").
					String("session_id", id).
					WithError(err).
					Log()
				continue
			}

			// Create a new priority_plan at origin (grooming), then promote to active so it
			// leaves the draft plane and is List-visible (create with status=active is rewritten).
			// The origin is grooming, not planning: planning is evolution_management's origin, and
			// naming it here wrote a status the priority_plan lifecycle does not define. It
			// persisted only because the create below runs under lifecycle break-glass.
			plan := map[string]any{
				objects.FieldKeyKind:        objects.KindPriorityPlan,
				objects.FieldKeyStatus:      objects.ObjectStatusGrooming,
				objects.FieldKeyTitle:       "Drift-Control: Session " + id + " Timeout",
				objects.FieldKeyDescription: "Convergence session exceeded 1 hour timeout. Automated re-alignment required.",
			}

			createCtx := pkgctx.WithLifecycleBreakGlass(ctx, "scheduler convergence_engine")
			if err := s.storage.Create(createCtx, pkgctx.NewSystemSecurityContext(), plan); err != nil {
				SchedulerDaemonLog(s.logger).Warn("convergence_engine: failed to create priority plan for stale session").
					String("session_id", id).
					WithError(err).
					Log()
			} else if planID, _ := plan[objects.FieldKeyID].(string); planID != emptyValue {
				if err := s.storage.Update(createCtx, pkgctx.NewSystemSecurityContext(), planID, map[string]any{
					objects.FieldKeyStatus: objects.ObjectStatusActive,
				}); err != nil {
					SchedulerDaemonLog(s.logger).Warn("convergence_engine: failed to activate priority plan for stale session").
						String("session_id", id).
						String("plan_id", planID).
						WithError(err).
						Log()
				}
			}
		}
	}
}
