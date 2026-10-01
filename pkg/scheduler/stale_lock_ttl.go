package scheduler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	// DefaultStaleLockTTL is the default duration after which an unattended lock is considered stale.
	DefaultStaleLockTTL = 120 * time.Second

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
type LockMetadata struct {
	PID       int       `json:"pid"`
	CreatedAt time.Time `json:"created_at"`
	JobID     string    `json:"job_id,omitempty"`
}

// IsProcessAlive checks whether the given PID represents a live running process.
func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix/POSIX, Signal(0) tests whether the process exists without delivering a signal.
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if err == syscall.EPERM {
		// Process exists but we do not have permission to signal it. It is alive.
		return true
	}
	return false
}

// ReadLockMetadata parses LockMetadata from a lock file, supporting both JSON and plaintext PID formats.
func ReadLockMetadata(path string) (*LockMetadata, error) {
	data, err := fileutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == emptyValue {
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
		return errfmt.Errorf(msgErrWriteLockMetadata, err)
	}
	writeErr := fileutil.WriteFile(path, data, paths.FilePerm644)
	if writeErr != nil {
		return errfmt.Errorf(msgErrWriteLockMetadata, writeErr)
	}
	return nil
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
