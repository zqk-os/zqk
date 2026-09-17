package scheduler

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestEmergencyManager_DetectsConsecutiveFailures(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, paths.ProjectDataDir, paths.StateDir)
	if err := fileutil.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	fail := capFailureSnapshot{ConsecutiveFailures: 5, LastFailure: "2026-08-05T12:00:00Z", LastStage: "cap_stage_review"}
	b, _ := json.Marshal(fail)
	if err := fileutil.WriteFile(filepath.Join(stateDir, capFailureTrackerFile), b, 0600); err != nil {
		t.Fatal(err)
	}

	h := NewEmergencyManagerHandler(root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*EmergencyManagerHandler)
	stashed := false
	h.gitStatusDirty = func() (bool, error) { return true, nil }
	h.gitStash = func(msg string) error { stashed = true; return nil }
	h.lastCAPSuccess = func() (time.Time, bool) { return time.Now().UTC().Add(-time.Minute), true }

	job := &ScheduledJob{ID: "SCH-emergency-manager", JobType: JobTypeEmergencyManager}
	if err := h.Execute(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if stashed {
		t.Fatal("stash must be gated off by default")
	}
	raw, err := fileutil.ReadFile(filepath.Join(stateDir, emergencyManagerStateFile))
	if err != nil {
		t.Fatal(err)
	}
	var report emergencyManagerReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if !report.CAPUnhealthy {
		t.Fatalf("expected unhealthy: %+v", report)
	}
	if report.StashPerformed {
		t.Fatal("stash should not run without metadata.allow_stash")
	}
}

func TestEmergencyManager_StashWhenAllowed(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, paths.ProjectDataDir, paths.StateDir)
	if err := fileutil.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	fail := capFailureSnapshot{ConsecutiveFailures: 4, LastFailure: "2026-08-05T12:00:00Z"}
	b, _ := json.Marshal(fail)
	_ = fileutil.WriteFile(filepath.Join(stateDir, capFailureTrackerFile), b, 0600)

	h := NewEmergencyManagerHandler(root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*EmergencyManagerHandler)
	stashed := false
	h.gitStatusDirty = func() (bool, error) { return true, nil }
	h.gitStash = func(msg string) error {
		stashed = true
		if msg == "" {
			t.Fatal("empty stash message")
		}
		return nil
	}
	h.lastCAPSuccess = func() (time.Time, bool) { return time.Now().UTC(), true }

	job := &ScheduledJob{
		ID:       "SCH-emergency-manager",
		JobType:  JobTypeEmergencyManager,
		Metadata: map[string]any{"allow_stash": true},
	}
	if err := h.Execute(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if !stashed {
		t.Fatal("expected stash when allow_stash=true and dirty")
	}
}

func TestEmergencyManager_HealthyNoAction(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, paths.ProjectDataDir, paths.StateDir)
	_ = fileutil.MkdirAll(stateDir, 0755)
	fail := capFailureSnapshot{ConsecutiveFailures: 0}
	b, _ := json.Marshal(fail)
	_ = fileutil.WriteFile(filepath.Join(stateDir, capFailureTrackerFile), b, 0600)

	h := NewEmergencyManagerHandler(root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*EmergencyManagerHandler)
	h.gitStatusDirty = func() (bool, error) { return true, nil }
	h.gitStash = func(msg string) error { t.Fatal("stash on healthy"); return nil }
	h.lastCAPSuccess = func() (time.Time, bool) { return time.Now().UTC().Add(-time.Minute), true }

	if err := h.Execute(context.Background(), &ScheduledJob{ID: "SCH-em", JobType: JobTypeEmergencyManager}); err != nil {
		t.Fatal(err)
	}
	raw, _ := fileutil.ReadFile(filepath.Join(stateDir, emergencyManagerStateFile))
	var report emergencyManagerReport
	_ = json.Unmarshal(raw, &report)
	if report.CAPUnhealthy {
		t.Fatalf("expected healthy: %+v", report)
	}
}
