package file

import (
	"errors"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage/locknames"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/utils/syscallutil"
)

// FileLockConfig allows configuring file lock behavior
type FileLockConfig struct {
	// EarlyBailoutThreshold is the maximum wait time before suggesting early bailout
	// If set to 0, early bailout is disabled
	EarlyBailoutThreshold time.Duration

	// EnableMetrics enables metrics collection (default: true)
	EnableMetrics bool
}

// FileLock provides cross-process file locking using flock
// This ensures atomic operations on shared file-based resources
type FileLock struct {
	file   *fileutil.File
	mu     sync.Mutex // Protects file operations within this process
	locked bool
	config FileLockConfig
}

// NewFileLock creates a new file lock for the given file path
// The file will be created if it doesn't exist
func NewFileLock(filePath string) (*FileLock, error) {
	return NewFileLockWithConfig(filePath, FileLockConfig{
		EnableMetrics: true,
	})
}

// NewFileLockWithConfig creates a new file lock with custom configuration
func NewFileLockWithConfig(filePath string, config FileLockConfig) (*FileLock, error) {
	// Create directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return nil, errfmt.Newf(ConstMiscFailedToCreateLockDirectory).Wrap(err)
	}

	// Open or create the lock file
	file, err := fileutil.OpenFile(filePath, fileutil.O_CREATE|fileutil.O_RDWR, paths.FilePerm644)
	if err != nil {
		return nil, errfmt.Newf(ConstMiscFailedToOpenLockFile).Wrap(err)
	}

	// Set defaults
	if config.EarlyBailoutThreshold == 0 {
		config.EarlyBailoutThreshold = 0 // Disabled by default
	}

	return &FileLock{
		file:   file,
		locked: false,
		config: config,
	}, nil
}

// Lock acquires an exclusive lock on the file (blocking)
// Returns an error if the lock cannot be acquired
func (fl *FileLock) Lock() error {
	start := time.Now()
	metrics := GetFileLockMetrics()
	if fl.config.EnableMetrics {
		metrics.RecordContentionAttempt()
	}

	return concurrency.RunInLockWithLogger(
		&fl.mu,
		locknames.LockNameFileLockLock,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if fl.locked {
				if fl.config.EnableMetrics {
					metrics.RecordFailure()
				}
				return errfmt.Errorf(ConstMiscFileLockAlreadyHeld)
			}

			// Use flock for advisory locking (works across processes)
			err := syscallutil.FileFlock(fl.file, unix.LOCK_EX)
			if err != nil {
				if fl.config.EnableMetrics {
					metrics.RecordFailure()
				}
				return errfmt.Newf(ConstMiscFailedToAcquireFileLock).Wrap(err)
			}

			acquisitionTime := time.Since(start)
			if fl.config.EnableMetrics {
				metrics.RecordAcquisition(acquisitionTime)
				metrics.IncrementHolders()
			}

			fl.locked = true
			return nil
		},
	)
}

// TryLock attempts to acquire an exclusive lock (non-blocking)
// Returns true if the lock was acquired, false if it's already held
func (fl *FileLock) TryLock() (bool, error) {
	start := time.Now()
	metrics := GetFileLockMetrics()
	if fl.config.EnableMetrics {
		metrics.RecordContentionAttempt()
	}

	var acquired bool
	var lockErr error
	err := concurrency.RunInLockWithLogger(
		&fl.mu, locknames.LockNameFileLockTryLock, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if fl.locked {
				if fl.config.EnableMetrics {
					metrics.RecordFailure()
				}
				lockErr = errfmt.Errorf(ConstMiscFileLockAlreadyHeld)
				return nil
			}

			// Use LOCK_NB (non-blocking) flag
			err := syscallutil.FileFlock(fl.file, unix.LOCK_EX|unix.LOCK_NB)
			if err != nil {
				if errors.Is(err, unix.EWOULDBLOCK) {
					if fl.config.EnableMetrics {
						metrics.RecordContention()
					}
					return nil // Lock is held by another process
				}
				if fl.config.EnableMetrics {
					metrics.RecordFailure()
				}
				lockErr = errfmt.Newf(ConstMiscFailedToAcquireFileLock).Wrap(err)
				return nil
			}

			acquisitionTime := time.Since(start)
			if fl.config.EnableMetrics {
				metrics.RecordAcquisition(acquisitionTime)
				metrics.IncrementHolders()
			}

			fl.locked = true
			acquired = true
			return nil
		},
	)
	if err != nil {
		return false, err
	}
	if lockErr != nil {
		return false, lockErr
	}
	return acquired, nil
}

