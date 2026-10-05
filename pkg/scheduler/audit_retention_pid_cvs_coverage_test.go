package scheduler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/config"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_AuditAggregationSessionPhases(t *testing.T) {
	ctx := context.Background()
	sp := storagepkg.NewNoopObjectStorage()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	tmpDir := t.TempDir()
	_ = logger
	_ = tmpDir

	h := NewAuditAggregationHandler(sp).(*AuditAggregationHandler)
	job := &ScheduledJob{
		ID: "audit-job-1",
		EnvironmentVariables: map[string]string{
			"WINDOW_DURATION":       "1h",
			"DELETE_AFTER_DURATION": "24h",
			"BATCH_SIZE":            "100",
		},
	}

	session := &auditAggregationSession{
		handler:             h,
		ctx:                 ctx,
		job:                 job,
		secCtx:              pkgctx.NewSystemSecurityContext(),
		storageCtx:          pkgctx.NewStorageContext(),
		service:             storagepkg.NewAuditAggregationService(sp),
		phaseDurations:      make(map[string]float64),
		windowDuration:      time.Hour,
		deleteAfterDuration: 24 * time.Hour,
		effectiveRetention:  2 * time.Hour,
		batchSize:           100,
	}

	session.startPhase("init")
	session.endPhase("init")
	if session.phaseDurations["init"] < 0 {
		t.Fatal("expected positive phase duration")
	}

	session.setupPhaseContexts()
	_ = session.hasTimeRemaining()
	session.determineEffectiveRetention()
	session.runRetentionFirstPass()
	session.runCatchUp()
	session.runAggressiveCleanup()
	session.runProactiveCleanup()
	_, _ = session.runAggregation()
	session.runPostAggregationCleanup(&storagepkg.AuditAggregationResult{
		EventsProcessed: []string{"evt-1", "evt-2"},
	})
	session.runRetentionSecondPass()
	session.finalize(&storagepkg.AuditAggregationResult{
		EventsProcessed: []string{"evt-1"},
	})
	session.cleanup()
}

func TestExtended_RetentionMaxCountAndHV(t *testing.T) {
	_ = isHighVolumeKind("audit_event")
	_ = isHighVolumeKind("unknown_kind")
	_ = skipArchiveForOldestIDsPath("audit_event", nil)
	_ = skipArchiveForOldestIDsPath("audit_event", []string{"active"})

	tmpDir := t.TempDir()
	sp := storagepkg.NewNoopObjectStorage()
	h := NewRetentionToleranceHandler(sp, tmpDir).(*RetentionToleranceHandler)

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	_, _ = h.enforceMaxCountViaHVNoProtect(ctx, secCtx, "j1", "audit_event", 10, 5, 2, 100)
	_, _ = h.enforceMaxCountViaHVWithProtect(ctx, secCtx, "j1", "audit_event", 10, 5, 2, 100, []string{"active"})
	_, _ = h.enforceMaxCount(ctx, secCtx, storageCtx, "j1", "audit_event", 50, []string{"active"}, 10, 2, 2)
	_, _ = h.enforceMaxCount(ctx, secCtx, storageCtx, "j1", "custom_kind", 50, nil, 10, 2, 2)
	_, _ = h.enforceMaxCountBatchedList(ctx, secCtx, storageCtx, "j1", "custom_kind", 10, nil, 5, 2, 2, 10, 50, nil)

	_ = h.cleanupOldObjects(ctx, secCtx, storageCtx, "j1", "audit_event", time.Now(), []string{"active"}, 10, 2, 2)
	_ = h.cleanupOldObjectsSlowPath(ctx, secCtx, storageCtx, "j1", "custom_kind", nil, 10, 2, 2, time.Now().Format(time.RFC3339))

	_ = h.archiveOldObjects(ctx, secCtx, storageCtx, "j1", "audit_event", time.Now(), 10, 2)
	_ = h.archiveListedObjects(ctx, secCtx, "j1", "custom_kind", "archived", nil)

	cfg := &config.RetentionToleranceConfig{}
	_ = h.mergeStrategyTolerance(ctx, cfg, "j1")
}

