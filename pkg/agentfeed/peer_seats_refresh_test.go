package agentfeed

import (
	"os"
	"path/filepath"
	"testing"

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
