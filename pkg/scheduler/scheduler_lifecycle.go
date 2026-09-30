package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const (
	schedulerCronStopTimeout          = 2 * time.Second
	schedulerTriggeredPoolStopTimeout = 5 * time.Second
)

// Stop stops the scheduler daemon
func (s *Scheduler) Stop() {
	stopTime := time.Now()

	// Check permission to manage scheduler
	if err := s.checkPermission("manage:scheduler"); err != nil {
		SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonPermissionDeniedStop).
			WithError(err).
			Log()
		return
	}

	var isRunning bool
	_ = concurrency.RunInLockWithLogger(
		&s.runningMu,
		LockNameSchedulerStopCheck,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			isRunning = s.running
			if s.running {
				s.running = false
			}
			return nil
		},
	)

	if !isRunning {
		return
	}

	SchedulerDaemonLog(s.logger).Info(LogEventSchedulerDaemonStopping).Log()

	SchedulerDaemonLog(s.logger).Debug("Stop: stopping anticipatory engine").Log()
	if s.anticipatoryEngine != nil {
		if err := s.anticipatoryEngine.Stop(); err != nil {
			SchedulerDaemonLog(s.logger).Warn("Failed to stop anticipatory engine").
				WithError(err).
				Log()
		} else {
			SchedulerDaemonLog(s.logger).Info("Anticipatory engine stopped successfully").Log()
		}
	}

	// Mark any currently running job as abandoned so events files have a final outcome (not just "started").
	// Collect IDs under lock (no I/O); write log entries outside lock to avoid holding mutex during I/O.
	SchedulerDaemonLog(s.logger).Debug("Stop: abandoning running jobs").Log()
	var runningJobIDs []string
	s.jobsMu.RLock()
	for _, job := range s.jobs {
		job.RunningMu.RLock()
		running := job.Running
		job.RunningMu.RUnlock()
		if running {
			runningJobIDs = append(runningJobIDs, job.ID)
		}
	}
	s.jobsMu.RUnlock()
	stopTimeStr := zqktime.FormatRFC3339UTC(stopTime)
	for _, jobID := range runningJobIDs {
		writeJobLogEntry(s.projectRoot, jobID, map[string]any{
			KeyEventType: JobLogEventAbandoned,
			KeyJobID:     jobID,
			KeyTimestamp: stopTimeStr,
		})
	}
	SchedulerDaemonLog(s.logger).Debug("Stop: closing job log writers").Log()
	if s.projectRoot != emptyValue {
		CloseAllJobLogWriters(s.projectRoot)
	}

	// Stop metric sampling (flush tickers + pending batches) before cron/storage teardown so shutdown
	// does not contend with WAL/hash drain and does not strand flushTickerLoop goroutines until process exit.
	SchedulerDaemonLog(s.logger).Debug("Stop: stopping metrics samplers").Log()
	const metricsSamplerStopBudget = 4 * time.Second
	if s.samplingPipeline != nil {
		reg := s.samplingPipeline.GetSamplerRegistry()
		if reg != nil {
			done := make(chan struct{})
			goroutinelabels.NewGoroutine("scheduler", "stop metrics samplers").StartSimple(func() {
				defer close(done)
				_ = reg.StopAll() //nolint:errcheck // best-effort; continue shutdown regardless
			})
			select {
			case <-done:
			case <-time.After(metricsSamplerStopBudget):
				SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonMetricsSamplerStopTimedOut).
					String("budget", metricsSamplerStopBudget.String()).
					Log()
			}
		}
	}

	// Remove keep-alive file FIRST (before stopping cron) to ensure cleanup happens
	// even if process is killed during shutdown
	if s.projectRoot != emptyValue {
		if err := removeKeepAlive(s.projectRoot); err != nil {
			SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonKeepAliveRemoveFailed).
				ProjectRoot(s.projectRoot).
				WithError(err).
				Log()
			// Don't fail shutdown - keep-alive cleanup is best effort
		}
	}

	// Stop cron scheduler with timeout to prevent hanging
	SchedulerDaemonLog(s.logger).Debug("Stop: stopping cron").Log()
	stopCtx := s.cron.Stop()
	select {
	case <-stopCtx.Done():
		// Cron stopped successfully
	case <-time.After(schedulerCronStopTimeout):
		// Timeout - log warning but continue shutdown
		SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonCronStopTimedOut).Log()
	}

	// Stop priority pool first so critical jobs can finish, then general triggered-job pool (releases budget slots).
	// Use bounded wait so shutdown completes even if in-flight jobs don't respect context cancellation promptly.
	SchedulerDaemonLog(s.logger).Debug("Stop: stopping pools").Log()
	stopTriggeredPool := func(p *goroutinelabels.Pool, name string) {
		if p == nil {
			return
		}
		poolDone := make(chan struct{})
		poolStopBud := goroutinelabels.DefaultBudget()
		poolStopBuilder := goroutinelabels.NewGoroutine("scheduler_triggered_pool_stop", name)
		if poolStopBud != nil {
			poolStopBuilder = poolStopBuilder.WithBudget(poolStopBud)
		}
		poolStopBuilder.WithCleanup(func() { close(poolDone) }).StartSimple(func() { p.Stop() })
		select {
		case <-poolDone:
		case <-time.After(schedulerTriggeredPoolStopTimeout):
			SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonTriggeredPoolStopTimedOut).Log()
		}
	}
	stopTriggeredPool(s.triggeredPriorityPool, "stopping priority triggered job pool during shutdown")
	s.triggeredPriorityPool = nil
	stopTriggeredPool(s.triggeredPool, "stopping triggered job pool during shutdown")
	s.triggeredPool = nil

	// Stop async router if running
	SchedulerDaemonLog(s.logger).Debug("Stop: stopping async router").Log()
	if s.asyncRouter != nil {
		if err := s.asyncRouter.Stop(); err != nil {
			SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonAsyncRouterStopFailed).
				WithError(err).
				Log()
		}
	}

	// CRITICAL: Shutdown process group manager to control all tracked subprocesses
	// This ensures all spawned processes are properly terminated
	// Timeout is configured in NewProcessGroupManager (3 seconds)
	if s.processGroupManager != nil {
		if err := s.processGroupManager.Shutdown("scheduler daemon stopped"); err != nil {
			SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonProcessGroupShutdownIssues).
				WithError(err).
				Log()
		}
	}

	// Stop notification context
	if s.notificationContext != nil {
		s.notificationContext.Stop()
	}

	// Stop activity cache writer goroutine (cleanup)
	cache := GetGlobalActivityCache()
	if cache != nil {
		cache.Stop()
	}

	// Drain all queues registered with the global shutdown coordinator (hash registry, IO queue,
	// audit buffers, async validation strategies, etc.). InitiateShutdown alone only signals
	// stop-accepting; Drain runs each handler's Drain() with the coordinator timeout (default 30s).
	// DrainAll is idempotent per coordinator instance (sync.Once), so concurrent callers are safe.
	shutdownCoordinator := storagepkg.GetGlobalShutdownCoordinator()
	if shutdownCoordinator != nil {
		const schedulerQueueDrainBudget = 45 * time.Second
		drainCtx, drainCancel := context.WithTimeout(pkgctx.NewSystemContext(), schedulerQueueDrainBudget)
		if err := shutdownCoordinator.DrainAll(drainCtx); err != nil {
			SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonQueueShutdownDrainIncomplete).
				WithError(err).
				String("budget", schedulerQueueDrainBudget.String()).
				Log()
		}
		drainCancel()
	}

	// Determinism: also shut down the concrete storage instance so storage-local background
	// workers (notably the ObjectWriteBehindWorker WAL replay/apply loop) stop promptly.
	// Relying only on the global shutdown coordinator cancels some workers, but the write-behind
	// worker is stopCh-driven and may otherwise continue WAL replay during a "stop --wait".
	type storageShutdowner interface {
		Shutdown(context.Context) error
	}
	if s.storage != nil {
		if sd, ok := s.storage.(storageShutdowner); ok {
			stopCtx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 8*time.Second)
			defer cancel()
			if err := sd.Shutdown(stopCtx); err != nil {
				SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonStorageShutdownFailed).
					WithError(err).
					Log()
			}
		}
	}

	// Record metrics
	if s.metrics != nil {
		s.metrics.RecordSchedulerStop(time.Since(stopTime))
	}

	// Remove PID file on shutdown
	if s.projectRoot != emptyValue {
		if err := removePIDFile(s.projectRoot); err != nil {
			SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonPidRemoveFailed).
				ProjectRoot(s.projectRoot).
				WithError(err).
				Log()
			// Don't fail shutdown - PID file cleanup is best effort
		}
	}

	// Update state via callback (not defer) for predictable execution order
	_ = concurrency.RunInLockWithLogger(
		&s.runningMu,
		LockNameSchedulerStopCheck,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			s.running = false
			return nil
		},
	)
	SchedulerDaemonLog(s.logger).Debug("Stop: finished teardown").Log()
	SchedulerDaemonLog(s.logger).Info(LogEventSchedulerDaemonStopped).Log()
}