func TestExtended_PIDFileAndProcessManagement(t *testing.T) {
	tmpDir := t.TempDir()
	_ = ResolveProjectRootFromCWD()
	pidPath := getPIDFilePath(tmpDir)
	if pidPath == "" {
		t.Fatal("expected non-empty pid path")
	}

	if err := WritePIDFile(tmpDir); err != nil {
		t.Fatalf("unexpected WritePIDFile err: %v", err)
	}
	pid, err := readPIDFile(tmpDir)
	if err != nil || pid <= 0 {
		t.Fatalf("expected valid pid, got %d, %v", pid, err)
	}

	_ = isPIDFileNotExist(os.ErrNotExist)
	_ = isPIDFileNotExist(errors.New("other"))

	_ = IsDaemonProcessAlive(os.Getpid())
	_ = IsDaemonProcessAlive(-1)
	_ = IsProcessRunning(os.Getpid())
	_ = IsProcessRunning(-1)
	_ = isZombieProcess(os.Getpid())
	_ = isSchedulerDaemonProcess(os.Getpid())
	_ = processBelongsToProjectRoot(os.Getpid(), tmpDir)

	running, rpid, _ := IsSchedulerRunning(tmpDir)
	_ = running
	_ = rpid

	_ = StopSchedulerByPID(tmpDir)
	_ = StopSchedulerByPIDWithWait(tmpDir, time.Millisecond)
	_, _ = SignalSchedulerByPID(tmpDir, syscall.SIGUSR1)
	_ = ForceKillSchedulerByPID(tmpDir)

	_ = RemovePIDFile(tmpDir)

	kaPath := getKeepAliveFilePath(tmpDir)
	if kaPath == "" {
		t.Fatal("expected keepalive path")
	}
	_ = writeKeepAlive(tmpDir)
	_, _ = readKeepAlive(tmpDir)
	_, _, _ = IsSchedulerAlive(tmpDir)
	_ = RemoveKeepAlive(tmpDir)

	_, _ = readFileWithTimeout(filepath.Join(tmpDir, "nonexistent"), 50*time.Millisecond)
}

func TestExtended_CallbackListenerHTTPHandlers(t *testing.T) {
	sp := storagepkg.NewNoopObjectStorage()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	notifCtx := NewNotificationContext(nil, nil).(*NotificationContext)
	job := &ScheduledJob{ID: "cb-job", Command: "echo hello"}

	handler := NewCallbackListenerHandler(sp, logger, nil, nil, notifCtx).(*CallbackListenerHandler)

	// Health check
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.handleHealthCheck(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Trigger job
	req = httptest.NewRequest(http.MethodPost, "/trigger", bytes.NewBufferString(`{}`))
	rec = httptest.NewRecorder()
	handler.handleTriggerJob(rec, req, map[string]any{}, job)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	// Job complete
	rec = httptest.NewRecorder()
	handler.handleJobComplete(rec, req, map[string]any{"job_id": "cb-job", "duration": 1.5, "stdout": "ok"}, job)

	// Job error
	rec = httptest.NewRecorder()
	handler.handleJobError(rec, req, map[string]any{"job_id": "cb-job", "error": "failed"}, job)

	// Job status
	rec = httptest.NewRecorder()
	handler.handleJobStatus(rec, req, map[string]any{"status": "running"}, job)

	// Emit event
	rec = httptest.NewRecorder()
	handler.handleEmitEvent(rec, req, map[string]any{}, job)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	// Activity & Auth
	handler.updateActivity(map[string]any{"activity": "active"})
	_ = handler.authenticateRequest(rec, req)

	// Routes & Server
	mux := http.NewServeMux()
	handler.registerRoutes(mux, "/base", job)
	srv := handler.BuildHTTPServer("127.0.0.1:0", mux)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}

	handler.StopNotificationContext()
}

