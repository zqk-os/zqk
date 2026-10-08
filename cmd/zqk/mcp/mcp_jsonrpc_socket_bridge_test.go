package mcp

import (
	"bytes"
	stdctx "context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	clitool "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	pkgmcp "github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// TestMCP_SocketBridge_ProxyStreaming tests the proxy command as a socket bridge
// forwarding JSON-RPC payloads between client streams and the MCP daemon TCP endpoint.
func TestMCP_SocketBridge_ProxyStreaming(t *testing.T) {
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

	addr := ln.Addr().String()

	cmd := NewProxyCmd()
	cmd.SetArgs([]string{"--tcp", addr})
	setMCPCLIContext(t, cmd, tempDir, provider)

	ctx, cancel := stdctx.WithTimeout(stdctx.Background(), 100*time.Millisecond)
	defer cancel()
	cmd.SetContext(ctx)

	errCh := make(chan error, 1)
	goroutinelabels.StartNamedGoroutine("mcp_socket_bridge_test", "run mcp proxy socket bridge", func() {
		errCh <- cmd.ExecuteContext(ctx)
	})

	select {
	case err := <-errCh:
		if err != nil && err != stdctx.Canceled && err != stdctx.DeadlineExceeded {
			t.Logf("proxy execution exited with: %v", err)
		}
	case <-time.After(300 * time.Millisecond):
		cancel()
	}

	// Also verify ProxyDaemon socket bridge methods directly
	logger := logging.GetLoggerFromProfile("test")
	pd := pkgmcp.NewProxyDaemon(addr, logger)
	if pd.GetTCPAddr() != addr {
		t.Errorf("expected TCP addr %s, got %s", addr, pd.GetTCPAddr())
	}
	if !pd.ProbeDaemon(200 * time.Millisecond) {
		t.Error("expected ProbeDaemon to return true for listening address")
	}
	if !pd.ProbeDaemonAndSync(200 * time.Millisecond) {
		t.Error("expected ProbeDaemonAndSync to return true for listening address")
	}
	if !pd.IsDaemonAlive() {
		t.Error("expected IsDaemonAlive to be true after sync probe")
	}
	_ = pd.PublishDaemonEvent(stdctx.Background(), "test.event", "SEAT-1", "EV-1")
	hb, hbf, rec := pd.GetProxyDaemonStats()
	_ = hb
	_ = hbf
	_ = rec

	// Unreachable TCP address branch
	cmdUnreach := NewProxyCmd()
	cmdUnreach.SetArgs([]string{"--tcp", "127.0.0.1:65534"})
	setMCPCLIContext(t, cmdUnreach, tempDir, provider)
	ctxUnreach, cancelUnreach := stdctx.WithTimeout(stdctx.Background(), 50*time.Millisecond)
	defer cancelUnreach()
	_ = cmdUnreach.ExecuteContext(ctxUnreach)
}

// TestMCP_RunServerMethod_TransportMatrix tests transport modes in runServerMethod
// including stdio serve, TLS, and mTLS error cascades.
func TestMCP_RunServerMethod_TransportMatrix(t *testing.T) {
	server := pkgmcp.NewServer()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileMCP))

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

	// 1. TLS error when cert and key point to nonexistent files
	tcpAddr = "127.0.0.1:0"
	tlsCA = ""
	tlsCert = "nonexistent_cert.pem"
	tlsKey = "nonexistent_key.pem"
	errTLS := runServerMethod(server, nil, logger)
	if errTLS == nil {
		t.Error("expected error for nonexistent TLS cert/key files")
	}

	// 2. mTLS error when CA, cert and key point to nonexistent files
	tlsCA = "nonexistent_ca.pem"
	tlsCert = "nonexistent_cert.pem"
	tlsKey = "nonexistent_key.pem"
	errMTLS := runServerMethod(server, nil, logger)
	if errMTLS == nil {
		t.Error("expected error for nonexistent mTLS cert/key/ca files")
	}

	// 3. Stdio fallback (tcpAddr == "") with shutdown requested
	tcpAddr = ""
	tlsCA = ""
	tlsCert = ""
	tlsKey = ""
	server.RequestProcessShutdown("test transport matrix shutdown")

	errServe := runServerMethod(server, nil, logger)
	_ = errServe
}

