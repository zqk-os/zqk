package meshbroker

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestMCPClientTransport_EnforcesTimeoutAndConcurrency(t *testing.T) {
	// Create a script that sleeps to simulate hanging MCP server
	tmpDir := t.TempDir()
	hangScript := filepath.Join(tmpDir, "hang.sh")
	if err := fileutil.WriteFile(hangScript, []byte("#!/bin/sh\nexec sleep 20\n"), paths.DirPerm755); err != nil {
		t.Fatalf("Failed to write hang script: %v", err)
	}

	transport := NewMCPClientTransport(hangScript)

	ctx := context.Background()
	errCh := make(chan error, 2)
	start := time.Now()

	go func() {
		_, err := transport.getClient(ctx, "endpoint1")
		errCh <- err
	}()

	go func() {
		time.Sleep(100 * time.Millisecond)
		_, err := transport.getClient(ctx, "endpoint2")
		errCh <- err
	}()

	timer := time.NewTimer(12 * time.Second)
	for i := 0; i < 2; i++ {
		select {
		case <-errCh:
		case <-timer.C:
			t.Fatalf("Test timed out! Transport did not enforce its own timeout or deadlocked.")
		}
	}

	duration := time.Since(start)
	if duration > 11*time.Second {
		t.Errorf("Took %v, expected concurrent execution to finish in < 11s", duration)
	}
}

func TestMCPClientTransport_defaultArgvUsesBrandCLI(t *testing.T) {
	origExe := brand.ExecutableName()
	t.Cleanup(func() { brand.SetExecutableName(origExe) })
	brand.SetExecutableName("acme-cli")

	cwd := t.TempDir()
	if err := fileutil.EnsureDir(filepath.Join(cwd, "bin")); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(cwd, "bin", "zqk-mcp"), []byte{0}, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(cwd)

	emptyRoot := t.TempDir()
	t.Setenv(zqkenv.Bin().Name(), "")
	t.Setenv(zqkenv.StableBinaryPath().Name(), "")
	t.Setenv(zqkenv.ProjectRoot().Name(), emptyRoot)
	t.Setenv(zqkenv.TestRoot().Name(), emptyRoot)

	bin, args := NewMCPClientTransport("").mcpArgv()
	if strings.Contains(bin, "zqk-mcp") || strings.Contains(bin, "./bin/") {
		t.Fatalf("default MCP argv used sidecar/cwd path %q", bin)
	}
	if !slices.Equal(args, paths.MCPServeArgs) {
		t.Fatalf("default MCP args %v, want %v", args, paths.MCPServeArgs)
	}

	override := filepath.Join(t.TempDir(), "hang.sh")
	obin, oargs := NewMCPClientTransport(override).mcpArgv()
	if obin != override || len(oargs) != 0 {
		t.Fatalf("override argv name=%q args=%v", obin, oargs)
	}
}
