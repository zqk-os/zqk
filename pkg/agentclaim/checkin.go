package agentclaim

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// Cadence check-in timer for a held claim.
//
// A claim records who owns a task but says nothing about whether they are still there.
// Nothing else does either: an agent that drops leaves the task in_progress with the
// claim held, `agent recover` only reaps tasks that reached error, and hourglass deadline
// expiry files a risk_blocker without releasing the claim. Two ATKs sat claimed for six
// days against a one-day estimate with no signal of any kind.
//
// The cadence is deliberately not derived from estimated_effort. That field is present on
// every occupiable task and enforced at transition, but its content is prose — "1d",
// "4 hours", "PT1H", bare "2", and t-shirt "S" all appear in live data — and its accuracy
// is unverified: across 76 ATK pairs the median actual was a quarter of the estimate with
// zero underestimates, which indicts the measurement rather than the estimators. A
// deadline computed from that number inherits its error and its unit ambiguity. Whether
// an update arrived before a wall-clock instant needs nobody's arithmetic to be true, and
// the check-ins accumulate the per-step provenance that would let estimate accuracy be
// judged honestly later.
//
// TRACK: PRI-STABILIZE-FAILCLOSED-READS-001
const (
	// TimerTypeCheckin marks a cadence timer. The scheduler's hourglass watcher treats
	// unrecognized types as stuck sync-loop processes and SIGKILLs the recorded pid, so
	// this value must stay in sync with the switch in pkg/scheduler/hourglass.go.
	TimerTypeCheckin = "checkin"

	// DefaultCheckinCadence is how long a claim may stay silent before the timer fires.
	DefaultCheckinCadence = 5 * time.Minute

	checkinFileSuffix = ".checkin.json"
	hourglassDirName  = "hourglass"
)

// CheckinTimer is the on-disk cadence record for one claimed task.
type CheckinTimer struct {
	TaskID         string `json:"task_id"`
	Kind           string `json:"kind,omitempty"`
	Type           string `json:"type"`
	ExpiresAt      string `json:"expires_at"`
	ClaimedBy      string `json:"claimed_by,omitempty"`
	CadenceSeconds int    `json:"cadence_seconds"`
	Misses         int    `json:"misses"`
	ArmedAt        string `json:"armed_at,omitempty"`
	LastCheckinAt  string `json:"last_checkin_at,omitempty"`
}

// Cadence returns the configured interval, falling back to the default when the record
// carries no usable value so a malformed file cannot produce a zero-length timer that
// fires on every scheduler tick.
func (t *CheckinTimer) Cadence() time.Duration {
	if t == nil || t.CadenceSeconds <= 0 {
		return DefaultCheckinCadence
	}
	return time.Duration(t.CadenceSeconds) * time.Second
}

// CheckinTimerPath returns the timer file for taskID.
//
// The suffix keeps it distinct from the deadline file the CAP orchestrator writes as
// <taskID>.json; sharing that name would make arming a cadence timer silently cancel a
// dispatch deadline.
func CheckinTimerPath(projectRoot, taskID string) string {
	schedulerRoot := paths.ResolvePathFromCacheOrConstant(projectRoot, paths.SchedulerDir,
		filepath.Join(paths.ProjectDataDir, paths.SchedulerDir))
	return filepath.Join(schedulerRoot, hourglassDirName, taskID+checkinFileSuffix)
}

// ArmCheckin opens a cadence window for a freshly claimed task.
func ArmCheckin(projectRoot, taskID, holder, kind string, cadence time.Duration) error {
	if projectRoot == "" || taskID == "" {
		return errfmt.Errorf("project root and task id are required to arm a check-in timer")
	}
	if cadence <= 0 {
		cadence = DefaultCheckinCadence
	}
	now := time.Now().UTC()
	return writeCheckin(projectRoot, &CheckinTimer{
		TaskID:         taskID,
		Kind:           kind,
		Type:           TimerTypeCheckin,
		ExpiresAt:      now.Add(cadence).Format(time.RFC3339),
		ClaimedBy:      holder,
		CadenceSeconds: int(cadence.Seconds()),
		ArmedAt:        now.Format(time.RFC3339),
	})
}

