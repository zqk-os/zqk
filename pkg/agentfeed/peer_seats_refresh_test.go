package agentfeed

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestRefreshPeerSeatsFromLivePIDs_AssignsAndPersists(t *testing.T) {
	root := t.TempDir()
	self := os.Getpid()
	prev := listPeerExecutablePIDsFn
	listPeerExecutablePIDsFn = func() ([]int, error) { return []int{self}, nil }
	t.Cleanup(func() { listPeerExecutablePIDsFn = prev })

	res, err := RefreshPeerSeatsFromLivePIDs(root, false)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if res.Assigned["peer-agent-1"] != self {
		t.Fatalf("assigned=%v want self=%d on peer-agent-1", res.Assigned, self)
	}
	path := filepath.Join(root, paths.ProjectDataDir, paths.StateDir, "mesh", "peer_seats.json")
	if _, err := fileutil.Stat(path); err != nil {
		t.Fatalf("expected seats file: %v", err)
	}
	loaded, err := LoadPeerSeats(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Seats["peer-agent-1"].PID != self {
		t.Fatalf("loaded pid=%d", loaded.Seats["peer-agent-1"].PID)
	}

	doc := InspectFeed(DoctorOptions{ProjectRoot: root})
	if doc.PeerSeatsLive < 1 {
		t.Fatalf("PeerSeatsLive=%d", doc.PeerSeatsLive)
	}
	if !doc.PeerWakeLive {
		t.Fatal("expected PeerWakeLive from live seats")
	}
}

func TestRefreshPeerSeatsFromLivePIDs_DryRunNoWrite(t *testing.T) {
	root := t.TempDir()
	self := os.Getpid()
	prev := listPeerExecutablePIDsFn
	listPeerExecutablePIDsFn = func() ([]int, error) { return []int{self}, nil }
	t.Cleanup(func() { listPeerExecutablePIDsFn = prev })

	_, err := RefreshPeerSeatsFromLivePIDs(root, true)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, paths.ProjectDataDir, paths.StateDir, "mesh", "peer_seats.json")
	if _, err := fileutil.Stat(path); !fileutil.IsNotExist(err) {
		t.Fatalf("dry-run should not write seats file, err=%v", err)
	}
}

func TestIsPIDAlive_Self(t *testing.T) {
	if !IsPIDAlive(os.Getpid()) {
		t.Fatal("expected self alive")
	}
	if IsPIDAlive(0) || IsPIDAlive(-1) {
		t.Fatal("expected invalid pids dead")
	}
}

func TestRefreshPeerSeatsFromLivePIDs_SeatWorkerAliveHeartbeats(t *testing.T) {
	root := t.TempDir()
	self := os.Getpid()
	prevExec := listPeerExecutablePIDsFn
	listPeerExecutablePIDsFn = func() ([]int, error) { return nil, nil }
	t.Cleanup(func() { listPeerExecutablePIDsFn = prevExec })

	if _, err := WriteSeatWorkerAlive(root, "peer-agent-1", "sess-test", time.Now().UTC()); err != nil {
		t.Fatalf("alive: %v", err)
	}
	// Rewrite alive with known pid=self (WriteSeatWorkerAlive uses os.Getpid already).
	_ = self

	res, err := RefreshPeerSeatsFromLivePIDs(root, false)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if res.Assigned["peer-agent-1"] != self {
		t.Fatalf("assigned=%v want self=%d on peer-agent-1 from heartbeat", res.Assigned, self)
	}
	if len(res.Issues) != 0 {
		t.Fatalf("unexpected issues with live heartbeat: %v", res.Issues)
	}
	doc := InspectFeed(DoctorOptions{ProjectRoot: root})
	if doc.PeerSeatsLive < 1 {
		t.Fatalf("PeerSeatsLive=%d", doc.PeerSeatsLive)
	}
}

func TestRefreshPeerSeatsFromLivePIDs_HeartbeatReplacesStickyLivePID(t *testing.T) {
	// TRACK: tpm-seat-pid-sticky — heartbeat must win over a still-live superseded agy pid.
	root := t.TempDir()
	self := os.Getpid()
	sticky := 1 // init is almost always alive on unix; treat as superseded agy stickiness
	if !IsPIDAlive(sticky) {
		t.Skip("pid 1 not alive; cannot assert sticky-live replacement")
	}
	prevExec := listPeerExecutablePIDsFn
	listPeerExecutablePIDsFn = func() ([]int, error) { return []int{sticky}, nil }
	t.Cleanup(func() { listPeerExecutablePIDsFn = prevExec })

	if err := SavePeerSeats(root, PeerSeatsFile{Seats: map[string]PeerSeatRecord{
		"peer-agent-1": {PID: sticky, Note: "sticky live agy"},
	}}); err != nil {
		t.Fatalf("seed seats: %v", err)
	}
	if _, err := WriteSeatWorkerAlive(root, "peer-agent-1", "sess-sticky", time.Now().UTC()); err != nil {
		t.Fatalf("alive: %v", err)
	}

	res, err := RefreshPeerSeatsFromLivePIDs(root, false)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if res.Assigned["peer-agent-1"] != self {
		t.Fatalf("assigned=%v want heartbeat self=%d (not sticky %d)", res.Assigned, self, sticky)
	}
	cleared := false
	for _, id := range res.StaleCleared {
		if id == "peer-agent-1" {
			cleared = true
			break
		}
	}
	if !cleared {
		t.Fatalf("expected peer-agent-1 in StaleCleared, got %v", res.StaleCleared)
	}
}

// TestRefreshPeerSeats_AgyCSRFEmptyDoesNotZeroLive proves peer_seats_live stays
// positive when agy executable discovery is empty (CSRF/infra_red wake path) as
// long as seat-worker heartbeats are fresh.
func TestRefreshPeerSeats_AgyCSRFEmptyDoesNotZeroLive(t *testing.T) {
	root := t.TempDir()
	self := os.Getpid()
	prevExec := listPeerExecutablePIDsFn
	// Simulate agy discovery empty / CSRF wake unavailable.
	listPeerExecutablePIDsFn = func() ([]int, error) { return nil, nil }
	t.Cleanup(func() { listPeerExecutablePIDsFn = prevExec })

	if _, err := WriteSeatWorkerAlive(root, "peer-agent-1", "sess-csrf", time.Now().UTC()); err != nil {
		t.Fatalf("alive: %v", err)
	}
	if _, err := WriteSeatWorkerAlive(root, "peer-agent-2", "sess-csrf-2", time.Now().UTC()); err != nil {
		t.Fatalf("alive2: %v", err)
	}

	res, err := RefreshPeerSeatsFromLivePIDs(root, false)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if len(res.Assigned) < 2 {
		t.Fatalf("assigned=%v want both seats from heartbeats despite empty agy list", res.Assigned)
	}
	doc := InspectFeed(DoctorOptions{ProjectRoot: root})
	if doc.PeerSeatsLive < 2 {
		t.Fatalf("PeerSeatsLive=%d want >=2 with seat-worker heartbeats (agy CSRF empty); issues=%v", doc.PeerSeatsLive, doc.Issues)
	}
	if res.Assigned["peer-agent-1"] != self && res.Assigned["peer-agent-2"] != self {
		t.Fatalf("expected self pid among assignments, got %v self=%d", res.Assigned, self)
	}
}
