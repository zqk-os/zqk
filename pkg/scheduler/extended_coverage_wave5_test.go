package scheduler

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_AllPipelines(t *testing.T) {
	ctx := context.Background()
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()

	sp := storagepkg.NewNoopObjectStorage()
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	notifCtx := NewNotificationContext(logger, nil)
	job := &ScheduledJob{ID: "test-pipeline-job", Command: "echo hello"}

	_ = RunMetricsCollectionViaPipeline(cancelCtx, &FileLockMetricsCollectionHandler{storage: sp}, job)
	_ = RunCallbackListenerViaPipeline(cancelCtx, NewCallbackListenerHandler(sp, logger, nil, nil, notifCtx).(*CallbackListenerHandler), job)
	_ = RunCachePrewarmViaPipeline(cancelCtx, NewCachePrewarmHandler(nil, nil, sp, tmpDir, nil).(*CachePrewarmHandler), job)
	_ = RunCacheInvalidationViaPipeline(cancelCtx, &CacheInvalidationHandler{storage: sp}, job)
	_ = RunCascadeUpdateViaPipeline(cancelCtx, &CascadeUpdateHandler{storage: sp}, job)
	_ = RunCleanupConfigViaPipeline(cancelCtx, NewCleanupConfigHandler(tmpDir, logger).(*CleanupConfigHandler), job)
	_ = RunContextRefreshViaPipeline(cancelCtx, NewContextRefreshHandler(sp, tmpDir).(*ContextRefreshHandler), job)
	_ = RunGenericMetricsCleanupViaPipeline(cancelCtx, &GenericMetricsCleanupHandler{storage: sp}, job)
	_ = RunIntegrityCheckViaPipeline(cancelCtx, &IntegrityCheckHandler{storage: sp}, job)
	_ = RunLifecycleCheckViaPipeline(cancelCtx, &LifecycleCheckHandler{storage: sp}, job)
	_ = RunObjectValidationViaPipeline(cancelCtx, NewObjectValidationHandler(tmpDir, logger).(*ObjectValidationHandler), job)
	_ = RunOperationExecutionViaPipeline(cancelCtx, &OperationExecutionHandler{storage: sp}, job)
	_ = RunRetentionToleranceViaPipeline(cancelCtx, NewRetentionToleranceHandler(sp, tmpDir).(*RetentionToleranceHandler), job)
	_ = RunSchedulerJobRetentionViaPipeline(cancelCtx, &SchedulerJobRetentionHandler{storage: sp}, job)
	_ = RunTestIOViaPipeline(cancelCtx, &TestIOHandler{storage: sp}, job)
	_ = RunWatchdogEvaluationViaPipeline(cancelCtx, &WatchdogEvaluationHandler{storage: sp}, job)
	_ = RunAutofixBatchCleanupViaPipeline(cancelCtx, &AutofixBatchCleanupHandler{storage: sp}, job)
	_ = RunAggregationMetricsCleanupViaPipeline(cancelCtx, &AggregationMetricsCleanupHandler{storage: sp}, job)
}

func TestExtended_RunWrapperExecutionLive(t *testing.T) {
	ctx := context.Background()
	sp := storagepkg.NewNoopObjectStorage()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	tmpDir := t.TempDir()

	h := NewRunWrapperHandlerWithProjectRoot(sp, logger, nil, nil, tmpDir).(*RunWrapperHandler)

	// 1. Success job
	jobSuccess := &ScheduledJob{
		ID:          "SCH-run-success",
		Command:     "echo",
		CommandArgs: []string{"hello world"},
	}
	if err := h.Execute(ctx, jobSuccess); err != nil {
		t.Fatalf("unexpected success job error: %v", err)
	}

	// 2. Shell inline command
	jobShell := &ScheduledJob{
		ID:          "SCH-run-shell",
		Command:     "sh",
		CommandArgs: []string{"-c", "echo inline shell"},
	}
	_ = h.Execute(ctx, jobShell)

	// 3. Failing job
	jobFail := &ScheduledJob{
		ID:          "SCH-run-fail",
		Command:     "sh",
		CommandArgs: []string{"-c", "exit 1"},
		RetryCount:  1,
	}
	_ = h.Execute(ctx, jobFail)

	// 4. Timeout job
	timeoutCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	jobSleep := &ScheduledJob{
		ID:          "SCH-run-sleep",
		Command:     "sleep",
		CommandArgs: []string{"2"},
	}
	_ = h.Execute(timeoutCtx, jobSleep)

	// 5. Test bundle job
	jobBundle := &ScheduledJob{
		ID:          "SCH-run-test-bundle-live",
		Command:     "echo",
		CommandArgs: []string{"PASS"},
		Metadata: map[string]any{
			KeyBundleCommandFingerprint: "fp-live",
		},
	}
	_ = h.Execute(ctx, jobBundle)
}

