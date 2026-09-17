package agentclaim

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func tempDir(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	t.Cleanup(func() {
		if q := GetGlobalCheckinWriteQueue(); q != nil {
			q.FlushWait()
		}
	})
	return d
}

func TestArmCheckin_writesTimerDistinctFromDeadlineFile(t *testing.T) {
	t.Parallel()
	root := tempDir(t)
	if err := ArmCheckin(root, "ATK-1", "antigravity-1", "agent_task", time.Minute); err != nil {
		t.Fatalf("ArmCheckin: %v", err)
	}
	path := CheckinTimerPath(root, "ATK-1")
	if !strings.HasSuffix(path, "ATK-1.checkin.json") {
		t.Fatalf("timer path %q must not collide with the CAP deadline file ATK-1.json", path)
	}
	if _, err := fileutil.Stat(filepath.Join(filepath.Dir(path), "ATK-1.json")); !fileutil.IsNotExist(err) {
		t.Fatal("arming a cadence timer must not create or clobber the deadline file")
	}
	timer, err := LoadCheckin(root, "ATK-1")
	if err != nil || timer == nil {
		t.Fatalf("LoadCheckin: %v (timer=%v)", err, timer)
	}
	if timer.Type != TimerTypeCheckin {
		t.Fatalf("type = %q, want %q; any other value routes to the SIGKILL branch", timer.Type, TimerTypeCheckin)
	}
	if timer.ClaimedBy != "antigravity-1" || timer.CadenceSeconds != 60 {
		t.Fatalf("holder/cadence not recorded: %+v", timer)
	}
}

func TestArmCheckin_zeroCadenceFallsBackToDefault(t *testing.T) {
	t.Parallel()
	root := tempDir(t)
	if err := ArmCheckin(root, "ATK-2", "who", "agent_task", 0); err != nil {
		t.Fatalf("ArmCheckin: %v", err)
	}
	timer, _ := LoadCheckin(root, "ATK-2")
	if timer.Cadence() != DefaultCheckinCadence {
		t.Fatalf("cadence = %v, want default %v; a zero window fires on every tick", timer.Cadence(), DefaultCheckinCadence)
	}
}

func TestRenewCheckin_extendsWindowAndClearsMisses(t *testing.T) {
	t.Parallel()
	root := tempDir(t)
	if err := ArmCheckin(root, "ATK-3", "who", "agent_task", time.Hour); err != nil {
		t.Fatalf("ArmCheckin: %v", err)
	}
	timer, _ := LoadCheckin(root, "ATK-3")
	timer.Misses = 2
	timer.ExpiresAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if err := writeCheckin(root, timer); err != nil {
		t.Fatalf("writeCheckin: %v", err)
	}
	if !mustLoad(t, root, "ATK-3").Expired(time.Now()) {
		t.Fatal("precondition: timer should be expired before renewal")
	}

	if err := RenewCheckin(root, "ATK-3"); err != nil {
		t.Fatalf("RenewCheckin: %v", err)
	}
	renewed := mustLoad(t, root, "ATK-3")
	if renewed.Expired(time.Now()) {
		t.Fatal("renewal must reopen the window")
	}
	if renewed.Misses != 0 {
		t.Fatalf("misses = %d, want 0: a check-in clears the silence record", renewed.Misses)
	}
	if renewed.LastCheckinAt == "" {
		t.Fatal("renewal must record last_checkin_at as the provenance of the update")
	}
}

func TestRenewCheckin_absentTimerIsNotAnError(t *testing.T) {
	t.Parallel()
	if err := RenewCheckin(tempDir(t), "ATK-missing"); err != nil {
		t.Fatalf("renewing an unarmed task must not fail a lifecycle transition: %v", err)
	}
}

func TestRearm_backsOffAndCaps(t *testing.T) {
	t.Parallel()
	root := tempDir(t)
	timer := &CheckinTimer{TaskID: "ATK-4", Type: TimerTypeCheckin, CadenceSeconds: 3600, Misses: 1}
	if err := Rearm(root, timer, 0); err != nil {
		t.Fatalf("Rearm: %v", err)
	}
	expires, err := time.Parse(time.RFC3339, mustLoad(t, root, "ATK-4").ExpiresAt)
	if err != nil {
		t.Fatalf("parse expires_at: %v", err)
	}
	if delta := time.Until(expires); delta < 90*time.Minute {
		t.Fatalf("second window = %v, want ~2x cadence so triage is not interrupted every tick", delta)
	}

	timer.Misses = 100
	if err := Rearm(root, timer, 2*time.Hour); err != nil {
		t.Fatalf("Rearm capped: %v", err)
	}
	expires, _ = time.Parse(time.RFC3339, mustLoad(t, root, "ATK-4").ExpiresAt)
	if delta := time.Until(expires); delta > 2*time.Hour+15*time.Minute {
		t.Fatalf("backoff = %v, want capped at 2h so a silent claim is still re-examined", delta)
	}
}

