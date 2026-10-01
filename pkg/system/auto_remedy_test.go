package system_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/system"
	"github.com/zqk-os/zqk/pkg/systemcheck"
)

func TestAutoRemedy_DiagnoseStaleLocksAndTemps(t *testing.T) {
	tmpDir := t.TempDir()

	zqkDir := filepath.Join(tmpDir, paths.ProjectDataDir)
	if err := os.MkdirAll(zqkDir, 0755); err != nil {
		t.Fatalf("failed to create .zqk dir: %v", err)
	}

	// Create a stale lock file (older than DefaultLockStaleAge 15m)
	staleLock := filepath.Join(zqkDir, "test.lock")
	if err := os.WriteFile(staleLock, []byte("lock content"), 0644); err != nil {
		t.Fatalf("failed to create stale lock: %v", err)
	}
	oldTime := time.Now().Add(-20 * time.Minute)
	if err := os.Chtimes(staleLock, oldTime, oldTime); err != nil {
		t.Fatalf("failed to set stale lock modtime: %v", err)
	}

	// Create an orphaned temp file (older than DefaultTempOrphanAge 30m)
	orphanTmp := filepath.Join(zqkDir, "test.tmp")
	if err := os.WriteFile(orphanTmp, []byte("temp content"), 0644); err != nil {
		t.Fatalf("failed to create orphan tmp: %v", err)
	}
	veryOldTime := time.Now().Add(-45 * time.Minute)
	if err := os.Chtimes(orphanTmp, veryOldTime, veryOldTime); err != nil {
		t.Fatalf("failed to set orphan tmp modtime: %v", err)
	}

	engine := system.NewDiagnosticsRemedyEngine(tmpDir)
	plans, err := engine.Diagnose(context.Background(), nil)
	if err != nil {
		t.Fatalf("diagnose failed: %v", err)
	}

	hasLockPlan := false
	hasTmpPlan := false
	for _, p := range plans {
		if p.ActionType == system.ActionRemoveFile {
			if filepath.Base(p.Target) == "test.lock" {
				hasLockPlan = true
			}
			if filepath.Base(p.Target) == "test.tmp" {
				hasTmpPlan = true
			}
		}
	}

	if !hasLockPlan {
		t.Errorf("expected plan for stale lock file, got: %+v", plans)
	}
	if !hasTmpPlan {
		t.Errorf("expected plan for orphaned tmp file, got: %+v", plans)
	}

	// Apply auto-remedy
	report, err := engine.Apply(context.Background(), plans)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if report.TotalApplied < 2 {
		t.Errorf("expected at least 2 applied remedies, got %d", report.TotalApplied)
	}

	// Verify files are cleaned up
	if _, err := os.Stat(staleLock); !os.IsNotExist(err) {
		t.Errorf("expected stale lock to be deleted, stat err: %v", err)
	}
	if _, err := os.Stat(orphanTmp); !os.IsNotExist(err) {
		t.Errorf("expected orphan tmp to be deleted, stat err: %v", err)
	}
}

func TestAutoRemedy_DiagnoseUnseededPreconditions(t *testing.T) {
	tmpDir := t.TempDir()

	engine := system.NewDiagnosticsRemedyEngine(tmpDir)
	policySeeded := false
	personaSeeded := false

	engine.PolicySeeder = func(root string) (int, error) {
		policySeeded = true
		return 3, nil
	}
	engine.PersonaSeeder = func(root string) (int, error) {
		personaSeeded = true
		return 5, nil
	}

	plans, err := engine.Diagnose(context.Background(), nil)
	if err != nil {
		t.Fatalf("diagnose failed: %v", err)
	}

	hasPolicyPlan := false
	hasPersonaPlan := false
	for _, p := range plans {
		if p.ID == "REMEDY-UNSEEDED-POLICIES" {
			hasPolicyPlan = true
		}
		if p.ID == "REMEDY-UNSEEDED-PERSONAS" {
			hasPersonaPlan = true
		}
	}

	if !hasPolicyPlan {
		t.Errorf("expected REMEDY-UNSEEDED-POLICIES in plans: %+v", plans)
	}
	if !hasPersonaPlan {
		t.Errorf("expected REMEDY-UNSEEDED-PERSONAS in plans: %+v", plans)
	}

	// Apply seed remedies
	report, err := engine.Apply(context.Background(), plans)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if !policySeeded {
		t.Errorf("expected PolicySeeder to be called")
	}
	if !personaSeeded {
		t.Errorf("expected PersonaSeeder to be called")
	}
	if report.TotalApplied != 2 {
		t.Errorf("expected 2 applied remedies, got %d", report.TotalApplied)
	}
}

func TestAutoRemedy_DiagnoseCheckIssues(t *testing.T) {
	tmpDir := t.TempDir()
	engine := system.NewDiagnosticsRemedyEngine(tmpDir)

	mockResults := []systemcheck.CheckResult{
		{
			ObjectID: "BLI-123",
			Issues: []systemcheck.Issue{
				{
					Message:    "Precondition not met: missing estimated_effort",
					FixCommand: "zqk object update BLI-123 --field estimated_effort=1d",
				},
				{
					Message:     "Hash mismatch in object registry",
					AutoFixable: true,
				},
			},
		},
	}

	plans, err := engine.Diagnose(context.Background(), mockResults)
	if err != nil {
		t.Fatalf("diagnose failed: %v", err)
	}

	if len(plans) < 2 {
		t.Fatalf("expected at least 2 plans from check issues, got %d", len(plans))
	}
}

func TestAutoRemedy_ApplyKillProcess(t *testing.T) {
	tmpDir := t.TempDir()
	engine := system.NewDiagnosticsRemedyEngine(tmpDir)

	plans := []system.RemedyPlan{
		{
			ID:          "REMEDY-ORPHAN-PROC-PID-9999",
			Title:       "Terminate Orphaned Process",
			ActionType:  system.ActionKillProcess,
			Target:      "PID 9999: zqk",
			Confidence:  1.0,
			AutoApply:   true,
		},
	}

	report, err := engine.Apply(context.Background(), plans)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if report.TotalApplied != 1 {
		t.Errorf("expected 1 applied remedy, got %d", report.TotalApplied)
	}
}