func TestExtended_ExecutorsCoverage(t *testing.T) {
	ctx := context.Background()
	native := &NativeExecutor{}
	cmd := native.CommandContext(ctx, "echo", "native")
	cmd.SetDir("")
	cmd.SetEnv([]string{"A=B"})
	cmd.SetStdin(nil)
	cmd.SetStdout(nil)
	cmd.SetStderr(nil)
	cmd.SetSysProcAttr(nil)
	_ = cmd.Start()
	_ = cmd.Wait()
	_ = cmd.GetPid()

	_, _ = native.Execute(ctx, "", nil, "echo", "test")

	mockCmd := &MockCmd{
		OutputBytes: []byte("mock output"),
	}
	_ = mockCmd.Start()
	_ = mockCmd.Wait()
	_ = mockCmd.Run()
	_, _ = mockCmd.Output()
	mockCmd.SetDir("/tmp")
	mockCmd.SetEnv([]string{"FOO=BAR"})
	mockCmd.SetStdin(nil)
	mockCmd.SetStdout(nil)
	mockCmd.SetStderr(nil)
	mockCmd.SetSysProcAttr(nil)
	if pid := mockCmd.GetPid(); pid != 999999 {
		t.Fatalf("expected 999999, got %d", pid)
	}

	mockExec := &MockExecutor{}
	c2 := mockExec.CommandContext(ctx, "cmd")
	if c2 == nil {
		t.Fatal("expected mock cmd")
	}
	_, _ = mockExec.Execute(ctx, "", nil, "cmd")

	memExec := NewMemoryExecutor(map[string]*MockCmd{"cmd": mockCmd})
	if memExec == nil {
		t.Fatal("expected memory executor")
	}
}

