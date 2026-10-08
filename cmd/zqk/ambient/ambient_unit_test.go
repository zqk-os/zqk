package ambient

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/daemon/singleton"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestNewAmbientCmd_Structure(t *testing.T) {
	cmd := NewAmbientCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "ambient", cmd.Use)
	assert.NotEmpty(t, cmd.Commands())

	subs := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subs[sub.Name()] = true
	}

	assert.True(t, subs["status"])
	assert.True(t, subs["automerge"])
	assert.True(t, subs["ingest"])
	assert.True(t, subs["wave"])
	assert.True(t, subs["daemon"])
	assert.True(t, subs["start"])
	assert.True(t, subs["stop"])
	assert.True(t, subs["ensure"])
}

func TestStatusCmd_Stopped(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", root)

	cmd := newStatusCmd()
	require.NotNil(t, cmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := cmd.Execute()
	require.NoError(t, err)
}

func TestAmbientDaemon_HelpersAndSubcommands(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", root)

	// File paths
	pidPath := ambientPIDFilePath(root)
	assert.Contains(t, pidPath, "daemon.pid")

	logPath := ambientLogFilePath(root)
	assert.Contains(t, logPath, "ambient-daemon.log")

	// refuseDetachedAmbientSpawn always returns true in test mode
	assert.True(t, refuseDetachedAmbientSpawn("/usr/local/bin/zqk"))

	// Subcommands creation
	daemonCmd := newDaemonCmd()
	assert.NotNil(t, daemonCmd)
	assert.Equal(t, "daemon", daemonCmd.Use)

	startCmd := newStartCmd()
	assert.NotNil(t, startCmd)
	assert.Equal(t, "start", startCmd.Use)

	ensureCmd := newEnsureCmd()
	assert.NotNil(t, ensureCmd)
	assert.Equal(t, "ensure", ensureCmd.Use)

	stopCmd := newStopCmd()
	assert.NotNil(t, stopCmd)
	assert.Equal(t, "stop", stopCmd.Use)

	// StopDaemon on empty directory
	err := StopDaemon(root)
	assert.NoError(t, err)
}

func TestStatusCmd_Running(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", root)

	pidPath := ambientPIDFilePath(root)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(pidPath)))
	require.NoError(t, fileutil.WriteStandardFile(pidPath, []byte(strconv.Itoa(os.Getpid()))))

	cmd := newStatusCmd()
	require.NotNil(t, cmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := cmd.Execute()
	require.NoError(t, err)
}

func TestReadAmbientPID_StaleAndClean(t *testing.T) {
	root := t.TempDir()

	// 1. Non-existent PID file
	pid, running := readAmbientPID(root)
	assert.False(t, running)
	assert.Equal(t, 0, pid)

	// 2. Stale PID file
	pidPath := ambientPIDFilePath(root)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(pidPath)))
	require.NoError(t, fileutil.WriteStandardFile(pidPath, []byte("99999999")))

	pid, running = readAmbientPID(root)
	assert.False(t, running)
	assert.Equal(t, 0, pid)

	// 3. Process table check
	pids := findAmbientPIDsFromProcessTable(root)
	assert.Empty(t, pids)

	// 4. findRunningAmbientPIDs with current pid
	require.NoError(t, fileutil.WriteStandardFile(pidPath, []byte(strconv.Itoa(os.Getpid()))))
	runningPids := findRunningAmbientPIDs(root)
	assert.Empty(t, runningPids)
}

func TestStartDaemon_Branches(t *testing.T) {
	root := t.TempDir()

	// Branch 1: Already running
	pidPath := ambientPIDFilePath(root)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(pidPath)))
	require.NoError(t, fileutil.WriteStandardFile(pidPath, []byte(strconv.Itoa(os.Getpid()))))

	buf := new(bytes.Buffer)
	err := StartDaemon(root, buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "already running")

	// Branch 2: Not running -> refused in test process
	_ = fileutil.Remove(pidPath)
	buf.Reset()
	err = StartDaemon(root, buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Skipping ambient daemon spawn from test process")
}

func TestEnsureDaemon_Branches(t *testing.T) {
	root := t.TempDir()

	// Empty project root
	assert.NoError(t, EnsureDaemon("", nil))

	// Already running
	pidPath := ambientPIDFilePath(root)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(pidPath)))
	require.NoError(t, fileutil.WriteStandardFile(pidPath, []byte(strconv.Itoa(os.Getpid()))))
	logger := logging.GetLoggerFromProfile("test")
	assert.NoError(t, EnsureDaemon(root, logger))

	// Not running -> triggers StartDaemon
	_ = fileutil.Remove(pidPath)
	assert.NoError(t, EnsureDaemon(root, logger))
}

