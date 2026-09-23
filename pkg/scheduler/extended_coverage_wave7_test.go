package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_CAPStages_Detailed(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	secCtx := pkgctx.NewSystemSecurityContext()
	_ = secCtx

	h := &CapOrchestratorHandler{
		storage:     sp,
		logger:      logger,
		projectRoot: tmpDir,
	}

	// 1. parseSystemCheckPublicBlockers
	val, err := parseSystemCheckPublicBlockers([]byte(`{"public_blockers": 5}`))
	if err != nil || val != 5 {
		t.Errorf("expected 5, got %d, err %v", val, err)
	}
	val, err = parseSystemCheckPublicBlockers([]byte(`{"other": 1}`))
	if err != nil || val != 0 {
		t.Errorf("expected 0, got %d, err %v", val, err)
	}
	_, err = parseSystemCheckPublicBlockers([]byte(`invalid json`))
	if err == nil {
		t.Error("expected error for invalid json")
	}

	// 2. Stage execution using "echo" as executable
	_ = h.executeReviewStage(ctx, "echo")
	_ = h.executeMetricsStage(ctx, "echo")
	_ = h.executeSelfImprovementStage(ctx, "echo")
	_ = h.executeGroomingStage(ctx, "echo", "PRI-test", 1, "Test Plan", "active", map[string]int{"planned": 1})
	_ = h.executeSentinelStage(ctx, "echo")

	// 3. Stage helpers and instructions
	_ = h.hasOpenTPMGroomingInstruction(ctx, "PRI-test")
	_ = h.capStageAGIInstruction(ctx, "grooming", "PRI-test")
	_ = h.autoRecoverPlanTasks(ctx, "PRI-test")

	// 4. Stage gates and state
	h.markStageEntered("grooming", "PRI-test")
	h.clearGroomingPlannedZeroLatch()
	_, _ = h.reviewDeliveryComplete()
	_, _ = h.groomingDeliveryComplete(ctx)
	_ = h.stateFileExists("nonexistent")
	_ = h.stateFileNonEmpty("nonexistent")
	_ = h.stateFileFresh("nonexistent", "1h")
	_ = h.maybeAdvanceCAPStage(ctx, "grooming")
	h.maybeWakeOnStageHold("grooming", "waiting on artifacts")

	// 5. State files
	stateFile := filepath.Join(tmpDir, "test_cap_state.json")
	h.writeStateFile(stateFile, map[string]any{"status": "ok"})
	_, _ = h.readPendingStage()
	time.Sleep(500 * time.Millisecond)
}

func TestExtended_AuditAggregationSession_Execution(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// Seed audit events
	for i := 0; i < 5; i++ {
		ev := map[string]any{
			objects.FieldKeyID:            "AUDIT-ev-" + string(rune('a'+i)),
			objects.FieldKeyKind:          objects.KindAuditEvent,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        "recorded",
			objects.FieldKeyCreatedAt:     time.Now().Add(-time.Duration(i+1) * time.Hour).Format(time.RFC3339),
			"event_type":                  "scheduler_job_completed",
			"target_kind":                 objects.KindSchedulerJob,
			"target_id":                   "SCH-test-job",
		}
		_ = sp.Create(ctx, secCtx, ev)
	}

	handler := NewAuditAggregationHandler(sp).(*AuditAggregationHandler)
	job := &ScheduledJob{
		ID: "SCH-audit-agg-test",
		EnvironmentVariables: map[string]string{
			EnvKeyAggregationWindow: "1h",
			EnvKeyRetentionDuration: "2h",
			EnvKeyBatchSize:         "50",
		},
	}

	session := &auditAggregationSession{
		handler:             handler,
		ctx:                 ctx,
		job:                 job,
		secCtx:              secCtx,
		storageCtx:          storageCtx,
		service:             storagepkg.NewAuditAggregationService(sp),
		phaseDurations:      make(map[string]float64),
		windowDuration:      1 * time.Hour,
		deleteAfterDuration: 2 * time.Hour,
		effectiveRetention:  2 * time.Hour,
		batchSize:           50,
		windowStart:         time.Now().Add(-2 * time.Hour),
		windowEnd:           time.Now(),
		retentionMaxBatches: 2,
		catchUpMaxBatches:   2,
	}

	_ = session.loadConfiguration()
	session.setupPhaseContexts()
	_, _ = session.runPreChecks()
	session.determineEffectiveRetention()
	_ = session.hasTimeRemaining()
	session.runRetentionFirstPass()
	session.runCatchUp()
	session.runAggressiveCleanup()
	session.runProactiveCleanup()
	res, _ := session.runAggregation()
	session.runPostAggregationCleanup(res)
	session.runRetentionSecondPass()
	session.finalize(res)
	session.cleanup()
	time.Sleep(200 * time.Millisecond)
}

func TestExtended_PIDFileAndProcesses(t *testing.T) {
	tmpDir := t.TempDir()

	// Test PID file writing and reading
	err := WritePIDFile(tmpDir)
	if err != nil {
		t.Fatalf("failed to write pid: %v", err)
	}

	readPID, err := readPIDFile(tmpDir)
	if err != nil {
		t.Errorf("readPIDFile err: %v", err)
	}
	t.Logf("Read PID: %d", readPID)

	// Test IsProcessRunning and IsDaemonProcessAlive
	_ = IsProcessRunning(readPID)
	_ = IsDaemonProcessAlive(readPID)
	_, _, _ = IsSchedulerRunning(tmpDir)

	// Test KeepAlive
	_ = writeKeepAlive(tmpDir)
	_, _ = readKeepAlive(tmpDir)
	_, _, _ = IsSchedulerAlive(tmpDir)
	_ = RemoveKeepAlive(tmpDir)

	// Test RemovePIDFile
	err = RemovePIDFile(tmpDir)
	if err != nil {
		t.Fatalf("failed to remove pid: %v", err)
	}

	// Read after remove
	_, err = readPIDFile(tmpDir)
	if err == nil {
		t.Error("expected error after remove")
	}

	// Test helpers
	pid := os.Getpid()
	_ = isZombieProcess(pid)
	_ = isSchedulerDaemonProcess(pid)
	_, _ = findSchedulerProcessesByCommand(tmpDir)
}
