package agentfeed

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// PeerSeatsRelPath is the default relative mesh seat map under project data (not process CAS).
// Prefer paths.PeerSeatsPath(projectRoot) for absolute resolution (path-cache aware).
var PeerSeatsRelPath = filepath.Join(paths.ProjectDataDir, paths.StateDir, paths.MeshStateSubdir, paths.PeerSeatsFile)

// PeerSeatRecord binds an agent_id to a local wake transport target.
// Wake selects the interrupt membrane (vendor-agnostic). Persona/vendor names
// must not be required for routing
type PeerSeatRecord struct {
	PID          int    `json:"pid,omitempty"`
	Conversation string `json:"conversation,omitempty"`
	// Wake is the interrupt membrane for this seat: agentapi | mcp | stamp.
	// Empty → inferred (legacy agent-id heuristics; do not add new hardcodes).
	Wake string `json:"wake,omitempty"`
	// PersonaRef is the seated kernel persona (plan primary, TPM, …).
	// Seat routing uses this, not a Go switch on PER-ORCH-*.
	PersonaRef string `json:"persona_ref,omitempty"`
	// Duty is coordinator | worker. Empty → inferred from wake membrane.
	Duty string `json:"duty,omitempty"`
	// Lane or WorkerLane specifies the designated swarm execution lane for this seat.
	Lane       string `json:"lane,omitempty"`
	WorkerLane string `json:"worker_lane,omitempty"`
	Note       string `json:"note,omitempty"`
}

// NormalizeSeatDuty returns SeatKindCoordinator, SeatKindWorker, or "".
func NormalizeSeatDuty(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case SeatKindCoordinator:
		return SeatKindCoordinator
	case SeatKindWorker:
		return SeatKindWorker
	default:
		return ""
	}
}

// Wake membrane names (peer_seats.wake + wake routing). Stable CLI/JSON tokens.
const (
	WakeMembraneAgentAPI = "agentapi" // terminal / agentapi notify (any worker seat)
	WakeMembraneMCP      = "mcp"      // MCP ActionRequired to IDE subscribers
	WakeMembraneStamp    = "stamp"    // JSONL stamp only (never Live alone)
)

// PeerSeatsFile is the on-disk seat registry.
type PeerSeatsFile struct {
	SchemaVersion string                    `json:"schema_version"`
	Seats         map[string]PeerSeatRecord `json:"seats"`
}

// NormalizeWakeMembrane returns a canonical wake membrane or "".
func NormalizeWakeMembrane(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case WakeMembraneAgentAPI, "agy", "agy_notify", "notify-agentapi":
		return WakeMembraneAgentAPI
	case WakeMembraneMCP, "mcp_action_required", "ide", "coordinator":
		return WakeMembraneMCP
	case WakeMembraneStamp, "tpm_stamp", "stamp-only":
		return WakeMembraneStamp
	default:
		return ""
	}
}

// ResolveWakeMembrane returns the interrupt membrane for toAgentID.
// Preference: peer_seats.wake → legacy agent-id heuristic → agentapi default for named seats.
func ResolveWakeMembrane(projectRoot, toAgentID string) string {
	id := strings.TrimSpace(toAgentID)
	if id == "" {
		return WakeMembraneStamp
	}
	if f, err := LoadPeerSeats(projectRoot); err == nil {
		if rec, ok := f.Seats[id]; ok {
			if m := NormalizeWakeMembrane(rec.Wake); m != "" {
				return m
			}
		}
	}
	// Legacy fallback only — new seats must declare wake in peer_seats.json.
	if legacyCoordinatorAgentID(id) {
		return WakeMembraneMCP
	}
	return WakeMembraneAgentAPI
}

// SeatKindForWakeMembrane maps a membrane to adapter seat-kind buckets.
func SeatKindForWakeMembrane(membrane string) string {
	switch NormalizeWakeMembrane(membrane) {
	case WakeMembraneMCP, WakeMembraneStamp:
		return SeatKindCoordinator
	default:
		return SeatKindWorker
	}
}

