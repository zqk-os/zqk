package swarm

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

func TestHygieneSweep_StaleLocksAndTempFiles(t *testing.T) {
	tempDir := t.TempDir()
	lockDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.LockDir)
	metricsDir := filepath.Join(tempDir, paths.ProjectDataDir, "metrics")
	processDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.ProcessDir)

	if err := fileutil.MkdirAll(lockDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create lockDir: %v", err)
	}
	if err := fileutil.MkdirAll(metricsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create metricsDir: %v", err)
	}
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create processDir: %v", err)
	}

	// 1. Create a stale lock file with an old modification time
	lockFile := filepath.Join(lockDir, "stale-worker.lock")
	if err := fileutil.WriteFile(lockFile, []byte(`{"pid":99999999}`), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write lock file: %v", err)
	}
	past := time.Now().Add(-10 * time.Minute)
	_ = fileutil.Chtimes(lockFile, past, past)

	// 2. Create orphaned .tmp files
	metricTmp := filepath.Join(metricsDir, "metric-sample.tmp")
	processTmp := filepath.Join(processDir, "sub-process.tmp")
	validFile := filepath.Join(processDir, "keep-me.json")

	_ = fileutil.WriteFile(metricTmp, []byte("temp metric"), paths.FilePerm644)
	_ = fileutil.WriteFile(processTmp, []byte("temp process"), paths.FilePerm644)
	_ = fileutil.WriteFile(validFile, []byte(`{"valid":true}`), paths.FilePerm644)

	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	store := newMockStorage()

	report, err := RunHygieneSweep(ctx, tempDir, store, sec, 1*time.Minute, false)
	if err != nil {
		t.Fatalf("RunHygieneSweep returned error: %v", err)
	}

	if report.StaleLocksCleared != 1 {
		t.Errorf("expected 1 stale lock cleared, got %d", report.StaleLocksCleared)
	}
	if fileutil.Exists(lockFile) {
		t.Errorf("expected lock file to be removed")
	}

	if report.TempFilesCleaned != 2 {
		t.Errorf("expected 2 temp files cleaned, got %d", report.TempFilesCleaned)
	}
	if fileutil.Exists(metricTmp) || fileutil.Exists(processTmp) {
		t.Errorf("expected temp files to be cleaned")
	}
	if !fileutil.Exists(validFile) {
		t.Errorf("expected non-tmp file to remain untouched")
	}
}

func TestHygieneSweep_CASIntegrityAndOrphans(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	store := newMockStorage()

	// Seed an orphan backlog item (no goal refs, no plan ref)
	orphanBLI := map[string]any{
		objects.FieldKeyID:     "BLI-ORPHAN-001",
		objects.FieldKeyKind:   objects.KindBacklogItem,
		objects.FieldKeyTitle:  "Orphan Task",
		objects.FieldKeyStatus: objects.ObjectStatusPlanned,
	}
	if err := store.Create(ctx, sec, orphanBLI); err != nil {
		t.Fatalf("store.Create BLI: %v", err)
	}

	// Seed an orphan criteria (no requirement ref)
	orphanCrit := map[string]any{
		objects.FieldKeyID:     "CRI-ORPHAN-001",
		objects.FieldKeyKind:   objects.KindCriteria,
		objects.FieldKeyTitle:  "Orphan Criteria",
		objects.FieldKeyStatus: objects.ObjectStatusActive,
	}
	if err := store.Create(ctx, sec, orphanCrit); err != nil {
		t.Fatalf("store.Create CRI: %v", err)
	}

	report, err := RunHygieneSweep(ctx, tempDir, store, sec, time.Minute, false)
	if err != nil {
		t.Fatalf("RunHygieneSweep error: %v", err)
	}

	if report.OrphanObjectCount < 2 {
		t.Errorf("expected at least 2 orphan alerts, got %d: %v", report.OrphanObjectCount, report.OrphanAlerts)
	}
	if report.MembraneHealthy {
		t.Errorf("expected MembraneHealthy=false when orphan objects present")
	}
}

