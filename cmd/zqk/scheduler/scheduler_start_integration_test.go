package scheduler

// BLI-177483 inventory: ZQK_TEST_ROOT in comments only (envWithoutZQKTestRoot); no RunProjectTestTeardown on a temp root.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

const (
	schedulerE2EStopCommandTimeout = 8 * time.Second
	schedulerE2EPIDSweepMax        = 5
	schedulerE2EPIDSweepInterval   = 100 * time.Millisecond
	schedulerE2EPostCleanupSettle  = 50 * time.Millisecond
)

// findModuleRoot returns the module root (directory containing go.mod) by walking up from cwd.
// When tests run under "go test", cwd is the package directory, not the module root.
func findModuleRoot() (string, error) {
	dir, err := fileutil.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found from current directory")
		}
		dir = parent
	}
}

// envWithoutZQKTestRoot returns a copy of the environment with ZQK_TEST_ROOT removed
// so the nested CLI uses persisted current_root (from "use") instead of env.
func envWithoutZQKTestRoot() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	prefix := zqkenv.TestRoot().Name() + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// TestSchedulerStart_EndToEnd runs a nested process using the test-scenario pattern:
// workspace with test-scenarios/scheduler-e2e, then "zqk use test-scenarios/scheduler-e2e"
// (which sets current_root and starts the scheduler). Verifies the scheduler is running,
// then stops it in cleanup so the test never hangs and no daemon is left behind.
//
// If the go test process is killed with SIGKILL (e.g. outer timeout), defers may not run and
// a detached daemon can survive; prefer a generous -timeout for this test or run it alone.
func TestSchedulerStart_EndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping end-to-end integration test in short mode")
	}

	workspace, err := fileutil.MkdirTemp("", "zqk-scheduler-e2e-workspace-*")
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	if err := fileutil.EnsureDir(filepath.Join(workspace, paths.ProjectDataDir)); err != nil {
		t.Fatalf("create workspace .zqk: %v", err)
	}

	moduleRoot, err := findModuleRoot()
	if err != nil {
		t.Fatalf("find module root: %v", err)
	}

	scenarioRel := "test-scenarios/scheduler-e2e"
	scenarioRoot := filepath.Join(workspace, scenarioRel)
	schedulerConfigDir := filepath.Join(scenarioRoot, paths.ProjectDataDir, paths.SchedulerSubdir)
	if err := fileutil.EnsureDir(schedulerConfigDir); err != nil {
		t.Fatalf("create scenario scheduler config dir: %v", err)
	}

	if err := testenvroot.CopyObjectSpecsFromProject(workspace, moduleRoot); err != nil {
		t.Fatalf("copy workspace object specs: %v", err)
	}

	if err := testenvroot.CopyObjectSpecsFromProject(scenarioRoot, moduleRoot); err != nil {
		t.Fatalf("copy scenario object specs: %v", err)
	}

	_ = fileutil.EnsureDir(filepath.Join(scenarioRoot, paths.ConfigDir))
	brandSettingsPath := filepath.Join(scenarioRoot, paths.ConfigDir, paths.ZqkConfigFileName)
	brandSettings := fmt.Sprintf(`# Minimal brand settings for scheduler e2e scenario
$schema: "https://zqk.dev/schemas/brand_settings.schema.json"
description: "Scheduler e2e test scenario settings"
version: "1.0.0"

paths:
  project_root: ""
  staleness_check_dirs:
    - paths.ProjectDataDir
    - ".zqk/agent-runtime"
  aliases:
    docs: "docs"
    process: "%s"
    architecture: "docs/architecture"
    zqk: paths.ProjectDataDir
    paths.ProjectDataDir: paths.ProjectDataDir
    streams: ".zqk/streams"
    cache: ".zqk/cache"

cli:
  default_context: "human"
`, paths.ProcessDir)
	if err := fileutil.WriteSecureFile(brandSettingsPath, []byte(brandSettings)); err != nil {
		t.Fatalf("write brand settings: %v", err)
	}
	_ = fileutil.WriteSecureFile(filepath.Join(scenarioRoot, paths.ConfigDir, paths.ZqkTestConfigFileName), []byte(brandSettings))
	configFile := filepath.Join(schedulerConfigDir, "config.yaml")
	if err := fileutil.WriteSecureFile(configFile, []byte("enabled: true\nproject_type: test\n")); err != nil {
		t.Fatalf("write scheduler config: %v", err)
	}

	binaryPath := filepath.Join(workspace, "bin", "zqk")
	if err := fileutil.EnsureDir(filepath.Dir(binaryPath)); err != nil {
		t.Fatalf("create workspace bin dir: %v", err)
	}
	buildCmd := execwrap.Command("go", "build", "-o", binaryPath, "./cmd/zqk")
	zqkenv.WireExecForIsolatedProject(buildCmd, moduleRoot)
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("build zqk binary: %v", err)
	}

	env := envWithoutZQKTestRoot()
	env = append(env, zqkenv.SchedulerMaxWallDuration().Name()+"=45m")
	env = append(env, "ZQK_TEST_BYPASS_AUTH=1")
	env = append(env, "ZQK_API_KEY="+pkgctx.TestHarnessAccountID)
	// Binary basename is "zqk" so ZQK_TEST_ROOT from WireExec gates session mandate (isTestRoot).
	// Do not name the binary zqk-test-binary — that uses ZQK_TEST_BINARY_* env keys and skips the session bypass.
	extras := []string{
		zqkenv.SchedulerMaxWallDuration().Name() + "=45m",
		"ZQK_TEST_BYPASS_AUTH=1",
		"ZQK_API_KEY=" + pkgctx.TestHarnessAccountID,
	}

	startCmd := execwrap.Command(binaryPath, "scheduler", "start", "--test-id="+t.Name())
	zqkenv.WireExecForIsolatedProjectWithExtras(startCmd, scenarioRoot, extras...)

	out, err := startCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("zqk scheduler start failed: %v\n%s", err, out)
	}

	statusCmd := execwrap.Command(binaryPath, "scheduler", "status", "--test-id="+t.Name())
	zqkenv.WireExecForIsolatedProjectWithExtras(statusCmd, scenarioRoot, extras...)
	statusOut, err := statusCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("zqk scheduler status failed: %v\n%s", err, statusOut)
	}
	statusStr := string(statusOut)
	if !strings.Contains(statusStr, "running") && !strings.Contains(statusStr, "PID") {
		t.Logf("scheduler status output (check manually): %s", statusOut)
	}
	daemonPID := parseSchedulerStatusDaemonPID(statusStr)

	defer schedulerE2ETeardown(t, binaryPath, workspace, scenarioRoot, env, daemonPID)

	t.Log("Scheduler started and verified running")
}

