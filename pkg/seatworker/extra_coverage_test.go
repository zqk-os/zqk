// BLI-STARTER-COMMUNITY-031 / PRI-STARTER-COMMUNITY-031 coverage elevation
package seatworker

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestInstall_EmptyRoot(t *testing.T) {
	t.Parallel()
	if err := Install(context.Background(), InstallConfig{}); err == nil {
		t.Fatal("empty root")
	}
}

func TestSanitizeAndUnitLabel(t *testing.T) {
	t.Parallel()
	if sanitizeSeat("  ") != "seat" {
		t.Fatal("blank")
	}
	if got := sanitizeSeat("peer/agent 1"); strings.Contains(got, "/") || strings.Contains(got, " ") {
		t.Fatalf("sanitize %q", got)
	}
	if !strings.Contains(UnitLabel("peer-1"), "peer-1") {
		t.Fatal("label")
	}
}

func TestResolveSeats_Requested(t *testing.T) {
	t.Parallel()
	got, err := resolveSeats("/tmp", []string{" a ", "", "b"})
	if err != nil || len(got) != 2 {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := resolveSeats("/tmp", []string{" ", ""}); err == nil {
		t.Fatal("all blank")
	}
}

func TestLinuxUnit_ExecuteNonComms(t *testing.T) {
	t.Parallel()
	body := linuxUnit("/proj", "/bin/zqk", "s1", objects.ConstPersonaDefaultOperator, 12, true)
	if !strings.Contains(body, "--execute-non-comms") {
		t.Fatal(body)
	}
	if !strings.Contains(body, "ExecStart=") {
		t.Fatal("unit")
	}
}

func TestPersonaForSeatDefault(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := fileutil.MkdirAll(filepath.Dir(paths.PeerSeatsPath(root)), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := agentfeed.SavePeerSeats(root, agentfeed.PeerSeatsFile{SchemaVersion: "1", Seats: map[string]agentfeed.PeerSeatRecord{}}); err != nil {
		t.Fatal(err)
	}
	if personaForSeat(root, "missing") != objects.ConstPersonaDefaultOperator {
		t.Fatal("default persona")
	}
}

func TestResolveWorkerBinary_Missing(t *testing.T) {
	t.Parallel()
	if _, err := resolveWorkerBinary(t.TempDir(), filepath.Join(t.TempDir(), "no-such-cli")); err == nil {
		t.Fatal("missing binary")
	}
}

func TestPersistSeatPersona_MCPDuty(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := fileutil.MkdirAll(filepath.Dir(paths.PeerSeatsPath(root)), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := agentfeed.SavePeerSeats(root, agentfeed.PeerSeatsFile{
		SchemaVersion: "1",
		Seats: map[string]agentfeed.PeerSeatRecord{
			"coord": {Wake: agentfeed.WakeMembraneMCP},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := persistSeatPersona(root, "coord", objects.ConstPersonaDefaultOperator); err != nil {
		t.Fatal(err)
	}
	loaded, err := agentfeed.LoadPeerSeats(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Seats["coord"].Duty != agentfeed.SeatKindCoordinator {
		t.Fatalf("duty=%q", loaded.Seats["coord"].Duty)
	}
}

func TestResolveSeats_NoAgentAPI(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := fileutil.MkdirAll(filepath.Dir(paths.PeerSeatsPath(root)), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := agentfeed.SavePeerSeats(root, agentfeed.PeerSeatsFile{
		SchemaVersion: "1",
		Seats: map[string]agentfeed.PeerSeatRecord{
			"stamp": {Wake: agentfeed.WakeMembraneStamp},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveSeats(root, nil); err == nil {
		t.Fatal("stamp-only")
	}
}

func TestProgramArgsAndUnitPaths(t *testing.T) {
	t.Parallel()
	xml := programArgsXML("/bin/zqk", "s1", "p", 9, true)
	if !strings.Contains(xml, "--execute-non-comms") || !strings.Contains(xml, "<string>") {
		t.Fatal(xml)
	}
	if _, err := linuxUnitPath("com.zqk.mesh.seat-worker.s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := darwinPlistPath("com.zqk.mesh.seat-worker.s1"); err != nil {
		t.Fatal(err)
	}
}

func TestExtraInstallWriteOnlyAndLoadDarwinCancel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	if paths.IsAgentWorktreePath(root) {
		t.Skip("temp dir classified as worktree")
	}
	bin := filepath.Join(root, "fake-cli")
	if err := fileutil.WriteFile(bin, []byte("#!/bin/sh\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := Install(context.Background(), InstallConfig{
		ProjectRoot:     root,
		Seats:           []string{"peer-1", "peer/2"},
		PersonaRef:      objects.ConstPersonaDefaultOperator,
		ExecuteNonComms: true,
		PollSeconds:     0,
		Binary:          bin,
		WriteOnly:       true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := Install(context.Background(), InstallConfig{
		ProjectRoot: filepath.Join(fileutil.TempDir(), "zqk-worktrees", "x"),
	}); err == nil {
		t.Fatal("expected worktree refuse")
	}
	t.Setenv("PATH", t.TempDir())
	t.Setenv(zqkenv.Bin().Name(), "")
	t.Setenv(zqkenv.StableBinaryPath().Name(), "")
	if _, err := resolveWorkerBinary(root, ""); err == nil {
		t.Fatal("missing product CLI")
	}
	if _, err := resolveWorkerBinary(root, root); err == nil {
		t.Fatal("dir is not binary")
	}
	_ = xmlText(`<>&"`)
	_ = darwinPlist(root, bin, "peer-1", objects.ConstPersonaDefaultOperator, 5, true, t.TempDir())
	_ = linuxUnit(root, bin, "peer-1", objects.ConstPersonaDefaultOperator, 5, true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	label := UnitLabel("extra-cov-061")
	_ = loadDarwin(ctx, label, filepath.Join(t.TempDir(), "missing.plist"))
	_ = loadDarwin(context.Background(), UnitLabel("extra-cov-061-retry"), filepath.Join(t.TempDir(), "missing.plist"))
	if err := Install(context.Background(), InstallConfig{
		ProjectRoot: root,
		Seats:       []string{"  "},
		Binary:      bin,
		WriteOnly:   true,
	}); err == nil {
		t.Fatal("expected no seats requested")
	}
	if err := Install(context.Background(), InstallConfig{
		ProjectRoot: root,
		Seats:       []string{"peer-empty-persona"},
		Binary:      bin,
		WriteOnly:   true,
	}); err != nil {
		t.Fatal(err)
	}
}
