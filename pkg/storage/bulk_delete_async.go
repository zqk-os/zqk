package storage

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/when"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
)

const (
	bulkDeleteJobStatusCompleted = "completed"
	bulkDeleteJobStatusFailed    = "failed"
	bulkDeleteJobStatusPartial   = "partial"
	bulkDeleteJobStatusPending   = "pending"
	bulkDeleteJobStatusRunning   = "running"
)

// BulkDeleteJob represents an async bulk delete job
type BulkDeleteJob struct {
	ID           string
	ObjectIDs    []string
	Status       string // "pending", "running", "completed", "failed", "partial"
	Progress     int    // Number of objects processed
	Total        int    // Total number of objects
	SuccessCount int
	FailureCount int
	Errors       []BulkOperationError
	StartedAt    *time.Time
	CompletedAt  *time.Time
	Duration     time.Duration
	mu           sync.RWMutex
}

// BulkDeleteJobManager manages async bulk delete jobs
type BulkDeleteJobManager struct {
	jobs               map[string]*BulkDeleteJob
	mu                 sync.RWMutex
	storage            ObjectStorageProvider
	logger             logging.Logger
	jobsCreatedTotal   atomic.Int64
	jobsCompletedTotal atomic.Int64
}

// NewBulkDeleteJobManager creates a new job manager
func NewBulkDeleteJobManager(storage ObjectStorageProvider) *BulkDeleteJobManager {
	return &BulkDeleteJobManager{
		jobs:    make(map[string]*BulkDeleteJob),
		storage: storage,
		logger:  logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// GetJobManagerStats returns lifetime counters for created and completed jobs.
func (m *BulkDeleteJobManager) GetJobManagerStats() (created, completed int64) {
	return m.jobsCreatedTotal.Load(), m.jobsCompletedTotal.Load()
}

// CreateJob creates a new bulk delete job
func (m *BulkDeleteJobManager) CreateJob(ctx context.Context, ids []string) (*BulkDeleteJob, error) {
	m.jobsCreatedTotal.Add(1)
	jobID := fmt.Sprintf(FmtBulkDeleteJobName, time.Now().UnixNano())

	job := &BulkDeleteJob{
		ID:        jobID,
		ObjectIDs: ids,
		Status:    bulkDeleteJobStatusPending,
		Total:     len(ids),
		Errors:    make([]BulkOperationError, 0),
	}

	err := concurrency.RunInLockWithLogger(
		&m.mu, locknames.LockNameBulkDeleteCreateJob, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			m.jobs[jobID] = job
			return nil
		},
	)
	if err != nil {
		return nil, errfmt.Newf(ErrMsgLockCreateJob).Wrap(err)
	}

	StorageLog(m.logger).Info(LogEventStorageBulkDeleteAsyncJobCreatedInfo).
		JobID(jobID).
		Int("object_count", len(ids)).
		Log()

	return job, nil
}

// ExecuteJob executes a bulk delete job asynchronously
func (m *BulkDeleteJobManager) ExecuteJob(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	jobID string,
	cascade bool,
	maxWorkers int,
) error {
	var job *BulkDeleteJob
	var exists bool
	if err := concurrency.RunInRLockWithLogger(
		&m.mu, locknames.LockNameBulkDeleteExecuteGetJob, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			job, ok = m.jobs[jobID]
			exists = ok
			return nil
		},
	); err != nil {
		return errfmt.Newf(ErrMsgLockGetJob).Wrap(err)
	}

	if !exists {
		return errfmt.Errorf(ErrMsgJobNotFound, jobID)
	}

	// Update status to running
	err := concurrency.RunInLockWithLogger(
		&job.mu, locknames.LockNameBulkDeleteExecuteStart, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if job.Status != bulkDeleteJobStatusPending {
				return errfmt.Errorf(ErrMsgJobNotPending, job.Status)
			}
			job.Status = bulkDeleteJobStatusRunning
			now := time.Now()
			job.StartedAt = &now
			return nil
		},
	)
	if err != nil {
		return err
	}

	// Execute in goroutine
	goroutinelabels.NewGoroutine(DescBulkDeleteAsyncExec, fmt.Sprintf(DescExecBulkDeleteJob, jobID)).
		WithCleanup(func() {
			err := concurrency.RunInLockWithLogger(
				&job.mu, locknames.LockNameBulkDeleteCleanup, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					now := time.Now()
					job.CompletedAt = &now
					if job.StartedAt != nil {
						job.Duration = now.Sub(*job.StartedAt)
					}

					// Determine final status
					when.When(func() bool { return job.FailureCount == 0 }).Then(func() {
						job.Status = bulkDeleteJobStatusCompleted
					}).OrElseWhen(func() bool { return job.SuccessCount == 0 }).Then(func() {
						job.Status = bulkDeleteJobStatusFailed
					}).OrElse(func() {
						job.Status = bulkDeleteJobStatusPartial
					}).Run()
					return nil
				},
			)
			if err != nil {
				StorageLog(m.logger).Error(LogEventStorageBulkDeleteAsyncCleanupFailedErr, err).
					JobID(jobID).
					Log()
			}

			StorageLog(m.logger).Info(LogEventStorageBulkDeleteAsyncJobCompletedInfo).
				JobID(jobID).
				String("status", job.Status).
				Int("success", job.SuccessCount).
				Int("failed", job.FailureCount).
				Log()

			m.jobsCompletedTotal.Add(1)
		}).
		StartSimple(func() {
			if m.storage == nil {
				StorageLog(m.logger).Error(LogEventStorageBulkDeleteAsyncJobFailedErr, errfmt.Errorf("storage provider is nil")).
					JobID(jobID).
					Log()
				_ = concurrency.RunInLockWithLogger(
					&job.mu, locknames.LockNameBulkDeleteJobFailed, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
					func() error {
						job.Status = bulkDeleteJobStatusFailed
						return nil
					},
				)
				return
			}

			// Use optimized bulk delete if available
			if fileStorage, ok := m.storage.(*FileObjectStorage); ok {
				result, err := fileStorage.BulkDeleteOptimized(ctx, secCtx, job.ObjectIDs, cascade, maxWorkers)
				if err != nil || result == nil {
					if err == nil {
						err = errfmt.Errorf("bulk delete returned nil result")
					}
					lockErr := concurrency.RunInLockWithLogger(
						&job.mu, locknames.LockNameBulkDeleteJobFailed, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
						func() error {
							job.Status = bulkDeleteJobStatusFailed
							return nil
						},
					)
					if lockErr != nil {
						StorageLog(m.logger).Error(LogEventStorageBulkDeleteAsyncLockFailedErr, lockErr).
							JobID(jobID).
							Log()
					}
					StorageLog(m.logger).Error(LogEventStorageBulkDeleteAsyncJobFailedErr, err).
						JobID(jobID).
						Log()
					return
				}

				lockErr := concurrency.RunInLockWithLogger(
					&job.mu, locknames.LockNameBulkDeleteJobUpdate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
					func() error {
						job.Progress = result.TotalCount
						job.SuccessCount = result.SuccessCount
						job.FailureCount = result.FailureCount
						job.Errors = result.Errors
						return nil
					},
				)
				if lockErr != nil {
					StorageLog(m.logger).Error(LogEventStorageBulkDeleteAsyncLockFailedErr, lockErr).
						JobID(jobID).
						Log()
				}
			} else {
				// Fallback to standard bulk delete
				result, err := m.storage.BulkDelete(ctx, secCtx, job.ObjectIDs, cascade)
				if err != nil || result == nil {
					if err == nil {
						err = errfmt.Errorf("bulk delete returned nil result")
					}
					lockErr := concurrency.RunInLockWithLogger(
						&job.mu, locknames.LockNameBulkDeleteJobFailed, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
						func() error {
							job.Status = bulkDeleteJobStatusFailed
							return nil
						},
					)
					if lockErr != nil {
						StorageLog(m.logger).Error(LogEventStorageBulkDeleteAsyncLockFailedErr, lockErr).
							JobID(jobID).
							Log()
					}
					StorageLog(m.logger).Error(LogEventStorageBulkDeleteAsyncJobFailedErr, err).
						JobID(jobID).
						Log()
					return
				}

				lockErr := concurrency.RunInLockWithLogger(
					&job.mu, locknames.LockNameBulkDeleteJobUpdate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
					func() error {
						job.Progress = result.TotalCount
						job.SuccessCount = result.SuccessCount
						job.FailureCount = result.FailureCount
						job.Errors = result.Errors
						return nil
					},
				)
				if lockErr != nil {
					StorageLog(m.logger).Error(LogEventStorageBulkDeleteAsyncLockFailedErr, lockErr).
						JobID(jobID).
						Log()
				}
			}
		})

	return nil
}