// IsRunning returns whether the scheduler is running
func (s *Scheduler) IsRunning() bool {
	var isRunning bool
	_ = concurrency.RunInRLockWithLogger(
		&s.runningMu,
		LockNameSchedulerIsRunning,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			isRunning = s.running
			return nil
		},
	)
	return isRunning
}

// GetMetricsSnapshot returns a point-in-time snapshot of scheduler metrics.
// Returns a zero-valued snapshot if the scheduler has no metrics collector.
// Used by dashboard/report commands (e.g. health-data, scheduler activity).
func (s *Scheduler) GetMetricsSnapshot() SchedulerMetricsSnapshot {
	if s.metrics == nil {
		return SchedulerMetricsSnapshot{}
	}
	return s.metrics.GetMetrics()
}

// GetRunningJobIDs returns job IDs currently executing (from conflict manager).
// Only populated when scheduler runs in this process; empty when daemon is in another process.
func (s *Scheduler) GetRunningJobIDs() []string {
	if s.conflictMgr == nil {
		return nil
	}
	return s.conflictMgr.GetRunningJobIDs()
}

// loadAndScheduleJobs, scheduleTimerJob, scheduleImmediateJob, registerTriggeredJob,
// createJobHandler, createHandlerWithStorage, GetAsyncRouter are defined in job_management.go
// executeJob, updateJobInStorage, and disableJobInStorage are defined in job_execution.go

