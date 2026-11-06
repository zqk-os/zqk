package hostservice_test

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scheduler/hostservice"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestMeshSeatWorkerUnitLabel_rootScopedDistinct(t *testing.T) {
	t.Parallel()
	a := hostservice.RootID("/Users/me/proj-a")
	b := hostservice.RootID("/Users/me/proj-b")
	if a == b {
		t.Fatal("expected distinct root ids")
	}
	la := hostservice.MeshSeatWorkerUnitLabel(a, "peer-agent-1")
	lb := hostservice.MeshSeatWorkerUnitLabel(b, "peer-agent-1")
	if la == lb {
		t.Fatalf("labels collide: %s", la)
	}
	if !strings.Contains(la, a) || !strings.Contains(lb, b) {
		t.Fatalf("labels missing root id: %s %s", la, lb)
	}
	legacy := hostservice.LegacyMeshSeatWorkerUnitLabel("peer-agent-1")
	if strings.Contains(legacy, a) {
		t.Fatalf("legacy should not embed root id: %s", legacy)
	}
}

func TestParseMeshSeatWorkerLabel(t *testing.T) {
	t.Parallel()
	root := hostservice.RootID("/stable/root")
	scoped := hostservice.MeshSeatWorkerUnitLabel(root, "peer-agent-1")
	p := hostservice.ParseMeshSeatWorkerLabel(scoped)
	if !p.Valid || p.Legacy || p.RootID != root || p.SeatID != "peer-agent-1" {
		t.Fatalf("%+v", p)
	}
	legacy := hostservice.LegacyMeshSeatWorkerUnitLabel("worker-1")
	lp := hostservice.ParseMeshSeatWorkerLabel(legacy)
	if !lp.Valid || !lp.Legacy || lp.SeatID != "worker-1" {
		t.Fatalf("%+v", lp)
	}
}

func TestRefuseMeshSeatWorkerRoot_TempDir(t *testing.T) {
	tmp := t.TempDir() // path contains TestRefuse...
	if err := hostservice.RefuseMeshSeatWorkerRoot(tmp); err == nil {
		t.Fatal("expected TempDir refuse")
	}
	if err := hostservice.RefuseMeshSeatWorkerRoot("/tmp/ATK-deadbeef/work"); err == nil {
		t.Fatal("expected ATK refuse")
	}
}

func TestRefuseMeshSeatWorkerRoot_stableUnderLaunchAgentsOverride(t *testing.T) {
	home := filepath.Join(fileutil.TempDir(), "zqk-hs-mesh-home-"+t.Name())
	_ = fileutil.RemoveAll(home)
	if err := fileutil.MkdirAll(home, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileutil.RemoveAll(home) })
	t.Setenv("HOME", home)
	root := filepath.Join(home, "Projects", "kernel-a")
	if err := fileutil.MkdirAll(root, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := hostservice.RefuseMeshSeatWorkerRoot(root); err != nil {
		t.Fatalf("stable root under fake home should be allowed: %v", err)
	}
}

func TestGCMeshSeatWorkerZombies_legacyAndTestRoot(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("LaunchAgents-shaped GC")
	}
	agents := t.TempDir()
	t.Setenv(zqkenv.LaunchAgentsDir().Name(), agents)
	// real home LaunchAgents must stay untouched — capture listing before/after
	realHome, _ := fileutil.UserHomeDir()
	realAgents := filepath.Join(realHome, "Library", "LaunchAgents")
	before, _ := fileutil.ReadDir(realAgents)

	legacyLabel := hostservice.LegacyMeshSeatWorkerUnitLabel("worker-1")
	legacyPlist := filepath.Join(agents, legacyLabel+".plist")
	zombieRoot := t.TempDir()
	body := `<?xml version="1.0"?><plist><dict>
  <key>WorkingDirectory</key><string>` + zombieRoot + `</string>
</dict></plist>`
	if err := fileutil.WriteFile(legacyPlist, []byte(body), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	liveHome := filepath.Join(fileutil.TempDir(), "zqk-hs-live-"+strings.ReplaceAll(t.Name(), "/", "_"))
	_ = fileutil.RemoveAll(liveHome)
	liveRoot := filepath.Join(liveHome, "proj")
	if err := fileutil.MkdirAll(liveRoot, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileutil.RemoveAll(liveHome) })
	liveLabel := hostservice.MeshSeatWorkerUnitLabel(hostservice.RootID(liveRoot), "peer-agent-1")
	livePlist := filepath.Join(agents, liveLabel+".plist")
	liveBody := `<?xml version="1.0"?><plist><dict>
  <key>WorkingDirectory</key><string>` + liveRoot + `</string>
</dict></plist>`
	if err := fileutil.WriteFile(livePlist, []byte(liveBody), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	removed, err := hostservice.GCMeshSeatWorkerZombies()
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0].Label != legacyLabel {
		t.Fatalf("removed=%+v", removed)
	}
	if _, err := fileutil.Stat(livePlist); err != nil {
		t.Fatal("live root-scoped unit must survive GC")
	}
	after, _ := fileutil.ReadDir(realAgents)
	if len(before) != len(after) {
		t.Fatalf("real LaunchAgents mutated: before=%d after=%d", len(before), len(after))
	}
}

func TestShouldCleanupLegacyMeshLabel(t *testing.T) {
	t.Parallel()
	legacy := hostservice.LegacyMeshSeatWorkerUnitLabel("peer-1")
	if !hostservice.ShouldCleanupLegacyMeshLabel(legacy, "") {
		t.Fatal("legacy must clean")
	}
	home := filepath.Join(fileutil.TempDir(), "zqk-hs-keep-"+t.Name())
	_ = fileutil.RemoveAll(home)
	root := filepath.Join(home, "k")
	if err := fileutil.MkdirAll(root, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileutil.RemoveAll(home) })
	scoped := hostservice.MeshSeatWorkerUnitLabel(hostservice.RootID(root), "peer-1")
	if hostservice.ShouldCleanupLegacyMeshLabel(scoped, root) {
		t.Fatal("live root-scoped must not cleanup-legacy")
	}
}
