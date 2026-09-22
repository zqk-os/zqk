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
