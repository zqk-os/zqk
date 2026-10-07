package mcp

import (
	"bytes"
	stdctx "context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	clitool "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/dispatch"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	pkgmcp "github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

var mcpTestMu sync.Mutex

func setupMCPTestProject(t *testing.T) (string, func()) {
	mcpTestMu.Lock()
	t.Cleanup(func() { mcpTestMu.Unlock() })

	p := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:            "cmd.mcp.comprehensive",
		SeedSchemaPlane: true,
	})
	return p.Root, func() {}
}

func executeMCPCommand(t *testing.T, projectRoot string, provider storage.ObjectStorageProvider, args ...string) (string, error) {
	rootCmd := clitool.NewCommandBuilder("zqk").Build()
	if rootCmd.PersistentFlags().Lookup("format") == nil {
		rootCmd.PersistentFlags().String("format", "table", "Output format")
	}
	mcpCmd := NewMCPCmd()
	rootCmd.AddCommand(mcpCmd)

	fullArgs := append([]string{"mcp"}, args...)
	rootCmd.SetArgs(fullArgs)

	var buf bytes.Buffer
	testCtx := setMCPCLIContext(t, rootCmd, projectRoot, provider)
	testCtx = pkgctx.WithCommandOutputWriter(testCtx, &buf)

	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetContext(testCtx)

	var propagate func(c *cobra.Command)
	propagate = func(c *cobra.Command) {
		c.SetContext(testCtx)
		c.SetOut(&buf)
		c.SetErr(&buf)
		for _, child := range c.Commands() {
			propagate(child)
		}
	}
	propagate(rootCmd)

	err := rootCmd.ExecuteContext(testCtx)
	return buf.String(), err
}

func setMCPCLIContext(t *testing.T, cmd *cobra.Command, projectRoot string, storageProvider storage.ObjectStorageProvider) stdctx.Context {
	cmdStandardContext := cmd.Context()
	if cmdStandardContext == nil {
		cmdStandardContext = stdctx.Background()
	}
	if cli.GetStorageProvider(cmdStandardContext) == nil {
		cmdStandardContext = cli.WithStorageProvider(cmdStandardContext, storageProvider)
	}
	cmd.SetContext(cmdStandardContext)

	initCtx := pkgctx.NewCliInitializationContext(func(s string) string { return projectRoot }, projectRoot)
	cliContextWrapper, err := cli.GetContextFromCommand(cmd, initCtx)
	if err != nil {
		t.Fatalf("Failed to get CLI context wrapper: %v", err)
	}
	cli.SetContext(cmd, cliContextWrapper)
	return cmdStandardContext
}

func TestInProcess_MCPCommandTree_Matrix(t *testing.T) {
	cmd := NewMCPCmd()
	if cmd.Use != "mcp" {
		t.Fatalf("unexpected use: %s", cmd.Use)
	}
	subcommands := map[string]bool{}
	for _, c := range cmd.Commands() {
		subcommands[c.Name()] = true
	}
	expected := []string{"serve", "ide-adapter", "cursor-adapter", "proxy", "list-tools", "install", "daemon", "ensure", "supervise"}
	for _, exp := range expected {
		if !subcommands[exp] {
			t.Errorf("missing expected subcommand %s", exp)
		}
	}
}

func TestInProcess_MCPListTools_Matrix(t *testing.T) {
	tempDir, cleanup := setupMCPTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	out, err := executeMCPCommand(t, tempDir, provider, "list-tools")
	if err != nil {
		t.Fatalf("list-tools failed: %v", err)
	}
	if out == "" {
		t.Logf("list-tools returned empty output (no tools exposed without root command setup)")
	}

	outCmd, err := executeMCPCommand(t, tempDir, provider, "list-tools", "--with-command")
	if err != nil {
		t.Fatalf("list-tools --with-command failed: %v", err)
	}
	_ = outCmd
}

func TestInProcess_MCPInstall_Matrix(t *testing.T) {
	tempDir, cleanup := setupMCPTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	out, err := executeMCPCommand(t, tempDir, provider, "install")
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}
	_ = out
}

