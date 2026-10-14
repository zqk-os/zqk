package daemon

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/daemon/overseer"
	"github.com/zqk-os/zqk/pkg/daemon/singleton"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func executeDaemonCmd(t *testing.T, projectRoot string, args ...string) (string, error) {
	t.Helper()
	cmd := NewDaemonCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cli.SetContext(cmd, cli.ContextForProjectRoot(projectRoot))
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func setupTestProject(t *testing.T) string {
	t.Helper()
	tempDir, err := fileutil.MkdirTemp("/tmp", "zd-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = fileutil.RemoveAll(tempDir) })
	stateDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.StateDir, "daemon")
	require.NoError(t, fileutil.MkdirAll(stateDir, paths.DirPerm755))
	return tempDir
}

func setupRunningOverseer(t *testing.T, projectRoot string) (*overseer.IPCServer, *overseer.Supervisor) {
	t.Helper()
	regPath := overseer.DefaultRegistryPath(projectRoot)
	reg := overseer.NewRegistry(regPath)
	require.NoError(t, reg.Load())

	// Add default test daemon to registry
	spec := &overseer.DaemonSpec{
		Name:          "test-worker",
		Description:   "Test worker daemon",
		Command:       []string{"echo", "worker"},
		DesiredState:  overseer.DesiredStateDisabled,
		RestartPolicy: overseer.RestartPolicyNever,
	}
	require.NoError(t, reg.Set(spec))

	pgMgr, err := overseer.NewProcessGroupManager(false)
	require.NoError(t, err)

	sup := overseer.NewSupervisor(projectRoot, reg, pgMgr)
	sockPath := overseer.SocketPath(projectRoot)
	server := overseer.NewIPCServer(sockPath, sup)
	require.NoError(t, server.Start())
	t.Cleanup(func() { _ = server.Stop() })

	client := overseer.NewIPCClient(sockPath)
	// Wait briefly for server readiness
	for i := 0; i < 20; i++ {
		if client.IsRunning() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.True(t, client.IsRunning(), "IPC server must be responding")

	return server, sup
}

func TestDaemonOfflineOperations(t *testing.T) {
	projectRoot := setupTestProject(t)

	// 1. Add daemon with valid flags
	out, err := executeDaemonCmd(t, projectRoot, "add", "worker-a", "--description", "Worker A", "--working-dir", "/tmp", "--max-restarts", "3", "--restart-policy", "always", "--enabled", "--", "echo", "run")
	require.NoError(t, err)
	assert.Contains(t, out, "worker-a")

	// 2. Add with invalid restart-policy fails
	_, err = executeDaemonCmd(t, projectRoot, "add", "worker-bad", "--restart-policy", "sometimes", "--", "echo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid restart-policy")

	// 3. Add with other valid restart policies: on-failure and never
	_, err = executeDaemonCmd(t, projectRoot, "add", "worker-fail", "--restart-policy", "on-failure", "--", "echo")
	require.NoError(t, err)
	_, err = executeDaemonCmd(t, projectRoot, "add", "worker-never", "--restart-policy", "never", "--", "echo")
	require.NoError(t, err)

	// 4. Status listing all daemons offline
	out, err = executeDaemonCmd(t, projectRoot, "status")
	require.NoError(t, err)
	assert.Contains(t, out, "worker-a")
	assert.Contains(t, out, "worker-fail")

	// 5. Status for specific target that exists
	out, err = executeDaemonCmd(t, projectRoot, "status", "worker-a")
	require.NoError(t, err)
	assert.Contains(t, out, "worker-a")

	// 6. Status for target that does not exist fails
	_, err = executeDaemonCmd(t, projectRoot, "status", "unknown-worker")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in registry")

	// 7. Enable / disable offline (handleDaemonStateToggle)
	out, err = executeDaemonCmd(t, projectRoot, "disable", "worker-a")
	require.NoError(t, err)
	assert.Contains(t, out, "disabled")

	out, err = executeDaemonCmd(t, projectRoot, "enable", "worker-a")
	require.NoError(t, err)
	assert.Contains(t, out, "enabled")

	// 8. Start offline sets desired state to enabled with note
	out, err = executeDaemonCmd(t, projectRoot, "start", "worker-a")
	require.NoError(t, err)
	assert.Contains(t, out, "Overseer is not running; daemon will start when overseer runs.")

	// 9. Stop offline sets desired state to disabled
	out, err = executeDaemonCmd(t, projectRoot, "stop", "worker-a")
	require.NoError(t, err)
	assert.Contains(t, out, "disabled")

	// 10. Restart offline returns error
	_, err = executeDaemonCmd(t, projectRoot, "restart", "worker-a")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot restart")

	// 11. Remove offline
	out, err = executeDaemonCmd(t, projectRoot, "remove", "worker-never")
	require.NoError(t, err)
	assert.Contains(t, out, "removed")
}

func TestDaemonOnlineOperationsViaIPC(t *testing.T) {
	projectRoot := setupTestProject(t)
	_, _ = setupRunningOverseer(t, projectRoot)

	// 1. Status online
	out, err := executeDaemonCmd(t, projectRoot, "status")
	require.NoError(t, err)
	assert.Contains(t, out, "test-worker")

	// 2. Status online with specific target
	out, err = executeDaemonCmd(t, projectRoot, "status", "test-worker")
	require.NoError(t, err)
	assert.Contains(t, out, "test-worker")

	// 3. Start online
	out, err = executeDaemonCmd(t, projectRoot, "start", "test-worker")
	require.NoError(t, err)
	assert.Contains(t, out, "test-worker")

	// 4. Restart online
	out, err = executeDaemonCmd(t, projectRoot, "restart", "test-worker")
	require.NoError(t, err)
	assert.Contains(t, out, "test-worker")

	// 5. Stop online
	out, err = executeDaemonCmd(t, projectRoot, "stop", "test-worker")
	require.NoError(t, err)
	assert.Contains(t, out, "test-worker")

	// 6. Enable online
	out, err = executeDaemonCmd(t, projectRoot, "enable", "test-worker")
	require.NoError(t, err)
	assert.Contains(t, out, "enabled")

	// 7. Disable online
	out, err = executeDaemonCmd(t, projectRoot, "disable", "test-worker")
	require.NoError(t, err)
	assert.Contains(t, out, "disabled")

	// 8. Add online
	out, err = executeDaemonCmd(t, projectRoot, "add", "online-worker", "--description", "Online added", "--", "echo", "hi")
	require.NoError(t, err)
	assert.Contains(t, out, "online-worker")

	// 9. Remove online
	out, err = executeDaemonCmd(t, projectRoot, "remove", "online-worker")
	require.NoError(t, err)
	assert.Contains(t, out, "online-worker")

	// 10. IPC error paths for nonexistent targets
	_, err = executeDaemonCmd(t, projectRoot, "status", "unknown-x")
	assert.Error(t, err)

	_, err = executeDaemonCmd(t, projectRoot, "start", "unknown-x")
	assert.Error(t, err)

	_, err = executeDaemonCmd(t, projectRoot, "stop", "unknown-x")
	assert.Error(t, err)

	_, err = executeDaemonCmd(t, projectRoot, "restart", "unknown-x")
	assert.Error(t, err)

	_, err = executeDaemonCmd(t, projectRoot, "enable", "unknown-x")
	assert.Error(t, err)

	_, err = executeDaemonCmd(t, projectRoot, "disable", "unknown-x")
	assert.Error(t, err)

	_, err = executeDaemonCmd(t, projectRoot, "remove", "unknown-x")
	assert.Error(t, err)
}

func TestDaemonRunSingletonAlreadyRunning(t *testing.T) {
	projectRoot := setupTestProject(t)

	// Acquire the lock first
	releaseLock, err := singleton.Guard(projectRoot, "overseer")
	require.NoError(t, err)
	defer releaseLock()

	// Running daemon run should detect already running instance and exit cleanly (return nil)
	out, err := executeDaemonCmd(t, projectRoot, "run")
	require.NoError(t, err)
	assert.Contains(t, out, "already running")
}

func TestDaemonServiceCommands(t *testing.T) {
	projectRoot := setupTestProject(t)

	// 1. Service Status
	out, err := executeDaemonCmd(t, projectRoot, "service", "status")
	if err == nil {
		assert.Contains(t, out, "label")
	}

	// 2. Service Cleanup Legacy
	out, err = executeDaemonCmd(t, projectRoot, "service", "cleanup-legacy")
	if err == nil {
		assert.Contains(t, out, "cleaned_legacy_units")
	}

	// 3. Service Uninstall
	out, err = executeDaemonCmd(t, projectRoot, "service", "uninstall")
	require.NoError(t, err)
	assert.Contains(t, out, "uninstalled")
}

func TestDaemonInternalHelpers(t *testing.T) {
	projectRoot := setupTestProject(t)

	// 1. resolveProjectRoot
	cmd := &cobra.Command{Use: "test"}
	cli.SetContext(cmd, cli.ContextForProjectRoot(projectRoot))
	assert.Equal(t, projectRoot, resolveProjectRoot(cmd))

	// Nil context fallback
	nilCmd := &cobra.Command{Use: "nil"}
	assert.NotEmpty(t, resolveProjectRoot(nilCmd))

	// 2. formatDaemonSuccessOutput
	var buf bytes.Buffer
	testCmd := &cobra.Command{Use: "format-test"}
	testCmd.SetOut(&buf)
	cli.SetContext(testCmd, cli.ContextForProjectRoot(projectRoot))
	err := formatDaemonSuccessOutput(testCmd, "worker-1", []string{"worker-1"})
	require.NoError(t, err)

	// 3. sendOverseerRequest error paths
	_, client := getDaemonClient(testCmd)
	_, err = sendOverseerAction(context.Background(), client, "status", "nonexistent")
	require.Error(t, err)

	// 4. loadDaemonRegistry on non-existent path
	emptyDir := t.TempDir()
	reg, err := loadDaemonRegistry(emptyDir)
	require.NoError(t, err)
	assert.NotNil(t, reg)
}

func TestDaemonCorruptRegistryHandling(t *testing.T) {
	projectRoot := setupTestProject(t)
	regPath := overseer.DefaultRegistryPath(projectRoot)
	require.NoError(t, fileutil.WriteFile(regPath, []byte("NOT_VALID_JSON{"), paths.FilePerm644))

	// 1. loadDaemonRegistry returns error
	_, err := loadDaemonRegistry(projectRoot)
	require.Error(t, err)

	// 2. setDesiredStateOffline returns error
	err = setDesiredStateOffline(projectRoot, "test-daemon", overseer.DesiredStateEnabled)
	require.Error(t, err)

	// 3. Status returns error
	_, err = executeDaemonCmd(t, projectRoot, "status")
	require.Error(t, err)

	// 4. Start returns error
	_, err = executeDaemonCmd(t, projectRoot, "start", "test-daemon")
	require.Error(t, err)

	// 5. Stop returns error
	_, err = executeDaemonCmd(t, projectRoot, "stop", "test-daemon")
	require.Error(t, err)

	// 6. Enable returns error
	_, err = executeDaemonCmd(t, projectRoot, "enable", "test-daemon")
	require.Error(t, err)

	// 7. Disable returns error
	_, err = executeDaemonCmd(t, projectRoot, "disable", "test-daemon")
	require.Error(t, err)

	// 8. Add returns error
	_, err = executeDaemonCmd(t, projectRoot, "add", "test-daemon", "--", "echo")
	require.Error(t, err)

	// 9. Remove returns error
	_, err = executeDaemonCmd(t, projectRoot, "remove", "test-daemon")
	require.Error(t, err)

	// 10. Run returns error when registry fails to load
	_, err = executeDaemonCmd(t, projectRoot, "run")
	require.Error(t, err)
}
