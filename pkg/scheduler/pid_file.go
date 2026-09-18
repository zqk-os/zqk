package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/mitchellh/go-ps"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const (
	// DefaultPIDFileName is the default name for the scheduler PID file
	DefaultPIDFileName = paths.SchedulerPIDFile
	// DefaultKeepAliveFileName is the default name for the scheduler keep-alive file
	DefaultKeepAliveFileName = paths.SchedulerKeepAliveFile
	// DefaultKeepAliveInterval is how often the scheduler should update the keep-alive file
	DefaultKeepAliveInterval = 30 * time.Second
	// DefaultKeepAliveTimeout is how long to wait before considering the daemon down
	DefaultKeepAliveTimeout = 2 * time.Minute
	// pidFileReadTimeout bounds PID/keep-alive file reads so status/start commands fail fast
	// instead of hanging indefinitely on blocked filesystem reads.
	pidFileReadTimeout = 2 * time.Second
	// pidFileRetryDelay is the pause before a single retry when the PID file read fails with
	// a transient error (timeout, contention) rather than file-not-found.
	pidFileRetryDelay = 150 * time.Millisecond
)

const (
	readFileWithTimeoutGoroutineName   = "scheduler_read_file_with_timeout"
	readFileWithTimeoutGoroutineReason = "read scheduler PID/keep-alive file with timeout cap"
	findSchedulerProcGoroutineName     = "scheduler_find_processes_by_command"
	findSchedulerProcGoroutineReason   = "scan processes for scheduler daemon candidates"
)

// readFileWithTimeout bounds file reads so command paths do not block forever if the
// underlying filesystem stalls. If timeout is hit, the caller gets a regular error and
// can continue with fallback behavior (e.g. assume not running and report diagnostics).
func readFileWithTimeout(path string, timeout time.Duration) ([]byte, error) {
	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	goroutinelabels.NewGoroutine(readFileWithTimeoutGoroutineName, readFileWithTimeoutGoroutineReason).
		StartSimple(func() {
			data, err := fileutil.ReadFile(path)
			done <- result{data: data, err: err}
		})

	ctx, cancel := context.WithTimeout(context.Background(), timeout) // Background: request-or-shutdown derived
	defer cancel()

	select {
	case r := <-done:
		return r.data, r.err
	case <-ctx.Done():
		return nil, errfmt.Errorf("timed out reading %s after %v", path, timeout)
	}
}

// ResolveProjectRootFromCWD walks up from the current working directory looking for
// a directory containing ProjectDataDir (.zqk). Used when the scheduler is started
// without a project root (e.g. by launchd) so the PID file and keep-alive are still written.
func ResolveProjectRootFromCWD() string {
	dir, err := fileutil.Getwd()
	if err != nil {
		return ""
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, paths.ProjectDataDir)); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// getPIDFilePath returns the path to the PID file
func getPIDFilePath(projectRoot string) string {
	return paths.SchedulerPIDFilePath(projectRoot)
}

// WritePIDFile writes the current process PID to the scheduler PID file.
// Exported so the CLI can register the daemon process early (before Scheduler.Start() runs),
// ensuring "scheduler status" sees the daemon even during slow initialization.
func WritePIDFile(projectRoot string) error {
	return writePIDFile(projectRoot)
}

// writePIDFile writes the current process PID to a file
func writePIDFile(projectRoot string) error {
	pidFilePath := getPIDFilePath(projectRoot)

	// Create scheduler directory if it doesn't exist
	schedulerDir := filepath.Dir(pidFilePath)
	if err := fileutil.MkdirAll(schedulerDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create scheduler directory").Wrap(err)
	}

	// Write PID to file
	pid := os.Getpid()
	pidStr := strconv.Itoa(pid)
	if err := fileutil.WriteFile(pidFilePath, []byte(pidStr), paths.FilePerm644); err != nil {
		return errfmt.Newf("failed to write PID file").Wrap(err)
	}

	return nil
}

// readPIDFile reads the PID from the PID file
func readPIDFile(projectRoot string) (int, error) {
	pidFilePath := getPIDFilePath(projectRoot)

	data, err := readFileWithTimeout(pidFilePath, pidFileReadTimeout)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return 0, errfmt.Errorf("PID file does not exist")
		}
		return 0, errfmt.Newf("failed to read PID file").Wrap(err)
	}

	pidStr := string(data)
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, errfmt.Newf("invalid PID in file").Wrap(err)
	}

	return pid, nil
}

