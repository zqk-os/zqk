package file

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/utils/syscallutil"
)

// DefaultStaleLockTTL is the default duration after which an unattended lock is considered stale.
const DefaultStaleLockTTL = 5 * time.Minute

// LockMetadata captures provenance information stored inside a file lock.
type LockMetadata struct {
	PID       int       `json:"pid"`
	CreatedAt time.Time `json:"created_at"`
	JobID     string    `json:"job_id,omitempty"`
}

// ReadLockMetadata parses LockMetadata from a lock file, supporting both JSON and plaintext PID formats.
func ReadLockMetadata(path string) (*LockMetadata, error) {
	data, err := fileutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return &LockMetadata{}, nil
	}

	var meta LockMetadata
	if jsonErr := json.Unmarshal(data, &meta); jsonErr == nil && meta.PID != 0 {
		return &meta, nil
	}

	// Plain integer fallback
	if pid, parseErr := strconv.Atoi(trimmed); parseErr == nil && pid > 0 {
		return &LockMetadata{
			PID: pid,
		}, nil
	}

	return &LockMetadata{}, nil
}

// WriteLockMetadata writes metadata into the lock file.
func WriteLockMetadata(path string, meta *LockMetadata) error {
	if meta == nil {
		return nil
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return fileutil.WriteFile(path, data, paths.FilePerm644)
}

// FileLockConfig allows configuring file lock behavior
type FileLockConfig struct {
	// EarlyBailoutThreshold is the maximum wait time before suggesting early bailout
	// If set to 0, early bailout is disabled
	EarlyBailoutThreshold time.Duration

	// EnableMetrics enables metrics collection (default: true)
	EnableMetrics bool

	// StaleLockTTL is the duration after which an unattended lock is considered stale.
	// Default is DefaultStaleLockTTL (5m).
	StaleLockTTL time.Duration
}

// FileLock provides cross-process file locking using flock
// This ensures atomic operations on shared file-based resources
type FileLock struct {
	file     *fileutil.File
	filePath string
	mu       sync.Mutex // Protects file operations within this process
	locked   bool
	config   FileLockConfig
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
	if config.StaleLockTTL == 0 {
		config.StaleLockTTL = DefaultStaleLockTTL
	}

	return &FileLock{
		file:     file,
		filePath: filePath,
		locked:   false,
		config:   config,
	}, nil
}

func (fl *FileLock) initLockMetrics() (time.Time, *FileLockMetrics) {
	start := time.Now()
	metrics := GetFileLockMetrics()
	if fl.config.EnableMetrics {
		metrics.RecordContentionAttempt()
	}
	return start, metrics
}

// Lock acquires an exclusive lock on the file (blocking)
// Returns an error if the lock cannot be acquired
func (fl *FileLock) Lock() error {
	start, metrics := fl.initLockMetrics()

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

			// Try non-blocking acquisition first to allow stale lock reclamation
			err := syscallutil.FileFlock(fl.file, syscallutil.LockEx|syscallutil.LockNB)
			if err == nil {
				fl.recordLockAcquired(start, metrics)
				return nil
			}
			if errors.Is(err, syscallutil.EWouldBlock) {
				if fl.config.StaleLockTTL > 0 && fl.reclaimIfStale() {
					fl.recordLockAcquired(start, metrics)
					return nil
				}
			}

			// Use flock for advisory locking (works across processes)
			err = syscallutil.FileFlock(fl.file, syscallutil.LockEx)
			if err != nil {
				if fl.config.EnableMetrics {
					metrics.RecordFailure()
				}
				return errfmt.Newf(ConstMiscFailedToAcquireFileLock).Wrap(err)
			}

			fl.recordLockAcquired(start, metrics)
			return nil
		},
	)
}

func (fl *FileLock) writeOwnershipMetadata() {
	if fl.file == nil {
		return
	}
	meta := LockMetadata{
		PID:       os.Getpid(),
		CreatedAt: time.Now().UTC(),
	}
	data, err := json.Marshal(&meta)
	if err != nil {
		return
	}
	data = append(data, '\n')
	_ = fl.file.Truncate(0)
	_, _ = fl.file.Seek(0, 0)
	_, _ = fl.file.Write(data)
	_ = fl.file.Sync()
}

func (fl *FileLock) clearOwnershipMetadata() {
	if fl.file == nil {
		return
	}
	_ = fl.file.Truncate(0)
	_ = fl.file.Sync()
}

func (fl *FileLock) recordLockAcquired(start time.Time, metrics *FileLockMetrics) {
	if fl.config.EnableMetrics {
		metrics.RecordAcquisition(time.Since(start))
		metrics.IncrementHolders()
	}
	fl.locked = true
	fl.writeOwnershipMetadata()
}

// reclaimIfStale checks if the lock file at fl.filePath is stale (dead owning PID or age > StaleLockTTL)
// and if so, breaks and reclaims the lock. Returns true if successfully reclaimed.
func (fl *FileLock) reclaimIfStale() bool {
	if fl.filePath == "" || fl.config.StaleLockTTL <= 0 {
		return false
	}

	info, err := fileutil.Stat(fl.filePath)
	if err != nil {
		return false
	}

	meta, readErr := ReadLockMetadata(fl.filePath)
	isDeadProcess := false
	if readErr == nil && meta != nil && meta.PID > 0 {
		if !syscallutil.IsProcessAlive(meta.PID) {
			isDeadProcess = true
		}
	}

	now := time.Now()
	lockTime := info.ModTime()
	if meta != nil && !meta.CreatedAt.IsZero() {
		lockTime = meta.CreatedAt
	}
	isExpired := fl.config.StaleLockTTL > 0 && now.Sub(lockTime) > fl.config.StaleLockTTL

	if !isDeadProcess && !isExpired {
		return false
	}

	// Break and reclaim the lock by removing the stale lock file
	_ = fileutil.Remove(fl.filePath)
	_ = fl.file.Close()

	newFile, err := fileutil.OpenFile(fl.filePath, fileutil.O_CREATE|fileutil.O_RDWR, paths.FilePerm644)
	if err != nil {
		return false
	}
	fl.file = newFile

	lockErr := syscallutil.FileFlock(fl.file, syscallutil.LockEx|syscallutil.LockNB)
	return lockErr == nil
}

// TryLock attempts to acquire an exclusive lock (non-blocking)
// Returns true if the lock was acquired, false if it's already held
func (fl *FileLock) TryLock() (bool, error) {
	start, metrics := fl.initLockMetrics()

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
			err := syscallutil.FileFlock(fl.file, syscallutil.LockEx|syscallutil.LockNB)
			if err != nil {
				if errors.Is(err, syscallutil.EWouldBlock) {
					if fl.config.StaleLockTTL > 0 && fl.reclaimIfStale() {
						fl.recordLockAcquired(start, metrics)
						acquired = true
						return nil
					}
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

			fl.recordLockAcquired(start, metrics)
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
	start, metrics := fl.initLockMetrics()

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

			fl.clearOwnershipMetadata()
			unlockErr = syscallutil.FileFlock(fl.file, syscallutil.LockUn)
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
				fl.clearOwnershipMetadata()
				var _err_82483302 = syscallutil.FileFlock(fl.file, syscallutil.LockUn)
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