func TestInProcess_MCPSupervise_Matrix(t *testing.T) {
	tempDir, cleanup := setupMCPTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	// supervise status
	outStatus, err := executeMCPCommand(t, tempDir, provider, "supervise", "--status", "--format", "json")
	if err != nil {
		t.Fatalf("supervise --status failed: %v", err)
	}
	if !strings.Contains(outStatus, "listening") {
		t.Errorf("expected supervise status json output, got: %s", outStatus)
	}

	// supervise stop
	outStop, err := executeMCPCommand(t, tempDir, provider, "supervise", "--stop")
	if err != nil {
		t.Fatalf("supervise --stop failed: %v", err)
	}
	_ = outStop
}

func setupMockDaemonBinary(t *testing.T, dir string) string {
	mockExe := filepath.Join(dir, "mock-zqk")
	if err := fileutil.WriteStandardFile(mockExe, []byte("#!/bin/sh\nexit 0\n")); err != nil {
		t.Fatalf("failed to write mock exe: %v", err)
	}
	if err := os.Chmod(mockExe, 0755); err != nil {
		t.Fatalf("failed to chmod mock exe: %v", err)
	}
	t.Setenv(zqkenv.Bin().Name(), mockExe)
	return mockExe
}

func TestInProcess_MCPEnsure_Matrix(t *testing.T) {
	tempDir, cleanup := setupMCPTestProject(t)
	defer cleanup()
	setupMockDaemonBinary(t, tempDir)

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	out, err := executeMCPCommand(t, tempDir, provider, "ensure", "--tcp", "127.0.0.1:0")
	if err != nil {
		t.Logf("ensure result: %v", err)
	}
	_ = out
}

func TestInProcess_MCPDaemonControl_Helpers(t *testing.T) {
	tempDir, cleanup := setupMCPTestProject(t)
	defer cleanup()

	// 1. resolveTCPFlag and tcpPort
	if got := resolveTCPFlag(""); got != DefaultMCPDaemonTCP {
		t.Errorf("expected default TCP, got %s", got)
	}
	if got := resolveTCPFlag("127.0.0.1:9090"); got != "127.0.0.1:9090" {
		t.Errorf("expected custom TCP, got %s", got)
	}
	if port := tcpPort("127.0.0.1:8888"); port != "8888" {
		t.Errorf("expected 8888, got %s", port)
	}

	// 2. projectRootOrResolve
	if r := projectRootOrResolve(tempDir); r != tempDir {
		t.Errorf("expected tempDir, got %s", r)
	}
	if r := projectRootOrResolve(""); r == "" {
		t.Error("expected non-empty root from resolve")
	}

	// 3. PID file operations
	pidFile := filepath.Join(tempDir, "daemon.pid")
	curPID := os.Getpid()
	if err := writePIDFile(pidFile, curPID); err != nil {
		t.Fatalf("writePIDFile failed: %v", err)
	}
	pid, ok := readPIDFile(pidFile)
	if !ok || pid != curPID {
		t.Errorf("readPIDFile got (%d, %v), want (%d, true)", pid, ok, curPID)
	}
	if gotPID, ok := getSupervisePID(pidFile); !ok || gotPID != curPID {
		t.Errorf("getSupervisePID got (%d, %v), want (%d, true)", gotPID, ok, curPID)
	}
	_ = mcpDaemonPIDPath(tempDir, "8888")
	_ = mcpSupervisePIDPath(tempDir, "8888")
	_, _ = getDaemonPIDFromPort("8888")
	killPIDBestEffort(999999)
}