// LoadPeerSeats reads the local seat map. Missing file → empty map (not an error).
func LoadPeerSeats(projectRoot string) (PeerSeatsFile, error) {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		return PeerSeatsFile{}, errfmt.Errorf("empty project root")
	}
	path := paths.PeerSeatsPath(root)
	b, err := fileutil.ReadFile(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return PeerSeatsFile{SchemaVersion: "1", Seats: map[string]PeerSeatRecord{}}, nil
		}
		return PeerSeatsFile{}, errfmt.Errorf("read peer seats: %w", err)
	}
	var f PeerSeatsFile
	if err := json.Unmarshal(b, &f); err != nil {
		return PeerSeatsFile{}, errfmt.Errorf("parse peer seats: %w", err)
	}
	if f.Seats == nil {
		f.Seats = map[string]PeerSeatRecord{}
	}
	if strings.TrimSpace(f.SchemaVersion) == "" {
		f.SchemaVersion = "1"
	}
	return f, nil
}

func sortedSeatIDs(f PeerSeatsFile) []string {
	ids := make([]string, 0, len(f.Seats))
	for id := range f.Seats {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func firstSeatIDByDutyThenWake(projectRoot, duty, wakeFallback string) string {
	f, err := LoadPeerSeats(projectRoot)
	if err != nil || len(f.Seats) == 0 {
		return ""
	}
	var wakeHit string
	for _, id := range sortedSeatIDs(f) {
		rec := f.Seats[id]
		if NormalizeSeatDuty(rec.Duty) == duty {
			return id
		}
		if wakeHit == "" && wakeFallback != "" && NormalizeWakeMembrane(rec.Wake) == wakeFallback {
			wakeHit = id
		}
	}
	return wakeHit
}

// CoordinatorSeatID returns the seating-configured coordinator (duty=coordinator,
// else wake=mcp). Deterministic when several match. Empty if none declared.
func CoordinatorSeatID(projectRoot string) string {
	return firstSeatIDByDutyThenWake(projectRoot, SeatKindCoordinator, WakeMembraneMCP)
}

// WorkerSeatID returns the seating-configured worker (duty=worker, else
// wake=agentapi). Deterministic when several match. Empty if none declared.
func WorkerSeatID(projectRoot string) string {
	return firstSeatIDByDutyThenWake(projectRoot, SeatKindWorker, WakeMembraneAgentAPI)
}

func loadSeatRecord(projectRoot, agentID string) (*PeerSeatRecord, bool) {
	id := strings.TrimSpace(agentID)
	if id == "" || strings.TrimSpace(projectRoot) == "" {
		return nil, false
	}
	f, err := LoadPeerSeats(projectRoot)
	if err != nil {
		return nil, false
	}
	rec, ok := f.Seats[id]
	if !ok {
		return nil, false
	}
	return &rec, true
}

// SeatPersonaRef returns peer_seats[agentID].persona_ref. Empty if unset or unknown.
// whats-next plan selection uses this so --agent-id is seat-scoped, not the
// caller's security-context personas. TRACK
func SeatPersonaRef(projectRoot, agentID string) string {
	rec, ok := loadSeatRecord(projectRoot, agentID)
	if !ok {
		return ""
	}
	return strings.TrimSpace(rec.PersonaRef)
}

// SeatLane returns peer_seats[agentID].lane (or worker_lane). Empty if unset or unknown.
func SeatLane(projectRoot, agentID string) string {
	rec, ok := loadSeatRecord(projectRoot, agentID)
	if !ok {
		return ""
	}
	if l := strings.TrimSpace(rec.Lane); l != "" {
		return l
	}
	return strings.TrimSpace(rec.WorkerLane)
}

// ResolvePeerPID returns pid for toAgentID from seat map, else 0.
func ResolvePeerPID(projectRoot, toAgentID string) (int, error) {
	id := strings.TrimSpace(toAgentID)
	if id == "" {
		return 0, nil
	}
	f, err := LoadPeerSeats(projectRoot)
	if err != nil {
		return 0, err
	}
	rec, ok := f.Seats[id]
	if !ok || rec.PID <= 0 {
		return 0, nil
	}
	return rec.PID, nil
}

// wakeScriptArgsForSeat builds wake-agy argv including optional --pid / --conversation.
func wakeScriptArgsForSeat(deliveryMode, pasteText string, pid int, conversation string) []string {
	args := wakeScriptArgs(deliveryMode, pasteText)
	// Insert targeting flags before the message (last element).
	if len(args) == 0 {
		return args
	}
	msg := args[len(args)-1]
	flags := args[:len(args)-1]
	if pid > 0 {
		flags = append(flags, "--pid", strconv.Itoa(pid))
	}
	if c := strings.TrimSpace(conversation); c != "" {
		flags = append(flags, "--conversation", c)
	}
	return append(flags, msg)
}
