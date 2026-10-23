package ambient

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/ambient"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/daemon/singleton"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

func TestCycle131_AmbientCommandHierarchy(t *testing.T) {
	cmd := NewAmbientCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "ambient", cmd.Use)
	assert.NotEmpty(t, cmd.Short)

	subcommands := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subcommands[sub.Name()] = true
	}

	expectedSubs := []string{
		"status", "automerge", "ingest", "wave", "daemon", "start", "stop", "ensure",
	}
	for _, expected := range expectedSubs {
		assert.True(t, subcommands[expected], "expected subcommand %s not found", expected)
	}

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})
	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "ambient")
}

func TestCycle131_StatusSubcommandExecution(t *testing.T) {
	tmpRoot := t.TempDir()
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(tmpRoot)
	})
	t.Setenv("ZQK_PROJECT_ROOT", tmpRoot)

	cmd := newStatusCmd()
	require.NotNil(t, cmd)
	buf := new(bytes.Buffer)
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), buf)
	cmd.SetContext(ctx)

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "stopped")

	releaseLock, err := singleton.Guard(tmpRoot, "ambient")
	require.NoError(t, err)
	t.Cleanup(func() {
		releaseLock()
	})

	buf.Reset()
	cmdRunning := newStatusCmd()
	ctxRunning := pkgctx.WithCommandOutputWriter(context.Background(), buf)
	cmdRunning.SetContext(ctxRunning)
	err = cmdRunning.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "running")
}

func TestCycle131_AutomergeSubcommandExecution(t *testing.T) {
	cmd := newAutomergeCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "automerge", cmd.Use)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd.SetContext(ctx)

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := cmd.Execute()
	require.NoError(t, err)
}

func TestCycle131_IngestSubcommandAndHandler(t *testing.T) {
	cmd := newIngestCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "ingest", cmd.Use)
	assert.NotNil(t, cmd.Flags().Lookup("port"))

	err := runAmbientIngest(&cobra.Command{}, nil)
	require.Error(t, err)

	hub := ambient.NewEventHub()
	logger := logging.GetLoggerFromContext(context.Background())
	handler := newAmbientIngestHandler(context.Background(), hub, logger)

	testIngestEndpoints(t, handler)
}

func testIngestEndpoints(t *testing.T, handler http.Handler) {
	t.Helper()

	reqGet := httptest.NewRequest(http.MethodGet, "/ingest", nil)
	recGet := httptest.NewRecorder()
	handler.ServeHTTP(recGet, reqGet)
	assert.Equal(t, http.StatusMethodNotAllowed, recGet.Code)

	reqInvalid := httptest.NewRequest(http.MethodPost, "/ingest", strings.NewReader("invalid-json"))
	recInvalid := httptest.NewRecorder()
	handler.ServeHTTP(recInvalid, reqInvalid)
	assert.Equal(t, http.StatusBadRequest, recInvalid.Code)

	validPayload := `{"type":"test.event","payload":{"key":"value"},"timestamp":"2026-10-23T12:00:00Z"}`
	reqValid := httptest.NewRequest(http.MethodPost, "/ingest", strings.NewReader(validPayload))
	recValid := httptest.NewRecorder()
	handler.ServeHTTP(recValid, reqValid)
	assert.Equal(t, http.StatusAccepted, recValid.Code)
	assert.Contains(t, recValid.Body.String(), "accepted")

	defaultTypePayload := `{"payload":{"foo":"bar"}}`
	reqDefault := httptest.NewRequest(http.MethodPost, "/ingest", strings.NewReader(defaultTypePayload))
	recDefault := httptest.NewRecorder()
	handler.ServeHTTP(recDefault, reqDefault)
	assert.Equal(t, http.StatusAccepted, recDefault.Code)
}

func TestCycle131_DaemonPID_SingletonAndProcessTable(t *testing.T) {
	tmpRoot := t.TempDir()
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(tmpRoot)
	})

	releaseLock, err := singleton.Guard(tmpRoot, "ambient")
	require.NoError(t, err)

	pid, running := readAmbientPID(tmpRoot)
	assert.True(t, running)
	assert.Greater(t, pid, 0)
	releaseLock()

	shCmd := testkit.ManagedCommand(t, t.Context(), "bash", "-c", "sleep 3 & wait # ambient daemon "+tmpRoot)
	err = shCmd.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		if shCmd.Process != nil {
			if killErr := shCmd.Process.Kill(); killErr != nil {
				// already terminated
			}
			if waitErr := shCmd.Wait(); waitErr != nil {
				// already waited
			}
		}
	})

	time.Sleep(100 * time.Millisecond)

	pids := findAmbientPIDsFromProcessTable(tmpRoot)
	if len(pids) > 0 {
		assert.Contains(t, pids, shCmd.Process.Pid)

		fallbackPid, fallbackRunning := readAmbientPID(tmpRoot)
		assert.True(t, fallbackRunning)
		assert.Equal(t, shCmd.Process.Pid, fallbackPid)

		runningPids := findRunningAmbientPIDs(tmpRoot)
		assert.Contains(t, runningPids, shCmd.Process.Pid)
	}
}