// isPIDFileNotExist returns true when the error from readPIDFile indicates the PID file
// does not exist (as opposed to a transient failure like a timeout).
func isPIDFileNotExist(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "does not exist")
}

// RemovePIDFile removes the PID file (exported for external cleanup)
func RemovePIDFile(projectRoot string) error {
	return removePIDFile(projectRoot)
}

// removePIDFile removes the PID file
func removePIDFile(projectRoot string) error {
	pidFilePath := getPIDFilePath(projectRoot)

	if err := fileutil.Remove(pidFilePath); err != nil {
		if fileutil.IsNotExist(err) {
			return nil // File doesn't exist - that's fine
		}
		return errfmt.Newf("failed to remove PID file").Wrap(err)
	}

	return nil
}

// IsDaemonProcessAlive reports whether pid refers to a live OS process (not defunct/zombie).
// Prefer this over raw Signal(0) in CLI wait loops so behavior matches IsSchedulerRunning / stop paths.
func IsDaemonProcessAlive(pid int) bool {
	return IsProcessRunning(pid)
}

// IsProcessRunning checks if a process with the given PID is running
func IsProcessRunning(pid int) bool {
	// Send signal 0 to check if process exists
	// This doesn't actually send a signal, just checks if the process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, sending signal 0 checks if process exists
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		return false
	}
	// A zombie can still have a PID and pass signal(0); treat it as not running.
	if isZombieProcess(pid) {
		return false
	}
	return true
}

// isZombieProcess returns true when OS process state includes "Z" (zombie/defunct).
// Uses a short timeout to keep status/start/stop paths responsive.
func isZombieProcess(pid int) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond) // Background: request-or-shutdown derived
	defer cancel()
	out, err := execwrap.CommandContext(ctx, "ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output() //nolint:gosec // intentional command
	if err != nil {
		return false
	}
	return strings.Contains(strings.TrimSpace(string(out)), "Z")
}

// isSchedulerDaemonProcess checks if the given PID is running the scheduler start --foreground command.
// This allows us to definitively separate legitimate daemon process doubles from other transient zqk commands.
func isSchedulerDaemonProcess(pid int) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond) // Background: request-or-shutdown derived
	defer cancel()
	out, err := execwrap.CommandContext(ctx, "ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output() //nolint:gosec
	if err != nil {
		return false
	}
	cmdLine := string(out)
	// Only match --foreground to avoid the child process mistaking the parent ('zqk scheduler start')
	// for an already-running daemon during the detachment phase.
	return strings.Contains(cmdLine, "scheduler start --foreground")
}

// orphanScanTimeout limits how long the process-list scan may run so status never hangs.
const orphanScanTimeout = 2 * time.Second

// schedulerExecutableName is the executable name we look for when scanning for the daemon.
// go-ps returns the base name (e.g. "zqk"), not the full path.
const schedulerExecutableName = "zqk"

// findSchedulerProcessesByCommand returns PIDs of running processes whose executable looks like
// a scheduler binary. When projectRoot is non-empty, only PIDs whose command line embeds that
// root path (or ZQK_PROJECT_ROOT=root) are returned — never host-wide kill/adopt of other roots.
// Uses a timeout so status never blocks indefinitely on systems with many processes.
func findSchedulerProcessesByCommand(projectRoot string) (map[int]string, error) {
	if zqkenv.IsInTest() {
		// In tests, we do not want to scan the OS for real daemons,
		// otherwise tests using a fake TempDir projectRoot will falsely detect
		// the real daemon on the machine as an orphaned process for their fake root.
		return make(map[int]string), nil
	}

	currentPID := os.Getpid()
	rootNorm := strings.TrimRight(filepath.Clean(projectRoot), string(filepath.Separator))

	type result struct {
		m   map[int]string
		err error
	}
	done := make(chan result, 1)
	goroutinelabels.NewGoroutine(findSchedulerProcGoroutineName, findSchedulerProcGoroutineReason).
		StartSimple(func() {
			procs, err := ps.Processes()
			if err != nil {
				done <- result{nil, errfmt.Newf("failed to list processes").Wrap(err)}
				return
			}
			m := make(map[int]string)
			for _, p := range procs {
				pid := p.Pid()
				if pid == currentPID {
					continue
				}
				exe := p.Executable()
				base := filepath.Base(exe)
				if base != schedulerExecutableName && base != "zqk-stable" && base != "zqk-scheduler" {
					continue
				}
				if !IsProcessRunning(pid) {
					continue
				}
				if rootNorm == emptyValue || !processBelongsToProjectRoot(pid, rootNorm) {
					continue
				}
				m[pid] = exe
			}
			done <- result{m, nil}
		})

	ctx, cancel := context.WithTimeout(context.Background(), orphanScanTimeout) // Background: request-or-shutdown derived
	defer cancel()

	select {
	case r := <-done:
		return r.m, r.err
	case <-ctx.Done():
		return nil, errfmt.Errorf("process list timed out after %v", orphanScanTimeout)
	}
}

