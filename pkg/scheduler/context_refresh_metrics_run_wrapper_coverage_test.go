package scheduler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

func TestExtended_ContextRefresh(t *testing.T) {
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

	// 1. Cadence and ISO duration parsing
	d1, err1 := parseCadence("1h")
	if err1 != nil || d1 != 1*time.Hour {
		t.Errorf("parseCadence 1h failed: %v, %v", err1, d1)
	}
	d2, err2 := parseCadence("PT30M")
	if err2 != nil || d2 != 30*time.Minute {
		t.Errorf("parseCadence PT30M failed: %v, %v", err2, d2)
	}
	d3, err3 := parseCadence("P1D")
	if err3 != nil || d3 != 24*time.Hour {
		t.Errorf("parseCadence P1D failed: %v, %v", err3, d3)
	}
	_, err4 := parseCadence("invalid-duration")
	if err4 == nil {
		t.Errorf("expected error for invalid cadence")
	}

	_, _ = parseISODuration("PT10S")
	_, _ = parseISODuration("PT2H30M")
	_, _ = parseISODuration("P1W")
	_, _ = parseISODuration("P1M")
	_, _ = parseISODuration("P1Y")
	_, _ = parseISODuration("INVALID")

	// 2. Handler execution
	h := NewContextRefreshHandler(sp, tmpDir).(*ContextRefreshHandler)

	schedObj := map[string]any{
		objects.FieldKeyID:            "CRS-test-1",
		objects.FieldKeyKind:          objects.KindContextRefreshSchedule,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "active",
		"cadence":                     "1h",
		"target":                      "all",
	}
	_ = sp.Create(ctx, secCtx, schedObj)

	job := &ScheduledJob{
		ID:       "SCH-ctx-refresh",
		JobType:  JobTypeContextRefresh,
		Category: CategoryMaintenance,
	}
	_ = h.Execute(ctx, job)
	_ = h.executeRefresh(ctx, "CRS-test-1", "all")
	_ = h.updateSchedule(ctx, secCtx, "CRS-test-1", map[string]any{"status": "completed"})
}

func TestExtended_MetricsCleanupHandlers_Lifecycle(t *testing.T) {
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

	// 1. ChangeJournalAggregationHandler
	cjHandler := NewChangeJournalAggregationHandler(sp)
	jobCJ := &ScheduledJob{
		ID:       "SCH-cj-test",
		JobType:  JobTypeChangeJournalAggregation,
		Category: CategoryMaintenance,
	}
	_ = cjHandler.Execute(ctx, jobCJ)

	// 2. AggregationMetricsCleanupHandler
	amHandler := NewAggregationMetricsCleanupHandler(sp)
	jobAM := &ScheduledJob{
		ID:       "SCH-am-test",
		JobType:  JobTypeCleanup,
		Category: CategoryMaintenance,
		EnvironmentVariables: map[string]string{
			"METRIC_KINDS": objects.KindAuditAggregationMetric,
		},
	}
	_ = amHandler.Execute(ctx, jobAM)

	// 3. GenericMetricsCleanupHandler
	gmHandler := NewGenericMetricsCleanupHandler(sp)
	jobGM := &ScheduledJob{
		ID:       "SCH-gm-test",
		JobType:  JobTypeCleanup,
		Category: CategoryMaintenance,
		EnvironmentVariables: map[string]string{
			"METRIC_KINDS": objects.KindAuditAggregationMetric,
		},
	}
	_ = gmHandler.Execute(ctx, jobGM)

	// 4. preExecutionHealthCheck
	_ = preExecutionHealthCheck(ctx, sp, logger, jobAM, objects.KindAuditAggregationMetric)

	// Create a metric object to exercise deletion
	metObj := map[string]any{
		objects.FieldKeyID:            "AAM-test-del",
		objects.FieldKeyKind:          objects.KindAuditAggregationMetric,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "active",
		objects.FieldKeyCreatedAt:     time.Now().Add(-100 * time.Hour).Format(time.RFC3339),
	}
	_ = sp.Create(ctx, secCtx, metObj)
	_ = amHandler.Execute(ctx, jobAM)
	_ = gmHandler.Execute(ctx, jobGM)
}

