package ambient

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/ambient"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objectidcache"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	ambientPIDFileRel = ".zqk/state/ambient/daemon.pid"
	ambientLogFileRel = ".zqk/logs/ambient-daemon.log"
)

func ambientPIDFilePath(projectRoot string) string {
	return filepath.Join(projectRoot, ambientPIDFileRel)
}

func ambientLogFilePath(projectRoot string) string {
	return filepath.Join(projectRoot, ambientLogFileRel)
}

func readAmbientPID(projectRoot string) (int, bool) {
	pidPath := ambientPIDFilePath(projectRoot)
	data, err := fileutil.ReadFile(pidPath)
	if err != nil {
		return 0, false
	}
	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid <= 0 {
		return 0, false
	}
	// Check if process is alive
	process, err := os.FindProcess(pid)
	if err != nil {
		return 0, false
	}
	if err := process.Signal(syscall.Signal(0)); err == nil {
		return pid, true
	}
	// Stale PID file
	_ = fileutil.Remove(pidPath)
	return 0, false
}

func newDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run the ambient filesystem and heuristics daemon in the foreground",
		RunE:  runAmbientDaemon,
	}
	return cmd
}

func runAmbientDaemon(cmd *cobra.Command, args []string) error {
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == "" {
		return fmt.Errorf("project root not found")
	}

	pidPath := ambientPIDFilePath(projectRoot)
	if err := fileutil.EnsureDir(filepath.Dir(pidPath)); err != nil {
		return fmt.Errorf("failed to create ambient state directory: %w", err)
	}

	currentPID := os.Getpid()
	if err := fileutil.WriteFile(pidPath, []byte(strconv.Itoa(currentPID)), 0644); err != nil {
		return fmt.Errorf("failed to write ambient daemon PID: %w", err)
	}
	defer func() {
		_ = fileutil.Remove(pidPath)
	}()

	_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Starting ZQK Ambient Daemon (PID: %d)...\nMonitoring: %s\n", currentPID, projectRoot)))

	ctx := cmd.Context()
	hub := ambient.NewEventHub()

	// Initialize heuristic processors and writers
	_ = ambient.NewArtifactWriter(hub)
	_ = ambient.NewCoachHeuristicsWithRoot(hub, projectRoot)

	// Initialize ambient ingest service
	secCtx := pkgctx.NewSecurityContext(pkgctx.SystemAccountID, []string{"system"}, []string{"*"})
	ingestService := ambient.NewAmbientIngestService(projectRoot, secCtx)
	ingestService.BindToHub(hub)

	// Create and start the FSWatcher
	watcher, err := ambient.NewFSWatcher(projectRoot, hub)
	if err != nil {
		return fmt.Errorf("failed to create fswatcher: %w", err)
	}

	// Invalidate object cache on filesystem events
	hub.Subscribe(ambient.EventTypeFilesystem, func(c context.Context, event ambient.Event) error {
		payloadMap, ok := event.Payload.(map[string]any)
		if !ok {
			return nil
		}
		target, ok := payloadMap[objects.FieldKeyTargetID].(string)
		if !ok {
			return nil
		}
		op, _ := payloadMap[objects.FieldKeyOperation].(string)

		if !strings.Contains(target, paths.ProcessDir+"/") || !strings.HasSuffix(target, ".yaml") {
			return nil
		}

		parts := strings.Split(target, string(filepath.Separator))
		for i, part := range parts {
			if part == "process" && i+2 < len(parts) {
				kind := parts[i+1]
				filename := parts[len(parts)-1]
				id := strings.TrimSuffix(filename, ".yaml")

				if op == "REMOVE" {
					objectidcache.InvalidateObjectIDCache(id)
				} else if op == "WRITE" || op == "CREATE" {
					_ = objectidcache.UpdateObjectIDCache(id, kind, target)
				}
				break
			}
		}
		return nil
	})

	if err := watcher.Start(ctx); err != nil {
		return fmt.Errorf("failed to start fswatcher: %w", err)
	}

	<-ctx.Done()
	return nil
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

	exe, err := os.Executable()
	if err != nil {
		exe = "zqk"
	}

	logPath := ambientLogFilePath(projectRoot)
	if err := fileutil.EnsureDir(filepath.Dir(logPath)); err != nil {
		return fmt.Errorf("failed to create ambient log directory: %w", err)
	}
	logFile, err := fileutil.OpenAppend(logPath)
	if err != nil {
		return fmt.Errorf("failed to open ambient log file: %w", err)
	}

	daemonCmd := execwrap.Command(exe, "ambient", "daemon")
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
	pid, ok := readAmbientPID(projectRoot)
	if !ok {
		fmt.Println("Ambient daemon is not running.")
		return nil
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		_ = fileutil.Remove(ambientPIDFilePath(projectRoot))
		fmt.Println("Ambient daemon is not running.")
		return nil
	}

	_ = process.Signal(syscall.SIGTERM)

	// Wait up to 3 seconds for clean exit
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := process.Signal(syscall.Signal(0)); err != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Force kill if still alive
	if err := process.Signal(syscall.Signal(0)); err == nil {
		_ = process.Signal(syscall.SIGKILL)
	}

	_ = fileutil.Remove(ambientPIDFilePath(projectRoot))
	fmt.Printf("Stopped ambient daemon (PID: %d).\n", pid)
	return nil
}