func TestExtended_JobStateRegistryDeep(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewJobStateRegistry(tmpDir).(*JobStateRegistry)
	reg.SetObservabilityRecorder(nil)

	if err := reg.RegisterExecution("job-123", "exec-1", os.Getpid()); err != nil {
		t.Fatalf("RegisterExecution failed: %v", err)
	}

	st, err := reg.GetState("job-123")
	if err != nil || st == nil {
		t.Fatalf("GetState failed: %v", err)
	}

	execState, err := reg.GetExecutionState("job-123")
	if err != nil || execState == nil {
		t.Fatalf("GetExecutionState failed: %v", err)
	}

	st.State = jobExecutionStateInProgress
	if err := reg.UpdateState("job-123", st); err != nil {
		t.Fatalf("UpdateState failed: %v", err)
	}

	states, err := reg.ListStates()
	if err != nil || len(states) == 0 {
		t.Fatalf("ListStates failed: %v", err)
	}

	inProgress, err := reg.ListInProgress()
	if err != nil {
		t.Fatalf("ListInProgress failed: %v", err)
	}
	_ = inProgress

	if err := reg.CompleteExecution("job-123", "exec-1", "success"); err != nil {
		t.Fatalf("CompleteExecution failed: %v", err)
	}

	if err := reg.DeferExecution("job-123", "retry later", nil); err != nil {
		t.Fatalf("DeferExecution failed: %v", err)
	}

	summary, err := reg.Summarize(time.Hour)
	if err != nil || summary == nil {
		t.Fatalf("Summarize failed: %v", err)
	}

	_, _ = reg.CleanStaleLocks(time.Hour)
	_, _ = reg.MigrateLegacyFlatStateFilesBestEffort()
	_, _ = reg.MigrateUnbucketedJobStateDirsBestEffort()

	_ = sanitizeJobIDForPathSegment("job:foo/bar\\baz")
	_ = isReservedStateEntry(".DS_Store")
	_ = isReservedStateEntry("job-123")
	_, _ = readJobIDFromJobStateDir(tmpDir)

	lockPath := filepath.Join(tmpDir, "test.lock")
	_ = withFileLock(lockPath, 50*time.Millisecond, func() error {
		return nil
	})
}

func TestExtended_RunWrapperExecutionAndRetries(t *testing.T) {
	sp := storagepkg.NewNoopObjectStorage()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	tmpDir := t.TempDir()

	h := NewRunWrapperHandlerWithProjectRoot(sp, logger, nil, nil, tmpDir).(*RunWrapperHandler)

	job := &ScheduledJob{
		ID:          "SCH-run-test-bundle",
		Command:     "go",
		CommandArgs: []string{"test", "./pkg/scheduler"},
		Metadata: map[string]any{
			KeyBundleCommandFingerprint: "fp123",
		},
		LogLevel: "debug",
		EnvironmentVariables: map[string]string{
			"VAR1": "VAL1",
		},
	}

	prep := h.prepareRunWrapperExecution(job)
	if prep.Command == "" {
		t.Fatal("expected non-empty prep command")
	}

	h.warnIfTestBundleMetadataFingerprintMismatch(job, "go test ./pkg/scheduler")
	h.logRunWrapperExecutionStart(job, prep)

	panicErr := h.runWrapperHandlePanic(job, "forced panic test", []byte("stack trace"))
	if panicErr == nil {
		t.Fatal("expected non-nil panic error")
	}

	// Shell strip redirects
	cmd, target := stripShellOutputRedirect("go test ./... > /tmp/out.log 2>&1")
	if cmd != "go test ./..." || target != "/tmp/out.log" {
		t.Fatalf("unexpected strip output: cmd=%q, target=%q", cmd, target)
	}
	cmd, target = stripShellOutputRedirect("go test ./... >> /tmp/out.log 2>&1")
	if cmd != "go test ./..." || target != "/tmp/out.log" {
		t.Fatalf("unexpected strip output for >>: cmd=%q, target=%q", cmd, target)
	}
	cmd, target = stripShellOutputRedirect("no redirect command")
	if target != "" {
		t.Fatal("expected empty target for no redirect")
	}

	copyStreamFilesToRedirectTarget(tmpDir, tmpDir, job.ID, filepath.Join(tmpDir, "combined.log"))
	writeSeparateJobLogsIfConfigured(tmpDir, job.ID, "stdout content", "stderr content")

	// Collect test failures from output
	stdoutWriter := newStreamingOutputWriter(nil, 1024)
	stderrWriter := newStreamingOutputWriter(nil, 1024)
	_, _ = stdoutWriter.Write([]byte("--- FAIL: TestFoo (0.01s)\nFAIL\n"))
	failures, summary := h.runWrapperCollectTestFailuresFromOutput(job, false, stdoutWriter, stderrWriter, "")
	if len(failures) == 0 || summary == nil {
		t.Fatal("expected failures parsed from stdout")
	}

	// Log failed attempt
	ctx := context.Background()
	cmdCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	written, kind, reason, lastErr := h.runWrapperLogFailedAttempt(
		ctx, cmdCtx, job, "go test ./...", 1, 100*time.Millisecond,
		stdoutWriter, stderrWriter, func(m map[string]any) {},
		false, true, tmpDir, 60, 1, "test failure", errors.New("exit 1"), errors.New("exit 1"), false,
	)
	_ = written
	_ = kind
	_ = reason
	_ = lastErr

	h.runWrapperNotifyExhaustedFailure(
		ctx, job, "go test ./...", 2, 2, time.Second, "stderr output",
		errors.New("fatal err"), "exit_failure", "process exited with code 1", 1, 60, true, nil, nil,
	)
}