func TestCycle131_RunAmbientDaemon_GuardFailure(t *testing.T) {
	cmd := newDaemonCmd()
	require.NotNil(t, cmd)

	err := runAmbientDaemon(cmd, "/dev/null/impossible-ambient-directory")
	require.Error(t, err)
}

func TestCycle131_StopDaemon_GracefulAndForced(t *testing.T) {
	tmpRoot := t.TempDir()
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(tmpRoot)
	})

	err := StopDaemon(tmpRoot)
	require.NoError(t, err)

	shCmd := testkit.ManagedCommand(t, t.Context(), "bash", "-c", "trap '' TERM; sleep 2 & wait # ambient daemon "+tmpRoot)
	err = shCmd.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		if shCmd.Process != nil {
			if killErr := shCmd.Process.Kill(); killErr != nil {
				// already terminated
			}
			if waitErr := shCmd.Wait(); waitErr != nil {
				// already waited
			}
		}
	})

	time.Sleep(100 * time.Millisecond)

	pidPath := ambientPIDFilePath(tmpRoot)
	err = fileutil.EnsureDir(filepath.Dir(pidPath))
	require.NoError(t, err)
	err = fileutil.WriteFile(pidPath, []byte(strconv.Itoa(shCmd.Process.Pid)), paths.FilePerm644)
	require.NoError(t, err)

	err = StopDaemon(tmpRoot)
	require.NoError(t, err)

	_, running := readAmbientPID(tmpRoot)
	assert.False(t, running)
}

func TestCycle131_WaveSubcommand_FullCoverage(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	tmpRoot := proj.Root
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(tmpRoot)
	})
	t.Setenv("ZQK_PROJECT_ROOT", tmpRoot)
	t.Setenv("TEST_BYPASS_AUTH", "1")

	secCtx := pkgctx.NewSystemSecurityContext()
	sysCtx := pkgctx.NewSystemContext()

	setupWaveMetricsFixtures(t, proj, sysCtx, secCtx)

	cmd := newWaveCmd()
	require.NotNil(t, cmd)
	cmd.SetContext(pkgctx.WithSecurityContext(context.Background(), secCtx))

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := cmd.Execute()
	require.NoError(t, err)

	rollupPath := filepath.Join(tmpRoot, paths.ProjectDataDir, paths.StateDir, "ambient", "metrics-rollup.json")
	data, err := fileutil.ReadFile(rollupPath)
	require.NoError(t, err)

	var rollup whatsnext.MetricsRollupSnapshot
	err = json.Unmarshal(data, &rollup)
	require.NoError(t, err)

	assert.Equal(t, 12, rollup.CommandErrors)
	assert.Equal(t, 2, rollup.SchedulerStuckCount)
	assert.Equal(t, 1, rollup.AuditEventsCount)
	assert.NotEmpty(t, rollup.RankedActions)
}

func setupWaveMetricsFixtures(t *testing.T, proj testkit.IsolatedTempProject, sysCtx context.Context, secCtx *pkgctx.SecurityContext) {
	t.Helper()

	err := proj.FileStorage.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:           "CMD-FAIL-1",
		objects.FieldKeyKind:         "command_metric",
		objects.FieldKeyFailureCount: 5.0,
	})
	require.NoError(t, err)

	err = proj.FileStorage.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:           "CMD-FAIL-2",
		objects.FieldKeyKind:         "command_metric",
		objects.FieldKeyFailureCount: 7.0,
	})
	require.NoError(t, err)

	err = proj.FileStorage.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:     "SHM-STUCK-1",
		objects.FieldKeyKind:   "scheduler_health_metric",
		objects.FieldKeyStatus: "error",
	})
	require.NoError(t, err)

	err = proj.FileStorage.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:     "SHM-STUCK-2",
		objects.FieldKeyKind:   "scheduler_health_metric",
		objects.FieldKeyStatus: objects.ObjectStatusError,
	})
	require.NoError(t, err)

	err = proj.FileStorage.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:   "AAM-1",
		objects.FieldKeyKind: "audit_aggregation_metric",
	})
	require.NoError(t, err)

	metricsDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.MetricsDir)
	err = fileutil.EnsureDir(metricsDir)
	require.NoError(t, err)

	metricsPath := filepath.Join(metricsDir, paths.CommandMetricsFile)
	metricsJSON := `{"records":[{"command":"test","duration_ms":5000,"exit_code":1}]}`
	err = fileutil.WriteFile(metricsPath, []byte(metricsJSON), paths.FilePerm644)
	require.NoError(t, err)
}
