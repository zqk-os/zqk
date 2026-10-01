package scheduler

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	clicontext "github.com/zqk-os/zqk/pkg/cliapp/context"
	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/scheduler/hostservice"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// startDetachedSchedulerDaemonProcess starts execCmd and asynchronously waits on it so the child is always
// reaped by this process if it exits while the parent CLI is still alive (slow PostRun/metrics teardown).
// Without Wait(), a short-lived daemon (immediate crash) becomes a zombie until the parent exits.
//
// The happy path is a long-running daemon: Wait() blocks in the background goroutine until stop/crash.
func startDetachedSchedulerDaemonProcess(execCmd *exec.Cmd) error {
	// Use os.StartProcess directly instead of execCmd.Start() to avoid Go's
	// exec.Cmd internal pipe bookkeeping. When the parent CLI process exits,
	// exec.Cmd's goroutines and finalizers tear down I/O state, which can
	// send SIGPIPE to the child and kill it. os.StartProcess + Release() is
	// the correct pattern for fire-and-forget daemon children.
	argv := append([]string{execCmd.Args[0]}, execCmd.Args[1:]...)
	attr := &os.ProcAttr{
		Dir:   execCmd.Dir,
		Env:   execCmd.Env,
		Files: []*fileutil.File{execCmd.Stdin.(*fileutil.File), execCmd.Stdout.(*fileutil.File), execCmd.Stderr.(*fileutil.File)},
		Sys:   execCmd.SysProcAttr,
	}
	proc, err := os.StartProcess(execCmd.Path, argv, attr)
	if err != nil {
		return err
	}
	// Release immediately: the child is in its own session (Setsid: true) with
	// all stdio pointing to /dev/null. It will be reparented to init/launchd
	// and reaped by the OS when it exits.
	if err := proc.Release(); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		schedulerpkg.SLog(logger).Debug("Failed to release detached scheduler daemon process").WithError(err).Log()
	}
	return nil
}

// attachSchedulerDaemonStdioToDevNull sets stdout/stderr to os.DevNull copies so the child never inherits
// the parent's terminal or pipe fds. Inheriting a blocked pipe/socket can strand new zqk processes in
// _dyld_start (vm_object lock cascade on macOS); see cmd/zqk/app/root.go watchdog comments.
func attachSchedulerDaemonStdioToDevNull(execCmd *exec.Cmd) (func(), error) {
	dupOut, err := fileutil.OpenAppend(filepath.Join(paths.ProjectDataDir, "daemon_crash.log"))
	if err != nil {
		return nil, errfmt.Errorf("open daemon_crash.log for scheduler daemon stdout/stderr: %w", err)
	}
	execCmd.Stdout = dupOut
	execCmd.Stderr = dupOut
	return func() {
		if err := dupOut.Close(); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			schedulerpkg.SLog(logger).Debug("Failed to close daemon_crash.log for detached daemon").WithError(err).Log()
		}
	}, nil
}

func rejectHostServiceOwnedBackgroundStart(projectRoot string) error {
	serviceEntry, err := hostservice.ResolveEntry(projectRoot)
	if err == nil {
		return errfmt.Errorf(
			schedulerErrHostServiceOwnsRootFmt,
			serviceEntry.UnitLabel,
			projectRoot,
			projectRoot,
		)
	}
	return nil
}

