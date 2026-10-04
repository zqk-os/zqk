package ambient

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/ambient"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/daemon/singleton"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scheduler/hostservice"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

var (
	ambientPIDFileRel = filepath.Join(paths.ProjectDataDir, paths.StateDir, "ambient", "daemon.pid")
	ambientLogFileRel = filepath.Join(paths.ProjectDataDir, paths.LogsDir, "ambient-daemon.log")
)

func ambientPIDFilePath(projectRoot string) string {
	return filepath.Join(projectRoot, ambientPIDFileRel)
}

func ambientLogFilePath(projectRoot string) string {
	return filepath.Join(projectRoot, ambientLogFileRel)
}

func findAmbientPIDsFromProcessTable(projectRoot string) []int {
	cleanRoot := filepath.Clean(projectRoot)
	out, err := execwrap.Command("ps", "-eo", "pid,args").Output()
	if err != nil {
		return nil
	}
	var pids []int
	lines := strings.Split(string(out), "\n")
	myPID := os.Getpid()
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "PID") {
			continue
		}
		if !strings.Contains(line, "ambient daemon") {
			continue
		}
		if !strings.Contains(line, cleanRoot) {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		pid, err := strconv.Atoi(parts[0])
		if err != nil || pid <= 0 || pid == myPID {
			continue
		}
		if proc, err := os.FindProcess(pid); err == nil {
			if err := proc.Signal(syscall.Signal(0)); err == nil {
				pids = append(pids, pid)
			}
		}
	}
	return pids
}

func findRunningAmbientPIDs(projectRoot string) []int {
	seen := make(map[int]bool)
	var pids []int
	addPID := func(pid int) {
		if pid > 0 && pid != os.Getpid() && !seen[pid] {
			if proc, err := os.FindProcess(pid); err == nil {
				if err := proc.Signal(syscall.Signal(0)); err == nil {
					seen[pid] = true
					pids = append(pids, pid)
				}
			}
		}
	}

	if running, pid, err := singleton.IsDaemonRunning(projectRoot, "ambient"); err == nil && running {
		addPID(pid)
	}
	pidPath := ambientPIDFilePath(projectRoot)
	if data, err := fileutil.ReadFile(pidPath); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			addPID(pid)
		}
	}
	for _, pid := range findAmbientPIDsFromProcessTable(projectRoot) {
		addPID(pid)
	}
	return pids
}

func readAmbientPID(projectRoot string) (int, bool) {
	if running, pid, err := singleton.IsDaemonRunning(projectRoot, "ambient"); err == nil && running {
		return pid, true
	}
	pidPath := ambientPIDFilePath(projectRoot)
	if data, err := fileutil.ReadFile(pidPath); err == nil {
		pidStr := strings.TrimSpace(string(data))
		if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
			if process, err := os.FindProcess(pid); err == nil {
				if err := process.Signal(syscall.Signal(0)); err == nil {
					return pid, true
				}
			}
			// Stale PID file
			_ = fileutil.Remove(pidPath)
		}
	}
	// Fallback: check process table for live ambient daemon belonging to projectRoot
	if pids := findAmbientPIDsFromProcessTable(projectRoot); len(pids) > 0 {
		pid := pids[0]
		_ = fileutil.EnsureDir(filepath.Dir(pidPath))
		_ = fileutil.WriteFile(pidPath, []byte(strconv.Itoa(pid)), paths.FilePerm644)
		return pid, true
	}
	return 0, false
}

func newDaemonCmd() *cobra.Command {
	var projectRoot string
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run the ambient filesystem and heuristics daemon in the foreground",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(projectRoot) == "" {
				projectRoot = cli.ResolveProjectRoot(".")
			}
			return runAmbientDaemon(cmd, projectRoot)
		},
	}
	cmd.Flags().StringVar(&projectRoot, "project-root", "", "Project root directory to monitor")
	return cmd
}

func runAmbientDaemon(cmd *cobra.Command, projectRoot string) error {
	cli.TouchMeaningfulActivity()

	if strings.TrimSpace(projectRoot) == "" {
		projectRoot = cli.ResolveProjectRoot(".")
	}
	if projectRoot == "" {
		return fmt.Errorf("project root not found")
	}

	cleanRoot := filepath.Clean(projectRoot)

	// Enforce single instance of ambient daemon per project root
	releaseLock, err := singleton.Guard(cleanRoot, "ambient")
	if err != nil {
		var alreadyRunning *singleton.ErrDaemonAlreadyRunning
		if errors.As(err, &alreadyRunning) {
			_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Ambient daemon is already running for %s (PID: %d). Existing instance retained.\n", cleanRoot, alreadyRunning.PID)))
			return nil
		}
		return err
	}
	defer releaseLock()

	// Terminate any stale orphaned ambient daemons running for this project root (e.g. from wiped workspace)
	currentPID := os.Getpid()
	for _, oldPID := range findRunningAmbientPIDs(cleanRoot) {
		if oldPID != currentPID {
			if oldProc, err := os.FindProcess(oldPID); err == nil {
				_ = oldProc.Signal(syscall.SIGTERM)
			}
		}
	}

	pidPath := ambientPIDFilePath(cleanRoot)
	if err := fileutil.EnsureDir(filepath.Dir(pidPath)); err != nil {
		return fmt.Errorf("failed to create ambient state directory: %w", err)
	}

	if err := fileutil.WriteFile(pidPath, []byte(strconv.Itoa(currentPID)), paths.FilePerm644); err != nil {
		return fmt.Errorf("failed to write ambient daemon PID: %w", err)
	}
	defer func() {
		_ = fileutil.Remove(pidPath)
	}()

	_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Starting ZQK Ambient Daemon (PID: %d)...\nMonitoring: %s\n", currentPID, cleanRoot)))

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	// Watchdog: If the project lock file is unlinked or removed from disk, exit cleanly.
	lockPath := singleton.LockFilePath(cleanRoot, "ambient")
	goroutinelabels.NewGoroutine("ambient_watchdog", "clean shutdown if lock file is removed").StartSimple(func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := fileutil.Stat(lockPath); err != nil {
					cancel()
					return
				}
			}
		}
	})

	return ambient.RunWatcherDaemon(ctx, cleanRoot, cli.TouchMeaningfulActivity)
}

func newStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the ambient daemon in the background",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot := cli.ResolveProjectRoot(".")
			return StartDaemon(projectRoot, cmd.OutOrStdout())
		},
	}
	return cmd
}

func newEnsureCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ensure",
		Short: "Ensure the ambient daemon is running (idempotent)",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot := cli.ResolveProjectRoot(".")
			return EnsureDaemon(projectRoot, nil)
		},
	}
	return cmd
}

func newStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the background ambient daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot := cli.ResolveProjectRoot(".")
			return StopDaemon(projectRoot)
		},
	}
	return cmd
}

// StartDaemon starts the ambient daemon in the background
func StartDaemon(projectRoot string, out interface{ Write([]byte) (int, error) }) error {
	if pid, ok := readAmbientPID(projectRoot); ok {
		if out != nil {
			_, _ = fmt.Fprintf(out, "Ambient daemon is already running (PID: %d)\n", pid)
		}
		return nil
	}

	exe := hostservice.ResolveServiceDaemonBinary(projectRoot, "amb")

	// Setsid + --timeout 0 from *.test orphans grandchildren onto PID 1.
	// Refuse here; tests that need a daemon must exec the product CLI and t.Cleanup(StopDaemon).
	if refuseDetachedAmbientSpawn(exe) {
		if out != nil {
			_, _ = fmt.Fprintf(out, "Skipping ambient daemon spawn from test process (%s)\n", filepath.Base(exe))
		}
		return nil
	}

	logPath := ambientLogFilePath(projectRoot)
	if err := fileutil.EnsureDir(filepath.Dir(logPath)); err != nil {
		return fmt.Errorf("failed to create ambient log directory: %w", err)
	}
	logFile, err := fileutil.OpenAppend(logPath)
	if err != nil {
		return fmt.Errorf("failed to open ambient log file: %w", err)
	}

	daemonCmd := execwrap.Command(exe, "ambient", "daemon", "--project-root", projectRoot)
	daemonCmd.Dir = projectRoot
	daemonCmd.Stdout = logFile
	daemonCmd.Stderr = logFile
	daemonCmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := daemonCmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("failed to spawn ambient daemon: %w", err)
	}
	_ = logFile.Close()

	// Wait up to 2 seconds for PID file to appear
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := readAmbientPID(projectRoot); ok {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if out != nil {
		_, _ = fmt.Fprintf(out, "Started ambient daemon (PID: %d)\nLogs: %s\n", daemonCmd.Process.Pid, logPath)
	}
	return nil
}

// EnsureDaemon ensures the ambient daemon is running in the background without noisy output
func EnsureDaemon(projectRoot string, logger logging.Logger) error {
	if projectRoot == "" {
		return nil
	}
	if pid, ok := readAmbientPID(projectRoot); ok {
		if logger != nil {
			logging.Fluent(logger).Debug("ambient daemon already running").Int("pid", pid).Log()
		}
		return nil
	}
	return StartDaemon(projectRoot, nil)
}

// StopDaemon stops the running ambient daemon
func StopDaemon(projectRoot string) error {
	logger := logging.GetLoggerFromProfile("cli")
	pids := findRunningAmbientPIDs(projectRoot)
	if len(pids) == 0 {
		logging.Fluent(logger).Info("Ambient daemon is not running.").Log()
		return nil
	}

	for _, pid := range pids {
		if process, err := os.FindProcess(pid); err == nil {
			_ = process.Signal(syscall.SIGTERM)
		}
	}

	// Wait up to 3 seconds for clean exit
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		allStopped := true
		for _, pid := range pids {
			if proc, err := os.FindProcess(pid); err == nil {
				if err := proc.Signal(syscall.Signal(0)); err == nil {
					allStopped = false
					break
				}
			}
		}
		if allStopped {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Force kill if still alive
	for _, pid := range pids {
		if proc, err := os.FindProcess(pid); err == nil {
			if err := proc.Signal(syscall.Signal(0)); err == nil {
				_ = proc.Signal(syscall.SIGKILL)
			}
		}
	}

	_ = fileutil.Remove(ambientPIDFilePath(projectRoot))
	lockPath := singleton.LockFilePath(projectRoot, "ambient")
	_ = fileutil.Remove(lockPath)

	for _, pid := range pids {
		logging.Fluent(logger).Info(fmt.Sprintf("Stopped ambient daemon (PID: %d).", pid)).Log()
	}
	return nil
}

func refuseDetachedAmbientSpawn(exe string) bool {
	if testing.Testing() || zqkenv.IsInTest() {
		return true
	}
	return zqkenv.IsGoTestBinary(exe)
}