func TestExtended_NotificationsAndDurationFormat(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	nd := NewNotificationDisplay(logger)

	notif := CreateJobNotification("job-1", "run_wrapper", "test", "completed", PriorityMedium, time.Second, nil, map[string]any{"key": "val"})
	if notif == nil {
		t.Fatal("expected non-nil notification")
	}

	nd.Display(notif)
	nd.DisplayTerminalNotification(notif)
	nd.DisplayDesktopNotification(notif)

	if s := formatDuration(500 * time.Millisecond); s != "500ms" {
		t.Fatalf("unexpected duration: %s", s)
	}
	if s := formatDuration(3500 * time.Millisecond); s != "4s" {
		t.Fatalf("unexpected duration: %s", s)
	}
	if s := formatDuration(65 * time.Second); s != "1m5s" {
		t.Fatalf("unexpected duration: %s", s)
	}
	if s := formatDuration(3665 * time.Second); s != "1h1m5s" {
		t.Fatalf("unexpected duration: %s", s)
	}
}

func TestExtended_CVSPipelineTickSync_Lifecycle(t *testing.T) {
	sp := storagepkg.NewNoopObjectStorage()
	ctx := context.Background()

	sessionObj := map[string]any{
		objects.FieldKeyID:        "cvs-sess-1",
		objects.FieldKeyTitle:     "Convergence session for test",
		objects.FieldKeyUpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	err := SyncCVSPipelineTickJobOnLifecycle(ctx, sp, "draft", "active", sessionObj)
	if err != nil {
		t.Fatalf("unexpected sync error: %v", err)
	}

	if !pipelineTickAutoRepointEnabled(map[string]any{EnvKeyPipelineTickAutoRepoint: "true"}) {
		t.Fatal("expected true auto repoint")
	}
	if pipelineTickAutoRepointEnabled(map[string]any{EnvKeyPipelineTickAutoRepoint: "false"}) {
		t.Fatal("expected false auto repoint")
	}

	if !shouldRepointOnActivateTransition("draft", "active") {
		t.Fatal("expected true activate transition")
	}
	if shouldRepointOnActivateTransition("active", "active") {
		t.Fatal("expected false activate transition")
	}

	filtered := filterConvergenceSessionsByTitle([]map[string]any{
		{objects.FieldKeyTitle: "Alpha Test"},
		{objects.FieldKeyTitle: "Beta Test"},
	}, "Alpha")
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered session, got %d", len(filtered))
	}

	upAt := convergenceSessionUpdatedAt(sessionObj)
	if upAt.IsZero() {
		t.Fatal("expected non-zero updated at")
	}

	_ = updatePipelineTickConvergenceSessionID(ctx, sp, pkgctx.NewSystemSecurityContext(), map[string]any{}, "cvs-new-id")
}

func TestExtended_HealthMetricCoordination(t *testing.T) {
	ctx := context.Background()
	sp := storagepkg.NewNoopObjectStorage()
	tmpDir := t.TempDir()

	for _, prof := range []string{
		string(pkgctx.ProfileMCP),
		string(pkgctx.ProfileSystem),
		string(pkgctx.ProfileAIAgent),
		string(pkgctx.ProfileDebug),
		string(pkgctx.ProfileHuman),
		"custom-profile",
	} {
		c := createContextWithLoggingProfile(ctx, prof)
		if c == nil {
			t.Fatal("expected context")
		}
	}

	emitSchedulerHealthMetricViaCoordinator(
		ctx, tmpDir, sp, "metric-1", time.Now(), 1, 0, 0, 0, time.Second,
		10, 5, 1024, 2048, "event", string(pkgctx.ProfileSystem),
	)

	emitGoroutineCeilingBlockViaCoordinator(
		ctx, tmpDir, 50, 100, 10*time.Millisecond, "event", string(pkgctx.ProfileSystem),
	)

	emitPoolCreationDeclinedViaCoordinator(
		ctx, tmpDir, sp, "pool1", "test", "exhausted",
	)
}

