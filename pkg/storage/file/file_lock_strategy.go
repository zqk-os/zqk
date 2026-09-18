package file

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
)

// FileLockStrategy defines how file locks should be acquired, managed, and cleaned up
// Different strategies can be used for different scenarios (index files, cache files, etc.)
type FileLockStrategy interface {
	// AcquireLock acquires a file lock using the strategy's approach
	// Returns a LockHandle that must be released via ReleaseLock()
	AcquireLock(lockPath string, timeout time.Duration) (LockHandle, error)

	// CleanupStaleLocks removes stale lock files before acquiring new locks
	// This prevents accumulation of lock files from crashed processes
	CleanupStaleLocks(lockPath string) error

	// Name returns a human-readable name for this strategy
	Name() string
}

// LockHandle represents an acquired file lock that must be released
type LockHandle interface {
	// Release releases the lock and performs any necessary cleanup
	Release() error

	// IsLocked returns whether the lock is currently held
	IsLocked() bool
}

// fileLockHandle implements LockHandle for FileLock instances
type fileLockHandle struct {
	fileLock     *FileLock
	lockPath     string
	resourceType string // Type of resource being locked (index, cache, object_file, etc.)
	strategyName string // Name of the strategy used
	acquiredAt   time.Time
	cleanup      func() error // Strategy-specific cleanup function
}

func (h *fileLockHandle) Release() error {
	if h.fileLock == nil {
		return nil
	}

	// Calculate lock duration for metrics
	lockDuration := time.Since(h.acquiredAt)

	// Close releases the flock() lock
	if err := h.fileLock.Close(); err != nil {
		return errfmt.Newf(ConstMiscFailedToCloseFileLock).Wrap(err)
	}

	// Record metrics
	metrics := GetFileLockStrategyMetrics()
	metrics.RecordAcquisition(h.strategyName, h.resourceType, lockDuration)

	// Perform strategy-specific cleanup (e.g., remove lock file)
	if h.cleanup != nil {
		if err := h.cleanup(); err != nil {
			// Log but don't fail - cleanup errors are non-critical
			return nil
		}
	}

	return nil
}

func (h *fileLockHandle) IsLocked() bool {
	if h.fileLock == nil {
		return false
	}
	return h.fileLock.IsLocked()
}

// AutoCleanupStrategy automatically removes lock files after release
// This prevents lock file accumulation and git confusion
type AutoCleanupStrategy struct {
	StaleLockThreshold time.Duration // Lock files older than this are considered stale
}

// NewAutoCleanupStrategy creates a new auto-cleanup file lock strategy
func NewAutoCleanupStrategy() *AutoCleanupStrategy {
	return &AutoCleanupStrategy{
		StaleLockThreshold: 1 * time.Minute, // Default: 1 minute
	}
}

// NewAutoCleanupStrategyWithThreshold creates a strategy with custom stale threshold
func NewAutoCleanupStrategyWithThreshold(threshold time.Duration) *AutoCleanupStrategy {
	return &AutoCleanupStrategy{
		StaleLockThreshold: threshold,
	}
}

func (s *AutoCleanupStrategy) Name() string {
	return "auto-cleanup"
}

func (s *AutoCleanupStrategy) CleanupStaleLocks(lockPath string) error {
	stat, err := fileutil.Stat(lockPath)
	if fileutil.IsNotExist(err) {
		// No lock file exists - nothing to clean
		return nil
	}
	if err != nil {
		return errfmt.Newf(ConstMiscFailedToStatLockFile).Wrap(err)
	}

	age := time.Since(stat.ModTime())
	if age > s.StaleLockThreshold {
		// Lock file is stale - likely from a crashed process
		// The actual flock() lock is released when the process dies,
		// but the file remains. Remove it to prevent git confusion.
		if err := fileutil.Remove(lockPath); err != nil && !fileutil.IsNotExist(err) {
			return errfmt.Newf(ConstMiscFailedToRemoveStaleLockFile).Wrap(err)
		}
	}

	return nil
}

func (s *AutoCleanupStrategy) AcquireLock(lockPath string, timeout time.Duration) (LockHandle, error) {
	cleanupStart := time.Now()

	// Clean up stale locks before acquiring
	if err := s.CleanupStaleLocks(lockPath); err != nil {
		// Log but don't fail - stale cleanup is best effort
		// Proceed with lock acquisition anyway
	} else {
		// Record stale cleanup metrics
		cleanupTime := time.Since(cleanupStart)
		if cleanupTime > 0 {
			metrics := GetFileLockStrategyMetrics()
			metrics.RecordStaleCleanup(cleanupTime)
		}
	}

	// Determine resource type from lock path
	resourceType := s.determineResourceType(lockPath)

	fileLock, err := NewFileLock(lockPath)
	if err != nil {
		metrics := GetFileLockStrategyMetrics()
		metrics.RecordFailure(s.Name(), resourceType)
		return nil, errfmt.Newf(ConstMiscFailedToCreateFileLock).Wrap(err)
	}

	// Acquire lock with timeout
	if err := fileLock.LockWithTimeout(timeout); err != nil {
		_ = fileLock.Close() // Clean up on failure
		metrics := GetFileLockStrategyMetrics()
		if strings.Contains(err.Error(), "timeout") {
			metrics.RecordTimeout(s.Name(), resourceType)
		} else {
			metrics.RecordFailure(s.Name(), resourceType)
		}
		return nil, errfmt.Newf(ConstMiscFailedToAcquireLock).Wrap(err)
	}

	return &fileLockHandle{
		fileLock:     fileLock,
		lockPath:     lockPath,
		resourceType: resourceType,
		strategyName: s.Name(),
		acquiredAt:   time.Now(),
		cleanup: func() error {
			// Remove lock file after releasing lock to prevent accumulation
			if err := fileutil.Remove(lockPath); err != nil && !fileutil.IsNotExist(err) {
				// Non-critical cleanup - return nil to allow operation to succeed
				return nil
			}
			return nil
		},
	}, nil
}

