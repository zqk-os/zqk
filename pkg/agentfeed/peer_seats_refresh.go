package agentfeed

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/mitchellh/go-ps"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// DefaultPeerSeatIDs are seeded when peer_seats.json is missing during refresh.
// Keys match scripts/mesh/peer_seats.example.json (vendor-neutral). Never
// peer-agent-01 — that collapse breaks COMMS. Vendor product names
// (antigravity-*, composer, …) belong in an uploaded seat map, not here.
var DefaultPeerSeatIDs = []string{"peer-agent-1", "peer-agent-2"}

// peerExecutableNames are argv0 bases treated as peer worker seats (matches wake-agy.sh).
// Seat-worker heartbeats also count (see listSeatWorkerAlivePIDs); do not rely on
// agy alone for peer_seats_live.
var peerExecutableNames = []string{"agy"}

// seatWorkerAliveMaxAge is how fresh a seat_workers/*.alive.json must be to count as live.
const seatWorkerAliveMaxAge = 3 * time.Minute

// listPeerExecutablePIDsFn is overridable in tests.
var listPeerExecutablePIDsFn = listPeerExecutablePIDs

// listSeatWorkerAlivePIDsFn is overridable in tests.
var listSeatWorkerAlivePIDsFn = listSeatWorkerAlivePIDs

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

// listSeatWorkerAlivePIDs returns seat-id → pid for fresh seat-worker heartbeats
// whose pid is still alive. Prefer this over agy argv0 matching when peers run
// `zqk agent seat-worker` (executable base is zqk/zqk-stable, not agy).
func listSeatWorkerAlivePIDs(projectRoot string) (map[string]int, error) {
	root := strings.TrimSpace(projectRoot)
	out := map[string]int{}
	if root == "" {
		return out, nil
	}
	dir := filepath.Join(root, paths.ProjectDataDir, paths.StateDir, "mesh", "seat_workers")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, errfmt.Newf("read seat_workers dir").Wrap(err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".alive.json") {
			continue
		}
		raw, err := fileutil.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var payload struct {
			Schema    string `json:"schema"`
			AgentID   string `json:"agent_id"`
			PID       int    `json:"pid"`
			UpdatedAt string `json:"updated_at"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			continue
		}
		if payload.Schema != "" && payload.Schema != SeatWorkerAliveSchema {
			continue
		}
		agentID := strings.TrimSpace(payload.AgentID)
		if agentID == "" || payload.PID <= 0 {
			continue
		}
		ts, err := time.Parse(time.RFC3339, strings.TrimSpace(payload.UpdatedAt))
		if err != nil || time.Since(ts) > seatWorkerAliveMaxAge {
			continue
		}
		if !IsPIDAlive(payload.PID) {
			continue
		}
		out[agentID] = payload.PID
	}
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
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
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
// workers: agy executables and/or fresh seat-worker alive heartbeats (preferred by
// seat id). Keeps live assignments; fills empty/stale seats in stable seat-id order.
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
	aliveBySeat, err := listSeatWorkerAlivePIDsFn(root)
	if err != nil {
		return res, err
	}
	for _, pid := range aliveBySeat {
		live = append(live, pid)
	}
	live = uniqueSortedPIDs(live)
	res.LivePIDs = append([]int(nil), live...)
	if len(live) == 0 {
		res.Issues = append(res.Issues, "no_live_peer_executables: expected running "+strings.Join(peerExecutableNames, "|")+" or fresh seat-worker alive heartbeats")
	}

	used := map[int]bool{}
	seatIDs := make([]string, 0, len(f.Seats))
	for id := range f.Seats {
		seatIDs = append(seatIDs, id)
	}
	sort.Strings(seatIDs)

	// Prefer seat-worker heartbeat PID matched by agent_id / seat id.
	for _, id := range seatIDs {
		pid, ok := aliveBySeat[id]
		if !ok || pid <= 0 || !IsPIDAlive(pid) {
			continue
		}
		rec := f.Seats[id]
		if rec.PID > 0 && rec.PID != pid && IsPIDAlive(rec.PID) {
			// Keep existing live assignment; heartbeat still proves liveness via live set.
			used[rec.PID] = true
			res.Assigned[id] = rec.PID
			continue
		}
		if rec.PID > 0 && rec.PID != pid {
			res.StaleCleared = append(res.StaleCleared, id)
		}
		rec.PID = pid
		f.Seats[id] = rec
		used[pid] = true
		res.Assigned[id] = pid
	}

	// Preserve still-live PIDs for seats not covered by heartbeats.
	for _, id := range seatIDs {
		if _, ok := res.Assigned[id]; ok {
			continue
		}
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

	// Assign remaining live PIDs (typically agy) to empty seats.
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

func uniqueSortedPIDs(in []int) []int {
	seen := map[int]bool{}
	var out []int
	for _, p := range in {
		if p <= 0 || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Ints(out)
	return out
}
