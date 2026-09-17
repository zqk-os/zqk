package agentclaim

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
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
	EvictedAt      string `json:"evicted_at,omitempty"`
	EvictionReason string `json:"eviction_reason,omitempty"`
	DegradedMode   bool   `json:"degraded_mode,omitempty"`
}

var walCache sync.Map

func fallbackWALPath(taskID string) string {
	return filepath.Join(os.TempDir(), "zqk-agentclaim-wal", taskID+".json")
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
	timer := &CheckinTimer{
		TaskID:         taskID,
		Kind:           kind,
		Type:           TimerTypeCheckin,
		ExpiresAt:      now.Add(cadence).Format(time.RFC3339),
		ClaimedBy:      holder,
		CadenceSeconds: int(cadence.Seconds()),
		ArmedAt:        now.Format(time.RFC3339),
	}
	walCache.Store(taskID, timer)
	if data, err := json.Marshal(timer); err == nil {
		walPath := fallbackWALPath(taskID)
		_ = fileutil.MkdirAll(filepath.Dir(walPath), 0755)
		_ = fileutil.WriteFile(walPath, data, 0644)
	}
	return writeCheckin(projectRoot, timer)
}

// RenewCheckin records a progress signal and reopens the window.
//
// Absent timer is not an error: renewal is called from progress paths that also run for
// tasks nobody armed (a claim taken before this existed, or work driven without a claim),
// and failing there would turn a missing timer into a failed lifecycle transition.
func RenewCheckin(projectRoot, taskID string) error {
	timer, err := LoadCheckin(projectRoot, taskID)
	if err != nil {
		// Explicit graceful degradation path (BLI-1788548256672449000-41a5bc43)
		// If kernel storage is unresponsive, back off, read from local WAL/cache, and flag degraded mode.
		var cachedTimer *CheckinTimer
		if cachedRaw, ok := walCache.Load(taskID); ok {
			cachedTimer = cachedRaw.(*CheckinTimer)
		} else {
			if walData, walErr := fileutil.ReadFile(fallbackWALPath(taskID)); walErr == nil {
				var wTimer CheckinTimer
				if json.Unmarshal(walData, &wTimer) == nil {
					cachedTimer = &wTimer
				}
			}
		}

		if cachedTimer != nil {
			timer = cachedTimer
			timer.DegradedMode = true
		} else {
			return err
		}
	} else if timer == nil {
		return nil
	} else {
		timer.DegradedMode = false
	}

	now := time.Now().UTC()
	if timer.DegradedMode {
		backoff := timer.Cadence() * 2
		if backoff > time.Hour {
			backoff = time.Hour
		}
		timer.ExpiresAt = now.Add(backoff).Format(time.RFC3339)
	} else {
		timer.ExpiresAt = now.Add(timer.Cadence()).Format(time.RFC3339)
	}

	timer.LastCheckinAt = now.Format(time.RFC3339)
	timer.Misses = 0

	// Save to WAL cache (memory and disk)
	walCache.Store(taskID, timer)
	if data, mErr := json.Marshal(timer); mErr == nil {
		walPath := fallbackWALPath(taskID)
		_ = fileutil.MkdirAll(filepath.Dir(walPath), 0755)
		_ = fileutil.WriteFile(walPath, data, 0644)
	}

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

	// The thundering herd on disk writes is mitigated by the CheckinWriteQueue reducer pattern.
	timer.ExpiresAt = time.Now().UTC().Add(backoff).Format(time.RFC3339)
	return writeCheckin(projectRoot, timer)
}

// ForceEvict explicitly terminates a check-in timer before its natural timeout,
// recording the reason (e.g. process death, admin override).
func ForceEvict(projectRoot, taskID, reason string) error {
	timer, err := LoadCheckin(projectRoot, taskID)
	if err != nil {
		return err
	}
	if timer == nil {
		return nil // nothing to evict
	}

	now := time.Now().UTC()
	timer.ExpiresAt = now.Format(time.RFC3339) // Force immediate expiration
	timer.EvictedAt = now.Format(time.RFC3339)
	timer.EvictionReason = reason
	return writeCheckin(projectRoot, timer)
}

// ClearCheckin removes the timer; used on release and on reaching a terminal status.
func ClearCheckin(projectRoot, taskID string) error {
	if projectRoot == "" || taskID == "" {
		return nil
	}
	path := CheckinTimerPath(projectRoot, taskID)
	if q := globalQueue; q != nil {
		q.mu.Lock()
		delete(q.items, path)
		q.mu.Unlock()
	}

	walCache.Delete(taskID)
	_ = fileutil.Remove(fallbackWALPath(taskID))

	err := fileutil.Remove(path)
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
	path := CheckinTimerPath(projectRoot, taskID)
	// Check queue first for latest pending write
	if q := globalQueue; q != nil {
		q.mu.Lock()
		req, ok := q.items[path]
		q.mu.Unlock()
		if ok {
			// Return a copy so callers don't mutate the queued item
			cp := *req.timer
			return &cp, nil
		}
	}
	return LoadCheckinFile(path)
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
	if t.EvictedAt != "" {
		return true
	}
	expires, err := time.Parse(time.RFC3339, t.ExpiresAt)
	if err != nil {
		return true
	}
	return now.After(expires)
}

func writeCheckin(projectRoot string, timer *CheckinTimer) error {
	GetGlobalCheckinWriteQueue().Enqueue(projectRoot, timer)
	return nil
}
