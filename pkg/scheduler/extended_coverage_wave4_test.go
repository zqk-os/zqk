package scheduler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtended_CAPOrchestratorMethodsDeep(t *testing.T) {
	ctx := context.Background()
	sp := storagepkg.NewNoopObjectStorage()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	tmpDir := t.TempDir()

	coh := NewCapOrchestratorHandler(sp, tmpDir, logger).(*CapOrchestratorHandler)

	_ = coh.capStateDir()
	p1 := coh.capStatePath("state.json")
	p2 := coh.legacyCapStatePath("legacy.json")
	if p1 == "" || p2 == "" {
		t.Fatal("expected non-empty state paths")
	}

	coh.markStageEntered("grooming", "PRI-1")
	coh.clearGroomingPlannedZeroLatch()
	_, _ = coh.groomingPlannedZeroExhausted(0)

	pending := capStagePending{Stage: "review", PlanID: "PRI-1"}
	_ = coh.capStageQuarantineRetryReady(pending, time.Now())
	_ = coh.liveCVSOrEmpty(ctx, "CVS-1")
	_, _ = coh.resolveBoundCVS(ctx, "PRI-1")

	_, _ = coh.stageDeliveryComplete(ctx, "review")
	_, _ = coh.reviewDeliveryComplete()
	_, _ = coh.groomingDeliveryComplete(ctx)
	_, _ = coh.listGroomingArtifactsSince(ctx, time.Now())
	_, _ = coh.dispatchDeliveryComplete(ctx, "dispatch")

	evStore := coh.capEvidenceStorage()
	if evStore == nil {
		t.Fatal("expected evidence storage")
	}

	_, _ = coh.verifiedStageReceipt(ctx, "review")
	_ = coh.stateFileFresh("test.json", "1h")
	_ = coh.maybeAdvanceCAPStage(ctx, "review")
	coh.maybeWakeOnStageHold("review", "waiting on review")

	_ = coh.executeSentinelStage(ctx, "echo")

	idx1 := coh.buildOpenAgentInstructionIndex(ctx)
	if idx1 == nil {
		t.Fatal("expected instruction index")
	}
	_ = coh.hasOpenAgentInstruction(ctx, "PRI-1", "tpm", "groom")

	idx2 := coh.buildOpenAgentTaskIndex(ctx)
	if idx2 == nil {
		t.Fatal("expected task index")
	}
	_ = coh.hasOpenTaskForPlan(ctx, "PRI-1", "task-1")

	_, _ = coh.resolveCLIExecutable()
	_, _, _ = coh.verifyCriticalPackagesHealth(ctx, pkgctx.NewSystemSecurityContext())
	_ = coh.autoRecoverPlanTasks(ctx, "PRI-1")
	coh.wakeAgentAndScheduleHourglass("task-1", "tpm")
	_ = coh.resolveWakePlanID(ctx, "PRI-1")
	_ = coh.topOpenPlanBLIs(ctx, "PRI-1", 3)
}

func TestExtended_LifecycleCoordinationDeep(t *testing.T) {
	ctx := context.Background()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	sp := storagepkg.NewNoopObjectStorage()

	s := &Scheduler{
		logger:                  logger,
		storage:                 sp,
		jobs:                    make(map[string]*ScheduledJob),
		lockFailureCountByJobID: make(map[string]int),
		running:                 false,
	}

	s.maybeReconcileSchedulerJobCASIndexBeforeReload(ctx)

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	s.watchJobChanges(cancelCtx)

	checked, recovered := s.checkAndRecoverMissedJobs(ctx)
	_ = checked
	_ = recovered

	timerJobs, has := s.collectTimerJobs()
	_ = timerJobs
	_ = has

	complMap := s.buildCompletionMapFromActivityCache()
	if complMap == nil {
		t.Fatal("expected non-nil completion map")
	}

	_ = s.countMissedJobs(timerJobs, time.Now(), time.Hour, complMap, nil)
}

