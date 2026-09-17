package mcp

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/brand"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// TestEnsureCommandWiresTCPFlag ensures mcp ensure exposes the expected flags.
func TestEnsureCommandWiresTCPFlag(t *testing.T) {
	cmd := NewEnsureCmd()
	f := cmd.Flags().Lookup("tcp")
	if f == nil {
		t.Fatal("expected --tcp flag on mcp ensure")
	}
	if f.DefValue != DefaultMCPDaemonTCP {
		t.Fatalf("expected --tcp default %s, got %q", DefaultMCPDaemonTCP, f.DefValue)
	}
	help := cmd.Long
	if !strings.Contains(help, "stable") {
		t.Fatalf("mcp ensure help should mention stable binary preference, got: %q", help)
	}
}

// TestResolveMCPDaemonBinPathPrefersZQKBinOverStable ensures mcp ensure honors ZQK_BIN
// over workshop stable when an explicit binary is set.
func TestResolveMCPDaemonBinPathPrefersZQKBinOverStable(t *testing.T) {
	tempRoot := t.TempDir()

	stablePath := paths.StableBinaryPath(tempRoot)
	if err := fileutil.EnsureDir(filepath.Dir(stablePath)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteExecutableFile(stablePath, []byte("stable")); err != nil {
		t.Fatal(err)
	}
	repoBin := paths.RepoBinPath(tempRoot)
	if err := fileutil.EnsureDir(filepath.Dir(repoBin)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteExecutableFile(repoBin, []byte("repo")); err != nil {
		t.Fatal(err)
	}

	t.Setenv(zqkenv.Bin().Name(), repoBin)
	bin := resolveMCPDaemonBinPath(tempRoot)
	if bin != repoBin {
		t.Fatalf("ZQK_BIN override: got %q want %q", bin, repoBin)
	}

	if !strings.HasSuffix(stablePath, brand.ZqkStableName) {
		t.Fatalf("stable path suffix: %q", stablePath)
	}
}

// TestResolveMCPDaemonBinPathPrefersStableOverTip ensures workshop stable wins over tip bin/zqk.
func TestResolveMCPDaemonBinPathPrefersStableOverTip(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv(zqkenv.Bin().Name(), "")

	stablePath := paths.StableBinaryPath(tempRoot)
	if err := fileutil.EnsureDir(filepath.Dir(stablePath)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteExecutableFile(stablePath, []byte("stable")); err != nil {
		t.Fatal(err)
	}
	repoBin := paths.RepoBinPath(tempRoot)
	if err := fileutil.EnsureDir(filepath.Dir(repoBin)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteExecutableFile(repoBin, []byte("tip")); err != nil {
		t.Fatal(err)
	}

	bin := resolveMCPDaemonBinPath(tempRoot)
	if bin != stablePath {
		t.Fatalf("stable preference: got %q want %q", bin, stablePath)
	}
}
