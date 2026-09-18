package agentfeed

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCoordinatorSeatID_dutyThenMCP(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, paths.ProjectDataDir, paths.StateDir, paths.MeshStateSubdir)
	if err := fileutil.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := PeerSeatsFile{
		SchemaVersion: "1",
		Seats: map[string]PeerSeatRecord{
			"peer-agent-1":    {Wake: WakeMembraneAgentAPI, Duty: SeatKindWorker},
			"peer-operator-1": {Wake: WakeMembraneMCP, Duty: SeatKindCoordinator},
			"peer-agent-2":    {Wake: WakeMembraneMCP},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(dir, paths.PeerSeatsFile), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := CoordinatorSeatID(root); got != "peer-operator-1" {
		t.Fatalf("CoordinatorSeatID=%q", got)
	}
}

func TestCoordinatorSeatID_emptyWithoutSeating(t *testing.T) {
	t.Parallel()
	if got := CoordinatorSeatID(t.TempDir()); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestWorkerSeatID_dutyThenAgentAPI(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writePeerSeats(t, root, map[string]PeerSeatRecord{
		"galaxy-worker-9": {Wake: WakeMembraneAgentAPI, Duty: SeatKindWorker},
		"ide-operator-3":  {Wake: WakeMembraneMCP, Duty: SeatKindCoordinator},
		"legacy-agy":      {Wake: WakeMembraneAgentAPI},
	})
	if got := WorkerSeatID(root); got != "galaxy-worker-9" {
		t.Fatalf("WorkerSeatID=%q", got)
	}
}

func TestSeatPersonaRef(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writePeerSeats(t, root, map[string]PeerSeatRecord{
		"antigravity-2": {Wake: WakeMembraneAgentAPI, Duty: SeatKindWorker, PersonaRef: "PER-ORCH-BETA"},
	})
	if got := SeatPersonaRef(root, "antigravity-2"); got != "PER-ORCH-BETA" {
		t.Fatalf("SeatPersonaRef=%q", got)
	}
	if got := SeatPersonaRef(root, "missing"); got != "" {
		t.Fatalf("missing seat = %q", got)
	}
	if got := SeatPersonaRef("", "antigravity-2"); got != "" {
		t.Fatalf("empty root = %q", got)
	}
}