// RenewCheckin records a progress signal and reopens the window.
//
// Absent timer is not an error: renewal is called from progress paths that also run for
// tasks nobody armed (a claim taken before this existed, or work driven without a claim),
// and failing there would turn a missing timer into a failed lifecycle transition.
func RenewCheckin(projectRoot, taskID string) error {
	timer, err := LoadCheckin(projectRoot, taskID)
	if err != nil || timer == nil {
		return err
	}
	now := time.Now().UTC()
	timer.ExpiresAt = now.Add(timer.Cadence()).Format(time.RFC3339)
	timer.LastCheckinAt = now.Format(time.RFC3339)
	timer.Misses = 0
	return writeCheckin(projectRoot, timer)
}

// Rearm reopens the window after a miss, backing off so one silent task does not wake the
// orchestrator on every scheduler tick while it is being triaged.
func Rearm(projectRoot string, timer *CheckinTimer, maxBackoff time.Duration) error {
	if timer == nil {
		return errfmt.Errorf("nil check-in timer")
	}
	backoff := timer.Cadence() * time.Duration(timer.Misses+1)
	if maxBackoff > 0 && backoff > maxBackoff {
		backoff = maxBackoff
	}

	timer.ExpiresAt = time.Now().UTC().Add(backoff).Format(time.RFC3339)
	return writeCheckinAsync(projectRoot, timer)
}

// ClearCheckin removes the timer; used on release and on reaching a terminal status.
func ClearCheckin(projectRoot, taskID string) error {
	if projectRoot == "" || taskID == "" {
		return nil
	}
	err := fileutil.Remove(CheckinTimerPath(projectRoot, taskID))
	if err != nil && !fileutil.IsNotExist(err) {
		return err
	}
	return nil
}

// LoadCheckin reads the timer for taskID, returning (nil, nil) when none is armed.
func LoadCheckin(projectRoot, taskID string) (*CheckinTimer, error) {
	if projectRoot == "" || taskID == "" {
		return nil, nil
	}
	return LoadCheckinFile(CheckinTimerPath(projectRoot, taskID))
}

// LoadCheckinFile reads a timer from an explicit path, for callers walking the directory.
func LoadCheckinFile(path string) (*CheckinTimer, error) {
	data, err := fileutil.ReadFile(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var timer CheckinTimer
	if err := json.Unmarshal(data, &timer); err != nil {
		return nil, errfmt.Newf("parse check-in timer %s", path).Wrap(err)
	}
	return &timer, nil
}

// Expired reports whether the window has closed. An unparseable expires_at counts as
// expired: a timer nobody can read is not evidence that the seat is alive.
func (t *CheckinTimer) Expired(now time.Time) bool {
	if t == nil {
		return false
	}
	expires, err := time.Parse(time.RFC3339, t.ExpiresAt)
	if err != nil {
		return true
	}
	return now.After(expires)
}

func writeCheckin(projectRoot string, timer *CheckinTimer) error {
	path := CheckinTimerPath(projectRoot, timer.TaskID)
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		return err
	}
	data, err := json.Marshal(timer)
	if err != nil {
		return err
	}

	ioq := storage.GetGlobalIOQueueManager(context.Background())
	resultCh := make(chan storage.IOResult, 1)
	err = ioq.Enqueue(&storage.IOOperation{
		Type:     storage.IOOperationWrite,
		FilePath: path,
		Data:     data,
		Perm:     paths.FilePerm644,
		Result:   resultCh,
	})
	if err != nil {
		return err
	}
	res := <-resultCh
	return res.Err
}

// writeCheckinAsync enqueues the write without blocking, avoiding thundering herds
func writeCheckinAsync(projectRoot string, timer *CheckinTimer) error {
	path := CheckinTimerPath(projectRoot, timer.TaskID)
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		return err
	}
	data, err := json.Marshal(timer)
	if err != nil {
		return err
	}

	ioq := storage.GetGlobalIOQueueManager(context.Background())
	return ioq.Enqueue(&storage.IOOperation{
		Type:     storage.IOOperationWrite,
		FilePath: path,
		Data:     data,
		Perm:     paths.FilePerm644,
		Result:   nil, // Fire and forget, workers handle batching
	})
}