func TestInProcess_StorageProviderAdapter(t *testing.T) {
	tempDir, cleanup := setupMCPTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	adapter := NewStorageProviderAdapter(provider)
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}
	if nilAdapter := NewStorageProviderAdapter(nil); nilAdapter != nil {
		t.Error("expected nil adapter for nil storage")
	}

	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// Create object via adapter
	obj := map[string]any{
		objects.FieldKeyID:     "PER-MCP-ADAPTER-TEST",
		objects.FieldKeyKind:   "persona",
		objects.FieldKeyTitle:  "MCP Adapter Test",
		objects.FieldKeyRole:   "tester",
		objects.FieldKeyStatus: "approved",
	}
	if err := adapter.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("adapter Create failed: %v", err)
	}

	// Read object via adapter
	readObj, err := adapter.Read(ctx, secCtx, "PER-MCP-ADAPTER-TEST")
	if err != nil {
		t.Fatalf("adapter Read failed: %v", err)
	}
	if objects.GetString(readObj, objects.FieldKeyID) != "PER-MCP-ADAPTER-TEST" {
		t.Errorf("unexpected read object: %+v", readObj)
	}

	// List objects via adapter
	listRes, err := adapter.List(ctx, secCtx, storageCtx, map[string]any{
		objects.FieldKeyKind: "persona",
	})
	if err != nil {
		t.Fatalf("adapter List failed: %v", err)
	}
	if listRes == nil {
		t.Error("expected non-nil list result")
	}
}

func TestInProcess_CoordinatorIntegration(t *testing.T) {
	tempDir, cleanup := setupMCPTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	server := pkgmcp.NewServer()
	SetupMCPCoordinatorIntegration(server, tempDir, provider)

	coordinator := CreateMCPCoordinatorWithRouters(tempDir, provider, server.GetMCPMetrics())
	if coordinator == nil {
		t.Fatal("expected non-nil coordinator")
	}
}

func TestInProcess_CLIRunnerAndAdapters(t *testing.T) {
	tempDir, cleanup := setupMCPTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	// 1. Adapters test
	specLoader := objects.NewSpecLoader("")
	loaderAdapter := &specLoaderAdapter{loader: specLoader}
	if spec, err := loaderAdapter.LoadSpecWithInheritance("persona.yaml"); err == nil && spec != nil {
		fields := spec.GetResolvedFields()
		_ = fields
	}

	logger := logging.GetLoggerFromProfile("test")
	logAdapter := &loggerAdapter{logger: logger}
	logAdapter.Debug("test debug log", pkgmcp.LogField{Key: "k", Value: "v"})

	// 2. newInProcessCLIRunner test
	rootCmd := clitool.NewCommandBuilder("zqk").Build()
	rootCmd.AddCommand(NewMCPCmd())
	cliExecMu := &sync.Mutex{}
	runner := newInProcessCLIRunner(rootCmd, tempDir, cliExecMu)

	dispatch.StartGlobalPool(stdctx.Background())
	defer dispatch.StopGlobalPool()

	ctx, cancel := stdctx.WithTimeout(stdctx.Background(), 2*time.Second)
	defer cancel()

	res, err := runner(ctx, "mcp list-tools", []string{"mcp", "list-tools"})
	if err != nil {
		t.Logf("in-process CLI runner executed: %v", err)
	}
	_ = res
}

