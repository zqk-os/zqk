package ambient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/ambient"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/daemon/singleton"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

type mockFailingEventHub struct{}

func (m *mockFailingEventHub) Subscribe(eventType ambient.EventType, handler ambient.EventHandler) {}

func (m *mockFailingEventHub) Publish(ctx context.Context, event ambient.Event) error {
	return errors.New("simulated hub publish failure")
}

func (m *mockFailingEventHub) Status() string {
	return "failing"
}

func (m *mockFailingEventHub) EnableEventSourcing(projectRoot string, secCtx *pkgctx.SecurityContext) {
}

func TestAmbientDaemon_LifecycleAndSignals(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", root)

	// 1. Start daemon when already running reports running PID
	pidPath := ambientPIDFilePath(root)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(pidPath)))
	require.NoError(t, fileutil.WriteStandardFile(pidPath, []byte(strconv.Itoa(os.Getpid()))))

	var buf bytes.Buffer
	err := StartDaemon(root, &buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "already running")

	// 2. Stop daemon sends SIGTERM to running process and clears PID/lock
	proc := exec.Command("sleep", "15")
	require.NoError(t, proc.Start())
	t.Cleanup(func() {
		if proc.Process != nil {
			_ = proc.Process.Kill()
		}
	})

	require.NoError(t, fileutil.WriteStandardFile(pidPath, []byte(strconv.Itoa(proc.Process.Pid))))
	lockPath := singleton.LockFilePath(root, "ambient")
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(lockPath)))
	require.NoError(t, fileutil.WriteStandardFile(lockPath, []byte(strconv.Itoa(proc.Process.Pid))))

	err = StopDaemon(root)
	require.NoError(t, err)
	assert.False(t, fileutil.Exists(pidPath), "PID file must be removed by StopDaemon")
	assert.False(t, fileutil.Exists(lockPath), "lock file must be removed by StopDaemon")

	// 3. Stop daemon with empty running list is a graceful no-op
	err = StopDaemon(root)
	require.NoError(t, err)
}

func TestAmbientDaemon_SupervisionAndOrphanReaping(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", root)

	// 1. Stale PID file cleans up dead process reference
	pidPath := ambientPIDFilePath(root)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(pidPath)))
	require.NoError(t, fileutil.WriteStandardFile(pidPath, []byte("99999999")))

	pid, ok := readAmbientPID(root)
	assert.False(t, ok)
	assert.Equal(t, 0, pid)
	assert.False(t, fileutil.Exists(pidPath), "stale PID file must be cleaned up")

	// 2. PID file discovery and validation
	proc := exec.Command("sleep", "10")
	require.NoError(t, proc.Start())
	t.Cleanup(func() {
		if proc.Process != nil {
			_ = proc.Process.Kill()
		}
	})

	require.NoError(t, fileutil.WriteStandardFile(pidPath, []byte(strconv.Itoa(proc.Process.Pid))))

	// readAmbientPID should detect it from PID file
	readPid, running := readAmbientPID(root)
	assert.True(t, running)
	assert.Equal(t, proc.Process.Pid, readPid)

	// findAmbientPIDsFromProcessTable filters out non-ambient processes
	discoveredPids := findAmbientPIDsFromProcessTable(root)
	assert.NotContains(t, discoveredPids, proc.Process.Pid)

	// 3. Orphan process reaping on startup:
	// A new daemon instance reaps existing orphaned process before taking over PID file
	cmd := newDaemonCmd()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel so runAmbientDaemon terminates quickly after cleanup
	cmd.SetContext(ctx)

	_ = runAmbientDaemon(cmd, root)

	// Verify old process was signaled SIGTERM and exited
	waitErr := make(chan error, 1)
	goroutinelabels.NewGoroutine("test_orphan_wait", "wait for orphan exit").StartSimple(func() {
		waitErr <- proc.Wait()
	})

	select {
	case err := <-waitErr:
		require.Error(t, err)
		assert.Contains(t, err.Error(), "signal: terminated")
	case <-time.After(2 * time.Second):
		t.Fatal("orphan process did not exit within timeout")
	}
}

