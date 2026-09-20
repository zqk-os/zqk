package mcp

import (
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	pkgmcp "github.com/zqk-os/zqk/pkg/mcp"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// waitForListener blocks until addr accepts a connection, so the test observes
// readiness instead of guessing at a fixed sleep.
func waitForListener(t *testing.T, addr string) {
	t.Helper()
	const (
		timeout = 10 * time.Second
		poll    = 10 * time.Millisecond
	)
	deadline := time.After(timeout)
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		conn, err := net.DialTimeout("tcp", addr, poll)
		if err == nil {
			_ = conn.Close()
			return
		}
		select {
		case <-deadline:
			t.Fatalf("server did not start listening on %s within %s", addr, timeout)
		case <-ticker.C:
		}
	}
}

func TestSuperviseStatusPayload_Diagnostics(t *testing.T) {
	// Create a temp project root
	tempDir, err := fileutil.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tempDir)

	// Create a dummy spec file
	specsDir := filepath.Join(tempDir, ".zqk", "mcp", "specs")
	if err := fileutil.MkdirAll(specsDir, 0755); err != nil {
		t.Fatalf("Failed to create specs dir: %v", err)
	}
	specContent := `
name: dummy_spec
prompts:
  - name: dummy_prompt
    description: dummy
`
	if err := fileutil.WriteFile(filepath.Join(specsDir, "dummy.yaml"), []byte(specContent), 0644); err != nil {
		t.Fatalf("Failed to write dummy spec: %v", err)
	}

	// Start an actual mcp.Server
	server := pkgmcp.NewServer()

	// Set ProjectRoot using initCtx
	initCtx := pkgctx.NewCliInitializationContext(func(s string) string { return tempDir }, tempDir)
	server.SetCliInitializationContext(initCtx)

	// We need to serve on a random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	tcpAddr := listener.Addr().String()
	_ = listener.Close()

	goroutinelabels.NewGoroutine("mcp_daemon_control_test_serve", "serving MCP TCP for supervise status probe").
		StartSimple(func() {
			_ = server.ServeTCP(tcpAddr)
		})
	waitForListener(t, tcpAddr)

	// Create mock ProxyDaemon request manually or via superviseStatusPayload
	payload := superviseStatusPayload(tcpAddr, tcpPort(tcpAddr), "")

	daemonStat, ok := payload["daemon"].(map[string]any)
	if !ok {
		t.Fatalf("Expected daemon map")
	}

	mcpSpec, ok := daemonStat["mcp_spec"].(map[string]any)
	if !ok {
		t.Fatalf("Expected mcp_spec diagnostics inside daemon status, got: %v", daemonStat)
	}

	if mcpSpec["provenance"] != pkgmcp.MCPSpecProvenanceFallbackFile {
		t.Errorf("Expected provenance %s, got %v", pkgmcp.MCPSpecProvenanceFallbackFile, mcpSpec["provenance"])
	}
	if fmt.Sprintf("%v", mcpSpec["default_count"]) != "1" {
		t.Errorf("Expected default_count 1, got %v", mcpSpec["default_count"])
	}

	// Shutdown server
	server.RequestProcessShutdown("test done")
}