func TestInProcess_MCPServe_TCPCancellation(t *testing.T) {
	tempDir, cleanup := setupMCPTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	ctx, cancel := stdctx.WithTimeout(stdctx.Background(), 200*time.Millisecond)
	defer cancel()

	cmd := clitool.NewCommandBuilder("zqk").Build()
	mcpCmd := NewMCPCmd()
	cmd.AddCommand(mcpCmd)
	cmd.SetArgs([]string{"mcp", "serve", "--tcp", "127.0.0.1:0"})

	setMCPCLIContext(t, cmd, tempDir, provider)
	cmd.SetContext(ctx)

	errCh := make(chan error, 1)
	goroutinelabels.StartNamedGoroutine("mcp_serve_test", "test mcp serve execution", func() {
		errCh <- cmd.ExecuteContext(ctx)
	})

	select {
	case err := <-errCh:
		if err != nil && err != stdctx.Canceled && err != stdctx.DeadlineExceeded {
			t.Logf("serve exit: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		cancel()
	}
}

func TestInProcess_MCPDaemon_PortFlag(t *testing.T) {
	tempDir, cleanup := setupMCPTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	cmd := NewDaemonCmd()
	cmd.SetArgs([]string{"--port", "65533"})
	if cmd.Flags().Lookup("port") == nil {
		t.Error("daemon missing --port flag")
	}
	if cmd.Flags().Lookup("tcp") == nil {
		t.Error("daemon missing --tcp flag")
	}
}

func TestInProcess_CommandPathFromTool_Matrix(t *testing.T) {
	if got := commandPathFromTool(&pkgmcp.Tool{InputSchema: nil}); got != "" {
		t.Errorf("expected empty for nil schema, got %q", got)
	}
	if got := commandPathFromTool(&pkgmcp.Tool{InputSchema: "invalid"}); got != "" {
		t.Errorf("expected empty for non-map schema, got %q", got)
	}
	if got := commandPathFromTool(&pkgmcp.Tool{InputSchema: map[string]any{"type": "object"}}); got != "" {
		t.Errorf("expected empty for schema without properties, got %q", got)
	}
	if got := commandPathFromTool(&pkgmcp.Tool{InputSchema: map[string]any{"properties": map[string]any{}}}); got != "" {
		t.Errorf("expected empty for empty properties, got %q", got)
	}
	schemaWithConst := map[string]any{
		"properties": map[string]any{
			"_command_path": map[string]any{
				"const": "zqk.goal.list",
			},
		},
	}
	if got := commandPathFromTool(&pkgmcp.Tool{InputSchema: schemaWithConst}); got != "zqk.goal.list" {
		t.Errorf("expected zqk.goal.list, got %q", got)
	}
	schemaWithDefault := map[string]any{
		"properties": map[string]any{
			"_command_path": map[string]any{
				"default": "zqk.plan.list",
			},
		},
	}
	if got := commandPathFromTool(&pkgmcp.Tool{InputSchema: schemaWithDefault}); got != "zqk.plan.list" {
		t.Errorf("expected zqk.plan.list, got %q", got)
	}
}

func TestInProcess_RunServerMethod_TLSValidation(t *testing.T) {
	tempDir, cleanup := setupMCPTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	server := pkgmcp.NewServer()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileMCP))

	// Save global flags
	origTCP := tcpAddr
	origCA := tlsCA
	origCert := tlsCert
	origKey := tlsKey
	defer func() {
		tcpAddr = origTCP
		tlsCA = origCA
		tlsCert = origCert
		tlsKey = origKey
	}()

	tcpAddr = "127.0.0.1:0"
	tlsCA = "ca.pem"
	tlsCert = ""
	tlsKey = ""
	if err := runServerMethod(server, nil, logger); err == nil || !strings.Contains(err.Error(), "--tls-ca requires both") {
		t.Errorf("expected --tls-ca validation error, got: %v", err)
	}

	tlsCA = ""
	tlsCert = "cert.pem"
	tlsKey = ""
	if err := runServerMethod(server, nil, logger); err == nil || !strings.Contains(err.Error(), "both --tls-cert and --tls-key are required") {
		t.Errorf("expected TLS pair error (missing key), got: %v", err)
	}

	tlsCert = ""
	tlsKey = "key.pem"
	if err := runServerMethod(server, nil, logger); err == nil || !strings.Contains(err.Error(), "both --tls-cert and --tls-key are required") {
		t.Errorf("expected TLS pair error (missing cert), got: %v", err)
	}
}

type testMockMCPRouter struct {
	emitted any
}

func (m *testMockMCPRouter) Emit(ctx stdctx.Context, eventCtx any) error {
	m.emitted = eventCtx
	return nil
}

type testMockCoordinator struct {
	emitted *coordination.EventContext
}

func (m *testMockCoordinator) Emit(ctx stdctx.Context, eventCtx *coordination.EventContext) error {
	m.emitted = eventCtx
	return nil
}

func (m *testMockCoordinator) Subscribe(subscriber coordination.OperationalEventSubscriber) string {
	return "sub-1"
}

func (m *testMockCoordinator) Unsubscribe(subscriberID string) {}

