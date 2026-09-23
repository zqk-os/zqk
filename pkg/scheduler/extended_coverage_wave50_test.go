package scheduler

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtended_EventsAggregation_Wave50(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	// 1. Path helpers
	evPath := EventsPath(tmpDir)
	sumPath := SummaryPath(tmpDir)
	if evPath == "" || sumPath == "" {
		t.Errorf("expected non-empty paths")
	}

	// 2. Write sample events file
	diagLine := `{"level":"info","event_type":"scheduler_job_completed","job_id":"SCH-diag-1","duration_ms":150,"status":"completed"}`
	eventsContent := diagLine + "\n" + `{"level":"error","event_type":"scheduler_job_failed","job_id":"SCH-diag-1","status":"failed"}` + "\n"
	_ = fileutil.MkdirAll(filepath.Dir(evPath), 0755)
	_ = fileutil.WriteFile(evPath, []byte(eventsContent), 0600)

	// 3. AggregateEventsFromFile & accumulateJobStatsFromDiagnosticsLine
	jobStats := make(map[string]JobExecutionStats)
	accumulateJobStatsFromDiagnosticsLine(jobStats, []byte(diagLine))
	if len(jobStats) == 0 {
		t.Errorf("expected jobStats to be populated")
	}

	summary, count, err := AggregateEventsFromFile(evPath)
	if err != nil || summary == nil {
		t.Errorf("AggregateEventsFromFile failed: %v, count: %d", err, count)
	}

	// 4. WriteSummary & RunAggregation
	wErr := WriteSummary(summary, sumPath)
	if wErr != nil {
		t.Errorf("WriteSummary failed: %v", wErr)
	}

	aggSum, aggCount, aErr := RunAggregation(tmpDir, evPath, sumPath)
	if aErr != nil || aggSum == nil {
		t.Errorf("RunAggregation failed: %v, count: %d", aErr, aggCount)
	}

	// 5. TSDB global provider
	SetGlobalTSDBProvider("dummy_provider")
	if GetGlobalTSDBProvider() != "dummy_provider" {
		t.Errorf("unexpected TSDB provider")
	}
	_, _, _ = AggregateEventsFromTSDB("dummy_provider")
	SetGlobalTSDBProvider(nil)
}

func TestExtended_EmergencyManager_Wave50(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	h := NewEmergencyManagerHandler(tmpDir, logger).(*EmergencyManagerHandler)

	job := &ScheduledJob{
		ID:       "SCH-em-1",
		JobType:  JobTypeEmergencyManager,
		Category: CategoryMaintenance,
		Metadata: map[string]any{
			"allow_stash": true,
		},
	}

	// 1. emergencyAllowStash
	if !emergencyAllowStash(job) {
		t.Errorf("expected emergencyAllowStash to be true")
	}
	if emergencyAllowStash(&ScheduledJob{}) {
		t.Errorf("expected false for empty env")
	}

	// 2. statePath
	_ = h.statePath("cap_failure_tracker.json")

	// 3. readCAPFailureTracker & readLastCAPSuccessFromEvents
	_, _ = h.readCAPFailureTracker()
	_, _ = h.readLastCAPSuccessFromEvents()

	// 4. Git helpers default
	_, _ = h.gitStatusDirtyDefault()
	_ = h.gitStashDefault("emergency stash")

	// 5. writeReport & appendEmergencyChat
	rep := emergencyManagerReport{
		CheckedAt:           time.Now().Format(time.RFC3339),
		CAPConsecutiveFails: 2,
		Reason:              "test report",
		WorktreeDirty:       false,
	}
	h.writeReport(rep)
	h.appendEmergencyChat("test emergency chat", capFailureSnapshot{})

	// 6. Execute
	_ = h.Execute(ctx, job)
}

func TestExtended_ConvergenceTerminalAndLedger_Wave50(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if cleanup := sp.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	defer func() { _ = sp.Shutdown(ctx) }()

	secCtx := pkgctx.NewSystemSecurityContext()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// 1. convergence_cvs_per_test_failure_ledger.go
	_ = cvsPerTestLedgerMaxFailuresFromEnv()
	_ = stringKeyedAnyMap(map[string]any{"k": "v"})
	_ = stringKeyedAnyMap(nil)
	_ = cloneStringAnyMap(map[string]any{"k": "v"})
	_ = cloneStringAnyMap(nil)

	rwHandler := NewRunWrapperHandlerWithProjectRoot(sp, logger, nil, nil, tmpDir).(*RunWrapperHandler)
	cvsObj := map[string]any{
		objects.FieldKeyID:            "CVS-ledger-1",
		objects.FieldKeyKind:          objects.KindConvergenceSession,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
	}
	_ = sp.Create(ctx, secCtx, cvsObj)

	job := &ScheduledJob{
		ID:       "SCH-rw-ledger",
		JobType:  JobTypeRunWrapper,
		Category: CategoryMaintenance,
		EnvironmentVariables: map[string]string{
			"CONVERGENCE_SESSION_ID": "CVS-ledger-1",
		},
	}
	rwHandler.maybePersistCVSPerTestFailureLedger(ctx, job, "fp1", []string{"TestA", "TestB"}, false)
	rwHandler.maybePersistCVSPerTestFailureLedger(ctx, job, "fp1", nil, true)

	// 2. convergence_terminal_tick.go
	snap := &TestBundleConvergenceSnapshot{
		HadFailureInWindow:     true,
		FailCount:              1,
		FailingFingerprintsNow: []string{"fp1"},
	}
	needed, tests := convergenceTerminalFollowUpNeeded(snap)
	if !needed || len(tests) == 0 {
		t.Errorf("expected follow up needed")
	}

	spawnDir := terminalFollowupSpawnDir(tmpDir)
	markerPath := terminalFollowupSpawnMarkerFile(tmpDir, "CVS-prior", "watermark1")
	if spawnDir == "" || markerPath == "" {
		t.Errorf("expected non-empty marker paths")
	}

	m := &terminalFollowupMarker{
		PriorSessionID:  "CVS-prior",
		NewSessionID:    "CVS-next",
		HealthWatermark: "watermark1",
	}
	_ = writeTerminalFollowupMarker(markerPath, m)
	readM, rErr := readTerminalFollowupMarker(markerPath)
	if rErr != nil || readM == nil {
		t.Errorf("readTerminalFollowupMarker failed: %v", rErr)
	}

	followObj := buildFollowupDraftConvergenceSessionObject("CVS-prior", cvsObj, snap)
	if followObj == nil {
		t.Fatalf("expected non-nil follow up object")
	}

	cvsHandler := NewConvergenceSessionTickHandler(sp, tmpDir).(*ConvergenceSessionTickHandler)
	_, _ = cvsHandler.maybeSpawnTerminalFollowupDraft(ctx, secCtx, job, "CVS-prior", cvsObj, snap)
	_ = cvsHandler.executeTerminalConvergenceSessionTick(ctx, secCtx, job, "CVS-prior", cvsObj, snap)
}
