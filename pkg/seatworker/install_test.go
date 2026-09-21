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
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestDarwinPlist_noVendorLLM(t *testing.T) {
	t.Parallel()
	body := darwinPlist("/proj", "/bin/product", "peer-agent-1", objects.ConstPersonaDefaultOperator, 30, true, "/proj/logs/mesh")
	if strings.Contains(body, "11434") || strings.Contains(strings.ToLower(body), "ollama") || strings.Contains(body, "qwen") {
		t.Fatal("plist must not embed vendor LLM defaults")
	}
	if !strings.Contains(body, "--execute-non-comms") {
		t.Fatal("expected execute-non-comms arg")
	}
	if !strings.Contains(body, xmlText(zqkenv.ProjectRoot().Name())) {
		t.Fatal("expected branded project-root env key")
	}
}

func TestInstall_writeOnly(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("supervisor units are darwin/linux")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
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
	label := UnitLabel("peer-agent-1")
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