func TestExtended_JobExecutionDeep(t *testing.T) {
	tmpDir := t.TempDir()
	job := &ScheduledJob{
		ID:                "job-exec-1",
		JobType:           JobTypeRunWrapper,
		Command:           "echo",
		MaxRuntimeSeconds: 10,
	}

	WriteJobOutcome(tmpDir, job.ID, job.JobType, map[string]any{"status": "ok"})
	WriteJobProgress(tmpDir, job.ID, map[string]any{"pct": 100})
	writeJobLogEntry(tmpDir, job.ID, map[string]any{"entry": "val"})

	logPath := filepath.Join(tmpDir, "test.log")
	_ = fileutil.WriteFile(logPath, []byte("line1\nline2\nline3\n"), 0644)
	trimJobLogFileIfNeeded(logPath, 2)

	dCtx, dCancel := dispatchContextForScheduledJob(job)
	if dCtx == nil {
		t.Fatal("expected context")
	}
	dCancel()

	_ = shouldInvokeConfiguredJobCallback(context.Background(), job)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	sp := storagepkg.NewNoopObjectStorage()
	s := &Scheduler{
		logger:                  logger,
		storage:                 sp,
		cron:                    cron.New(cron.WithSeconds(), cron.WithLocation(time.UTC)),
		jobs:                    map[string]*ScheduledJob{job.ID: job},
		lockFailureCountByJobID: make(map[string]int),
	}

	s.handleJobExecutionOutcome(context.Background(), job, nil, time.Second, nil, "op-1")
	s.updateJobInStorage(context.Background(), job)
	s.DisableJobInStorage(context.Background(), job)
}

func TestExtended_HourglassDeep(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	sp := storagepkg.NewNoopObjectStorage()

	s := &Scheduler{
		projectRoot: tmpDir,
		logger:      logger,
		storage:     sp,
		jobs:        make(map[string]*ScheduledJob),
	}

	secCtx := pkgctx.NewSystemSecurityContext()

	s.checkHourglassTimers(ctx)
	s.sweepStaleAgentTasks(ctx)

	staleFile := filepath.Join(tmpDir, "stale.json")
	_ = os.WriteFile(staleFile, []byte(`{"task_id": "t1", "expires_at": "2020-01-01T00:00:00Z", "type": "deadline"}`), 0644)
	s.handleMissedCheckin(ctx, secCtx, staleFile)

	s.recordSilentClaimBlocker(ctx, secCtx, "task-1")
	s.escalateMissedDeadline(ctx, secCtx, "task-1", "agent_task", "Task title")
}

func TestExtended_HandlersMetricsCleanupDeep(t *testing.T) {
	ctx := context.Background()
	sp := storagepkg.NewNoopObjectStorage()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	job := &ScheduledJob{ID: "metrics-job"}

	cj := NewChangeJournalAggregationHandler(sp)
	_ = cj.Execute(ctx, job)

	am := NewAggregationMetricsCleanupHandler(sp)
	_ = am.Execute(ctx, job)

	gm := NewGenericMetricsCleanupHandler(sp)
	_ = gm.Execute(ctx, job)

	_ = preExecutionHealthCheck(ctx, sp, logger, job, "audit_aggregation_metric")
}

func TestExtended_HandlersMeshLeaseDeep(t *testing.T) {
	ctx := context.Background()
	sp := storagepkg.NewNoopObjectStorage()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	tmpDir := t.TempDir()

	ml := NewMeshLeaseSupervisionHandler(sp, tmpDir, logger).(*MeshLeaseSupervisionHandler)
	ml.reapStaleSubprocesses(map[string]bool{"lease-1": true})

	if f := getFloat(42.5); f != 42.5 {
		t.Fatalf("expected 42.5, got %f", f)
	}
	if f := getFloat("not-float"); f != 0 {
		t.Fatalf("expected 0, got %f", f)
	}

	_ = ml.Execute(ctx, &ScheduledJob{ID: "mesh-job"})
}

func TestExtended_AutofixBatchCleanup(t *testing.T) {
	rec := buildBatchMetricErrorRecord("m1", "b1", "2026-01-01T00:00:00Z", errors.New("sample error"))
	if rec == nil || rec[objects.FieldKeyID] != "m1" {
		t.Fatalf("unexpected record: %v", rec)
	}

	sp := storagepkg.NewNoopObjectStorage()
	tmpDir := t.TempDir()
	h := NewAutofixBatchCleanupHandler(sp, tmpDir)
	_ = h.Execute(context.Background(), &ScheduledJob{ID: "autofix-job"})
}

