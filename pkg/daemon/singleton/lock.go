package singleton

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/utils/syscallutil"
)

// ErrDaemonAlreadyRunning indicates that a daemon instance is already active for this project root.
type ErrDaemonAlreadyRunning struct {
	DaemonName  string
	ProjectRoot string
	PID         int
	LockPath    string
}

func (e *ErrDaemonAlreadyRunning) Error() string {
	if e.PID > 0 {
		return fmt.Sprintf("daemon %q is already running for project root %q (PID: %d)", e.DaemonName, e.ProjectRoot, e.PID)
	}
	return fmt.Sprintf("daemon %q is already running for project root %q", e.DaemonName, e.ProjectRoot)
}

// DaemonLock represents an exclusive lock held by an active daemon for a specific project root.
// The lock uses non-blocking advisory file locking (flock) which is automatically released
// by the operating system kernel when the owning process exits or terminates.
type DaemonLock struct {
	DaemonName  string
	ProjectRoot string
	LockPath    string
	PID         int
	file        *fileutil.File
}

// LockFilePath returns the canonical lock file path for a named daemon under projectRoot.
func LockFilePath(projectRoot string, daemonName string) string {
	cleanRoot := filepath.Clean(projectRoot)
	cleanName := strings.ToLower(strings.TrimSpace(daemonName))
	return filepath.Join(cleanRoot, paths.ProjectDataDir, paths.StateDir, "daemon_locks", cleanName+".lock")
}

// AcquireDaemonLock attempts to acquire an exclusive lock for daemonName under projectRoot.
// If another instance of the daemon is already running, it returns *ErrDaemonAlreadyRunning.
// Invariant: only a single instance of any particular daemon may run per project root.
func AcquireDaemonLock(projectRoot string, daemonName string) (*DaemonLock, error) {
	if strings.TrimSpace(projectRoot) == "" {
		return nil, errors.New("project root cannot be empty")
	}
	if strings.TrimSpace(daemonName) == "" {
		return nil, errors.New("daemon name cannot be empty")
	}

	cleanRoot := filepath.Clean(projectRoot)
	cleanName := strings.ToLower(strings.TrimSpace(daemonName))

	// Invariant: project roots must not be nested
	if err := paths.ValidateProjectRootNesting(cleanRoot); err != nil {
		return nil, fmt.Errorf("daemon lock refused: %w", err)
	}

	lockPath := LockFilePath(cleanRoot, cleanName)
	if err := fileutil.EnsureDir(filepath.Dir(lockPath)); err != nil {
		return nil, fmt.Errorf("failed to create daemon lock directory: %w", err)
	}

	f, err := fileutil.OpenFile(lockPath, fileutil.O_CREATE|fileutil.O_RDWR, paths.FilePerm644)
	if err != nil {
		return nil, fmt.Errorf("failed to open daemon lock file %s: %w", lockPath, err)
	}

	// Non-blocking exclusive lock: fails immediately if held by another live process
	flockErr := syscallutil.FileFlock(f, syscall.LOCK_EX|syscall.LOCK_NB)
	if flockErr != nil {
		existingPID := parseLockPID(f)
		if closeErr := f.Close(); closeErr != nil {
			// file close error during error exit
		}
		return nil, &ErrDaemonAlreadyRunning{
			DaemonName:  cleanName,
			ProjectRoot: cleanRoot,
			PID:         existingPID,
			LockPath:    lockPath,
		}
	}

	// Acquired exclusive lock: write ownership metadata
	currentPID := os.Getpid()
	if truncErr := f.Truncate(0); truncErr != nil {
		if closeErr := f.Close(); closeErr != nil {
			// ignore close error during cleanup
		}
		return nil, fmt.Errorf("failed to truncate daemon lock file: %w", truncErr)
	}
	if _, seekErr := f.Seek(0, 0); seekErr != nil {
		if closeErr := f.Close(); closeErr != nil {
			// ignore close error during cleanup
		}
		return nil, fmt.Errorf("failed to seek daemon lock file: %w", seekErr)
	}
	metadata := fmt.Sprintf("pid: %d\ndaemon: %s\nproject_root: %s\nstarted_at: %s\n",
		currentPID, cleanName, cleanRoot, time.Now().UTC().Format(time.RFC3339))
	if _, writeErr := f.WriteString(metadata); writeErr != nil {
		if closeErr := f.Close(); closeErr != nil {
			// ignore close error during cleanup
		}
		return nil, fmt.Errorf("failed to write daemon lock metadata: %w", writeErr)
	}
	if syncErr := f.Sync(); syncErr != nil {
		if closeErr := f.Close(); closeErr != nil {
			// ignore close error during cleanup
		}
		return nil, fmt.Errorf("failed to sync daemon lock metadata: %w", syncErr)
	}

	return &DaemonLock{
		DaemonName:  cleanName,
		ProjectRoot: cleanRoot,
		LockPath:    lockPath,
		PID:         currentPID,
		file:        f,
	}, nil
}