func TestInProcess_CoordinatorAdapters_Emit(t *testing.T) {
	mockRouter := &testMockMCPRouter{}
	routerAdapter := &mcpMetricsRouterAdapter{mcpRouter: mockRouter}
	evCtx := &coordination.EventContext{OperationID: "EVT-TEST-001"}

	if err := routerAdapter.Emit(stdctx.Background(), evCtx); err != nil {
		t.Fatalf("routerAdapter Emit failed: %v", err)
	}
	if mockRouter.emitted != evCtx {
		t.Errorf("unexpected emitted event: %+v", mockRouter.emitted)
	}

	mockCoord := &testMockCoordinator{}
	coordAdapter := &coordinatorEventAdapter{coordinator: mockCoord}
	if err := coordAdapter.Emit(stdctx.Background(), evCtx); err != nil {
		t.Fatalf("coordAdapter Emit failed: %v", err)
	}
	if mockCoord.emitted != evCtx {
		t.Errorf("unexpected coordinator event: %+v", mockCoord.emitted)
	}
	// Non-matching type does nothing
	if err := coordAdapter.Emit(stdctx.Background(), "invalid-payload"); err != nil {
		t.Errorf("expected nil error for unrecognized event payload, got %v", err)
	}
}

func TestInProcess_DaemonControl_DirectHelpers(t *testing.T) {
	tempDir, cleanup := setupMCPTestProject(t)
	defer cleanup()

	logPath := mcpSuperviseLogPath(tempDir)
	if !strings.Contains(logPath, "mcp-daemon-supervise.log") {
		t.Errorf("unexpected supervise log path: %s", logPath)
	}

	stableBin := getStableBinPath(tempDir)
	if stableBin == "" {
		t.Error("expected non-empty stable bin path")
	}

	if pid, ok := getDaemonPIDFromPort("not-a-number"); ok || pid != 0 {
		t.Errorf("expected 0, false for invalid port, got %d, %v", pid, ok)
	}
}

func TestInProcess_Supervise_Branches(t *testing.T) {
	tempDir, cleanup := setupMCPTestProject(t)
	defer cleanup()
	setupMockDaemonBinary(t, tempDir)

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	// Direct call to runEnsureOnce
	runEnsureOnce(tempDir, "127.0.0.1:0")

	// Supervise with already running PID
	supPidPath := mcpSupervisePIDPath(tempDir, "9998")
	_ = writePIDFile(supPidPath, os.Getpid())

	rootCmd := clitool.NewCommandBuilder("zqk").Build()
	mcpCmd := NewMCPCmd()
	rootCmd.AddCommand(mcpCmd)
	rootCmd.SetArgs([]string{"mcp", "supervise", "--tcp", "127.0.0.1:9998"})
	setMCPCLIContext(t, rootCmd, tempDir, provider)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("supervise already-running branch failed: %v", err)
	}

	// Supervise with --loop and immediate cancelled context
	loopCtx, cancel := stdctx.WithCancel(stdctx.Background())
	cancel() // cancel immediately

	loopCmd := clitool.NewCommandBuilder("zqk").Build()
	loopMcpCmd := NewMCPCmd()
	loopCmd.AddCommand(loopMcpCmd)
	loopCmd.SetArgs([]string{"mcp", "supervise", "--tcp", "127.0.0.1:9997", "--loop"})
	setMCPCLIContext(t, loopCmd, tempDir, provider)
	loopCmd.SetContext(loopCtx)

	if err := loopCmd.ExecuteContext(loopCtx); err != nil && err != stdctx.Canceled {
		t.Fatalf("supervise loop cancelled context failed: %v", err)
	}
}

