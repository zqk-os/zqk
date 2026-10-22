package mcp

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	clitool "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	pkgmcp "github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestCycle118_IDEAdapter_Cmd_And_CursorCmd(t *testing.T) {
	tempDir, cleanup := setupMCPTestProject(t)
	defer cleanup()

	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	require.NoError(t, err)
	require.NoError(t, w.Close())
	os.Stdin = r
	defer func() {
		os.Stdin = oldStdin
	}()

	cmd := NewIDEAdapterCmd()
	cmd.SetArgs([]string{"--tcp", "127.0.0.1:8443"})
	rootCmd := clitool.NewCommandBuilder("zqk").Build()
	rootCmd.AddCommand(cmd)
	rootCmd.SetArgs([]string{"ide-adapter", "--tcp", "127.0.0.1:8443"})
	setMCPCLIContext(t, rootCmd, tempDir, nil)

	err = cmd.Execute()
	require.NoError(t, err)

	cursorCmd := NewCursorAdapterCmd()
	require.NotNil(t, cursorCmd)
	assert.Equal(t, "cursor-adapter", cursorCmd.Use)
}

func TestCycle118_StorageProviderAdapter_List(t *testing.T) {
	require.Nil(t, NewStorageProviderAdapter(nil))

	_, provider, cleanup := setupMCPTestProjectWithStorage(t)
	defer cleanup()

	adapter := NewStorageProviderAdapter(provider)
	require.NotNil(t, adapter)

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	filter := map[string]any{
		objects.FieldKeyKind: "goal",
		"filters":            map[string]any{"status": "active"},
	}
	res, err := adapter.List(ctx, secCtx, storageCtx, filter)
	require.NoError(t, err)
	m, ok := res.(map[string]any)
	require.True(t, ok)
	require.Contains(t, m, "objects")
	require.Contains(t, m, "meta")
}

func TestCycle118_StorageProviderAdapter_FilterToListFilter(t *testing.T) {
	_, provider, cleanup := setupMCPTestProjectWithStorage(t)
	defer cleanup()

	adapter := NewStorageProviderAdapter(provider).(*storageProviderAdapter)

	lfNil := adapter.filterToListFilter(nil)
	assert.Empty(t, lfNil.Kind)
	assert.Nil(t, lfNil.Filters)

	lfInvalid := adapter.filterToListFilter("not-a-map")
	assert.Empty(t, lfInvalid.Kind)

	filterMap := map[string]any{
		objects.FieldKeyKind: "requirement",
		"filters":            map[string]any{"priority": "high"},
	}
	lfValid := adapter.filterToListFilter(filterMap)
	assert.Equal(t, "requirement", lfValid.Kind)
	assert.NotNil(t, lfValid.Filters)
}

func TestCycle118_StorageProviderAdapter_CreateAndRead(t *testing.T) {
	_, provider, cleanup := setupMCPTestProjectWithStorage(t)
	defer cleanup()

	adapter := NewStorageProviderAdapter(provider)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	obj := map[string]any{
		"id":                 "GOAL-CYCLE118-001",
		objects.FieldKeyKind: "goal",
		"title":              "Test Goal Cycle 118",
	}
	err := adapter.Create(ctx, secCtx, obj)
	require.NoError(t, err)

	readObj, err := adapter.Read(ctx, secCtx, "GOAL-CYCLE118-001")
	require.NoError(t, err)
	assert.Equal(t, "GOAL-CYCLE118-001", readObj["id"])
}

func TestCycle118_DaemonControl_ProcessAlive(t *testing.T) {
	assert.True(t, processAlive(os.Getpid()))
	assert.False(t, processAlive(-1))
	assert.False(t, processAlive(99999999))
}

func TestCycle118_DaemonControl_GetSupervisePID(t *testing.T) {
	tempDir := t.TempDir()
	pidFile := filepath.Join(tempDir, "test.pid")

	pid, ok := getSupervisePID(pidFile)
	assert.False(t, ok)
	assert.Equal(t, 0, pid)

	err := writePIDFile(pidFile, os.Getpid())
	require.NoError(t, err)
	pid, ok = getSupervisePID(pidFile)
	assert.True(t, ok)
	assert.Equal(t, os.Getpid(), pid)

	err = writePIDFile(pidFile, 99999999)
	require.NoError(t, err)
	pid, ok = getSupervisePID(pidFile)
	assert.False(t, ok)
	assert.Equal(t, 0, pid)
}