// LockWithTimeout attempts to acquire a lock with a timeout
// Returns an error if the lock cannot be acquired within the timeout
// If EarlyBailoutThreshold is configured and exceeded, returns an error suggesting early bailout
func (fl *FileLock) LockWithTimeout(timeout time.Duration) error {
	start := time.Now()
	metrics := GetFileLockMetrics()
	if fl.config.EnableMetrics {
		metrics.RecordContentionAttempt()
	}

	deadline := time.Now().Add(timeout)
	backoff := 5 * time.Millisecond
	maxBackoff := 100 * time.Millisecond

	for {
		acquired, err := fl.TryLock()
		if err != nil && err.Error() != ConstMiscFileLockAlreadyHeld {
			if fl.config.EnableMetrics {
				metrics.RecordFailure()
			}
			return err
		}
		if acquired {
			// Record wait time (time spent before successful acquisition)
			waitTime := time.Since(start)
			if fl.config.EnableMetrics {
				metrics.RecordWaitTime(waitTime)
			}
			return nil
		}

		// Check for early bailout threshold
		elapsed := time.Since(start)
		if fl.config.EarlyBailoutThreshold > 0 && elapsed > fl.config.EarlyBailoutThreshold {
			if fl.config.EnableMetrics {
				metrics.RecordTimeout(elapsed)
			}
			return errfmt.Errorf(ConstMiscEarlyBailoutLockContentionDetectedAfterV, elapsed, fl.config.EarlyBailoutThreshold)
		}

		if time.Now().After(deadline) {
			waitTime := time.Since(start)
			if fl.config.EnableMetrics {
				metrics.RecordTimeout(waitTime)
			}
			return errfmt.Errorf(ConstMiscTimeoutWaitingForFileLockAfterV, timeout)
		}

		time.Sleep(backoff)
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// Unlock releases the file lock
func (fl *FileLock) Unlock() error {
	metrics := GetFileLockMetrics()

	var unlockErr error
	err := concurrency.RunInLockWithLogger(
		&fl.mu,
		locknames.LockNameFileLockUnlock,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if !fl.locked {
				return errfmt.Errorf(ConstMiscFileLockNotHeld)
			}

			unlockErr = syscallutil.FileFlock(fl.file, unix.LOCK_UN)
			if unlockErr != nil {
				return errfmt.Newf(ConstMiscFailedToReleaseFileLock).Wrap(unlockErr)
			}

			if fl.config.EnableMetrics {
				metrics.DecrementHolders()
			}
			fl.locked = false
			return nil
		},
	)
	if err != nil {
		return err
	}
	return unlockErr
}

// Close closes the lock file
// This also releases the lock if it's still held
func (fl *FileLock) Close() error {
	metrics := GetFileLockMetrics()

	err := concurrency.RunInLockWithLogger(
		&fl.mu,
		locknames.LockNameFileLockClose,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if fl.locked {
				var _err_82483302 = syscallutil.FileFlock(fl.file, unix.LOCK_UN)
				if //nolint:errcheck // Lock cleanup errors are non-critical
				_err_82483302 != nil {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_82483302).Log()
				}
				if fl.config.EnableMetrics {
					metrics.DecrementHolders()
				}
				fl.locked = false
			}
			return nil
		},
	)
	if err != nil {
		return err
	}

	return fl.file.Close()
}

// IsLocked returns whether the lock is currently held
func (fl *FileLock) IsLocked() bool {
	var locked bool
	_ = concurrency.RunInLockOrLog(
		&fl.mu,
		locknames.LockNameFileLockIsLocked,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			locked = fl.locked
			return nil
		},
	)
	return locked
}

// WithLock executes a function while holding the file lock
// This is a convenience method that ensures proper lock/unlock
func (fl *FileLock) WithLock(fn func() error) error {
	if err := fl.Lock(); err != nil {
		return err
	}
	defer func() {
		var _err_82484171 = fl.Unlock()
		if _err_82484171 != //nolint:errcheck // Lock cleanup errors are non-critical
			nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_82484171).Log()
		}
	}()
	return fn()
}

// WithLockTimeout executes a function while holding the file lock (with timeout)
func (fl *FileLock) WithLockTimeout(timeout time.Duration, fn func() error) error {
	if err := fl.LockWithTimeout(timeout); err != nil {
		return err
	}
	defer func() {
		var _err_82484516 = fl.Unlock()
		if _err_82484516 != //nolint:errcheck // Lock cleanup errors are non-critical
			nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_82484516).Log()
		}
	}()
	return fn()
}