// Release unlocks and cleans up the daemon lock file.
func (l *DaemonLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	var firstErr error
	if unlockErr := syscallutil.FileFlock(l.file, syscall.LOCK_UN); unlockErr != nil && firstErr == nil {
		firstErr = unlockErr
	}
	if closeErr := l.file.Close(); closeErr != nil && firstErr == nil {
		firstErr = closeErr
	}
	if remErr := fileutil.Remove(l.LockPath); remErr != nil && !os.IsNotExist(remErr) && firstErr == nil {
		firstErr = remErr
	}
	l.file = nil
	return firstErr
}

// RunGuarded executes fn while holding an exclusive singleton daemon lock for daemonName under projectRoot.
// It guarantees deterministic lock acquisition, wraps any error with consistent error formatting,
// and ensures the lock is safely released on return or panic.
func RunGuarded(projectRoot string, daemonName string, fn func() error) error {
	lock, err := AcquireDaemonLock(projectRoot, daemonName)
	if err != nil {
		return errfmt.Errorf("acquire %s daemon lock: %w", daemonName, err)
	}
	defer func() {
		if relErr := lock.Release(); relErr != nil {
			// lock release failure during deferred cleanup
		}
	}()
	return fn()
}

// Guard acquires an exclusive singleton daemon lock for daemonName under projectRoot and returns a release function.
// If acquisition fails, a wrapped error is returned.
func Guard(projectRoot string, daemonName string) (func(), error) {
	lock, err := AcquireDaemonLock(projectRoot, daemonName)
	if err != nil {
		return nil, errfmt.Errorf("acquire %s daemon lock: %w", daemonName, err)
	}
	return func() {
		if relErr := lock.Release(); relErr != nil {
			// lock release failure
		}
	}, nil
}

// IsDaemonRunning checks whether daemonName is actively running under projectRoot
// by testing if its flock is held. It does not modify lock contents.
func IsDaemonRunning(projectRoot string, daemonName string) (bool, int, error) {
	if strings.TrimSpace(projectRoot) == "" || strings.TrimSpace(daemonName) == "" {
		return false, 0, nil
	}
	lockPath := LockFilePath(projectRoot, daemonName)
	if _, err := fileutil.Stat(lockPath); err != nil {
		return false, 0, nil
	}

	f, err := fileutil.OpenFile(lockPath, fileutil.O_RDWR, 0)
	if err != nil {
		f, err = fileutil.OpenFile(lockPath, fileutil.O_RDONLY, 0)
		if err != nil {
			return false, 0, nil
		}
	}
	defer f.Close()

	if err := syscallutil.FileFlock(f, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		// Advisory lock is held by a live process
		pid := parseLockPID(f)
		return true, pid, nil
	}

	// Lock was acquired -> no active daemon holding it
	if unlockErr := syscallutil.FileFlock(f, syscall.LOCK_UN); unlockErr != nil {
		// unlock failure on cleanup
	}
	return false, 0, nil
}

// parseLockPID reads the PID from an open lock file.
func parseLockPID(f *fileutil.File) int {
	if _, seekErr := f.Seek(0, 0); seekErr != nil {
		return 0
	}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "pid:") {
			pidStr := strings.TrimSpace(strings.TrimPrefix(line, "pid:"))
			if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
				return pid
			}
		}
	}
	return 0
}

// ActiveDaemonPIDs returns a map of daemonName -> PID for all actively held daemon locks under projectRoot.
func ActiveDaemonPIDs(projectRoot string) (map[string]int, error) {
	if strings.TrimSpace(projectRoot) == "" {
		return make(map[string]int), nil
	}
	locksDir := filepath.Join(filepath.Clean(projectRoot), paths.ProjectDataDir, paths.StateDir, "daemon_locks")
	entries, err := fileutil.ReadDir(locksDir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return make(map[string]int), nil
		}
		return nil, err
	}

	active := make(map[string]int)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lock") {
			continue
		}
		daemonName := strings.TrimSuffix(entry.Name(), ".lock")
		running, pid, _ := IsDaemonRunning(projectRoot, daemonName)
		if running && pid > 0 {
			active[daemonName] = pid
		}
	}
	return active, nil
}