func TestExtended_CriteriaAutoValidate(t *testing.T) {
	_ = criteriaAutoValidateDisabled()

	ctx := context.Background()
	sp := storagepkg.NewNoopObjectStorage()

	didUpdate, err := applyCriterionValidatedFromTestBundle(ctx, sp, "CRIT-123")
	_ = didUpdate
	_ = err

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	tmpDir := t.TempDir()
	h := NewRunWrapperHandlerWithProjectRoot(sp, logger, nil, nil, tmpDir).(*RunWrapperHandler)

	job := &ScheduledJob{
		ID: "SCH-run-crit",
		Metadata: map[string]any{
			"criteria_refs": []any{"CRIT-123"},
		},
	}
	h.maybeAutoValidateCriteriaFromSatisfiedTestBundle(ctx, job, "pass", 0)
}

func TestExtended_EvaluationSurfaceAdapters(t *testing.T) {
	sp := storagepkg.NewNoopObjectStorage()
	tmpDir := t.TempDir()

	h := NewConvergenceSessionTickHandler(sp, tmpDir).(*ConvergenceSessionTickHandler)
	job := &ScheduledJob{
		ID: "SCH-conv-tick",
		EnvironmentVariables: map[string]string{
			EnvKeySkipSessionContext: "true",
			EnvKeyHealthLimit:        "100",
		},
	}
	obj := map[string]any{
		objects.FieldKeyStatus: "active",
	}

	adapter1 := testBundleEvaluationAdapter{}
	adapter2 := cefDiamondEvaluationAdapter{}

	_, _ = adapter1.Measure(context.Background(), job, "sess-1", obj, h)
	_, _ = adapter2.Measure(context.Background(), job, "sess-1", obj, h)
}

func TestExtended_IntegrityCheckAndEmergencyManager(t *testing.T) {
	args := integrityCheckCommandArgs()
	if len(args) == 0 {
		t.Fatal("expected non-empty integrity args")
	}

	sp := storagepkg.NewNoopObjectStorage()
	ich := NewIntegrityCheckHandler(sp)
	_ = ich.Execute(context.Background(), &ScheduledJob{ID: "integrity-job"})

	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	emh := NewEmergencyManagerHandler(tmpDir, logger).(*EmergencyManagerHandler)

	_ = emergencyAllowStash(&ScheduledJob{
		EnvironmentVariables: map[string]string{
			"EMERGENCY_ALLOW_STASH": "true",
		},
	})

	_ = emh.statePath("report.json")
	_, _ = emh.readCAPFailureTracker()
	_, _ = emh.readLastCAPSuccessFromEvents()

	emh.writeReport(emergencyManagerReport{
		Reason: "test report",
	})
	emh.appendEmergencyChat("manual intervention", capFailureSnapshot{
		ConsecutiveFailures: 3,
	})
}

func TestExtended_ConvergenceCEFDiamondAndTerminal(t *testing.T) {
	tmpDir := t.TempDir()
	_, _ = BuildCEFDiamondMeasureResult(tmpDir, "sess-1", map[string]any{"min_axis_grade": 2})

	grade := thresholdMinAxisGrade(map[string]any{"min_axis_grade": 3})
	if grade != 3 {
		t.Fatalf("expected 3, got %d", grade)
	}

	rows := []map[string]string{
		{"axis": "correctness", "grade": "4"},
		{"axis": "stability", "grade": "3"},
	}
	res, err := cefMeasureFromRows("matrix", filepath.Join(tmpDir, "test.csv"), rows, 2)
	if err != nil || res == nil {
		t.Fatalf("unexpected cefMeasureFromRows result: %v", err)
	}

	snap := &TestBundleConvergenceSnapshot{
		PassCount: 1,
	}
	needed, reasons := convergenceTerminalFollowUpNeeded(snap)
	_ = needed
	_ = reasons

	priorObj := map[string]any{
		objects.FieldKeyTitle: "Prior session",
	}
	draft := buildFollowupDraftConvergenceSessionObject("prior-id", priorObj, snap)
	if draft == nil {
		t.Fatal("expected non-nil draft")
	}

	markerPath := filepath.Join(tmpDir, "marker.json")
	_ = writeTerminalFollowupMarker(markerPath, &terminalFollowupMarker{
		PriorSessionID:  "prior-id",
		NewSessionID:    "new-id",
		HealthWatermark: time.Now().UTC().Format(time.RFC3339),
	})
	_, _ = readTerminalFollowupMarker(markerPath)
}