// processBelongsToProjectRoot reports whether pid's command line references this project root.
// Prefer matching env-style ZQK_PROJECT_ROOT=<root> or an absolute root path segment.
func processBelongsToProjectRoot(pid int, rootNorm string) bool {
	if rootNorm == emptyValue {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), orphanScanTimeout) // Background: request-or-shutdown derived
	defer cancel()
	out, err := execwrap.CommandContext(ctx, "ps", "-o", "command=", "-E", "-p", strconv.Itoa(pid)).Output() //nolint:gosec
	if err != nil {
		// Fallback without -E (Linux ps may use different flags); try plain command.
		out, err = execwrap.CommandContext(ctx, "ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output() //nolint:gosec
		if err != nil {
			return false
		}
	}
	cmdLine := string(out)
	if !strings.Contains(cmdLine, "scheduler start --foreground") {
		return false
	}
	projEnv := zqkenv.ProjectRoot().Name() + "=" + rootNorm
	if strings.Contains(cmdLine, projEnv) {
		return true
	}
	return strings.Contains(cmdLine, rootNorm)
}

// IsSchedulerRunning checks if the scheduler daemon is running by checking the PID file
// Also scans for orphaned processes that might not have a PID file
func IsSchedulerRunning(projectRoot string) (isRunning bool, pid int, err error) {
	pid, err = readPIDFile(projectRoot)
	if err != nil && !isPIDFileNotExist(err) {
		// Transient failure (timeout, contention) — retry once after a short delay
		// to avoid false negatives when the filesystem is momentarily slow.
		time.Sleep(pidFileRetryDelay)
		pid, err = readPIDFile(projectRoot)
	}
	if err == nil {
		// PID file exists - check if process is actually running
		if IsProcessRunning(pid) {
			return true, pid, nil
		}
		// Process is dead - clean up stale PID file and keep-alive file so status checks
		// and orphan scan (which uses recent keep-alive as daemon signal) stay accurate.
		_ = removePIDFile(projectRoot) //nolint:errcheck // Cleanup errors are non-critical
		if projectRoot != emptyValue {
			_ = RemoveKeepAlive(projectRoot) //nolint:errcheck // Cleanup errors are non-critical
		}
	}

	// PID file doesn't exist or points to dead process - scan for orphaned processes
	// This catches cases where scheduler was started without PID file tracking
	// Only scan if PID file doesn't exist (not if it was stale) to avoid false positives
	// when processes are starting up
	if err != nil {
		// PID file doesn't exist - safe to scan for orphaned processes
		orphanedProcs, scanErr := findSchedulerProcessesByCommand(projectRoot)
		if scanErr == nil && len(orphanedProcs) > 0 {
			// Found orphaned zqk processes - verify they are actually daemons by checking command line
			var legitDaemons []int
			for foundPID := range orphanedProcs {
				if IsProcessRunning(foundPID) && isSchedulerDaemonProcess(foundPID) {
					legitDaemons = append(legitDaemons, foundPID)
				}
			}

			if len(legitDaemons) > 1 {
				// Process doubles detected! Automatically resolve the collision by forcefully
				// killing the duplicates, keeping only the most recently spawned daemon (last in the list).
				// This robust self-healing ensures the system never deadlocks over file locks.
				survivorPID := legitDaemons[len(legitDaemons)-1]
				for _, duplicatePID := range legitDaemons {
					if duplicatePID != survivorPID {
						if p, err := os.FindProcess(duplicatePID); err == nil {
							_ = p.Kill() // Force kill the duplicate clone
						}
					}
				}

				// Update PID file to point to the surviving daemon
				pidFilePath := getPIDFilePath(projectRoot)
				schedulerDir := filepath.Dir(pidFilePath)
				if mkdirErr := fileutil.MkdirAll(schedulerDir, paths.DirPerm755); mkdirErr == nil {
					pidStr := strconv.Itoa(survivorPID)
					_ = fileutil.WriteFile(pidFilePath, []byte(pidStr), paths.FilePerm644) //nolint:errcheck // Best effort
				}
				return true, survivorPID, nil
			} else if len(legitDaemons) == 1 {
				foundPID := legitDaemons[0]
				// Found exactly one legitimate daemon. Update PID file to point to it.
				pidFilePath := getPIDFilePath(projectRoot)
				schedulerDir := filepath.Dir(pidFilePath)
				if mkdirErr := fileutil.MkdirAll(schedulerDir, paths.DirPerm755); mkdirErr == nil {
					pidStr := strconv.Itoa(foundPID)
					_ = fileutil.WriteFile(pidFilePath, []byte(pidStr), paths.FilePerm644) //nolint:errcheck // Best effort
				}
				return true, foundPID, nil
			}
		}
	}

	return false, 0, nil
}

