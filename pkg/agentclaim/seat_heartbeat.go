package agentclaim

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Seat heartbeat hourglass — CAP/Composer seat liveness, distinct from ATK check-in.
//
// Claim check-in (TimerTypeCheckin) covers occupied ATKs. Cursor CAP seats still drop to
// waiting_for_input after a summary turn with no ATK timer armed, so nothing wakes them.
// Seat heartbeats are keyed by mesh agent_id under .zqk/scheduler/hourglass/seat-*.seat.json.
// Progress stamps (feed append, seat-worker ticks, CAP wake) renew the window; silence
// within the cadence wakes that seat via mesh notify (not SIGKILL, not empty CAP pings).
// Applicability: POL-AGENT-ORCH-HOURGLASS-001 (seat heartbeat section).
const (
	// TimerTypeSeatHeartbeat marks a seat liveness timer. Must stay registered in
	// pkg/scheduler/hourglass.go actionForExpiredTimer (wake path, never kill).
	TimerTypeSeatHeartbeat = "seat_heartbeat"

	// DefaultSeatHeartbeatCadence is how long a seated CAP agent may stay silent.
	DefaultSeatHeartbeatCadence = 5 * time.Minute

	// KindAgentSeat is the timer Kind for seat-keyed files (not an object kind).
	KindAgentSeat = "agent_seat"

	seatHeartbeatFileSuffix = ".seat.json"
	seatHeartbeatFilePrefix = "seat-"
)

// SeatHeartbeatPath returns the hourglass file for a mesh seat agent_id.
func SeatHeartbeatPath(projectRoot, seatID string) string {
	schedulerRoot := paths.ResolvePathFromCacheOrConstant(projectRoot, paths.SchedulerDir,
		filepath.Join(paths.ProjectDataDir, paths.SchedulerDir))
	return filepath.Join(schedulerRoot, hourglassDirName, seatHeartbeatFilePrefix+sanitizeClaimant(seatID)+seatHeartbeatFileSuffix)
}

// ArmSeatHeartbeat opens (or resets) a seat liveness window. CAP orchestrator and
// seat-worker call this when a seat is expected to stay productive.
func ArmSeatHeartbeat(projectRoot, seatID string, cadence time.Duration) error {
	seatID = strings.TrimSpace(seatID)
	if projectRoot == "" || seatID == "" {
		return errfmt.Errorf("project root and seat id are required to arm a seat heartbeat")
	}
	if cadence <= 0 {
		cadence = DefaultSeatHeartbeatCadence
	}
	now := time.Now().UTC()
	timer := &CheckinTimer{
		TaskID:         seatID,
		Kind:           KindAgentSeat,
		Type:           TimerTypeSeatHeartbeat,
		ExpiresAt:      now.Add(cadence).Format(time.RFC3339),
		ClaimedBy:      seatID,
		CadenceSeconds: int(cadence.Seconds()),
		ArmedAt:        now.Format(time.RFC3339),
	}
	return writeSeatHeartbeat(projectRoot, timer)
}

// RenewSeatHeartbeat reopens the window after a progress stamp.
//
// Absent timer is not an error: feed/CLI renew paths also run for seats nobody armed
// (pre-CAP session, human-only Composer). Failing there would turn a missing arm into a
// failed emit-status.
func RenewSeatHeartbeat(projectRoot, seatID string) error {
	seatID = strings.TrimSpace(seatID)
	if projectRoot == "" || seatID == "" || seatID == "unknown" {
		return nil
	}
	timer, err := LoadSeatHeartbeat(projectRoot, seatID)
	if err != nil {
		return err
	}
	if timer == nil {
		return nil
	}
	now := time.Now().UTC()
	timer.ExpiresAt = now.Add(timer.Cadence()).Format(time.RFC3339)
	timer.LastCheckinAt = now.Format(time.RFC3339)
	timer.Misses = 0
	return writeSeatHeartbeat(projectRoot, timer)
}

// ClearSeatHeartbeat removes the seat timer (seat-worker exit / session end).
func ClearSeatHeartbeat(projectRoot, seatID string) error {
	seatID = strings.TrimSpace(seatID)
	if projectRoot == "" || seatID == "" {
		return nil
	}
	path := SeatHeartbeatPath(projectRoot, seatID)
	err := fileutil.Remove(path)
	if err != nil && !fileutil.IsNotExist(err) {
		return err
	}
	return nil
}

// LoadSeatHeartbeat reads the timer for seatID, returning (nil, nil) when none is armed.
func LoadSeatHeartbeat(projectRoot, seatID string) (*CheckinTimer, error) {
	seatID = strings.TrimSpace(seatID)
	if projectRoot == "" || seatID == "" {
		return nil, nil
	}
	return LoadCheckinFile(SeatHeartbeatPath(projectRoot, seatID))
}

// RearmSeatHeartbeat reopens the window after a miss with backoff (same shape as claim Rearm).
func RearmSeatHeartbeat(projectRoot string, timer *CheckinTimer, maxBackoff time.Duration) error {
	if timer == nil {
		return errfmt.Errorf("nil seat heartbeat timer")
	}
	backoff := timer.Cadence() * time.Duration(timer.Misses+1)
	if maxBackoff > 0 && backoff > maxBackoff {
		backoff = maxBackoff
	}
	timer.ExpiresAt = time.Now().UTC().Add(backoff).Format(time.RFC3339)
	return writeSeatHeartbeat(projectRoot, timer)
}

// IsSeatHeartbeatPath reports whether a hourglass filename is a seat heartbeat file.
func IsSeatHeartbeatPath(name string) bool {
	base := filepath.Base(name)
	return strings.HasPrefix(base, seatHeartbeatFilePrefix) && strings.HasSuffix(base, seatHeartbeatFileSuffix)
}

func writeSeatHeartbeat(projectRoot string, timer *CheckinTimer) error {
	if timer == nil {
		return errfmt.Errorf("nil seat heartbeat timer")
	}
	path := SeatHeartbeatPath(projectRoot, timer.TaskID)
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		return err
	}
	data, err := json.Marshal(timer)
	if err != nil {
		return err
	}
	return fileutil.WriteFile(path, data, paths.FilePerm644)
}
