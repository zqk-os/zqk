package scheduler

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// daemonStdioCleanupWait is the max time to wait for stdout/stderr copy goroutines
// during shutdown. If copy goroutines are blocked on disk I/O or pipe write,
// we proceed to close files and exit so the process does not hang (sample showed
// main thread stuck in copyWg.Wait() after "daemon shutting down").
const daemonStdioCleanupWait = 5 * time.Second

// DaemonStdioRedirectResult holds the result of setting up daemon stdio redirect.
// Caller must call Cleanup when the daemon is shutting down.
// CoordinatorWriter is the writer to use for router.AddDestination (structured log events);
// it is closed by Cleanup.
type DaemonStdioRedirectResult struct {
	Cleanup           func()
	CoordinatorWriter io.Writer
}

// SetupDaemonStdioRedirect redirects os.Stdout and os.Stderr to rolling log files.
// Always uses combined mode: daemon.stdio (combined stdout+stderr) and diagnostics.jsonl (structured events only).
//
// Returns a result with Cleanup (call on shutdown) and CoordinatorWriter (for router events).
// If setup fails, Cleanup is nil and CoordinatorWriter is nil.
func SetupDaemonStdioRedirect(projectRoot string) DaemonStdioRedirectResult {
	daemonLogDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir)
	if err := fileutil.EnsureDir(daemonLogDir); err != nil {
		return DaemonStdioRedirectResult{}
	}

	policy := logging.DefaultRollingPolicy()
	factory := logging.NewRollingWriterFactory()
	return setupCombinedDaemonStdio(daemonLogDir, factory, policy)
}

// startCopyGoroutine starts a single io.Copy goroutine with the standard budget and wait group pattern.
func startCopyGoroutine(wg *sync.WaitGroup, name, purpose string, dst io.Writer, src io.Reader) {
	bud := goroutinelabels.DefaultBudget()
	builder := goroutinelabels.NewGoroutine(name, purpose).WithWaitGroup(wg)
	if bud != nil {
		builder = builder.WithBudget(bud)
	}
	builder.StartSimple(func() { _, _ = io.Copy(dst, src) })
}

func setupCombinedDaemonStdio(daemonLogDir string, factory *logging.RollingWriterFactory, policy logging.RollingPolicy) DaemonStdioRedirectResult {
	// Events-only file for router (diagnostics.jsonl)
	daemonLogPath := filepath.Join(daemonLogDir, "diagnostics.jsonl")
	legacyDaemonLogPath := filepath.Join(daemonLogDir, "scheduler-events.json")
	_ = fileutil.Rename(legacyDaemonLogPath, daemonLogPath)
	daemonLog, errLog := factory.CreateWriter(daemonLogPath, policy)
	if errLog != nil {
		return DaemonStdioRedirectResult{}
	}
	// Combined stdout+stderr stream (daemon.stdio)
	daemonStdioPath := filepath.Join(daemonLogDir, "daemon.stdio")
	daemonStdio, errStdio := factory.CreateWriter(daemonStdioPath, policy)
	if errStdio != nil {
		_ = daemonLog.Close()
		return DaemonStdioRedirectResult{}
	}

	stdoutR, stdoutW, _ := os.Pipe()
	stderrR, stderrW, _ := os.Pipe()

	// CRITICAL: Use syscall.Dup2 to override OS supervisor redirections (e.g., launchd StandardOutPath/StandardErrorPath).
	// This ensures file descriptors 1 and 2 point to our pipes so we write to daemon.stdio and diagnostics.jsonl.
	// The daemon does not create or use daemon.log; if that file exists (e.g. from an old plist), it may be empty—use daemon.stdio in plist instead.
	// Note: syscall.Dup2 is only available on Unix systems (macOS, Linux); scheduler is not expected to run on Windows
	closeAndAbort := func() DaemonStdioRedirectResult {
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		_ = stderrR.Close()
		_ = stderrW.Close()
		_ = daemonStdio.Close()
		_ = daemonLog.Close()
		return DaemonStdioRedirectResult{}
	}
	if runtime.GOOS != "windows" {
		if err := dupToStdFD(stdoutW, 1); err != nil {
			return closeAndAbort()
		}
		if err := dupToStdFD(stderrW, 2); err != nil {
			return closeAndAbort()
		}
	}

	// Update Go's os.Stdout/os.Stderr to match the redirected file descriptors
	os.Stdout = stdoutW
	os.Stderr = stderrW

	var copyWg sync.WaitGroup
	startCopyGoroutine(&copyWg, "scheduler_daemon_stdout_copy", "copying stdout to daemon.stdio", daemonStdio, stdoutR)
	startCopyGoroutine(&copyWg, "scheduler_daemon_stderr_copy", "copying stderr to daemon.stdio", daemonStdio, stderrR)

	cleanup := func() {
		_ = stdoutW.Close()
		_ = stderrW.Close()
		// Bounded wait so shutdown completes even if copy goroutines are stuck (e.g. blocked on disk I/O).
		done := make(chan struct{})
		goroutinelabels.NewGoroutine("scheduler_daemon", "cleanup stdio copy goroutines").
			StartSimple(func() {
				copyWg.Wait()
				close(done)
			})
		select {
		case <-done:
			// Copy goroutines finished
		case <-time.After(daemonStdioCleanupWait):
			// Timeout: proceed so process can exit; copy goroutines may still be running
		}
		_ = daemonStdio.Close()
		_ = daemonLog.Close()
	}

	return DaemonStdioRedirectResult{Cleanup: cleanup, CoordinatorWriter: daemonLog}
}
