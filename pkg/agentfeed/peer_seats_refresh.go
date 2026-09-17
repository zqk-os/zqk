package agentfeed

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/mitchellh/go-ps"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// DefaultPeerSeatIDs are seeded when peer_seats.json is missing during refresh.
// Keys match scripts/mesh/peer_seats.example.json (vendor-neutral). Never
// peer-agent-01 — that collapse breaks COMMS. Vendor product names
// (antigravity-*, composer, …) belong in an uploaded seat map, not here.
// TRACK: TDE-MESH-DEFAULT-SEAT-PEER-AGENT-01-001
var DefaultPeerSeatIDs = []string{"peer-agent-1", "peer-agent-2"}

// peerExecutableNames are argv0 bases treated as peer worker seats (matches wake-agy.sh).
var peerExecutableNames = []string{"agy"}

// listPeerExecutablePIDsFn is overridable in tests.
var listPeerExecutablePIDsFn = listPeerExecutablePIDs

// IsPIDAlive reports whether pid accepts signal 0 (Unix process liveness).
func IsPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil
}

func listPeerExecutablePIDs() ([]int, error) {
	if zqkenv.IsInTest() {
		return nil, nil
	}
	procs, err := ps.Processes()
	if err != nil {
		return nil, errfmt.Newf("list processes").Wrap(err)
	}
	want := make(map[string]struct{}, len(peerExecutableNames))
	for _, n := range peerExecutableNames {
		want[n] = struct{}{}
	}
	var out []int
	self := os.Getpid()
	for _, p := range procs {
		pid := p.Pid()
		if pid == self || pid <= 0 {
			continue
		}
		base := filepath.Base(p.Executable())
		if _, ok := want[base]; !ok {
			continue
		}
		if !IsPIDAlive(pid) {
			continue
		}
		out = append(out, pid)
	}
	sort.Ints(out)
	return out, nil
}

// SavePeerSeats writes the seat map atomically under the mesh state dir.
func SavePeerSeats(projectRoot string, f PeerSeatsFile) error {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		return errfmt.Errorf("empty project root")
	}
	if f.Seats == nil {
		f.Seats = map[string]PeerSeatRecord{}
	}
	if strings.TrimSpace(f.SchemaVersion) == "" {
		f.SchemaVersion = "1"
	}
	path := paths.PeerSeatsPath(root)
	if err := fileutil.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errfmt.Newf("mkdir peer seats dir").Wrap(err)
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return errfmt.Newf("marshal peer seats").Wrap(err)
	}
	b = append(b, '\n')
	if err := fileutil.WriteSecureFile(path, b); err != nil {
		return errfmt.Newf("write peer seats").Wrap(err)
	}
	return nil
}

// RefreshPeerSeatsResult is the outcome of refreshing peer_seats.json from live PIDs.
type RefreshPeerSeatsResult struct {
	Path         string         `json:"path"`
	DryRun       bool           `json:"dry_run"`
	LivePIDs     []int          `json:"live_pids"`
	Assigned     map[string]int `json:"assigned"`
	Unassigned   []int          `json:"unassigned,omitempty"`
	StaleCleared []string       `json:"stale_cleared,omitempty"`
	Issues       []string       `json:"issues,omitempty"`
}

// RefreshPeerSeatsFromLivePIDs updates peer_seats.json so seat PIDs match live peer
// executables (agy). Keeps live assignments; fills empty/stale seats in stable seat-id order.
func RefreshPeerSeatsFromLivePIDs(projectRoot string, dryRun bool) (RefreshPeerSeatsResult, error) {
	root := strings.TrimSpace(projectRoot)
	res := RefreshPeerSeatsResult{
		DryRun:   dryRun,
		Assigned: map[string]int{},
	}
	if root == "" {
		return res, errfmt.Errorf("empty project root")
	}
	res.Path = paths.PeerSeatsPath(root)

	f, err := LoadPeerSeats(root)
	if err != nil {
		return res, err
	}
	if len(f.Seats) == 0 {
		f.Seats = map[string]PeerSeatRecord{}
		for _, id := range DefaultPeerSeatIDs {
			f.Seats[id] = PeerSeatRecord{Note: "auto-seeded; refresh after peer restart"}
		}
	}

	live, err := listPeerExecutablePIDsFn()
	if err != nil {
		return res, err
	}
	res.LivePIDs = append([]int(nil), live...)
	if len(live) == 0 {
		res.Issues = append(res.Issues, "no_live_peer_executables: expected running "+strings.Join(peerExecutableNames, "|"))
	}

	used := map[int]bool{}
	seatIDs := make([]string, 0, len(f.Seats))
	for id := range f.Seats {
		seatIDs = append(seatIDs, id)
	}
	sort.Strings(seatIDs)

	// Preserve still-live PIDs.
	for _, id := range seatIDs {
		rec := f.Seats[id]
		if rec.PID > 0 && IsPIDAlive(rec.PID) {
			used[rec.PID] = true
			res.Assigned[id] = rec.PID
			continue
		}
		if rec.PID > 0 {
			res.StaleCleared = append(res.StaleCleared, id)
			rec.PID = 0
			f.Seats[id] = rec
		}
	}

	// Assign remaining live PIDs to empty seats.
	var pool []int
	for _, pid := range live {
		if !used[pid] {
			pool = append(pool, pid)
		}
	}
	for _, id := range seatIDs {
		rec := f.Seats[id]
		if rec.PID > 0 {
			continue
		}
		if len(pool) == 0 {
			break
		}
		pid := pool[0]
		pool = pool[1:]
		rec.PID = pid
		f.Seats[id] = rec
		used[pid] = true
		res.Assigned[id] = pid
	}
	res.Unassigned = pool

	if len(res.Assigned) == 0 && len(live) > 0 {
		res.Issues = append(res.Issues, "live_pids_present_but_no_seats_to_assign")
	}

	if dryRun {
		return res, nil
	}
	if err := SavePeerSeats(root, f); err != nil {
		return res, err
	}
	return res, nil
}
