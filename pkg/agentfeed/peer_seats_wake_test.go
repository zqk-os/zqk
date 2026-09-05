package agentfeed

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func writePeerSeats(t *testing.T, root string, seats map[string]PeerSeatRecord) {
	t.Helper()
	dir := filepath.Join(root, paths.ProjectDataDir, paths.StateDir, paths.MeshStateSubdir)
	if err := fileutil.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f := PeerSeatsFile{SchemaVersion: "1", Seats: seats}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(paths.PeerSeatsPath(root), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveWakeMembrane_fromPeerSeats(t *testing.T) {
	root := t.TempDir()
	writePeerSeats(t, root, map[string]PeerSeatRecord{
		"any-vendor-ide-07": {Wake: "mcp"},
		"worker-seat-9":     {Wake: "agentapi", PID: 42},
	})
	if got := ResolveWakeMembrane(root, "any-vendor-ide-07"); got != WakeMembraneMCP {
		t.Fatalf("ide membrane=%q", got)
	}
	if got := ResolveWakeMembrane(root, "worker-seat-9"); got != WakeMembraneAgentAPI {
		t.Fatalf("worker membrane=%q", got)
	}
	if SeatKindForAgentIn(root, "any-vendor-ide-07") != SeatKindCoordinator {
		t.Fatal("mcp seat should use coordinator adapter bucket")
	}
}

func TestWakePeer_arbitrarySeatMCPMembraneIsLive(t *testing.T) {
	root := t.TempDir()
	writePeerSeats(t, root, map[string]PeerSeatRecord{
		"galaxy-composer-3": {Wake: WakeMembraneMCP},
	})
	fake := &fakeWakeAdapter{endpoint: "adapter://mcp"}
	res := WakePeerOpts(context.Background(), WakePeerOptions{
		ProjectRoot:          root,
		ToAgentID:            "galaxy-composer-3",
		Message:              "ATTN seat: bus probe",
		DeliveryMode:         datacell.DeliveryModeNotify,
		Adapter:              fake,
		MCPIPCDelivered:      true,
		MCPSubscribersProbed: true,
		MCPSubscriberCount:   1,
	})
	if !res.Live || res.Transport != TransportMCPActionRequired {
		t.Fatalf("live=%v transport=%q", res.Live, res.Transport)
	}
	if fake.last().SeatKind != SeatKindCoordinator {
		t.Fatalf("seat kind=%q", fake.last().SeatKind)
	}
	if !res.IdeBridgeQueued {
		t.Fatal("expected ide bridge wake attn queued")
	}
}

func TestApplyMCPLiveInterrupt_ignoresAgentAPIMembrane(t *testing.T) {
	res := PeerWakeResult{Attempted: true, Live: true, Transport: TransportAGYNotify}
	ApplyMCPLiveInterrupt(&res, WakeMembraneAgentAPI, datacell.DeliveryModeNotify, true, 0, true)
	if !res.Live || res.Transport != TransportAGYNotify {
		t.Fatalf("agentapi seat must not be rewritten: live=%v transport=%q", res.Live, res.Transport)
	}
}
