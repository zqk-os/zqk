package scheduler

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// maxLockFilenameJobIDLen is the maximum job ID length used for the lock filename.
// Longer IDs are replaced by a SHA256 hash (hex) to avoid "file name too long" (e.g. NAME_MAX 255).
const maxLockFilenameJobIDLen = 200

// RetentionToleranceSingletonLockID is the logical job ID used for the project-level "only one
// retention_tolerance at a time" lock. Both the CLI (zqk system retention-tolerance) and the
// daemon's maintenance runner must use this same ID so that only one retention run (CLI or
// maintenance) proceeds; others skip or exit. Prevents parallel retention-tolerance from degrading
// performance (contention on storage/locks).
const RetentionToleranceSingletonLockID = "retention_tolerance_singleton"

// AuditAggregationSingletonLockID is the project-level "only one audit aggregation at a time"
// lock. Two independent callers compete: the scheduler (SCH-002 cron job) and the
// MaintenanceRunner which bypasses executeJob/ConflictManager. Without this lock both paths can
// run simultaneously, read the same audit event window, and each append a separate
// audit_aggregation_metric to the stream — producing duplicates. All callers (scheduler handler,
// maintenance runner, CLI) must use this same ID so only one proceeds while others skip.
const AuditAggregationSingletonLockID = "audit_aggregation_singleton"

// JobLockConfig configures job lock behavior
type JobLockConfig struct {
	// LockTimeout is the maximum time to wait for a lock before giving up
	LockTimeout time.Duration

	// StaleLockThreshold is the maximum age of a lock file before it's considered stale
	// If a lock file is older than this, it will be removed and a new lock acquired
	StaleLockThreshold time.Duration

	// LockDir is the directory where lock files are stored
	LockDir string
}

// DefaultJobLockConfig returns a default configuration
func DefaultJobLockConfig(projectRoot string) JobLockConfig {
	return JobLockConfig{
		LockTimeout:        30 * time.Second,
		StaleLockThreshold: 1 * time.Hour,
		LockDir:            filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir, paths.SchedulerLocksDir),
	}
}

// JobLock provides distributed locking for scheduler job execution
// Uses file-based locking via pkg/storage/file_lock.go to prevent
// duplicate job execution across multiple scheduler processes
type JobLock struct {
	jobID      string
	lockPath   string
	fileLock   *storagepkg.FileLock
	config     JobLockConfig
	acquiredAt *time.Time
}

// NewJobLock creates a new job lock for the given job ID
func NewJobLock(jobID, projectRoot string) (JobLockInterface, error) {
	return NewJobLockWithConfig(jobID, DefaultJobLockConfig(projectRoot))
}

// lockFilenameForJobID returns a filesystem-safe lock filename for the given job ID.
// Long IDs (e.g. legacy SCH-<ts>-scheduler-job-<parent> chains) cause "file name too long";
// we use a SHA256 hash (hex) for those so the lock still uniquely corresponds to the job.
func lockFilenameForJobID(jobID string) string {
	if len(jobID) <= maxLockFilenameJobIDLen {
		return jobID + ".lock"
	}
	sum := sha256.Sum256([]byte(jobID))
	return hex.EncodeToString(sum[:]) + ".lock"
}

// NewJobLockWithConfig creates a new job lock with custom configuration
func NewJobLockWithConfig(jobID string, config JobLockConfig) (JobLockInterface, error) {
	// Ensure lock directory exists
	if err := fileutil.MkdirAll(config.LockDir, paths.DirPerm755); err != nil {
		return nil, errfmt.Newf("failed to create lock directory").Wrap(err)
	}

	// Create lock file path: {LockDir}/{safeJobID}.lock (safe = jobID or hash if too long)
	lockPath := filepath.Join(config.LockDir, lockFilenameForJobID(jobID))

	// Create file lock
	fileLock, err := storagepkg.NewFileLock(lockPath)
	if err != nil {
		return nil, errfmt.Newf("failed to create file lock").Wrap(err)
	}

	return &JobLock{
		jobID:    jobID,
		lockPath: lockPath,
		fileLock: fileLock,
		config:   config,
	}, nil
}