func TestParseSchedulerStatusDaemonPID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"Scheduler daemon: Running (PID: 42)\n", 42},
		{"foo (PID: 7)", 7},
		{"no pid here", 0},
	}
	for _, tc := range cases {
		if got := parseSchedulerStatusDaemonPID(tc.in); got != tc.want {
			t.Fatalf("parseSchedulerStatusDaemonPID(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

var schedulerStatusDaemonPIDRe = regexp.MustCompile(`(?i)\(PID:\s*([0-9]+)\)`)

// parseSchedulerStatusDaemonPID extracts the daemon PID from "zqk scheduler status" text
// (e.g. "Scheduler daemon: Running (PID: 12345)").
func parseSchedulerStatusDaemonPID(statusOut string) int {
	m := schedulerStatusDaemonPIDRe.FindStringSubmatch(statusOut)
	if len(m) < 2 {
		return 0
	}
	var pid int
	_, _ = fmt.Sscanf(m[1], "%d", &pid) //nolint:errcheck // sscanf pattern is digits from regex
	return pid
}

// schedulerE2ETeardown stops the scheduler via CLI, then force-kills any daemon still recorded
// in the PID file (background daemons use argv[0]=zqk-scheduler; see scheduler_core.go).
func schedulerE2ETeardown(t *testing.T, binaryPath, workspace, scenarioRoot string, env []string, statusDaemonPID int) {
	t.Helper()

	stopCmd := execwrap.Command(binaryPath, "scheduler", "stop", "--test-id="+t.Name())
	zqkenv.WireExecForIsolatedProject(stopCmd, workspace)
	stopCmd.Env = env
	if err := stopCmd.Start(); err != nil {
		t.Logf("cleanup: scheduler stop start failed: %v", err)
		killSchedulerDaemonPIDIfAlive(t, statusDaemonPID)
		killSchedulerDaemonByPIDFile(t, scenarioRoot)
		killProcessesHoldingTestBinary(t, binaryPath)
		killCLIProcessesMatchingExecutablePath(t, binaryPath)
		return
	}

	done := make(chan struct{})
	goroutinelabels.StartTestGoroutine("scheduler_e2e_cleanup_wait", "wait for scheduler stop to exit in e2e cleanup", func() {
		_ = stopCmd.Wait() //nolint:errcheck // Reap; outcome already handled by timeout/kill
		close(done)
	})

	ctx, cancel := context.WithTimeout(context.Background(), schedulerE2EStopCommandTimeout)
	defer cancel()
	select {
	case <-done:
	case <-ctx.Done():
		t.Logf("cleanup: scheduler stop did not exit within %v; killing by PID file", schedulerE2EStopCommandTimeout)
	}

	sweepSchedulerPIDFileUntilRemoved(t, scenarioRoot)
	killSchedulerDaemonPIDIfAlive(t, statusDaemonPID)
	killProcessesHoldingTestBinary(t, binaryPath)
	killCLIProcessesMatchingExecutablePath(t, binaryPath)
	time.Sleep(schedulerE2EPostCleanupSettle)
}

// killSchedulerDaemonPIDIfAlive sends SIGKILL to the PID we observed in "scheduler status" if still running.
// Catches cases where the PID file was removed or points at the wrong root but the daemon remains.
func killSchedulerDaemonPIDIfAlive(t *testing.T, pid int) {
	t.Helper()
	if pid <= 0 || pid == os.Getpid() {
		return
	}
	if runtime.GOOS == "windows" {
		return
	}
	if err := syscall.Kill(pid, 0); err != nil {
		return
	}
	_ = syscall.Kill(pid, syscall.SIGKILL) //nolint:errcheck // Best-effort teardown
}

// killProcessesHoldingTestBinary kills any process that still has this executable open (lsof).
// Matches detached daemons whose argv[0] is zqk-scheduler so pkill -f <binary path> does not see them.
func killProcessesHoldingTestBinary(t *testing.T, binaryPath string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	abs, err := filepath.Abs(binaryPath)
	if err != nil {
		t.Logf("cleanup: abs binary path %q: %v", binaryPath, err)
		return
	}
	out, err := execwrap.Command("lsof", "-t", abs).CombinedOutput()
	if err != nil {
		return
	}
	testIDFlag := "--test-id=" + t.Name()
	self := os.Getpid()
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var pid int
		if _, err := fmt.Sscanf(line, "%d", &pid); err != nil || pid <= 0 || pid == self {
			continue
		}

		// Prevent friendly fire in parallel tests: verify the process belongs to this test
		psOut, err := execwrap.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "args=").Output() //nolint:gosec
		if err == nil && !strings.Contains(string(psOut), testIDFlag) {
			continue // belongs to another test
		}

		_ = syscall.Kill(pid, syscall.SIGKILL) //nolint:errcheck // Best-effort
	}
}

// sweepSchedulerPIDFileUntilRemoved SIGKILLs the PID from the scheduler pid file and removes the file,
// repeating until the pid file is gone or attempts are exhausted.
func sweepSchedulerPIDFileUntilRemoved(t *testing.T, projectRoot string) {
	t.Helper()
	pidPath := paths.SchedulerPIDFilePath(projectRoot)
	for range schedulerE2EPIDSweepMax {
		killSchedulerDaemonByPIDFile(t, projectRoot)
		if _, err := fileutil.Stat(pidPath); errors.Is(err, fileutil.ErrNotExist) {
			return
		}
		time.Sleep(schedulerE2EPIDSweepInterval)
	}
}

// killSchedulerDaemonByPIDFile reads the scheduler PID file, sends SIGKILL, and removes the file.
// Background daemons use argv[0]=zqk-scheduler, so pkill -f <path/to/test/binary> does not match them.
func killSchedulerDaemonByPIDFile(t *testing.T, projectRoot string) {
	t.Helper()
	pidPath := paths.SchedulerPIDFilePath(projectRoot)
	data, err := fileutil.ReadFile(pidPath)
	if err != nil {
		t.Logf("cleanup: read PID file %s: %v", pidPath, err)
		return
	}
	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); err != nil {
		t.Logf("cleanup: parse PID from %s: %v", pidPath, err)
		return
	}
	if runtime.GOOS != "windows" {
		_ = syscall.Kill(pid, syscall.SIGKILL) //nolint:errcheck // Best effort
	} else {
		proc, _ := os.FindProcess(pid) //nolint:errcheck
		if proc != nil {
			_ = proc.Kill() //nolint:errcheck
		}
	}
	_ = fileutil.Remove(pidPath) //nolint:errcheck // Avoid stale PID confusing the next test
}

// killCLIProcessesMatchingExecutablePath sends SIGKILL to processes whose command line contains
// this exact executable path (pkill -f). Catches stuck nested CLI parents (e.g. "zqk use"); it does
// not match detached scheduler children (see killSchedulerDaemonByPIDFile).
func killCLIProcessesMatchingExecutablePath(t *testing.T, executablePath string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	absPath, err := filepath.Abs(executablePath)
	if err != nil {
		t.Logf("cleanup: abs executable path %q: %v", executablePath, err)
		return
	}
	pat := regexp.QuoteMeta(absPath)
	cmd := execwrap.Command("pkill", "-9", "-f", pat+".*--test-id="+regexp.QuoteMeta(t.Name())) //nolint:gosec
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return
		}
		t.Logf("cleanup: pkill -f %q: %v", absPath, err)
	}
}