func TestClearCheckin_removesTimerAndIsIdempotent(t *testing.T) {
	t.Parallel()
	root := tempDir(t)
	if err := ArmCheckin(root, "ATK-5", "who", "agent_task", time.Minute); err != nil {
		t.Fatalf("ArmCheckin: %v", err)
	}
	if err := ClearCheckin(root, "ATK-5"); err != nil {
		t.Fatalf("ClearCheckin: %v", err)
	}
	if timer, _ := LoadCheckin(root, "ATK-5"); timer != nil {
		t.Fatal("cleared timer must not survive; it would wake the orchestrator about an unheld task")
	}
	if err := ClearCheckin(root, "ATK-5"); err != nil {
		t.Fatalf("second clear must be a no-op, got %v", err)
	}
}

func TestExpired_unparseableDeadlineCountsAsExpired(t *testing.T) {
	t.Parallel()
	timer := &CheckinTimer{TaskID: "ATK-6", Type: TimerTypeCheckin, ExpiresAt: "not-a-timestamp"}
	if !timer.Expired(time.Now()) {
		t.Fatal("a timer nobody can parse is not evidence the seat is alive; it must fail closed")
	}
}

func TestLoadCheckinFile_corruptJSONReportsError(t *testing.T) {
	t.Parallel()
	root := tempDir(t)
	path := CheckinTimerPath(root, "ATK-7")
	if err := fileutil.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := fileutil.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadCheckinFile(path); err == nil {
		t.Fatal("corrupt timer must surface an error, not silently read as an armed timer")
	}
}

// TestCheckinTimer_wireShapeMatchesWatcher guards the contract with the scheduler's
// hourglass watcher, which unmarshals these files into its own struct. A renamed json tag
// would leave the timer with an empty type, and the watcher SIGKILLs the pid recorded in
// any timer whose type it does not recognize.
func TestCheckinTimer_wireShapeMatchesWatcher(t *testing.T) {
	t.Parallel()
	data, err := json.Marshal(&CheckinTimer{TaskID: "ATK-8", Type: TimerTypeCheckin, ExpiresAt: "2026-01-01T00:00:00Z"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var watcher struct {
		TaskID    string `json:"task_id"`
		PID       int    `json:"pid"`
		ExpiresAt string `json:"expires_at"`
		Type      string `json:"type,omitempty"`
		Kind      string `json:"kind,omitempty"`
	}
	if err := json.Unmarshal(data, &watcher); err != nil {
		t.Fatalf("watcher cannot read the timer: %v", err)
	}
	if watcher.Type != TimerTypeCheckin || watcher.TaskID != "ATK-8" || watcher.ExpiresAt == "" {
		t.Fatalf("watcher read %+v; task_id/type/expires_at must survive the wire", watcher)
	}
	if watcher.PID != 0 {
		t.Fatal("a check-in timer must never carry a pid: the watcher kills pids it finds on unrecognized timers")
	}
}

func mustLoad(t *testing.T, root, taskID string) *CheckinTimer {
	t.Helper()
	timer, err := LoadCheckin(root, taskID)
	if err != nil || timer == nil {
		t.Fatalf("LoadCheckin(%s): %v (timer=%v)", taskID, err, timer)
	}
	return timer
}

func TestForceEvict_expiresImmediately(t *testing.T) {
	t.Parallel()
	root := tempDir(t)
	taskID := "ATK-FORCE-EVICT"

	if err := ArmCheckin(root, taskID, "antigravity", "agent_task", time.Hour); err != nil {
		t.Fatalf("ArmCheckin: %v", err)
	}

	if err := ForceEvict(root, taskID, "process_death_signal"); err != nil {
		t.Fatalf("ForceEvict: %v", err)
	}

	timer, err := LoadCheckin(root, taskID)
	if err != nil || timer == nil {
		t.Fatalf("LoadCheckin: %v", err)
	}

	if timer.EvictedAt == "" {
		t.Error("EvictedAt was not set")
	}
	if timer.EvictionReason != "process_death_signal" {
		t.Errorf("EvictionReason = %q, want 'process_death_signal'", timer.EvictionReason)
	}

	if !timer.Expired(time.Now().UTC().Add(-time.Hour)) {
		t.Error("Expected evicted timer to be expired regardless of time")
	}
}

func TestForceEvict_noTimer(t *testing.T) {
	t.Parallel()
	root := tempDir(t)
	// Should not error if timer does not exist
	if err := ForceEvict(root, "NONEXISTENT", "reason"); err != nil {
		t.Errorf("ForceEvict on non-existent timer failed: %v", err)
	}
}