func TestAmbientDaemon_WatchdogLockfileRemoval(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", root)

	cmd := newDaemonCmd()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd.SetContext(ctx)

	doneCh := make(chan error, 1)
	goroutinelabels.NewGoroutine("test_ambient_daemon", "watchdog test").StartSimple(func() {
		doneCh <- runAmbientDaemon(cmd, root)
	})

	// Wait for lockfile to be created by singleton.Guard
	lockPath := singleton.LockFilePath(root, "ambient")
	require.Eventually(t, func() bool {
		return fileutil.Exists(lockPath)
	}, 2*time.Second, 50*time.Millisecond)

	// Remove the lockfile to trigger watchdog cancellation
	require.NoError(t, fileutil.Remove(lockPath))

	select {
	case err := <-doneCh:
		assert.True(t, err == nil || errors.Is(err, context.Canceled))
	case <-time.After(4 * time.Second):
		cancel()
		t.Fatal("watchdog failed to detect removed lockfile within timeout")
	}
}

func TestAmbientStatus_FormattingAndEdges(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", root)

	// 1. Status stopped JSON output
	cmdStopped := newStatusCmd()
	var bufStopped bytes.Buffer
	cmdStopped.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufStopped))
	cmdStopped.SetArgs([]string{"--format", "json"})
	require.NoError(t, cmdStopped.Execute())

	var stoppedData map[string]any
	require.NoError(t, json.Unmarshal(bufStopped.Bytes(), &stoppedData))
	assert.Equal(t, "ambient", stoppedData["component"])
	assert.Equal(t, "stopped", stoppedData["status"])

	// 2. Status running JSON output
	pidPath := ambientPIDFilePath(root)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(pidPath)))
	require.NoError(t, fileutil.WriteStandardFile(pidPath, []byte(strconv.Itoa(os.Getpid()))))

	cmdRunning := newStatusCmd()
	var bufRunning bytes.Buffer
	cmdRunning.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufRunning))
	cmdRunning.SetArgs([]string{"--format", "json"})
	require.NoError(t, cmdRunning.Execute())

	var runningData map[string]any
	require.NoError(t, json.Unmarshal(bufRunning.Bytes(), &runningData))
	assert.Equal(t, "ambient", runningData["component"])
	assert.Equal(t, "running", runningData["status"])
	assert.Equal(t, float64(os.Getpid()), runningData["pid"])
	assert.Equal(t, root, runningData["project_root"])
}

func TestAmbientDaemon_ErrorScenarios(t *testing.T) {
	// 1. Unwritable state directory prevents PID creation
	root1 := t.TempDir()
	stateDir := filepath.Join(root1, paths.ProjectDataDir, paths.StateDir)
	require.NoError(t, fileutil.EnsureDir(stateDir))
	ambientBlocker := filepath.Join(stateDir, "ambient")
	require.NoError(t, fileutil.WriteStandardFile(ambientBlocker, []byte("blocker")))

	cmd1 := newDaemonCmd()
	err1 := runAmbientDaemon(cmd1, root1)
	require.Error(t, err1)
	assert.Contains(t, err1.Error(), "failed to create ambient state directory")

	// 2. Unwritable PID file path (directory in place of file)
	root2 := t.TempDir()
	pidPath := ambientPIDFilePath(root2)
	require.NoError(t, fileutil.EnsureDir(pidPath))

	cmd2 := newDaemonCmd()
	err2 := runAmbientDaemon(cmd2, root2)
	require.Error(t, err2)
	assert.Contains(t, err2.Error(), "failed to write ambient daemon PID")

	// 3. Port collision in runAmbientIngest
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()

	port := l.Addr().(*net.TCPAddr).Port
	cmdIngest := newIngestCmd()
	cmdIngest.SetArgs([]string{"--port", strconv.Itoa(port)})
	err = cmdIngest.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "address already in use")
}

