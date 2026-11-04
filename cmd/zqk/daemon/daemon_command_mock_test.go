package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/daemon/overseer"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func setupMockIPCServerWithHandler(t *testing.T, projectRoot string, handler func(req overseer.IPCRequest) overseer.IPCResponse) {
	t.Helper()
	sockPath := overseer.SocketPath(projectRoot)
	_ = os.Remove(sockPath)
	_ = fileutil.MkdirAll(filepath.Dir(sockPath), paths.DirPerm700)

	l, err := net.Listen("unix", sockPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = l.Close()
		_ = os.Remove(sockPath)
	})

	goroutinelabels.NewGoroutine("mock_ipc_server.accept", "accepting connections on mock unix domain socket").StartSimple(func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			c := conn
			goroutinelabels.NewGoroutine("mock_ipc_server.handler", "handling mock ipc request").StartSimple(func() {
				defer c.Close()
				var req overseer.IPCRequest
				if err := json.NewDecoder(c).Decode(&req); err != nil {
					return
				}
				resp := handler(req)
				b, _ := json.Marshal(resp)
				b = append(b, '\n')
				_, _ = c.Write(b)
			})
		}
	})

	client := overseer.NewIPCClient(sockPath)
	for i := 0; i < 30; i++ {
		if client.IsRunning() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.True(t, client.IsRunning(), "Mock IPC server must report running")
}

func TestResolveProjectRootEdgeCases(t *testing.T) {
	// 1. nil command fallback
	rootNil := resolveProjectRoot(nil)
	assert.NotEmpty(t, rootNil)

	// 2. Command with empty ProjectRoot in context
	cmdEmptyCtx := &cobra.Command{Use: "empty-ctx"}
	cli.SetContext(cmdEmptyCtx, cli.ContextForProjectRoot(""))
	rootEmpty := resolveProjectRoot(cmdEmptyCtx)
	assert.NotEmpty(t, rootEmpty)

	// 3. Command with explicit project root in context
	customRoot := "/tmp/explicit-custom-root"
	cmdCustom := &cobra.Command{Use: "custom"}
	cli.SetContext(cmdCustom, cli.ContextForProjectRoot(customRoot))
	assert.Equal(t, customRoot, resolveProjectRoot(cmdCustom))
}

func TestMockIPCServerFailureResponses(t *testing.T) {
	projectRoot := setupTestProject(t)

	// Handler returns success for health/running probes (empty target status),
	// but returns failure for specific operational commands.
	setupMockIPCServerWithHandler(t, projectRoot, func(req overseer.IPCRequest) overseer.IPCResponse {
		if req.Action == "status" && req.Target == "" {
			return overseer.IPCResponse{
				Success: true,
				Daemons: []overseer.DaemonStatus{
					{
						Name:         "mock-daemon",
						DesiredState: overseer.DesiredStateEnabled,
						ActualState:  overseer.ActualStateRunning,
					},
				},
			}
		}
		return overseer.IPCResponse{
			Success: false,
			Error:   "simulated overseer error for " + req.Action,
		}
	})

	// 1. Status with target failure
	_, err := executeDaemonCmd(t, projectRoot, "status", "mock-daemon")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated overseer error for status")

	// 2. Start failure online
	_, err = executeDaemonCmd(t, projectRoot, "start", "mock-daemon")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated overseer error for start")

	// 3. Stop failure online
	_, err = executeDaemonCmd(t, projectRoot, "stop", "mock-daemon")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated overseer error for stop")

	// 4. Restart failure online
	_, err = executeDaemonCmd(t, projectRoot, "restart", "mock-daemon")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated overseer error for restart")

	// 5. Enable failure online
	_, err = executeDaemonCmd(t, projectRoot, "enable", "mock-daemon")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated overseer error for enable")

	// 6. Disable failure online
	_, err = executeDaemonCmd(t, projectRoot, "disable", "mock-daemon")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated overseer error for disable")

	// 7. Add failure online
	_, err = executeDaemonCmd(t, projectRoot, "add", "mock-daemon", "--", "echo", "run")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated overseer error for add")

	// 8. Remove failure online
	_, err = executeDaemonCmd(t, projectRoot, "remove", "mock-daemon")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated overseer error for remove")
}

func TestMockIPCServerConnectionDrops(t *testing.T) {
	projectRoot := setupTestProject(t)
	sockPath := overseer.SocketPath(projectRoot)
	_ = os.Remove(sockPath)
	_ = fileutil.MkdirAll(filepath.Dir(sockPath), paths.DirPerm700)

	l, err := net.Listen("unix", sockPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = l.Close()
		_ = os.Remove(sockPath)
	})

	// Server answers first status probe to pass IsRunning(), then closes connection immediately
	firstProbe := true
	goroutinelabels.NewGoroutine("mock_ipc_drop_server.accept", "accepting connections for connection drop test").StartSimple(func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			if firstProbe {
				firstProbe = false
				var req overseer.IPCRequest
				_ = json.NewDecoder(conn).Decode(&req)
				resp := overseer.IPCResponse{Success: true}
				b, _ := json.Marshal(resp)
				b = append(b, '\n')
				_, _ = conn.Write(b)
				_ = conn.Close()
			} else {
				// Abrupt drop
				_ = conn.Close()
			}
		}
	})

	client := overseer.NewIPCClient(sockPath)
	require.True(t, client.IsRunning())

	// Executing a command now triggers decode / read error in sendOverseerRequest
	_, err = executeDaemonCmd(t, projectRoot, "start", "any-daemon")
	require.Error(t, err)
}