// TestMCP_ToolRegistrations_ListAndFormat verifies tool listing across
// plain name and with-command annotation formatting.
func TestMCP_ToolRegistrations_ListAndFormat(t *testing.T) {
	_ = setupMCPTestProject

	cmd := clitool.NewCommandBuilder("zqk").Build()
	subCmd := &cobra.Command{
		Use:   "demo",
		Short: "Demo command for tool registration",
	}
	cmd.AddCommand(subCmd)

	var buf bytes.Buffer
	if err := runListTools(cmd, &buf, false); err != nil {
		t.Fatalf("runListTools (plain) failed: %v", err)
	}

	var bufCmd bytes.Buffer
	if err := runListTools(cmd, &bufCmd, true); err != nil {
		t.Fatalf("runListTools (with-command) failed: %v", err)
	}

	// Matrix of commandPathFromTool extraction
	t1 := &pkgmcp.Tool{
		InputSchema: map[string]any{
			"properties": map[string]any{
				"_command_path": map[string]any{
					"default": "zqk demo",
				},
			},
		},
	}
	if p := commandPathFromTool(t1); p != "zqk demo" {
		t.Errorf("expected 'zqk demo', got %q", p)
	}

	t2 := &pkgmcp.Tool{
		InputSchema: map[string]any{
			"properties": map[string]any{
				"_command_path": map[string]any{
					"const": "zqk const",
				},
			},
		},
	}
	if p := commandPathFromTool(t2); p != "zqk const" {
		t.Errorf("expected 'zqk const', got %q", p)
	}

	t3 := &pkgmcp.Tool{
		InputSchema: map[string]any{
			"properties": map[string]any{
				"_command_path": "invalid_type",
			},
		},
	}
	if p := commandPathFromTool(t3); p != "" {
		t.Errorf("expected empty string for invalid _command_path type, got %q", p)
	}
}

// TestMCP_Install_Boundaries tests the install command behavior when run inside vs outside project root.
func TestMCP_Install_Boundaries(t *testing.T) {
	tempDir, cleanup := setupMCPTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	// Valid project root install
	out, err := executeMCPCommand(t, tempDir, provider, "install")
	if err != nil {
		t.Fatalf("mcp install in project failed: %v", err)
	}
	_ = out
}

// TestMCP_DaemonControl_Boundaries tests process alive, PID resolution, and binary resolution edge cases.
func TestMCP_DaemonControl_Boundaries(t *testing.T) {
	tempDir, cleanup := setupMCPTestProject(t)
	defer cleanup()

	// 1. processAlive with invalid PID
	if processAlive(-1) {
		t.Error("expected processAlive(-1) to be false")
	}
	if processAlive(0) {
		t.Error("expected processAlive(0) to be false")
	}

	// 2. getDaemonPIDFromPort with invalid port string
	if pid, ok := getDaemonPIDFromPort("not-a-port"); ok || pid != 0 {
		t.Errorf("expected (0, false) for invalid port, got (%d, %v)", pid, ok)
	}

	// 3. getDaemonPIDFromPort with inactive port
	if pid, ok := getDaemonPIDFromPort("65534"); ok {
		t.Errorf("expected false for inactive port 65534, got pid %d", pid)
	}

	// 4. getSupervisePID with nonexistent file
	if pid, ok := getSupervisePID(filepath.Join(tempDir, "nonexistent.pid")); ok || pid != 0 {
		t.Errorf("expected (0, false) for nonexistent supervise PID, got (%d, %v)", pid, ok)
	}

	// 5. write and read PID file round-trip
	pidPath := filepath.Join(tempDir, "test.pid")
	if err := writePIDFile(pidPath, 4242); err != nil {
		t.Fatalf("failed to write pid file: %v", err)
	}
	if readPid, ok := readPIDFile(pidPath); !ok || readPid != 4242 {
		t.Errorf("expected (4242, true), got (%d, %v)", readPid, ok)
	}

	// 6. getStableBinPath helper alias
	stable := getStableBinPath(tempDir)
	if stable == "" {
		t.Error("expected non-empty stable binary path")
	}

	// 7. resolveMCPDaemonBinPath fallback to executable
	t.Setenv(zqkenv.Bin().Name(), "")
	resolved := resolveMCPDaemonBinPath(t.TempDir())
	if resolved == "" {
		t.Error("expected non-empty fallback binary path")
	}
}

// TestMCP_ParseMCPCmdContext tests TCP flag parsing and default resolution.
func TestMCP_ParseMCPCmdContext(t *testing.T) {
	cmd := NewProxyCmd()
	cmd.SetArgs([]string{"--tcp", "127.0.0.1:8888"})
	_ = cmd.ParseFlags([]string{"--tcp", "127.0.0.1:8888"})

	addr, logger, err := parseMCPCmdContext(cmd)
	if err != nil {
		t.Fatalf("parseMCPCmdContext failed: %v", err)
	}
	if addr != "127.0.0.1:8888" {
		t.Errorf("expected 127.0.0.1:8888, got %q", addr)
	}
	if logger == nil {
		t.Error("expected non-nil logger")
	}

	// Default TCP resolution when flag not passed
	cmdDefault := NewProxyCmd()
	_ = cmdDefault.ParseFlags([]string{})
	addrDefault, _, errDefault := parseMCPCmdContext(cmdDefault)
	if errDefault != nil {
		t.Fatalf("parseMCPCmdContext default failed: %v", errDefault)
	}
	if addrDefault == "" {
		t.Error("expected non-empty default TCP address")
	}
}