func TestExtended_ContextRefreshAndTickDeep(t *testing.T) {
	d, err := parseCadence("1h")
	if err != nil || d != time.Hour {
		t.Fatalf("unexpected parseCadence result: %v, %v", d, err)
	}
	_, _ = parseCadence("PT30M")

	d, err = parseISODuration("PT1H")
	if err != nil || d != time.Hour {
		t.Fatalf("unexpected parseISODuration result: %v, %v", d, err)
	}
	_, _ = parseISODuration("P1D")

	m := mergeStringAnyMaps(map[string]any{"a": 1}, map[string]any{"b": 2})
	if len(m) != 2 {
		t.Fatalf("expected 2 items, got %d", len(m))
	}

	s := truncateOrchestrateOutputPreview("very long string here", 4)
	if len(s) == 0 {
		t.Fatal("expected non-empty truncated preview")
	}

	maxTicks := thresholdMaxTicksPerHour(map[string]any{"max_ticks_per_hour": 10})
	if maxTicks != 10 {
		t.Fatalf("expected 10, got %d", maxTicks)
	}

	activity := []any{
		map[string]any{
			"action":    "measure_test_bundle_health",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	}
	ticks := countMeasureTicksInLastHour(activity, time.Now().UTC())
	if ticks != 1 {
		t.Fatalf("expected 1 tick, got %d", ticks)
	}

	sp := storagepkg.NewNoopObjectStorage()
	tmpDir := t.TempDir()
	crh := NewContextRefreshHandler(sp, tmpDir).(*ContextRefreshHandler)
	_ = crh.updateSchedule(context.Background(), pkgctx.NewSystemSecurityContext(), "sch-1", map[string]any{"last_run": "now"})
}

func TestExtended_ConvergenceSuggestedFieldsDeep(t *testing.T) {
	snap := &TestBundleConvergenceSnapshot{
		PassCount: 1,
	}

	mergeCompletionGateObservabilityIntoPhaseRouter(map[string]any{}, map[string]any{})

	suggested, err := BuildSuggestedConvergenceSessionFields(
		snap, "measure", "standard", map[string]any{}, nil, false, nil, false, "", nil,
	)
	if err != nil || suggested == nil {
		t.Fatalf("unexpected BuildSuggestedConvergenceSessionFields: %v, %v", suggested, err)
	}

	updateBody := BuildConvergenceObjectUpdateBody(suggested, false, snap, nil, false, "")
	if updateBody == nil {
		t.Fatal("expected update body")
	}

	log1 := BuildConvergenceActivityLogEntryNoNewWatermark("measure", snap, time.Now().UTC())
	if log1 == nil {
		t.Fatal("expected log entry")
	}

	log2 := BuildConvergenceActivityLogEntry(suggested, snap)
	if log2 == nil {
		t.Fatal("expected log entry")
	}
}

func TestExtended_SwarmWorkerAndEventsAggregation(t *testing.T) {
	_ = fallbackChatModel()

	s, ok := orphanRecoveryStatus("in_progress")
	if !ok || s != objects.ObjectStatusApproved {
		t.Fatalf("expected approved status, got %s, %v", s, ok)
	}
	_, ok = orphanRecoveryStatus("completed")
	if ok {
		t.Fatal("expected false for completed")
	}

	tmpDir := t.TempDir()
	_ = resolveSwarmClaimant(tmpDir, map[string]any{})
	_ = resolveSwarmMCPPath(tmpDir)

	ep := EventsPath(tmpDir)
	sp := SummaryPath(tmpDir)
	if ep == "" || sp == "" {
		t.Fatal("expected non-empty paths")
	}

	SetGlobalTSDBProvider(nil)
	_ = GetGlobalTSDBProvider()
	_ = WriteSummary(&SchedulerMetricsSummary{}, filepath.Join(tmpDir, "summary.json"))
	accumulateJobStatsFromDiagnosticsLine(map[string]JobExecutionStats{}, []byte(`{"job_id": "j1", "duration_ms": 100}`))
}

func TestExtended_ActivityCacheDeep(t *testing.T) {
	if !activityCacheableJobID("SCH-job-1") {
		t.Fatal("expected true for SCH-job-1")
	}

	_ = GetGlobalActivityCache()

	tmpDir := t.TempDir()
	c := NewActivityCache().(*ActivityCache)

	_ = c.GetMetadata()
	fp := c.getCacheFilePath(tmpDir)
	if fp == "" {
		t.Fatal("expected cache file path")
	}

	c.UpdateEvent("j1", "completed", time.Second, nil)
	_ = c.SaveCache(tmpDir)
	_ = c.LoadCache(tmpDir)

	_, _ = c.GetEntry("j1")
	_ = c.GetAllEntries()
	_ = c.GetEntriesForJobs([]string{"j1"})
	c.Stop()
}