func TestCycle118_DaemonControl_GetDaemonPIDFromPort(t *testing.T) {
	pid, ok := getDaemonPIDFromPort("not_a_number")
	assert.False(t, ok)
	assert.Equal(t, 0, pid)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, listener.Close())
	}()

	addr := listener.Addr().String()
	_, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	pid, ok = getDaemonPIDFromPort(portStr)
	assert.True(t, ok)
	assert.Equal(t, os.Getpid(), pid)
}

func TestCycle118_DaemonControl_ResolveMCPDaemonBinPath(t *testing.T) {
	tempRoot := t.TempDir()

	customBin := filepath.Join(tempRoot, "custom-zqk-bin")
	err := fileutil.WriteExecutableFile(customBin, []byte("#!/bin/sh\nexit 0\n"))
	require.NoError(t, err)

	t.Setenv(zqkenv.Bin().Name(), customBin)
	resolved := resolveMCPDaemonBinPath(tempRoot)
	assert.Equal(t, customBin, resolved)

	t.Setenv(zqkenv.Bin().Name(), "")
	stablePath := paths.StableBinaryPath(tempRoot)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(stablePath)))
	require.NoError(t, fileutil.WriteExecutableFile(stablePath, []byte("stable")))
	assert.Equal(t, stablePath, resolveMCPDaemonBinPath(tempRoot))

	assert.Equal(t, stablePath, getStableBinPath(tempRoot))
}

func TestCycle118_InstallCmd_OutsideProject(t *testing.T) {
	tempDir, cleanup := setupMCPTestProject(t)
	defer cleanup()

	cmd := NewInstallCmd()
	rootCmd := clitool.NewCommandBuilder("zqk").Build()
	rootCmd.AddCommand(cmd)
	rootCmd.SetArgs([]string{"install"})

	setMCPCLIContext(t, rootCmd, tempDir, nil)
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestCycle118_ProxyCmd_FlagError(t *testing.T) {
	cmd := NewProxyCmd()
	cmd.SetArgs([]string{"--invalid-flag-12345"})
	err := cmd.Execute()
	require.Error(t, err)
}

func TestCycle118_SuperviseCmd_StopBranches(t *testing.T) {
	tempDir, provider, cleanup := setupMCPTestProjectWithStorage(t)
	defer cleanup()

	port := "59876"
	addr := "127.0.0.1:" + port
	supPidFile := mcpSupervisePIDPath(tempDir, port)
	pidFile := mcpDaemonPIDPath(tempDir, port)

	require.NoError(t, writePIDFile(supPidFile, 99999998))
	require.NoError(t, writePIDFile(pidFile, 99999999))

	rootCmd := clitool.NewCommandBuilder("zqk").Build()
	mcpCmd := NewMCPCmd()
	rootCmd.AddCommand(mcpCmd)
	rootCmd.SetArgs([]string{"mcp", "supervise", "--tcp", addr, "--stop"})
	setMCPCLIContext(t, rootCmd, tempDir, provider)

	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestCycle118_EnsureCmd_Branches(t *testing.T) {
	tempDir, provider, cleanup := setupMCPTestProjectWithStorage(t)
	defer cleanup()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, listener.Close())
	}()

	addr := listener.Addr().String()

	rootCmd := clitool.NewCommandBuilder("zqk").Build()
	mcpCmd := NewMCPCmd()
	rootCmd.AddCommand(mcpCmd)
	rootCmd.SetArgs([]string{"mcp", "ensure", "--tcp", addr})
	setMCPCLIContext(t, rootCmd, tempDir, provider)

	err = rootCmd.Execute()
	require.NoError(t, err)
}