// Acquire attempts to acquire the lock with timeout
// Returns an error if the lock cannot be acquired within the timeout
func (jl *JobLock) Acquire() error {
	// Check for stale lock first
	if err := jl.checkAndCleanStaleLock(); err != nil {
		return errfmt.Newf("failed to check stale lock").Wrap(err)
	}

	// Acquire lock with timeout
	timeout := jl.config.LockTimeout
	if timeout == 0 {
		timeout = 30 * time.Second // Default timeout
	}

	err := jl.fileLock.LockWithTimeout(timeout)
	if err != nil {
		return errfmt.Errorf("failed to acquire job lock for %s: %w", jl.jobID, err)
	}

	now := time.Now()
	jl.acquiredAt = &now

	return nil
}

// TryAcquire attempts to acquire the lock without blocking
// Returns true if the lock was acquired, false if it's already held
func (jl *JobLock) TryAcquire() (bool, error) {
	// Check for stale lock first
	if err := jl.checkAndCleanStaleLock(); err != nil {
		return false, errfmt.Newf("failed to check stale lock").Wrap(err)
	}

	acquired, err := jl.fileLock.TryLock()
	if err != nil {
		return false, errfmt.Errorf("failed to try acquire job lock for %s: %w", jl.jobID, err)
	}

	if acquired {
		now := time.Now()
		jl.acquiredAt = &now
	}

	return acquired, nil
}

// Release releases the lock
func (jl *JobLock) Release() error {
	if jl.fileLock == nil {
		return errfmt.Errorf("job lock not initialized")
	}

	err := jl.fileLock.Unlock()
	if err != nil {
		return errfmt.Errorf("failed to release job lock for %s: %w", jl.jobID, err)
	}

	jl.acquiredAt = nil
	return nil
}

// Close closes the lock file and releases the lock if still held
func (jl *JobLock) Close() error {
	if jl.fileLock == nil {
		return nil
	}

	err := jl.fileLock.Close()
	if err != nil {
		return errfmt.Errorf("failed to close job lock for %s: %w", jl.jobID, err)
	}

	jl.acquiredAt = nil
	return nil
}

// IsLocked returns whether the lock is currently held
func (jl *JobLock) IsLocked() bool {
	if jl.fileLock == nil {
		return false
	}
	return jl.fileLock.IsLocked()
}

// AcquiredAt returns the time when the lock was acquired, or nil if not acquired
func (jl *JobLock) AcquiredAt() *time.Time {
	return jl.acquiredAt
}

// checkAndCleanStaleLock checks if the lock file exists and is stale, and removes it if so
func (jl *JobLock) checkAndCleanStaleLock() error {
	// Check if lock file exists
	info, err := fileutil.Stat(jl.lockPath)
	if fileutil.IsNotExist(err) {
		// Lock file doesn't exist, nothing to clean
		return nil
	}
	if err != nil {
		return errfmt.Newf("failed to stat lock file").Wrap(err)
	}

	// Check if lock file is stale
	threshold := jl.config.StaleLockThreshold
	if threshold == 0 {
		threshold = 1 * time.Hour // Default threshold
	}

	age := time.Since(info.ModTime())
	if age > threshold {
		// Lock file is stale, remove it
		// Note: We can't safely remove a lock file that might be held by another process
		// However, if the file is truly stale (older than threshold), it's likely
		// from a crashed process. We'll attempt to remove it, but the actual lock
		// will be released when the file is closed or the process exits.
		// The file lock mechanism will handle the actual lock release.
		if err := fileutil.Remove(jl.lockPath); err != nil && !fileutil.IsNotExist(err) {
			return errfmt.Newf("failed to remove stale lock file").Wrap(err)
		}
	}

	return nil
}