// GetJobStatus returns the current status of a job
func (m *BulkDeleteJobManager) GetJobStatus(jobID string) (*BulkDeleteJob, error) {
	var job *BulkDeleteJob
	var exists bool
	err := concurrency.RunInRLockWithLogger(
		&m.mu, locknames.LockNameBulkDeleteGetStatusManager, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			job, ok = m.jobs[jobID]
			exists = ok
			return nil
		},
	)
	if err != nil {
		return nil, errfmt.Newf(ErrMsgLockGetJobStatus).Wrap(err)
	}

	if !exists {
		return nil, errfmt.Errorf(ErrMsgJobNotFound, jobID)
	}

	// Return a copy to avoid race conditions
	var jobCopy BulkDeleteJob
	err = concurrency.RunInRLockWithLogger(
		&job.mu, locknames.LockNameBulkDeleteGetStatusJob, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			jobCopy = BulkDeleteJob{
				ID:           job.ID,
				ObjectIDs:    job.ObjectIDs,
				Status:       job.Status,
				Progress:     job.Progress,
				Total:        job.Total,
				SuccessCount: job.SuccessCount,
				FailureCount: job.FailureCount,
				Errors:       job.Errors,
				StartedAt:    job.StartedAt,
				CompletedAt:  job.CompletedAt,
				Duration:     job.Duration,
			}
			return nil
		},
	)
	if err != nil {
		return nil, errfmt.Newf(ErrMsgLockCopyJobStatus).Wrap(err)
	}

	return &jobCopy, nil
}

// CleanupFailedJobs creates a cleanup job for failed deletions
func (m *BulkDeleteJobManager) CleanupFailedJobs(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	jobID string,
	maxWorkers int,
) (*BulkDeleteJob, error) {
	// Get the original job
	originalJob, err := m.GetJobStatus(jobID)
	if err != nil {
		return nil, err
	}

	// Collect failed IDs
	failedIDs := make([]string, 0)
	for _, err := range originalJob.Errors {
		failedIDs = append(failedIDs, err.ID)
	}

	if len(failedIDs) == 0 {
		return nil, errfmt.Errorf(ErrMsgNoFailedDeletionsCleanup)
	}

	// Create cleanup job
	cleanupJob, err := m.CreateJob(ctx, failedIDs)
	if err != nil {
		return nil, err
	}

	// Execute cleanup job
	if err := m.ExecuteJob(ctx, secCtx, cleanupJob.ID, false, maxWorkers); err != nil {
		return nil, err
	}

	return cleanupJob, nil
}
