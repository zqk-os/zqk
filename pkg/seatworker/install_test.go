package seatworker

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scheduler/hostservice"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestDarwinPlist_noVendorLLM(t *testing.T) {
	t.Parallel()
	rootID := hostservice.RootID("/proj")
	body := darwinPlist("/proj", rootID, "/bin/product", "peer-agent-1", objects.ConstPersonaDefaultOperator, 30, true, "/proj/logs/mesh")
	if strings.Contains(body, "11434") || strings.Contains(strings.ToLower(body), "ollama") || strings.Contains(body, "qwen") {
		t.Fatal("plist must not embed vendor LLM defaults")
	}
	if !strings.Contains(body, "--execute-non-comms") {
		t.Fatal("expected execute-non-comms arg")
	}
	if !strings.Contains(body, xmlText(zqkenv.ProjectRoot().Name())) {
		t.Fatal("expected branded project-root env key")
	}
	if !strings.Contains(body, rootID) {
		t.Fatal("expected root_id in Label")
	}
}

func stableTestRoot(t *testing.T) (home, root string) {
	t.Helper()
	home = filepath.Join(fileutil.TempDir(), "zqk-seatworker-home-"+strings.ReplaceAll(t.Name(), "/", "_"))
	_ = fileutil.RemoveAll(home)
	root = filepath.Join(home, "Projects", "kernel")
	if err := fileutil.MkdirAll(root, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileutil.RemoveAll(home) })
	t.Setenv("HOME", home)
	t.Setenv(zqkenv.LaunchAgentsDir().Name(), filepath.Join(home, "Library", "LaunchAgents"))
	return home, root
}

func TestInstall_writeOnly(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("supervisor units are darwin/linux")
	}
	home, root := stableTestRoot(t)
	bin := filepath.Join(root, "product")
	if err := fileutil.WriteFile(bin, []byte("#!/bin/sh\n"), paths.FilePerm755); err != nil {
		t.Fatal(err)
	}
	seats := agentfeed.PeerSeatsFile{
		SchemaVersion: "1",
		Seats: map[string]agentfeed.PeerSeatRecord{
			"peer-agent-1": {Wake: agentfeed.WakeMembraneAgentAPI},
		},
	}
	if err := fileutil.MkdirAll(filepath.Dir(paths.PeerSeatsPath(root)), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := agentfeed.SavePeerSeats(root, seats); err != nil {
		t.Fatal(err)
	}
	if err := Install(context.Background(), InstallConfig{
		ProjectRoot:     root,
		Binary:          bin,
		ExecuteNonComms: true,
		PollSeconds:     15,
		WriteOnly:       true,
	}); err != nil {
		t.Fatal(err)
	}
	loaded, err := agentfeed.LoadPeerSeats(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Seats["peer-agent-1"].PersonaRef != objects.ConstPersonaDefaultOperator {
		t.Fatalf("persona=%q", loaded.Seats["peer-agent-1"].PersonaRef)
	}
	label := UnitLabel(hostservice.RootID(root), "peer-agent-1")
	var unit string
	if runtime.GOOS == "darwin" {
		unit = filepath.Join(home, "Library", "LaunchAgents", label+".plist")
	} else {
		unit = filepath.Join(home, ".config", "systemd", "user", label+".service")
	}
	raw, err := fileutil.ReadFile(unit)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "seat-worker") {
		t.Fatalf("unit missing seat-worker: %s", raw)
	}
	if !strings.Contains(string(raw), hostservice.RootID(root)) {
		t.Fatalf("unit missing root_id: %s", raw)
	}
}

func TestInstall_rejectsWorktreeRoot(t *testing.T) {
	err := Install(context.Background(), InstallConfig{
		ProjectRoot: "/tmp/zqk-worktrees/repo/ATK-1",
		WriteOnly:   true,
	})
	if err == nil {
		t.Fatal("expected worktree refuse")
	}
}

func TestInstall_rejectsGoTestTempDir(t *testing.T) {
	err := Install(context.Background(), InstallConfig{
		ProjectRoot: t.TempDir(),
		WriteOnly:   true,
	})
	if err == nil {
		t.Fatal("expected Go test TempDir refuse")
	}
}

func TestInstall_twoRootsDistinctLabels(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("supervisor units are darwin/linux")
	}
	base := filepath.Join(fileutil.TempDir(), "zqk-seatworker-two-roots")
	_ = fileutil.RemoveAll(base)
	t.Cleanup(func() { _ = fileutil.RemoveAll(base) })
	home := filepath.Join(base, "home")
	agents := filepath.Join(home, "Library", "LaunchAgents")
	t.Setenv("HOME", home)
	t.Setenv(zqkenv.LaunchAgentsDir().Name(), agents)
	writeRoot := func(name string) string {
		root := filepath.Join(base, name)
		if err := fileutil.MkdirAll(root, paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
		bin := filepath.Join(root, "bin-cli")
		if err := fileutil.WriteFile(bin, []byte("#!/bin/sh\n"), paths.FilePerm755); err != nil {
			t.Fatal(err)
		}
		seats := agentfeed.PeerSeatsFile{
			SchemaVersion: "1",
			Seats:         map[string]agentfeed.PeerSeatRecord{"peer-agent-1": {Wake: agentfeed.WakeMembraneAgentAPI}},
		}
		if err := fileutil.MkdirAll(filepath.Dir(paths.PeerSeatsPath(root)), paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
		if err := agentfeed.SavePeerSeats(root, seats); err != nil {
			t.Fatal(err)
		}
		if err := Install(context.Background(), InstallConfig{
			ProjectRoot: root, Binary: bin, WriteOnly: true,
		}); err != nil {
			t.Fatal(err)
		}
		return root
	}
	a := writeRoot("proj-a")
	b := writeRoot("proj-b")
	la := UnitLabel(hostservice.RootID(a), "peer-agent-1")
	lb := UnitLabel(hostservice.RootID(b), "peer-agent-1")
	if la == lb {
		t.Fatal("labels collided")
	}
	if _, err := fileutil.Stat(filepath.Join(agents, la+".plist")); err != nil && runtime.GOOS == "darwin" {
		t.Fatal(err)
	}
	if _, err := fileutil.Stat(filepath.Join(agents, lb+".plist")); err != nil && runtime.GOOS == "darwin" {
		t.Fatal(err)
	}
}