func TestExtended_RetentionToleranceWithRealStorage(t *testing.T) {
	tmpDir := t.TempDir()
	stor, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil || stor == nil {
		t.Fatalf("failed to create test storage: %v", err)
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// Create objects of custom_kind
	for i := 1; i <= 5; i++ {
		obj := map[string]any{
			objects.FieldKeyID:        filepath.Join(tmpDir, "obj"),
			objects.FieldKeyKind:      "custom_kind",
			objects.FieldKeyStatus:    "closed",
			objects.FieldKeyCreatedAt: time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
		}
		_ = stor.Create(ctx, secCtx, obj)
	}

	h := NewRetentionToleranceHandler(stor, tmpDir).(*RetentionToleranceHandler)
	deleted, err := h.enforceMaxCount(ctx, secCtx, storageCtx, "j1", "custom_kind", 2, nil, 2, 2, 2)
	_ = deleted
	_ = err

	job := &ScheduledJob{
		ID: "retention-live",
		EnvironmentVariables: map[string]string{
			"BATCH_SIZE": "2",
			"MAX_BATCHES": "2",
		},
	}
	_ = h.Execute(ctx, job)
}

func TestExtended_FileLockMetricsAndHooks(t *testing.T) {
	ctx := context.Background()
	sp := storagepkg.NewNoopObjectStorage()
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	flm := NewFileLockMetricsCollectionHandler(sp)
	_ = flm.Execute(ctx, &ScheduledJob{ID: "flm-1"})

	h := NewRunWrapperHandlerWithProjectRoot(sp, logger, nil, nil, tmpDir).(*RunWrapperHandler)
	h.maybeRunConvergenceTickAfterTestBundleHealth(ctx, &ScheduledJob{ID: "SCH-run-tick"})

	res := &PhaseRouterResult{
		RoutingProfileID:        "std",
		MeasurementImpliedPhase: ConvergencePhase("measure"),
		SuggestedCurrentPhase:   ConvergencePhase("measure"),
		PhaseAlignment:          "aligned",
	}
	EmitConvergenceEvaluationEvent(ctx, tmpDir, "sess-1", "standard", res, map[string]any{"res": "ok"})

	adapter := &ConvergenceStorageAdapter{storage: sp}
	_, _ = adapter.ListConvergenceSessions(ctx)
	_ = adapter.UpdateSessionStatus(ctx, "sess-1", "active")
	_ = adapter.CreatePriorityPlanForTimeout(ctx, "sess-1")

	_ = GetRuntimeThreadCount()
	_ = isJobTypeIntentionalNoHandler("unknown_job_type")
	_ = jobTypeHandlerRegistryKeys()
	_ = handlerKeyRegistry()

	ath := NewAutoTransitionHandler(sp)
	_ = ath.Execute(ctx, &ScheduledJob{ID: "ath-1"})

	p := healthMissingStreakFilePath(tmpDir)
	if p == "" {
		t.Fatal("expected streak path")
	}
	resetHealthMissingStreakForJob(tmpDir, "j1")
	streak := bumpHealthMissingSkipStreak(tmpDir, "j1")
	if streak != 1 {
		t.Fatalf("expected streak 1, got %d", streak)
	}
	budget := parseHealthFileMissingSkipBudget(&ScheduledJob{
		EnvironmentVariables: map[string]string{
			EnvKeyHealthFileMissingSkipBudget: "5",
		},
	})
	if budget != 5 {
		t.Fatalf("expected budget 5, got %d", budget)
	}

	cand := retentionArchiveCandidate(ctx, secCtxForTesting(), sp, objects.KindPriorityPlan, map[string]any{"id": "c1", "status": objects.ObjectStatusComplete}, "archived")
	if !cand {
		t.Fatal("expected true for complete priority plan")
	}

	rth := NewRetentionToleranceHandler(sp, tmpDir).(*RetentionToleranceHandler)
	_ = rth.archiveViaPromoteMembrane(ctx, secCtxForTesting(), "j1", "seed-1")

	sched := &Scheduler{
		logger: logger,
	}
	_ = sched.TriggerImmediate(ctx, &ScheduledJob{ID: "imm-1"})
}

func secCtxForTesting() *pkgctx.SecurityContext {
	return pkgctx.NewSystemSecurityContext()
}

func TestExtended_CallbackListenerHTTPAdvanced(t *testing.T) {
	sp := storagepkg.NewNoopObjectStorage()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	notifCtx := NewNotificationContext(logger, nil)

	handler := NewCallbackListenerHandler(sp, logger, nil, nil, notifCtx).(*CallbackListenerHandler)

	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	_ = handler.authenticateRequest(nil, req)
	handler.shutdownServer()
}

func TestExtended_MeshLeaseEnsureSubprocess(t *testing.T) {
	ctx := context.Background()
	sp := storagepkg.NewNoopObjectStorage()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	tmpDir := t.TempDir()

	ml := NewMeshLeaseSupervisionHandler(sp, tmpDir, logger).(*MeshLeaseSupervisionHandler)
	_ = ml.ensureSubprocess(ctx, "lease-1", "custom-ref")
}

func TestExtended_HourglassWatcherAndCheckin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	sp := storagepkg.NewNoopObjectStorage()

	s := &Scheduler{
		projectRoot: tmpDir,
		logger:      logger,
		storage:     sp,
		jobs:        make(map[string]*ScheduledJob),
	}

	s.startHourglassWatcher(ctx, nil)
}