func TestCycle118_ListTools_CommandPathVariants(t *testing.T) {
	assert.Empty(t, commandPathFromTool(&pkgmcp.Tool{InputSchema: nil}))
	assert.Empty(t, commandPathFromTool(&pkgmcp.Tool{InputSchema: "not-map"}))
	assert.Empty(t, commandPathFromTool(&pkgmcp.Tool{InputSchema: map[string]any{}}))

	schemaNoCmd := map[string]any{
		"properties": map[string]any{"arg1": map[string]any{}},
	}
	assert.Empty(t, commandPathFromTool(&pkgmcp.Tool{InputSchema: schemaNoCmd}))

	schemaConst := map[string]any{
		"properties": map[string]any{
			"_command_path": map[string]any{"const": "workflow status"},
		},
	}
	assert.Equal(t, "workflow status", commandPathFromTool(&pkgmcp.Tool{InputSchema: schemaConst}))

	schemaDefault := map[string]any{
		"properties": map[string]any{
			"_command_path": map[string]any{"default": "object list"},
		},
	}
	assert.Equal(t, "object list", commandPathFromTool(&pkgmcp.Tool{InputSchema: schemaDefault}))
}

func TestCycle118_ListTools_RunListTools_WithCommand(t *testing.T) {
	cmd := NewListToolsCmd()
	require.NotNil(t, cmd)

	var buf bytes.Buffer
	err := runListTools(cmd, &buf, true)
	require.NoError(t, err)
}

func TestCycle118_ClearCobraCommandContexts(t *testing.T) {
	clearCobraCommandContexts(nil)

	root := &cobra.Command{Use: "root"}
	child := &cobra.Command{Use: "child"}
	root.AddCommand(child)

	root.SetContext(context.Background())
	child.SetContext(context.Background())

	clearCobraCommandContexts(root)
	assert.Nil(t, root.Context())
	assert.Nil(t, child.Context())
}

func TestCycle118_RunServe_TLSValidationErrors(t *testing.T) {
	server := pkgmcp.NewServer()
	initCtx := pkgctx.NewCliInitializationContext(func(s string) string { return "." }, ".")
	logger := logging.GetLoggerFromProfile("")

	tcpAddr = "127.0.0.1:8443"
	defer func() {
		tcpAddr = ""
		tlsCert = ""
		tlsKey = ""
		tlsCA = ""
	}()

	tlsCA = "ca.pem"
	tlsCert = ""
	tlsKey = ""
	err := runServerMethod(server, initCtx, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--tls-ca requires both")

	tlsCA = ""
	tlsCert = "cert.pem"
	tlsKey = ""
	err = runServerMethod(server, initCtx, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "both --tls-cert and --tls-key are required")

	tlsCert = ""
	tlsKey = "key.pem"
	err = runServerMethod(server, initCtx, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "both --tls-cert and --tls-key are required")
}

func TestCycle118_RunDaemon_FlagError(t *testing.T) {
	cmd := NewDaemonCmd()
	cmd.SetArgs([]string{"--invalid-flag-daemon"})
	err := cmd.Execute()
	require.Error(t, err)
}

func TestCycle118_RunDaemon_WithPort(t *testing.T) {
	tempDir, cleanup := setupMCPTestProject(t)
	defer cleanup()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	freeAddr := listener.Addr().String()
	_, freePortStr, err := net.SplitHostPort(freeAddr)
	require.NoError(t, err)
	require.NoError(t, listener.Close())

	freePort, err := strconv.Atoi(freePortStr)
	require.NoError(t, err)

	cmd := NewDaemonCmd()
	cmd.SetArgs([]string{"--port", strconv.Itoa(freePort)})
	setMCPCLIContext(t, cmd, tempDir, nil)

	errCh := make(chan error, 1)
	goroutinelabels.StartNamedGoroutine("mcp_cycle118_daemon", "run daemon on port", func() {
		errCh <- cmd.Execute()
	})

	waitForListener(t, fmt.Sprintf("127.0.0.1:%d", freePort))

	err = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	require.NoError(t, err)

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("daemon did not exit after SIGTERM within 3 seconds")
	}
}