func TestOfflineTargetNotFoundErrors(t *testing.T) {
	projectRoot := setupTestProject(t)

	// Ensure registry exists with default entries
	reg, err := loadDaemonRegistry(projectRoot)
	require.NoError(t, err)
	require.NotNil(t, reg)

	// 1. Offline start for non-existent target fails
	_, err = executeDaemonCmd(t, projectRoot, "start", "non-existent-worker")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in registry")

	// 2. Offline stop for non-existent target fails
	_, err = executeDaemonCmd(t, projectRoot, "stop", "non-existent-worker")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in registry")

	// 3. Offline enable for non-existent target fails
	_, err = executeDaemonCmd(t, projectRoot, "enable", "non-existent-worker")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in registry")

	// 4. Offline disable for non-existent target fails
	_, err = executeDaemonCmd(t, projectRoot, "disable", "non-existent-worker")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in registry")

	// 5. Offline remove for non-existent target fails
	_, err = executeDaemonCmd(t, projectRoot, "remove", "non-existent-worker")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in registry")
}

func TestOfflineSaveFailureBranches(t *testing.T) {
	projectRoot := setupTestProject(t)

	// Seed a valid daemon into registry
	regPath := overseer.DefaultRegistryPath(projectRoot)
	reg := overseer.NewRegistry(regPath)
	require.NoError(t, reg.Load())
	require.NoError(t, reg.Set(&overseer.DaemonSpec{
		Name:          "save-test-daemon",
		Command:       []string{"echo", "hi"},
		DesiredState:  overseer.DesiredStateEnabled,
		RestartPolicy: overseer.RestartPolicyNever,
	}))

	// Make registry directory read-only
	regDir := filepath.Dir(regPath)
	require.NoError(t, os.Chmod(regDir, 0555))
	t.Cleanup(func() { _ = os.Chmod(regDir, 0755) })

	// Add when saving fails
	_, addErr := executeDaemonCmd(t, projectRoot, "add", "new-daemon", "--", "echo", "fail")
	require.Error(t, addErr)

	// Remove when saving fails
	_, remErr := executeDaemonCmd(t, projectRoot, "remove", "save-test-daemon")
	require.Error(t, remErr)
}

func TestRunCommandErrorBranches(t *testing.T) {
	// 1. singleton.Guard fails with non-already-running error (project root is a file)
	f, err := os.CreateTemp("/tmp", "not-a-dir-*.txt")
	require.NoError(t, err)
	tempFile := f.Name()
	_ = f.Close()
	t.Cleanup(func() { _ = os.Remove(tempFile) })

	_, err = executeDaemonCmd(t, tempFile, "run")
	require.Error(t, err)

	// 2. ipcServer.Start fails when socket path is excessively long (>104 chars for AF_UNIX)
	longDir, err := fileutil.MkdirTemp("/tmp", "long-socket-path-exceeding-standard-unix-domain-length-limit-012345678901234567890123456789-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = fileutil.RemoveAll(longDir) })

	stateDir := filepath.Join(longDir, paths.ProjectDataDir, paths.StateDir, "daemon")
	require.NoError(t, fileutil.MkdirAll(stateDir, paths.DirPerm755))

	_, err = executeDaemonCmd(t, longDir, "run")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start IPC server")
}

func TestDaemonServiceCommandFlagsAndOutput(t *testing.T) {
	projectRoot := setupTestProject(t)

	// 1. Service install flag verification
	installCmd := newServiceInstallCmd()
	require.NotNil(t, installCmd)
	assert.NotNil(t, installCmd.Flags().Lookup("root"))
	assert.NotNil(t, installCmd.Flags().Lookup("binary"))

	// 2. Format daemon output helper with empty slice
	var buf bytes.Buffer
	testCmd := &cobra.Command{Use: "format-empty"}
	testCmd.SetOut(&buf)
	cli.SetContext(testCmd, cli.ContextForProjectRoot(projectRoot))
	testCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	err := formatDaemonSuccessOutput(testCmd, "empty-worker", []overseer.DaemonStatus{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "empty-worker")

	// 3. Service status execution check
	out, err := executeDaemonCmd(t, projectRoot, "service", "status")
	if err == nil {
		assert.Contains(t, out, "label")
	}

	// 4. Service cleanup-legacy execution check
	out, err = executeDaemonCmd(t, projectRoot, "service", "cleanup-legacy")
	if err == nil {
		assert.Contains(t, out, "cleaned_legacy_units")
	}
}

func TestDaemonCommandHelpAndFindability(t *testing.T) {
	rootCmd := NewDaemonCmd()
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--help"})
	err := rootCmd.Execute()
	require.NoError(t, err)
	helpText := buf.String()
	assert.Contains(t, helpText, "daemon")
	for _, sub := range []string{"status", "start", "stop", "restart", "enable", "disable", "add", "remove", "run", "service"} {
		assert.True(t, strings.Contains(helpText, sub), "help should mention %s", sub)
	}
}
