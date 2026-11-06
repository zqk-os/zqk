package scheduler

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/utils/syscallutil"
)

const (
	// DefaultStaleLockTTL is the default duration after which an unattended lock is considered stale.
	DefaultStaleLockTTL = storagepkg.DefaultStaleLockTTL

	msgErrEmptyLockPath          = "lock path cannot be empty"
	msgErrAcquireLockWithTimeout = "failed to acquire lock within timeout: %w"
	msgErrCreateFileLock         = "failed to create file lock: %w"
	msgErrReadLockDir            = "failed to read lock directory: %w"
	msgErrStatLockFile           = "failed to stat lock file: %w"
	msgErrWriteLockMetadata      = "failed to write lock metadata: %w"
	msgErrRemoveStaleLock        = "failed to remove stale lock file: %w"
	msgErrLockAlreadyLocked      = "lock already acquired"
)

// LockMetadata captures provenance information stored inside a self-healing lock file.
type LockMetadata = storagepkg.LockMetadata

// IsProcessAlive checks whether the given PID represents a live running process.
func IsProcessAlive(pid int) bool {
	return syscallutil.IsProcessAlive(pid)
}

// ReadLockMetadata parses LockMetadata from a lock file, supporting both JSON and plaintext PID formats.
func ReadLockMetadata(path string) (*LockMetadata, error) {
	return storagepkg.ReadLockMetadata(path)
}

// WriteLockMetadata writes metadata into the lock file.
func WriteLockMetadata(path string, meta *LockMetadata) error {
	return storagepkg.WriteLockMetadata(path, meta)
}

// ReconcileStaleLock checks whether the lock file at path is stale (older than TTL or dead PID)
// and cleans it up if no other live process holds the flock.
// Returns true if a stale lock was safely removed.
func ReconcileStaleLock(path string, ttl time.Duration) (bool, error) {
	if strings.TrimSpace(path) == emptyValue {
		return false, errfmt.Errorf(msgErrEmptyLockPath)
	}
	info, err := fileutil.Stat(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return false, nil
		}
		return false, errfmt.Errorf(msgErrStatLockFile, err)
	}

	if ttl <= 0 {
		ttl = DefaultStaleLockTTL
	}

	now := time.Now()
	age := now.Sub(info.ModTime())

	meta, readMetaErr := ReadLockMetadata(path)
	isDeadProcess := false
	if readMetaErr == nil && meta != nil && meta.PID > 0 {
		if !IsProcessAlive(meta.PID) {
			isDeadProcess = true
		}
	}

	isOlderThanTTL := age > ttl
	if !isOlderThanTTL && !isDeadProcess {
		// Lock is neither expired nor owned by a dead process
		return false, nil
	}

	// Try acquiring non-blocking file lock to verify no active process holds the kernel flock.
	fl, err := storagepkg.NewFileLock(path)
	if err != nil {
		return false, nil
	}

	acquired, lockErr := fl.TryLock()
	if !acquired || lockErr != nil {
		closeErr := fl.Close()
		if closeErr != nil {
			return false, nil
		}
		return false, nil
	}

	// Lock acquired: safely unlink stale file
	removeErr := fileutil.Remove(path)
	unlockErr := fl.Unlock()
	closeErr := fl.Close()
	if unlockErr != nil || closeErr != nil {
		// Log or ignore lock release failure on unlinked file
	}

	if removeErr != nil && !fileutil.IsNotExist(removeErr) {
		return false, errfmt.Errorf(msgErrRemoveStaleLock, removeErr)
	}

	return true, nil
}

// CleanStaleLocksWithPID scans dir and removes all stale .lock files considering both age and PID liveness.
func CleanStaleLocksWithPID(dir string, ttl time.Duration) (int, error) {
	if strings.TrimSpace(dir) == emptyValue {
		return 0, errfmt.Errorf("%s", msgLockDirRequired)
	}
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return 0, nil
		}
		return 0, errfmt.Errorf(msgErrReadLockDir, err)
	}

	cleanedCount := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), lockFileSuffix) {
			continue
		}
		fullPath := filepath.Join(dir, e.Name())
		cleaned, recErr := ReconcileStaleLock(fullPath, ttl)
		if recErr == nil && cleaned {
			cleanedCount++
		}
	}
	return cleanedCount, nil
}

// AcquireSelfHealingLock reconciles any stale lock at path before acquiring an exclusive lock.
// Upon successful acquisition, it records current process PID into the lock file.
func AcquireSelfHealingLock(path string, timeout time.Duration, ttl time.Duration) (*storagepkg.FileLock, error) {
	if strings.TrimSpace(path) == emptyValue {
		return nil, errfmt.Errorf(msgErrEmptyLockPath)
	}

	if _, recErr := ReconcileStaleLock(path, ttl); recErr != nil {
		// Proceed to lock acquisition attempt
	}

	fl, err := storagepkg.NewFileLock(path)
	if err != nil {
		return nil, errfmt.Errorf(msgErrCreateFileLock, err)
	}

	if timeout > 0 {
		if lockErr := fl.LockWithTimeout(timeout); lockErr != nil {
			closeErr := fl.Close()
			if closeErr != nil {
				return nil, errfmt.Errorf(msgErrAcquireLockWithTimeout, lockErr)
			}
			return nil, errfmt.Errorf(msgErrAcquireLockWithTimeout, lockErr)
		}
	} else {
		if lockErr := fl.Lock(); lockErr != nil {
			closeErr := fl.Close()
			if closeErr != nil {
				return nil, lockErr
			}
			return nil, lockErr
		}
	}

	// Write ownership metadata
	meta := &LockMetadata{
		PID:       os.Getpid(),
		CreatedAt: time.Now(),
	}
	if writeErr := WriteLockMetadata(path, meta); writeErr != nil {
		// Non-critical ownership telemetry error
	}

	return fl, nil
}