func TestCycle118_SuperviseCmd_StatusOutputs(t *testing.T) {
	tempDir, provider, cleanup := setupMCPTestProjectWithStorage(t)
	defer cleanup()

	port := "59777"
	addr := "127.0.0.1:" + port

	rootCmd := clitool.NewCommandBuilder("zqk").Build()
	mcpCmd := NewMCPCmd()
	rootCmd.AddCommand(mcpCmd)
	rootCmd.SetArgs([]string{"mcp", "supervise", "--tcp", addr, "--status"})
	setMCPCLIContext(t, rootCmd, tempDir, provider)

	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestCycle118_DaemonControl_TCPPort_And_ParseContext(t *testing.T) {
	assert.Equal(t, "invalid-addr", tcpPort("invalid-addr"))
	assert.Equal(t, "9000", tcpPort("127.0.0.1:9000"))

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("tcp", "127.0.0.1:8443", "")
	addr, _, err := parseMCPCmdContext(cmd)
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:8443", addr)
}

func TestCycle118_InProcessCLIRunner_DeadlineAndEnv(t *testing.T) {
	tempDir, cleanup := setupMCPTestProject(t)
	defer cleanup()

	rootCmd := clitool.NewCommandBuilder("zqk").Build()
	runner := newInProcessCLIRunner(rootCmd, tempDir, &sync.Mutex{})
	require.NotNil(t, runner)

	t.Setenv(zqkenv.IsParentZqk().Name(), "1")
	t.Setenv(zqkenv.ParentPID().Name(), "12345")

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer cancel()

	_, err := runner(ctx, "", []string{"--help"})
	require.NoError(t, err)

	assert.Equal(t, "1", zqkenv.IsParentZqk().Get())
	assert.Equal(t, "12345", zqkenv.ParentPID().Get())
}

func TestCycle118_IDEAdapter_FlagError(t *testing.T) {
	cmd := NewIDEAdapterCmd()
	cmd.SetArgs([]string{"--invalid-flag-ide"})
	err := cmd.Execute()
	require.Error(t, err)
}

func TestCycle118_ProxyCmd_StartWithCancel(t *testing.T) {
	cmd := NewProxyCmd()
	cmd.SetArgs([]string{"--tcp", "127.0.0.1:0"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd.SetContext(ctx)

	err := cmd.Execute()
	if err != nil {
		assert.Contains(t, err.Error(), "context canceled")
	}
}

type failingStorageProvider struct {
	storage.ObjectStorageProvider
}

func (f *failingStorageProvider) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	return nil, assert.AnError
}

func TestCycle118_StorageProviderAdapter_ListError(t *testing.T) {
	adapter := NewStorageProviderAdapter(&failingStorageProvider{})
	require.NotNil(t, adapter)

	res, err := adapter.List(context.Background(), pkgctx.NewSystemSecurityContext(), pkgctx.NewStorageContext(), nil)
	require.Error(t, err)
	assert.Nil(t, res)
}

func TestCycle118_Subcommand_FlagErrors_Direct(t *testing.T) {
	tempDir, provider, cleanup := setupMCPTestProjectWithStorage(t)
	defer cleanup()
	t.Setenv(zqkenv.ProjectRoot().Name(), tempDir)
	t.Setenv(zqkenv.TestRoot().Name(), tempDir)

	bareCmd := &cobra.Command{Use: "bare"}
	setMCPCLIContext(t, bareCmd, tempDir, provider)

	err := runProxy(bareCmd, nil)
	require.Error(t, err)

	err = runIDEAdapter(bareCmd, nil)
	require.Error(t, err)

	err = runDaemon(bareCmd, nil)
	require.Error(t, err)

	err = runSupervise(bareCmd, nil)
	require.Error(t, err)

	err = runEnsure(bareCmd, nil)
	require.Error(t, err)
}

func TestCycle118_SuperviseCmd_StopWithAlivePIDs(t *testing.T) {
	tempDir, provider, cleanup := setupMCPTestProjectWithStorage(t)
	defer cleanup()

	port := "59776"
	addr := "127.0.0.1:" + port
	supPidFile := mcpSupervisePIDPath(tempDir, port)
	pidFile := mcpDaemonPIDPath(tempDir, port)

	child := testkit.ManagedCommand(t, t.Context(), "sleep", "10")
	require.NoError(t, child.Start())
	defer func() {
		if child.Process != nil {
			killErr := child.Process.Kill()
			if killErr != nil {
				// Process was already killed by supervise stop
			}
		}
	}()

	require.NoError(t, writePIDFile(supPidFile, child.Process.Pid))
	require.NoError(t, writePIDFile(pidFile, 99999999))

	rootCmd := clitool.NewCommandBuilder("zqk").Build()
	mcpCmd := NewMCPCmd()
	rootCmd.AddCommand(mcpCmd)
	rootCmd.SetArgs([]string{"mcp", "supervise", "--tcp", addr, "--stop"})
	setMCPCLIContext(t, rootCmd, tempDir, provider)

	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestCycle118_ListTools_CommandPathEmptyFallback(t *testing.T) {
	schemaEmptyConst := map[string]any{
		"properties": map[string]any{
			"_command_path": map[string]any{"const": ""},
		},
	}
	assert.Empty(t, commandPathFromTool(&pkgmcp.Tool{InputSchema: schemaEmptyConst}))

	schemaEmptyDefault := map[string]any{
		"properties": map[string]any{
			"_command_path": map[string]any{"default": ""},
		},
	}
	assert.Empty(t, commandPathFromTool(&pkgmcp.Tool{InputSchema: schemaEmptyDefault}))
}

func TestCycle118_RunDaemon_TCPAddrResolveBranch(t *testing.T) {
	tempDir, cleanup := setupMCPTestProject(t)
	defer cleanup()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	freeAddr := listener.Addr().String()
	require.NoError(t, listener.Close())

	cmd := NewDaemonCmd()
	cmd.SetArgs([]string{"--tcp", freeAddr})
	setMCPCLIContext(t, cmd, tempDir, nil)

	errCh := make(chan error, 1)
	goroutinelabels.StartNamedGoroutine("mcp_daemon_tcp_branch", "run daemon on tcp addr", func() {
		errCh <- cmd.Execute()
	})

	waitForListener(t, freeAddr)

	err = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	require.NoError(t, err)

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("daemon did not exit after SIGTERM within 3 seconds")
	}
}

func TestCycle118_EnsureCmd_DaemonSpawnFailure(t *testing.T) {
	tempDir, provider, cleanup := setupMCPTestProjectWithStorage(t)
	defer cleanup()

	t.Setenv(zqkenv.Bin().Name(), filepath.Join(tempDir, "non_existent_binary"))

	rootCmd := clitool.NewCommandBuilder("zqk").Build()
	mcpCmd := NewMCPCmd()
	rootCmd.AddCommand(mcpCmd)
	rootCmd.SetArgs([]string{"mcp", "ensure", "--tcp", "127.0.0.1:59998"})
	setMCPCLIContext(t, rootCmd, tempDir, provider)

	err := rootCmd.Execute()
	require.Error(t, err)
}

func TestCycle118_RunInstall_NoConfigsFound(t *testing.T) {
	tempDir, provider, cleanup := setupMCPTestProjectWithStorage(t)
	defer cleanup()

	require.NoError(t, fileutil.RemoveAll(filepath.Join(tempDir, ".cursor")))
	require.NoError(t, fileutil.RemoveAll(filepath.Join(tempDir, ".vscode")))

	cmd := NewInstallCmd()
	rootCmd := clitool.NewCommandBuilder("zqk").Build()
	rootCmd.AddCommand(cmd)
	rootCmd.SetArgs([]string{"install"})

	setMCPCLIContext(t, rootCmd, tempDir, provider)
	err := cmd.Execute()
	if err != nil {
		assert.Contains(t, err.Error(), "no supported IDE configurations")
	}
}

func TestCycle118_SuperviseCmd_AlreadyRunningBranch(t *testing.T) {
	tempDir, provider, cleanup := setupMCPTestProjectWithStorage(t)
	defer cleanup()

	addr := "127.0.0.1:59997"
	port := "59997"
	supPidFile := mcpSupervisePIDPath(tempDir, port)
	require.NoError(t, writePIDFile(supPidFile, os.Getpid()))

	rootCmd := clitool.NewCommandBuilder("zqk").Build()
	mcpCmd := NewMCPCmd()
	rootCmd.AddCommand(mcpCmd)
	rootCmd.SetArgs([]string{"mcp", "supervise", "--tcp", addr})
	setMCPCLIContext(t, rootCmd, tempDir, provider)

	err := rootCmd.Execute()
	require.NoError(t, err)
}