// startSchedulerInBackground starts the scheduler daemon in a detached child process and returns immediately.
// Default for "zqk scheduler start" is background; the child is invoked with --foreground so it runs startScheduler (no double-fork).
func startSchedulerInBackground(ctx *cli.Context, cmd *cobra.Command) error {
	projectRoot := resolveSchedulerCLIProjectRoot(ctx)
	if projectRoot == emptyValue {
		return errors.New(schedulerErrProjectRootNotFound)
	}
	if abs, err := filepath.Abs(projectRoot); err == nil {
		projectRoot = abs
	}
	if err := rejectHostServiceOwnedBackgroundStart(projectRoot); err != nil {
		return err
	}

	if _, errLoad := clicontext.LoadBrandSettings(projectRoot); errLoad != nil {
		return errfmt.Errorf(schedulerErrBrandSettingsRequired, errLoad)
	}

	if err := schedulerpkg.RemoveNoAutoRestartFile(projectRoot); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		schedulerpkg.SLog(logger).Debug("Failed to remove no-auto-restart file during background start").WithError(err).Log()
	}

	// Same prechecks as foreground start
	_, errRunning := requireSchedulerNotRunning(ctx)
	if errRunning != nil {
		return errRunning
	}

	daemonExe, err := schedulerpkg.ResolveSchedulerDaemonBinary(projectRoot)
	if err != nil {
		return err
	}

	// Child must run in foreground (the actual daemon); otherwise it would default to background and fork again.
	args := detachedDaemonStartArgs(daemonExe)

	minimal, errMinimal := cmd.Flags().GetBool("minimal")
	if errMinimal != nil {
		return errfmt.Newf("failed to parse 'minimal' flag").Wrap(errMinimal)
	}
	if minimal {
		args = append(args, "--minimal")
	}

	testID, _ := cmd.Flags().GetString("test-id")
	if testID != "" {
		args = append(args, "--test-id="+testID)
	}

	execCmd := execwrap.Command(daemonExe, args...)
	// NOTE: Do NOT override execCmd.Args[0] to "zqk-scheduler".
	// The brand system derives env var prefix from Args[0] (e.g. "zqk-scheduler" -> ZQK_SCHEDULER_),
	// which makes the child look for ZQK_SCHEDULER_API_KEY instead of ZQK_API_KEY, causing auth failure.
	//
	// NOTE: Do NOT use WireExecForIsolatedProject — it sets ZQK_TEST_ROOT / ZQK_TEST_BYPASS_AUTH
	// which makes the child resolve test configuration instead of project configuration.
	// The daemon is a production process that should inherit the parent's full environment.
	execCmd.Dir = projectRoot
	execCmd.Env = scrubDaemonInheritEnv(append(os.Environ(),
		zqkenv.ProjectRoot().Name()+"="+projectRoot,
		zqkenv.SchedulerDaemonMode().Name()+"=1",
	))

	// Stdin: detach from terminal.
	// IMPORTANT: We must NOT close these /dev/null fds until the child process has fully
	// started and written its PID file. On macOS, closing the parent's copy of an fd that
	// was inherited by the child can invalidate the child's fd (they share the same file
	// description entry in the kernel). If the child tries to write to stdout/stderr after
	// the parent closes the fd, it receives SIGPIPE and is killed. We collect closers and
	// run them after the child verification sleep below.
	devNull, err := fileutil.Open(fileutil.DevNull)
	if err != nil {
		return errfmt.Errorf(schedulerErrOpenFmt, fileutil.DevNull, err)
	}
	execCmd.Stdin = devNull

	closeDup, err := attachSchedulerDaemonStdioToDevNull(execCmd)
	if err != nil {
		_ = devNull.Close() //nolint:errcheck // best-effort cleanup on error path
		return err
	}

	// Defer fd cleanup: runs AFTER the child verification sleep (line ~873+),
	// giving the child time to detach its own stdio during initialization.
	defer func() {
		closeDup()
		if err := devNull.Close(); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			schedulerpkg.SLog(logger).Debug("Failed to close devNull for detached daemon").WithError(err).Log()
		}
	}()

	if runtime.GOOS != schedulerGOOSWindows {
		execCmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}

	if err := startDetachedSchedulerDaemonProcess(execCmd); err != nil {
		return errfmt.Newf("failed to start scheduler in background").Wrap(err)
	}

	// Give child time to start and write PID file.
	// 2 seconds allows for slow initialization under memory pressure or heavy CPU load.
	select {
	case <-cmd.Context().Done():
		return cmd.Context().Err()
	case <-time.After(2 * time.Second):
	}
	running, pid, errRunningCheck := schedulerpkg.IsSchedulerRunning(projectRoot)
	if errRunningCheck != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		schedulerpkg.SLog(logger).Warn("Failed to verify if scheduler is running after background start").WithError(errRunningCheck).Log()
	}
	daemonLogPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir, schedulerEventsFileName)

	// Verify process is actually still running before reporting PID
	// (process might have crashed immediately after writing PID file).
	// IsSchedulerRunning now treats zombie/defunct PIDs as not running.
	if running && pid > 0 {
		// Process exists and is running. Write with max wait so we never block on broken/full pipe (concurrency.RunWithMaxWait).
		jobLogsPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, paths.SchedulerJobLogsSubdir, "<job-id>")
		concurrency.RunWithMaxWait(func() {
			var b strings.Builder
			if _, err := fmt.Fprintf(&b, "Scheduler started in background (PID: %d).\n", pid); err != nil {
				return
			}
			if _, err := fmt.Fprintf(&b, "  Daemon binary: %s\n", daemonExe); err != nil {
				return
			}
			if _, err := fmt.Fprintf(&b, "  Scheduler events: %s (e.g. tail -f %s)\n", daemonLogPath, daemonLogPath); err != nil {
				return
			}
			if _, err := fmt.Fprintf(&b, "  Per-job logs: %s/<job-id>.log\n", jobLogsPath); err != nil {
				return
			}
			b.WriteString(schedulerStatusHintLine)
			if err := cli.WriteOutput(cmd, []byte(b.String())); err != nil {
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				schedulerpkg.SLog(logger).Debug("Failed to write scheduler status output").WithError(err).Log()
			}
		}, 2*time.Second)
		return nil
	}

	// Process not running or crashed - check daemon log for errors (max wait to avoid blocking)
	concurrency.RunWithMaxWait(func() {
		var b strings.Builder
		if _, err := fmt.Fprintf(&b, "Scheduler started in background but may have exited. Check scheduler events: %s\n", daemonLogPath); err != nil {
			return
		}
		b.WriteString(schedulerStatusHintLine)
		if err := cli.WriteOutput(cmd, []byte(b.String())); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			schedulerpkg.SLog(logger).Debug("Failed to write scheduler exit warning").WithError(err).Log()
		}
	}, 2*time.Second)
	return nil
}

