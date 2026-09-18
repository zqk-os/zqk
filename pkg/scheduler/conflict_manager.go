package scheduler

import (
	"sync"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
)

// ConflictManager handles job execution conflicts
type ConflictManager struct {
	runningJobs map[string]*ScheduledJob
	mu          sync.RWMutex
	// projectRoot enables PersistRunningJobsSnapshot for out-of-process CLI activity.
	projectRoot string
}

// CanRun checks if a job can run (no conflicts)
func (cm *ConflictManager) CanRun(job *ScheduledJob) bool {
	var canRun bool
	if err := concurrency.RunInRLockWithLogger(
		&cm.mu, LockNameSchedulerConflictManagerCanRun, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Same job ID must not run twice (e.g. cron + health recovery both submitting)
			if _, alreadyRunning := cm.runningJobs[job.ID]; alreadyRunning {
				canRun = false
				return nil
			}

			// If concurrent execution is allowed for this job type, allow this job ID
			if job.ConcurrentAllowed {
				canRun = true
				return nil
			}

			// For most job types, block if another *non-concurrent* job of the same type is running
			// (e.g. audit_event_aggregation: different scheduler_job IDs must not overlap — duplicate stream metrics).
			// run_wrapper is different: concurrency is per scheduler_job ID only (same ID blocked above).
			// Different maintenance scripts (distinct job IDs) may run concurrently; package concurrency
			// still limits hot go-test paths where applicable.
			if job.JobType != JobTypeRunWrapper {
				for _, runningJob := range cm.runningJobs {
					if runningJob.JobType != job.JobType || !runningJob.Running {
						continue
					}
					if runningJob.ConcurrentAllowed {
						continue
					}
					canRun = false
					return nil
				}
			}

			canRun = true
			return nil
		},
	); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Error("ConflictManager: CanRun lock failed", err).Log()
	}
	return canRun
}

// HasRunningJob reports whether jobID is currently registered with a running flag in this process.
// Used to avoid stacking follow-up triggers (e.g. envelope tick) while the target job is executing.
func (cm *ConflictManager) HasRunningJob(jobID string) bool {
	if cm == nil || jobID == "" {
		return false
	}
	var busy bool
	if err := concurrency.RunInRLockWithLogger(
		&cm.mu, LockNameSchedulerConflictManagerHasRunningJob, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			j, ok := cm.runningJobs[jobID]
			busy = ok && j != nil && j.Running
			return nil
		},
	); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Error("ConflictManager: HasRunningJob lock failed", err).Log()
	}
	return busy
}

// RegisterRunning registers a job as running
func (cm *ConflictManager) RegisterRunning(job *ScheduledJob) {
	if err := concurrency.RunInLockWithLogger(
		&cm.mu, LockNameSchedulerConflictManagerRegister, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			cm.runningJobs[job.ID] = job
			return nil
		},
	); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Error("ConflictManager: RegisterRunning lock failed", err).Log()
	}
	cm.persistRunningSnapshot()
}

// UnregisterRunning unregisters a job as running
func (cm *ConflictManager) UnregisterRunning(job *ScheduledJob) {
	if err := concurrency.RunInLockWithLogger(
		&cm.mu, LockNameSchedulerConflictManagerUnregister, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			delete(cm.runningJobs, job.ID)
			return nil
		},
	); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Error("ConflictManager: UnregisterRunning lock failed", err).Log()
	}
	cm.persistRunningSnapshot()
}

// persistRunningSnapshot best-effort writes running job IDs for CLI activity (daemon vs CLI process).
func (cm *ConflictManager) persistRunningSnapshot() {
	if cm == nil || cm.projectRoot == "" {
		return
	}
	ids := cm.GetRunningJobIDs()
	if err := PersistRunningJobsSnapshot(cm.projectRoot, ids); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Debug("ConflictManager: PersistRunningJobsSnapshot failed").WithError(err).Log()
	}
}

// GetRunningJobIDs returns a copy of job IDs currently registered as running.
// Used by CLI (e.g. scheduler activity busyness) when scheduler runs in-process.
func (cm *ConflictManager) GetRunningJobIDs() []string {
	var ids []string
	if err := concurrency.RunInRLockWithLogger(
		&cm.mu, LockNameSchedulerConflictManagerGetRunning, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			ids = make([]string, 0, len(cm.runningJobs))
			for id := range cm.runningJobs {
				ids = append(ids, id)
			}
			return nil
		},
	); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Error("ConflictManager: GetRunningJobIDs lock failed", err).Log()
	}
	return ids
}