func TestInProcess_ResolveMCPDaemonBinPath_Branches(t *testing.T) {
	tempDir, cleanup := setupMCPTestProject(t)
	defer cleanup()

	// 1. ZQK_BIN environment variable branch
	dummyBin := filepath.Join(tempDir, "custom-bin")
	if err := fileutil.WriteStandardFile(dummyBin, []byte("#!/bin/sh\n")); err != nil {
		t.Fatalf("failed to write custom bin: %v", err)
	}
	t.Setenv(zqkenv.Bin().Name(), dummyBin)
	if got := resolveMCPDaemonBinPath(tempDir); got != dummyBin {
		t.Errorf("expected %s, got %s", dummyBin, got)
	}

	// 2. Stable binary candidates branch
	t.Setenv(zqkenv.Bin().Name(), "")
	candidates := paths.StableBinaryCandidates(tempDir)
	if len(candidates) > 0 {
		stablePath := candidates[0]
		if err := fileutil.EnsureDir(filepath.Dir(stablePath)); err == nil {
			_ = fileutil.WriteStandardFile(stablePath, []byte("#!/bin/sh\n"))
			if got := resolveMCPDaemonBinPath(tempDir); got != stablePath {
				t.Errorf("expected stable path %s, got %s", stablePath, got)
			}
			_ = fileutil.Remove(stablePath)
		}
	}

	// 3. Repo bin path branch
	repoBin := paths.RepoBinPath(tempDir)
	if err := fileutil.EnsureDir(filepath.Dir(repoBin)); err == nil {
		_ = fileutil.WriteStandardFile(repoBin, []byte("#!/bin/sh\n"))
		if got := resolveMCPDaemonBinPath(tempDir); got != repoBin {
			t.Errorf("expected repo bin %s, got %s", repoBin, got)
		}
		_ = fileutil.Remove(repoBin)
	}
}

func TestInProcess_SpecAdapter_Branches(t *testing.T) {
	loader := objects.NewSpecLoader("")
	adapter := &specLoaderAdapter{loader: loader}

	// Error branch: loading nonexistent file
	spec, err := adapter.LoadSpecWithInheritance("nonexistent_spec_file.yaml")
	if err == nil || spec != nil {
		t.Errorf("expected error loading nonexistent spec, got spec=%v, err=%v", spec, err)
	}

	// loggerAdapter branch with fields
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileMCP))
	la := &loggerAdapter{logger: logger}
	la.Debug("test debug without fields")
	la.Debug("test debug with fields", pkgmcp.LogField{Key: "k1", Value: "v1"})
}

func TestInProcess_MCPEnsure_AlreadyListening(t *testing.T) {
	tempDir, cleanup := setupMCPTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen on tcp: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	rootCmd := clitool.NewCommandBuilder("zqk").Build()
	mcpCmd := NewMCPCmd()
	rootCmd.AddCommand(mcpCmd)
	rootCmd.SetArgs([]string{"mcp", "ensure", "--tcp", addr})
	setMCPCLIContext(t, rootCmd, tempDir, provider)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("ensure already listening failed: %v", err)
	}
}

func TestInProcess_Supervise_SpawnMockSupervisor(t *testing.T) {
	tempDir, cleanup := setupMCPTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	mockExe := filepath.Join(tempDir, "mock-zqk")
	if err := fileutil.WriteStandardFile(mockExe, []byte("#!/bin/sh\nexit 0\n")); err != nil {
		t.Fatalf("failed to write mock exe: %v", err)
	}
	if err := os.Chmod(mockExe, 0755); err != nil {
		t.Fatalf("failed to chmod mock exe: %v", err)
	}
	t.Setenv(zqkenv.Bin().Name(), mockExe)

	rootCmd := clitool.NewCommandBuilder("zqk").Build()
	mcpCmd := NewMCPCmd()
	rootCmd.AddCommand(mcpCmd)
	rootCmd.SetArgs([]string{"mcp", "supervise", "--tcp", "127.0.0.1:9996"})
	setMCPCLIContext(t, rootCmd, tempDir, provider)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("supervise spawn branch failed: %v", err)
	}
}

func TestInProcess_DaemonControl_ProcessAliveAndPID(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Error("expected current process to be alive")
	}
	if err := writePIDFile("/dev/null/impossible/pid.pid", 1234); err == nil {
		t.Error("expected error writing to invalid pid path")
	}
}