func TestAlignmentPulse_RecalculatesAndPersists(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	store := newMockStorage()

	// Seed a goal
	goalObj := map[string]any{
		objects.FieldKeyID:     "GOAL-001",
		objects.FieldKeyKind:   objects.KindGoal,
		objects.FieldKeyTitle:  "Core Platform Goal",
		objects.FieldKeyStatus: objects.ObjectStatusActive,
	}
	_ = store.Create(ctx, sec, goalObj)

	// Seed 2 backlog items: 1 aligned, 1 unaligned
	bliAligned := map[string]any{
		objects.FieldKeyID:       "BLI-001",
		objects.FieldKeyKind:     objects.KindBacklogItem,
		objects.FieldKeyStatus:   objects.ObjectStatusPlanned,
		objects.FieldKeyGoalRefs: []any{"GOAL-001"},
	}
	_ = store.Create(ctx, sec, bliAligned)

	bliUnaligned := map[string]any{
		objects.FieldKeyID:     "BLI-002",
		objects.FieldKeyKind:   objects.KindBacklogItem,
		objects.FieldKeyStatus: objects.ObjectStatusPlanned,
	}
	_ = store.Create(ctx, sec, bliUnaligned)

	report, err := RunAlignmentPulse(ctx, tempDir, store, sec, false)
	if err != nil {
		t.Fatalf("RunAlignmentPulse failed: %v", err)
	}

	if report.AlignmentScore != 50.0 {
		t.Errorf("expected alignment score 50.0%%, got %.2f%%", report.AlignmentScore)
	}
	if report.GoalsCount != 1 {
		t.Errorf("expected 1 goal, got %d", report.GoalsCount)
	}
	if report.ItemsWithGoals != 1 || report.ItemsWithoutGoals != 1 {
		t.Errorf("expected 1 aligned and 1 gap, got with=%d without=%d", report.ItemsWithGoals, report.ItemsWithoutGoals)
	}

	// Verify file was written to .zqk/state/ambient/align-latest.json
	alignPath := filepath.Join(tempDir, paths.ProjectDataDir, whatsnext.AlignLatestRelativePath)
	data, err := fileutil.ReadFile(alignPath)
	if err != nil {
		t.Fatalf("failed to read persisted align cache: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to parse align cache JSON: %v", err)
	}
	score, _ := parsed["alignment_score"].(float64)
	if score != 50.0 {
		t.Errorf("persisted score expected 50.0, got %v", score)
	}
}

func TestWakeupBus_DispatchesWakeupAndDetectsWork(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	store := newMockStorage()

	// 1. Seed unblocked work item
	unblockedBLI := map[string]any{
		objects.FieldKeyID:     "BLI-UNBLOCKED",
		objects.FieldKeyKind:   objects.KindBacklogItem,
		objects.FieldKeyStatus: objects.ObjectStatusPlanned,
	}
	_ = store.Create(ctx, sec, unblockedBLI)

	// 2. Seed pending approval item
	pendingItem := map[string]any{
		objects.FieldKeyID:     "APPR-001",
		objects.FieldKeyKind:   objects.KindBacklogItem,
		objects.FieldKeyStatus: "pending_approval",
	}
	_ = store.Create(ctx, sec, pendingItem)

	report, err := RunWakeupBus(ctx, tempDir, store, sec, false)
	if err != nil {
		t.Fatalf("RunWakeupBus failed: %v", err)
	}

	if report.UnblockedWorkCount != 1 {
		t.Errorf("expected 1 unblocked work item, got %d", report.UnblockedWorkCount)
	}
	if report.PendingApprovalCount != 1 {
		t.Errorf("expected 1 pending approval, got %d", report.PendingApprovalCount)
	}
	if !report.WakeupDispatched {
		t.Errorf("expected WakeupDispatched=true")
	}
	if report.EventID == "" {
		t.Errorf("expected non-empty EventID")
	}
}

func TestHeartbeatPulse_FullCycle(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	store := newMockStorage()

	opts := HeartbeatOptions{
		ProjectRoot:     tempDir,
		Cadence:         15 * time.Minute,
		Once:            true,
		Storage:         store,
		SecurityContext: sec,
	}

	report, err := RunHeartbeatPulse(ctx, opts)
	if err != nil {
		t.Fatalf("RunHeartbeatPulse failed: %v", err)
	}

	if report.Cadence != 15*time.Minute {
		t.Errorf("expected cadence 15m, got %v", report.Cadence)
	}
	if !report.Wakeup.WakeupDispatched {
		t.Errorf("expected wakeup dispatched")
	}
}