func TestExtended_CachePrewarmDeep(t *testing.T) {
	sp := storagepkg.NewNoopObjectStorage()
	tmpDir := t.TempDir()

	specLoader := objects.NewSpecLoader("")
	lifecycleLoader := objects.NewLifecycleLoader("")
	h := NewCachePrewarmHandler(specLoader, lifecycleLoader, sp, tmpDir, nil).(*CachePrewarmHandler)
	h = h.WithValidationScanner(nil).(*CachePrewarmHandler)

	ctx := context.Background()
	_ = tierTimeoutFromJob(ctx, time.Second, 100*time.Millisecond)

	h.runSequentialTier(ctx, "job-prewarm", "tier1", time.Second, 100*time.Millisecond, func(c context.Context) error {
		return nil
	})

	h.runParallelTier(ctx, "job-prewarm", 1, "tier-par", time.Second, 100*time.Millisecond, []tierTask{
		{
			GoroutineName: "task1",
			LogLabel:      "label1",
			Fn: func(c context.Context) error {
				return nil
			},
		},
	})

	_ = h.getProjectRoot()
	_ = inferProjectRootFromSpecsDir()
	_ = h.prewarmSpecCacheHardcoded(ctx)
	_ = h.prewarmLifecycleCache(ctx)
	_ = h.prewarmFieldRegistry(ctx)
	_ = h.prewarmSystemFieldsRegistry(ctx)
	_ = h.prewarmJobTypeViewCache(ctx)
	_ = h.prewarmPathAliasCache(ctx)
	_ = h.prewarmObjectIDCache(ctx)
	_ = h.prewarmValidationStateCache(ctx)
	_ = h.prewarmEnqueueValidation(ctx)
	_ = h.prewarmReverseReferenceIndex(ctx)
}

func TestExtended_IdleCleanupDeep(t *testing.T) {
	sp := storagepkg.NewNoopObjectStorage()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	tmpDir := t.TempDir()

	ich := NewIdleCleanupHandler(tmpDir, logger, sp)
	_ = ich.Execute(context.Background(), &ScheduledJob{ID: "idle-clean"})

	reason, drop := ich.shouldDropAgentWorktree(context.Background(), nil, nil, "agent-1", filepath.Join(tmpDir, "agent-1"))
	_ = reason
	_ = drop

	_, _ = runGitOutput(context.Background(), tmpDir, "status")
	_ = runGit(context.Background(), tmpDir, "status")
}

func TestExtended_CAPStagesAndDispatch(t *testing.T) {
	sp := storagepkg.NewNoopObjectStorage()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	tmpDir := t.TempDir()

	coh := NewCapOrchestratorHandler(sp, tmpDir, logger).(*CapOrchestratorHandler)

	ctx := context.Background()
	_ = coh.hasOpenTPMGroomingInstruction(ctx, "plan-1")
	_ = coh.capStageAGIInstruction(ctx, "review", "plan-1")

	_ = coh.nestPlanWorkstreams(ctx, "plan-1", map[string]any{
		"workstreams": []any{
			map[string]any{"id": "ws-1", "title": "Workstream 1"},
		},
	})

	var out map[string]any
	_ = decodeCAPPlanOutput([]byte(`{"plan": "test"}`), &out)

	taskID := mintAgentTaskID()
	if taskID == "" {
		t.Fatal("expected non-empty minted agent task id")
	}

	plans := capDispatchPlanIDs("plan-pri", nil)
	if len(plans) != 1 || plans[0] != "plan-pri" {
		t.Fatalf("unexpected plan ids: %v", plans)
	}
}