func TestExtended_RunWrapperCallbacks_ContextRefresh(t *testing.T) {
	ctx := context.Background()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// 1. Webhook server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	payload := map[string]any{
		"job_id": "SCH-test-cb",
		"status": "success",
	}

	executeWebhookWithLogger(ctx, logger, server.URL, payload)
	executeCommandWithLogger(ctx, logger, "true", payload)
	executeCallbackDirectWithLogger(ctx, logger, "webhook", server.URL, payload)
	executeCallbackDirectWithLogger(ctx, logger, "command", "true", payload)

	job := &ScheduledJob{
		ID:                   "SCH-cb-job",
		JobType:              JobTypeRunWrapper,
		CallbackOnCompletion: server.URL,
		CallbackOnError:      server.URL,
	}
	InvokeJobCallback(ctx, logger, nil, job, "on_completion", payload)
	InvokeJobCallback(ctx, logger, nil, job, "on_error", payload)
}

func TestExtended_CapDispatchAndJobExecution(t *testing.T) {
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
	sched := NewScheduler(sp, objects.GetGlobalSpecLoader(), objects.GetGlobalLifecycleLoader()).(*Scheduler)

	// 1. CAP dispatch helpers
	var decoded map[string]any
	_ = decodeCAPPlanOutput([]byte(`{"key":"value"}`), &decoded)
	_ = decodeCAPPlanOutput([]byte(`invalid json`), &decoded)

	taskID := mintAgentTaskID()
	if len(taskID) == 0 {
		t.Errorf("expected non-empty minted task ID")
	}

	plans := []whatsnext.WhatsNextPriorityPlan{
		{ID: "PRI-plan-1"},
		{ID: "PRI-plan-2"},
	}
	planIDs := capDispatchPlanIDs("PRI-plan-lead", plans)
	if len(planIDs) != 3 || planIDs[0] != "PRI-plan-lead" {
		t.Errorf("unexpected capDispatchPlanIDs: %v", planIDs)
	}
	_ = capDispatchPlanIDs("", plans)

	capHandler := NewCapOrchestratorHandler(sp, tmpDir, logger).(*CapOrchestratorHandler)
	_ = capHandler.resumePendingVerificationTasks(ctx, "echo", "PRI-plan-1")
	_ = capHandler.nestPlanWorkstreams(ctx, "PRI-plan-1", map[string]any{"id": "PRI-plan-1"})

	// 2. Job execution storage & log helpers
	job := &ScheduledJob{
		ID:          "SCH-exec-helpers",
		JobType:     JobTypeCachePrewarm,
		Category:    CategoryMaintenance,
		TriggerType: "timer",
		Enabled:     true,
	}

	jobObj := map[string]any{
		objects.FieldKeyID:            job.ID,
		objects.FieldKeyKind:          objects.KindSchedulerJob,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "active",
		objects.FieldKeyJobType:       job.JobType,
	}
	_ = sp.Create(ctx, secCtx, jobObj)

	sched.jobsMu.Lock()
	sched.jobs[job.ID] = job
	sched.jobsMu.Unlock()

	sched.updateJobInStorage(ctx, job)
	sched.DisableJobInStorage(ctx, job)
	sched.disableJobInStorage(ctx, job)

	_ = shouldInvokeConfiguredJobCallback(ctx, job)
	dispCtx, cancel := dispatchContextForScheduledJob(job)
	cancel()
	_ = dispCtx

	WriteJobOutcome(tmpDir, job.ID, job.JobType, map[string]any{"outcome": "success"})
	WriteJobProgress(tmpDir, job.ID, map[string]any{"progress": "50%"})
	writeJobLogEntry(tmpDir, job.ID, map[string]any{"log": "sample entry"})

	logPath := filepath.Join(tmpDir, ".zqk", "logs", "scheduler", job.ID+".jsonl")
	_ = fileutil.WriteFile(logPath, []byte("line1\nline2\nline3\n"), 0600)
	trimJobLogFileIfNeeded(logPath, 2)
	trimJobLogFileIfNeeded(logPath, 100)
}