func TestAmbientIngest_BoundariesAndHubErrors(t *testing.T) {
	eventLogger := logging.GetLoggerFromContext(context.Background())
	hub := ambient.NewEventHub()
	handler := newAmbientIngestHandler(context.Background(), hub, eventLogger)

	// 1. Non-POST method -> 405 Method Not Allowed
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ingest", nil)
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)

	// 2. Oversized body (>1MB) -> 400 Bad Request
	oversized := bytes.Repeat([]byte("x"), (1<<20)+16)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewReader(oversized))
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 3. Malformed JSON payload -> 400 Bad Request
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/ingest", strings.NewReader("not-json-content"))
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 4. Missing event type defaults to "unknown", parses valid timestamp -> 202 Accepted
	rec = httptest.NewRecorder()
	validPayload := `{"type":"","payload":{"key":"val"},"timestamp":"2026-10-09T01:30:00Z"}`
	req = httptest.NewRequest(http.MethodPost, "/ingest", strings.NewReader(validPayload))
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusAccepted, rec.Code)

	// 5. Hub Publish failure -> 500 Internal Server Error
	failHandler := newAmbientIngestHandler(context.Background(), &mockFailingEventHub{}, eventLogger)
	recFail := httptest.NewRecorder()
	reqFail := httptest.NewRequest(http.MethodPost, "/ingest", strings.NewReader(`{"type":"ambient_signal"}`))
	failHandler.ServeHTTP(recFail, reqFail)
	assert.Equal(t, http.StatusInternalServerError, recFail.Code)
}

func TestAmbientAutomerge_QueuePollingEdges(t *testing.T) {
	cmd := newAutomergeCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "automerge", cmd.Use)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel context to verify clean shutdown without goroutine leak
	cmd.SetContext(ctx)

	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestAmbientWave_BoundariesAndEdges(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	tmpDir := proj.Root
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)
	t.Setenv("TEST_BYPASS_AUTH", "1")

	secCtx := pkgctx.NewSystemSecurityContext()
	sysCtx := pkgctx.NewSystemContext()

	// 1. Healthy kernel boundary: clean project with 0 errors reports healthy monitor action
	cmdHealthy := newWaveCmd()
	cmdHealthy.SetContext(pkgctx.WithSecurityContext(context.Background(), secCtx))
	bufHealthy := new(bytes.Buffer)
	cmdHealthy.SetOut(bufHealthy)
	cmdHealthy.SetErr(bufHealthy)
	require.NoError(t, cmdHealthy.Execute())

	rollupPath := filepath.Join(tmpDir, paths.ProjectDataDir, paths.StateDir, "ambient", "metrics-rollup.json")
	data, err := fileutil.ReadFile(rollupPath)
	require.NoError(t, err)

	var rollupHealthy whatsnext.MetricsRollupSnapshot
	require.NoError(t, json.Unmarshal(data, &rollupHealthy))
	assert.Contains(t, rollupHealthy.RankedActions, "monitor: kernel healthy")
	assert.Equal(t, "monitor: kernel healthy", rollupHealthy.NextAdminAction)

	// 2. Metrics with int failure count, stuck scheduler status, and resource hygiene anomalies
	require.NoError(t, proj.FileStorage.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:           "CMD-INT-FAIL",
		objects.FieldKeyKind:         "command_metric",
		objects.FieldKeyFailureCount: int(7),
	}))

	require.NoError(t, proj.FileStorage.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:     "SHM-STUCK-1",
		objects.FieldKeyKind:   "scheduler_health_metric",
		objects.FieldKeyStatus: objects.ObjectStatusError,
	}))

	// Introduce a stale lock to trigger resource hygiene action
	stateDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.StateDir)
	require.NoError(t, fileutil.EnsureDir(stateDir))
	require.NoError(t, fileutil.WriteStandardFile(filepath.Join(stateDir, "stale_test.lock"), []byte("stale-lock-data")))

	cmdAnomalies := newWaveCmd()
	cmdAnomalies.SetContext(pkgctx.WithSecurityContext(context.Background(), secCtx))
	bufAnomalies := new(bytes.Buffer)
	cmdAnomalies.SetOut(bufAnomalies)
	cmdAnomalies.SetErr(bufAnomalies)
	require.NoError(t, cmdAnomalies.Execute())

	data, err = fileutil.ReadFile(rollupPath)
	require.NoError(t, err)

	var rollupAnomalies whatsnext.MetricsRollupSnapshot
	require.NoError(t, json.Unmarshal(data, &rollupAnomalies))
	assert.Equal(t, 7, rollupAnomalies.CommandErrors)
	assert.Equal(t, 1, rollupAnomalies.SchedulerStuckCount)
}
