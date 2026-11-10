package hostservice_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scheduler/hostservice"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestGCLegacyHostGlobalMCPAndPW_removesWhenRootScopedPresent(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("LaunchAgents-shaped GC")
	}
	agents := t.TempDir()
	t.Setenv(zqkenv.LaunchAgentsDir().Name(), agents)

	legacyMCP := filepath.Join(agents, hostservice.LegacyMCPDaemonUnitLabel()+".plist")
	legacyPW := filepath.Join(agents, hostservice.LegacyPrivilegedWriterUnitLabel()+".plist")
	rootMCP := filepath.Join(agents, hostservice.MCPDaemonUnitLabel("abcdef0123456789")+".plist")
	rootPW := filepath.Join(agents, hostservice.PrivilegedWriterUnitLabel("abcdef0123456789")+".plist")
	body := `<?xml version="1.0"?><plist><dict></dict></plist>`
	for _, p := range []string{legacyMCP, legacyPW, rootMCP, rootPW} {
		if err := fileutil.WriteFile(p, []byte(body), paths.FilePerm644); err != nil {
			t.Fatal(err)
		}
	}

	removed, err := hostservice.GCLegacyHostGlobalMCPAndPW()
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 {
		t.Fatalf("want 2 removed, got %+v", removed)
	}
	if _, err := fileutil.Stat(legacyMCP); err == nil {
		t.Fatal("legacy mcp-daemon must be removed")
	}
	if _, err := fileutil.Stat(legacyPW); err == nil {
		t.Fatal("legacy privileged-writer must be removed")
	}
	if _, err := fileutil.Stat(rootMCP); err != nil {
		t.Fatal("root-scoped mcp-daemon must survive")
	}
	if _, err := fileutil.Stat(rootPW); err != nil {
		t.Fatal("root-scoped privileged-writer must survive")
	}
}

func TestGCLegacyHostGlobalMCPAndPW_noopWithoutRootScoped(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("LaunchAgents-shaped GC")
	}
	agents := t.TempDir()
	t.Setenv(zqkenv.LaunchAgentsDir().Name(), agents)
	legacyMCP := filepath.Join(agents, hostservice.LegacyMCPDaemonUnitLabel()+".plist")
	body := `<?xml version="1.0"?><plist><dict></dict></plist>`
	if err := fileutil.WriteFile(legacyMCP, []byte(body), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	removed, err := hostservice.GCLegacyHostGlobalMCPAndPW()
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 0 {
		t.Fatalf("must not remove sole legacy unit: %+v", removed)
	}
	if _, err := fileutil.Stat(legacyMCP); err != nil {
		t.Fatal("sole legacy unit must survive")
	}
}