// StopSchedulerShutdownWait is the default poll budget for WaitForSchedulerDaemonExit when the CLI
// passes a non-positive --max-wait. It is not a guarantee that graceful shutdown finishes within
// this window; if the process is still alive after maxWait, we retain the PID file so operators can
// retry stop, raise --max-wait, or use --force (see WaitForSchedulerDaemonExit).
const StopSchedulerShutdownWait = 3 * time.Minute

// WaitForSchedulerDaemonExit polls until pid is gone or maxWait elapses.
// When the process exits, it removes the PID file and returns nil.
// If maxWait elapses while the daemon is still alive, it returns an error and keeps the PID file
// so zqk scheduler stop --force / another --wait can still target the same PID (removing the file
// while the process runs made the daemon uncontrollable from the CLI).
// Uses IsDaemonProcessAlive so behavior matches CLI stop paths (zombies count as exited).
func WaitForSchedulerDaemonExit(projectRoot string, pid int, maxWait time.Duration) error {
	if maxWait <= 0 {
		maxWait = StopSchedulerShutdownWait
	}
	deadline := time.Now().Add(maxWait)
	pollInterval := 200 * time.Millisecond
	for time.Now().Before(deadline) {
		if !IsDaemonProcessAlive(pid) {
			_ = removePIDFile(projectRoot) //nolint:errcheck
			return nil
		}
		time.Sleep(pollInterval)
	}
	if !IsDaemonProcessAlive(pid) {
		_ = removePIDFile(projectRoot) //nolint:errcheck
		return nil
	}
	return errfmt.Errorf("scheduler daemon (PID %d) still running after %v wait; PID file kept — run zqk scheduler stop --force, or stop --wait --max-wait <longer>", pid, maxWait)
}

// StopSchedulerByPID stops the scheduler by sending SIGTERM to the process and waiting for it to exit.
// Polls until the process is gone or the wait budget elapses; removes the PID file only after confirming
// exit (or if the process is already gone). On timeout while still running, the PID file is kept.
func StopSchedulerByPID(projectRoot string) error {
	return StopSchedulerByPIDWithWait(projectRoot, StopSchedulerShutdownWait)
}

// StopSchedulerByPIDWithWait is like StopSchedulerByPID but uses maxWait as the poll budget.
// If maxWait <= 0, StopSchedulerShutdownWait is used.
func StopSchedulerByPIDWithWait(projectRoot string, maxWait time.Duration) error {
	pid, err := SignalSchedulerByPID(projectRoot, syscall.SIGTERM)
	if err != nil {
		return err
	}
	return WaitForSchedulerDaemonExit(projectRoot, pid, maxWait)
}

// SignalSchedulerByPID sends the provided signal to the scheduler daemon PID for projectRoot.
// Unlike StopSchedulerByPID, this does NOT wait for exit and does NOT remove PID/keep-alive files.
// This supports "initiate shutdown and return immediately" behavior for CLI UX (avoid misleading timeouts).
func SignalSchedulerByPID(projectRoot string, sig syscall.Signal) (pid int, err error) {
	running, pid, err := IsSchedulerRunning(projectRoot)
	if err != nil {
		return 0, err
	}
	if !running {
		return 0, fmt.Errorf("%w: scheduler daemon is not running", ErrSchedulerDown)
	}
	if err := RefuseSupervisorSelfStop(pid); err != nil {
		return pid, err
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return pid, errfmt.Errorf("failed to find process %d: %w", pid, err)
	}
	if err := process.Signal(sig); err != nil {
		return pid, errfmt.Errorf("failed to send signal %v to process %d: %w", sig, pid, err)
	}
	return pid, nil
}