// StartSchedulerDaemonForRoot starts the scheduler daemon for the given project root in a detached child process.
// Used by the use command after switching roots so the new root's scheduler runs without user intervention.
// If the scheduler is already running for projectRoot, returns nil. Does not write to cmd output.
func StartSchedulerDaemonForRoot(projectRoot string) error {
	running, _, errRunning := schedulerpkg.IsSchedulerRunning(projectRoot)
	if errRunning != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		schedulerpkg.SLog(logger).Warn("Failed to check if scheduler is running for root").WithError(errRunning).Log()
	}
	if running {
		return nil
	}
	if err := schedulerpkg.RemoveNoAutoRestartFile(projectRoot); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		schedulerpkg.SLog(logger).Debug("Failed to remove no-auto-restart file during root transition").WithError(err).Log()
	}
	daemonExe, err := schedulerpkg.ResolveSchedulerDaemonBinary(projectRoot)
	if err != nil {
		return err
	}
	execCmd := execwrap.Command(daemonExe, detachedDaemonStartArgs(daemonExe)...)

	// Pass down --test-id if it was provided to the parent process
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "--test-id=") {
			execCmd.Args = append(execCmd.Args, arg)
			break
		}
	}

	// Pass down --session-id if provided via environment to track spawned daemons
	if sessionID := zqkenv.SessionID().Get(); sessionID != "" {
		execCmd.Args = append(execCmd.Args, "--session-id="+sessionID)
	}

	// NOTE: Do NOT override Args[0] or use WireExecForIsolatedProject —
	// see startSchedulerInBackground comment about brand prefix and test settings.
	execCmd.Dir = projectRoot
	execCmd.Env = scrubDaemonInheritEnv(append(os.Environ(),
		zqkenv.ProjectRoot().Name()+"="+projectRoot,
		zqkenv.SchedulerDaemonMode().Name()+"=1",
	))

	devNull, err := fileutil.Open(fileutil.DevNull)
	if err != nil {
		return errfmt.Errorf(schedulerErrOpenFmt, fileutil.DevNull, err)
	}
	execCmd.Stdin = devNull
	closeDup, err := attachSchedulerDaemonStdioToDevNull(execCmd)
	if err != nil {
		_ = devNull.Close() //nolint:errcheck // best-effort cleanup on error path
		return err
	}
	// Defer fd cleanup until after child process has started and detached its own stdio.
	defer func() {
		closeDup()
		if err := devNull.Close(); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			schedulerpkg.SLog(logger).Debug("Failed to close devNull during root transition").WithError(err).Log()
		}
	}()
	if runtime.GOOS != schedulerGOOSWindows {
		execCmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}
	if err := startDetachedSchedulerDaemonProcess(execCmd); err != nil {
		return errfmt.Newf("failed to start scheduler for new root").Wrap(err)
	}
	return nil
}