func TestRunAmbientDaemon_ErrorsAndAlreadyRunning(t *testing.T) {
	cmd := newDaemonCmd()
	require.NotNil(t, cmd)

	root := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", root)

	// Already running
	releaseLock, err := singleton.Guard(root, "ambient")
	require.NoError(t, err)
	defer releaseLock()

	err = runAmbientDaemon(cmd, root)
	assert.NoError(t, err)
}

func TestRunAmbientDaemon_ContextCancelled(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", root)

	cmd := newDaemonCmd()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	cmd.SetContext(ctx)

	err := runAmbientDaemon(cmd, root)
	// Context cancelled returns nil or context.Canceled
	assert.True(t, err == nil || errors.Is(err, context.Canceled))
}

func TestStopDaemon_RunningProcess(t *testing.T) {
	root := t.TempDir()

	// Spawn a real short-lived process so findRunningAmbientPIDs finds an active PID
	proc := exec.Command("sleep", "10")
	require.NoError(t, proc.Start())
	defer func() {
		if proc.Process != nil {
			_ = proc.Process.Kill()
		}
	}()

	pidPath := ambientPIDFilePath(root)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(pidPath)))
	require.NoError(t, fileutil.WriteStandardFile(pidPath, []byte(strconv.Itoa(proc.Process.Pid))))

	err := StopDaemon(root)
	assert.NoError(t, err)
	assert.False(t, fileutil.Exists(pidPath))
}

func TestAmbientSubcommands_Execution(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", root)

	// Test newStartCmd execution
	startCmd := newStartCmd()
	require.NoError(t, startCmd.Execute())

	// Test newEnsureCmd execution
	ensureCmd := newEnsureCmd()
	require.NoError(t, ensureCmd.Execute())

	// Test newStopCmd execution
	stopCmd := newStopCmd()
	require.NoError(t, stopCmd.Execute())

	// Test newDaemonCmd with already guarded singleton
	releaseLock, err := singleton.Guard(root, "ambient")
	require.NoError(t, err)
	defer releaseLock()

	daemonCmd := newDaemonCmd()
	daemonCmd.SetArgs([]string{"--project-root", root})
	require.NoError(t, daemonCmd.Execute())

	// Test newDaemonCmd default projectRoot resolution branch
	daemonCmdDefault := newDaemonCmd()
	daemonCmdDefault.SetArgs([]string{})
	require.NoError(t, daemonCmdDefault.Execute())
}

func TestRunAmbientIngest_Execution(t *testing.T) {
	cmd := newIngestCmd()
	require.NotNil(t, cmd)

	// With an invalid port, ListenAndServe will error out immediately after building server
	cmd.SetArgs([]string{"--port", "-1"})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestWaveCmd_InProcess(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	tmpDir := proj.Root
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)
	t.Setenv("TEST_BYPASS_AUTH", "1")

	storageProvider := proj.FileStorage
	secCtx := pkgctx.NewSystemSecurityContext()
	sysCtx := pkgctx.NewSystemContext()

	require.NoError(t, storageProvider.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:           "CMD-1",
		objects.FieldKeyKind:         "command_metric",
		objects.FieldKeyFailureCount: 3,
	}))

	require.NoError(t, storageProvider.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:     "SHM-1",
		objects.FieldKeyKind:   "scheduler_health_metric",
		objects.FieldKeyStatus: objects.ObjectStatusError,
	}))

	require.NoError(t, storageProvider.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:   "AAM-1",
		objects.FieldKeyKind: "audit_aggregation_metric",
	}))

	sysCheckPath := filepath.Join(tmpDir, paths.ProjectDataDir, paths.LogsDir, "system-check.json")
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(sysCheckPath)))
	require.NoError(t, fileutil.WriteFile(sysCheckPath, []byte(`{
		"summary": {
			"total_objects": 50,
			"total_issues": 3,
			"blocking_issues": 1,
			"ghost_ref_count": 2
		}
	}`), paths.FilePerm644))

	_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("command_metric", 2*time.Second)
	_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_health_metric", 2*time.Second)
	_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("audit_aggregation_metric", 2*time.Second)

	cmd := newWaveCmd()
	ctx := pkgctx.WithSecurityContext(context.Background(), secCtx)
	cmd.SetContext(ctx)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := cmd.Execute()
	require.NoError(t, err)

	rollupPath := filepath.Join(tmpDir, paths.ProjectDataDir, paths.StateDir, "ambient", "metrics-rollup.json")
	_, err = fileutil.Stat(rollupPath)
	assert.NoError(t, err)
}