// determineResourceType determines the resource type from the lock path
func (s *AutoCleanupStrategy) determineResourceType(lockPath string) string {
	// Extract resource type from path patterns
	base := filepath.Base(lockPath)
	dir := filepath.Dir(lockPath)

	// Check for index files
	if strings.Contains(base, ".index.lock") {
		return "index"
	}

	// Check for cache files
	if strings.Contains(dir, "cache") || strings.Contains(base, "cache") {
		return "cache"
	}

	// Check for system objects (internal directories)
	if strings.Contains(dir, paths.ProjectDataDir) || strings.Contains(dir, "_internal") {
		return "system_object"
	}

	// Check for public objects (process directory)
	if strings.Contains(dir, "process") {
		return "public_object"
	}

	// Default to generic file lock
	return "file"
}

// PersistentLockStrategy keeps lock files after release (for debugging/monitoring)
// Use this when you want to track lock usage patterns
type PersistentLockStrategy struct {
	StaleLockThreshold time.Duration
}

// NewPersistentLockStrategy creates a strategy that keeps lock files
func NewPersistentLockStrategy() *PersistentLockStrategy {
	return &PersistentLockStrategy{
		StaleLockThreshold: 1 * time.Minute,
	}
}

func (s *PersistentLockStrategy) Name() string {
	return "persistent"
}

func (s *PersistentLockStrategy) CleanupStaleLocks(lockPath string) error {
	// Still clean up stale locks, but keep active ones
	stat, err := fileutil.Stat(lockPath)
	if fileutil.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return errfmt.Newf(ConstMiscFailedToStatLockFile).Wrap(err)
	}

	age := time.Since(stat.ModTime())
	if age > s.StaleLockThreshold {
		// Remove only stale locks
		if err := fileutil.Remove(lockPath); err != nil && !fileutil.IsNotExist(err) {
			return errfmt.Newf(ConstMiscFailedToRemoveStaleLockFile).Wrap(err)
		}
	}

	return nil
}

func (s *PersistentLockStrategy) AcquireLock(lockPath string, timeout time.Duration) (LockHandle, error) {
	cleanupStart := time.Now()

	if err := s.CleanupStaleLocks(lockPath); err != nil {
		// Log but continue
	} else {
		cleanupTime := time.Since(cleanupStart)
		if cleanupTime > 0 {
			metrics := GetFileLockStrategyMetrics()
			metrics.RecordStaleCleanup(cleanupTime)
		}
	}

	resourceType := s.determineResourceType(lockPath)

	fileLock, err := NewFileLock(lockPath)
	if err != nil {
		metrics := GetFileLockStrategyMetrics()
		metrics.RecordFailure(s.Name(), resourceType)
		return nil, errfmt.Newf(ConstMiscFailedToCreateFileLock).Wrap(err)
	}

	if err := fileLock.LockWithTimeout(timeout); err != nil {
		_ = fileLock.Close()
		metrics := GetFileLockStrategyMetrics()
		if strings.Contains(err.Error(), "timeout") {
			metrics.RecordTimeout(s.Name(), resourceType)
		} else {
			metrics.RecordFailure(s.Name(), resourceType)
		}
		return nil, errfmt.Newf(ConstMiscFailedToAcquireLock).Wrap(err)
	}

	return &fileLockHandle{
		fileLock:     fileLock,
		lockPath:     lockPath,
		resourceType: resourceType,
		strategyName: s.Name(),
		acquiredAt:   time.Now(),
		cleanup: func() error {
			// Don't remove lock file - keep it for monitoring
			// Update modification time to indicate it's still active
			now := time.Now()
			var _err_82708973 = fileutil.Chtimes(lockPath, now, now)
			if //nolint:errcheck // Non-critical
			_err_82708973 != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

					// determineResourceType determines the resource type from the lock path
					Error(ErrMsgSwallowedError, _err_82708973).Log()

			}
			return nil
		},
	}, nil
}

func (s *PersistentLockStrategy) determineResourceType(lockPath string) string {
	base := filepath.Base(lockPath)
	dir := filepath.Dir(lockPath)

	if strings.Contains(base, ".index.lock") {
		return "index"
	}
	if strings.Contains(dir, "cache") || strings.Contains(base, "cache") {
		return "cache"
	}
	if strings.Contains(dir, paths.ProjectDataDir) || strings.Contains(dir, "_internal") {
		return "system_object"
	}
	if strings.Contains(dir, "process") {
		return "public_object"
	}
	return "file"
}

// WithFileLock executes a function while holding a file lock using the specified strategy
// This is a convenience function that ensures proper lock acquisition and cleanup
func WithFileLock(strategy FileLockStrategy, lockPath string, timeout time.Duration, fn func() error) error {
	handle, err := strategy.AcquireLock(lockPath, timeout)
	if err != nil {
		return errfmt.Newf(ConstMiscFailedToAcquireLock).Wrap(err)
	}
	defer func() {
		if releaseErr := handle.Release(); releaseErr != nil {
			// Log but don't fail - cleanup errors are non-critical
		}
	}()

	return fn()
}