// handleCoordinationEvents watches for coordination events and handles drift detection.
func (s *Scheduler) handleCoordinationEvents(ctx context.Context) {
	if s.coordinationChannel == nil {
		return
	}
	ch := s.coordinationChannel.Subscribe()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			switch ev.Type {
			case "drift_detected":
				if ev.JobID != "" {
					if job, ok := s.GetJob(ev.JobID); ok {
						SchedulerDaemonLog(s.logger).Warn("Pausing errant job due to drift").
							String("job_id", ev.JobID).
							Int("process_id", ev.ProcessID).
							Log()

						job.RunningMu.RLock()
						cancel := job.executionCancel
						job.RunningMu.RUnlock()
						if cancel != nil {
							cancel()
						}

						// Enqueue trigger request for respawn
						_ = s.TriggerJob(ctx, job.ID)
					}
				}
			case "process_started":
				if s.notificationContext != nil {
					notif := CreateJobNotification(ev.JobID, "subagent", "swarm", notificationEventStatusUpdate, PriorityLow, 0, nil, ev.Metadata)
					notif.Title = "Subagent Task Started"
					notif.Message = fmt.Sprintf("Subagent task %s has started execution in the swarm.", ev.JobID)
					s.notificationContext.Notify(notif)
				}
			case "process_completed":
				if s.notificationContext != nil {
					notif := CreateJobNotification(ev.JobID, "subagent", "swarm", notificationEventCompleted, PriorityMedium, 0, nil, ev.Metadata)
					notif.Title = "Subagent Task Completed"
					notif.Message = fmt.Sprintf("Subagent task %s completed successfully.", ev.JobID)
					s.notificationContext.Notify(notif)
				}
			case "process_errored":
				if s.notificationContext != nil {
					var execErr error
					if errMsg, ok := ev.Metadata["error"].(string); ok && errMsg != "" {
						execErr = fmt.Errorf("%s", errMsg)
					}
					notif := CreateJobNotification(ev.JobID, "subagent", "swarm", notificationEventFailed, PriorityHigh, 0, execErr, ev.Metadata)
					notif.Title = "Subagent Task Failed"
					notif.Message = fmt.Sprintf("Subagent task %s has failed.", ev.JobID)
					s.notificationContext.Notify(notif)
				}
			}
		}
	}
}