// ForceKillSchedulerByPID sends SIGKILL to the scheduler daemon PID for projectRoot and removes PID/keep-alive files.
// Use when a graceful stop is not completing and an operator needs an abrupt shutdown.
func ForceKillSchedulerByPID(projectRoot string) error {
	pid, err := SignalSchedulerByPID(projectRoot, syscall.SIGKILL)
	if err != nil {
		return err
	}
	// Best effort cleanup so status/start don't keep seeing stale PID.
	_ = removePIDFile(projectRoot)   //nolint:errcheck
	_ = RemoveKeepAlive(projectRoot) //nolint:errcheck
	_ = pid                          // keep for future structured logging if needed
	return nil
}

// getKeepAliveFilePath returns the path to the keep-alive file
func getKeepAliveFilePath(projectRoot string) string {
	dataDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir)
	return filepath.Join(dataDir, DefaultKeepAliveFileName)
}

// writeKeepAlive writes the current timestamp to the keep-alive file
func writeKeepAlive(projectRoot string) error {
	keepAlivePath := getKeepAliveFilePath(projectRoot)

	// Create scheduler directory if it doesn't exist
	schedulerDir := filepath.Dir(keepAlivePath)
	if err := fileutil.MkdirAll(schedulerDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create scheduler directory").Wrap(err)
	}

	// Write current timestamp (RFC3339 format)
	timestamp := zqktime.NowRFC3339UTC()
	if err := fileutil.WriteFile(keepAlivePath, []byte(timestamp), paths.FilePerm644); err != nil {
		return errfmt.Newf("failed to write keep-alive file").Wrap(err)
	}

	return nil
}

// readKeepAlive reads the timestamp from the keep-alive file
func readKeepAlive(projectRoot string) (time.Time, error) {
	keepAlivePath := getKeepAliveFilePath(projectRoot)

	data, err := readFileWithTimeout(keepAlivePath, pidFileReadTimeout)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return time.Time{}, errfmt.Errorf("keep-alive file does not exist")
		}
		return time.Time{}, errfmt.Newf("failed to read keep-alive file").Wrap(err)
	}

	timestampStr := string(data)
	timestamp, err := time.Parse(time.RFC3339, timestampStr)
	if err != nil {
		return time.Time{}, errfmt.Newf("invalid timestamp in keep-alive file").Wrap(err)
	}

	return timestamp, nil
}

// RemoveKeepAlive removes the keep-alive file (exported for external cleanup)
func RemoveKeepAlive(projectRoot string) error {
	return removeKeepAlive(projectRoot)
}

// removeKeepAlive removes the keep-alive file
func removeKeepAlive(projectRoot string) error {
	keepAlivePath := getKeepAliveFilePath(projectRoot)

	if err := fileutil.Remove(keepAlivePath); err != nil {
		if fileutil.IsNotExist(err) {
			return nil // File doesn't exist - that's fine
		}
		return errfmt.Newf("failed to remove keep-alive file").Wrap(err)
	}

	return nil
}

// IsSchedulerAlive checks if the scheduler daemon is alive by checking the keep-alive file
// Returns: (isAlive, lastKeepAlive, error)
// If keep-alive is within DefaultKeepAliveTimeout, the daemon is considered alive
func IsSchedulerAlive(projectRoot string) (bool, time.Time, error) {
	// First check if scheduler is running via PID file
	running, _, err := IsSchedulerRunning(projectRoot)
	if err != nil {
		return false, time.Time{}, err
	}
	if !running {
		// Clean up stale keep-alive file if it exists
		_ = removeKeepAlive(projectRoot) //nolint:errcheck // Cleanup errors are non-critical
		return false, time.Time{}, nil
	}

	// Check keep-alive file
	lastKeepAlive, err := readKeepAlive(projectRoot)
	if err != nil {
		// Keep-alive file doesn't exist or can't be read
		return false, time.Time{}, nil
	}

	// Check if keep-alive is recent enough
	now := time.Now().UTC()
	age := now.Sub(lastKeepAlive)
	if age > DefaultKeepAliveTimeout {
		// Keep-alive is stale - daemon may be hung or dead
		return false, lastKeepAlive, nil
	}

	return true, lastKeepAlive, nil
}
