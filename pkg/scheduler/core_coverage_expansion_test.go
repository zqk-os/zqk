package scheduler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/agentclaim"
	"github.com/zqk-os/zqk/pkg/cleanup"
	"github.com/zqk-os/zqk/pkg/config"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/pipeline"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestDirectExecution_NegativeAndEdgeCases(t *testing.T) {
	t.Parallel()

	// 1. Scheduler with nil storage provider fails fail-closed
	s := &Scheduler{
		storage: nil,
	}
	job := &ScheduledJob{
		ID: "SCH-DIRECT-FAIL",
	}
	err := s.TriggerImmediate(context.Background(), job)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "storage provider not available")
}

func TestJobAdmission_OneTimeImmediatePredicates_Comprehensive(t *testing.T) {
	t.Parallel()

	// nil job
	assert.False(t, oneTimeImmediateShouldStartOnReload(nil, false))
	assert.False(t, jobWarrantsAdmissionHourglass(nil))

	// pending job
	job := &ScheduledJob{
		ID:            "SCH-TEST-1",
		Enabled:       true,
		Status:        StatusActive,
		ExecutionMode: ExecutionModeOneTime,
		TriggerType:   TriggerTypeImmediate,
	}
	assert.False(t, oneTimeImmediateShouldStartOnReload(job, true))

	// already ran (LastRunAt != nil)
	now := time.Now()
	jobRan := &ScheduledJob{
		ID:            "SCH-TEST-RAN",
		Enabled:       true,
		Status:        StatusActive,
		ExecutionMode: ExecutionModeOneTime,
		TriggerType:   TriggerTypeImmediate,
		LastRunAt:     &now,
	}
	assert.False(t, oneTimeImmediateShouldStartOnReload(jobRan, false))
	assert.False(t, jobWarrantsAdmissionHourglass(jobRan))

	// disabled / archived
	jobDisabled := &ScheduledJob{
		ID:            "SCH-TEST-DISABLED",
		Enabled:       false,
		Status:        StatusDisabled,
		ExecutionMode: ExecutionModeOneTime,
		TriggerType:   TriggerTypeImmediate,
	}
	assert.False(t, oneTimeImmediateShouldStartOnReload(jobDisabled, false))
	assert.False(t, jobWarrantsAdmissionHourglass(jobDisabled))

	jobArchived := &ScheduledJob{
		ID:            "SCH-TEST-ARCHIVED",
		Enabled:       true,
		Status:        objects.ObjectStatusArchived,
		ExecutionMode: ExecutionModeOneTime,
		TriggerType:   TriggerTypeImmediate,
	}
	assert.False(t, oneTimeImmediateShouldStartOnReload(jobArchived, false))
	assert.False(t, jobWarrantsAdmissionHourglass(jobArchived))

	// recurring mode (not one time)
	jobRecurring := &ScheduledJob{
		ID:            "SCH-TEST-REC",
		Enabled:       true,
		Status:        StatusActive,
		ExecutionMode: ExecutionModeReusable,
		TriggerType:   TriggerTypeImmediate,
	}
	assert.False(t, oneTimeImmediateShouldStartOnReload(jobRecurring, false))
	assert.False(t, jobWarrantsAdmissionHourglass(jobRecurring))

	// trigger type timer (not immediate)
	jobTimer := &ScheduledJob{
		ID:            "SCH-TEST-TIMER",
		Enabled:       true,
		Status:        StatusActive,
		ExecutionMode: ExecutionModeOneTime,
		TriggerType:   TriggerTypeTimer,
	}
	assert.False(t, oneTimeImmediateShouldStartOnReload(jobTimer, false))
	assert.False(t, jobWarrantsAdmissionHourglass(jobTimer))

	// testing bundle / category testing
	jobTestBundle := &ScheduledJob{
		ID:            "SCH-run-test-bundle-123",
		Enabled:       true,
		Status:        StatusActive,
		ExecutionMode: ExecutionModeOneTime,
		TriggerType:   TriggerTypeImmediate,
	}
	assert.False(t, oneTimeImmediateShouldStartOnReload(jobTestBundle, false))
	assert.False(t, jobWarrantsAdmissionHourglass(jobTestBundle))

	jobTestCat := &ScheduledJob{
		ID:            "SCH-NORMAL-ID",
		Category:      CategoryTesting,
		Enabled:       true,
		Status:        StatusActive,
		ExecutionMode: ExecutionModeOneTime,
		TriggerType:   TriggerTypeImmediate,
	}
	assert.False(t, oneTimeImmediateShouldStartOnReload(jobTestCat, false))
	assert.False(t, jobWarrantsAdmissionHourglass(jobTestCat))

	// callbacks set
	jobWithCallback := &ScheduledJob{
		ID:              "SCH-CALLBACK-JOB",
		Enabled:         true,
		Status:          StatusActive,
		ExecutionMode:   ExecutionModeOneTime,
		TriggerType:     TriggerTypeImmediate,
		CallbackOnError: "callback:err",
	}
	assert.True(t, oneTimeImmediateShouldStartOnReload(jobWithCallback, false))
	assert.True(t, jobWarrantsAdmissionHourglass(jobWithCallback))

	// manual / user category
	jobManual := &ScheduledJob{
		ID:            "SCH-MANUAL",
		Category:      CategoryManual,
		Enabled:       true,
		Status:        StatusActive,
		ExecutionMode: ExecutionModeOneTime,
		TriggerType:   TriggerTypeImmediate,
	}
	assert.True(t, oneTimeImmediateShouldStartOnReload(jobManual, false))

	// priority high / critical
	jobHigh := &ScheduledJob{
		ID:            "SCH-HIGH",
		Priority:      JobPriorityHigh,
		Enabled:       true,
		Status:        StatusActive,
		ExecutionMode: ExecutionModeOneTime,
		TriggerType:   TriggerTypeImmediate,
	}
	assert.True(t, oneTimeImmediateShouldStartOnReload(jobHigh, false))

	jobCrit := &ScheduledJob{
		ID:            "SCH-CRIT",
		Priority:      JobPriorityCritical,
		Enabled:       true,
		Status:        StatusActive,
		ExecutionMode: ExecutionModeOneTime,
		TriggerType:   TriggerTypeImmediate,
	}
	assert.True(t, oneTimeImmediateShouldStartOnReload(jobCrit, false))

	// normal priority with no callbacks -> false
	jobLow := &ScheduledJob{
		ID:            "SCH-LOW",
		Priority:      JobPriorityNormal,
		Enabled:       true,
		Status:        StatusActive,
		ExecutionMode: ExecutionModeOneTime,
		TriggerType:   TriggerTypeImmediate,
	}
	assert.False(t, oneTimeImmediateShouldStartOnReload(jobLow, false))
}

func TestJobAdmission_PendingDispatchTracking(t *testing.T) {
	t.Parallel()

	var nilScheduler *Scheduler
	nilScheduler.markOneTimeImmediatePending(nil)
	assert.False(t, nilScheduler.isImmediateDispatchPending("ANY"))
	nilScheduler.clearImmediateDispatchPending("ANY")
	assert.False(t, nilScheduler.markAdmissionFailed("ANY"))

	s := &Scheduler{}

	// Empty ID
	assert.False(t, s.isImmediateDispatchPending(""))
	s.clearImmediateDispatchPending("")
	assert.False(t, s.markAdmissionFailed(""))

	// Non-immediate job ignores mark
	recurringJob := &ScheduledJob{
		ID:            "SCH-REC-1",
		ExecutionMode: ExecutionModeReusable,
	}
	s.markOneTimeImmediatePending(recurringJob)
	assert.False(t, s.isImmediateDispatchPending("SCH-REC-1"))

	// Valid immediate job marks pending
	immJob := &ScheduledJob{
		ID:            "SCH-IMM-1",
		ExecutionMode: ExecutionModeOneTime,
		TriggerType:   TriggerTypeImmediate,
	}
	s.markOneTimeImmediatePending(immJob)
	assert.True(t, s.isImmediateDispatchPending("SCH-IMM-1"))

	s.clearImmediateDispatchPending("SCH-IMM-1")
	assert.False(t, s.isImmediateDispatchPending("SCH-IMM-1"))

	// markAdmissionFailed returns true on first call (newly marked), false on repeat
	assert.True(t, s.markAdmissionFailed("SCH-FAIL-1"))
	assert.False(t, s.markAdmissionFailed("SCH-FAIL-1"))
}

func TestNoAutoRestart_Operations(t *testing.T) {
	t.Parallel()

	// Empty root handled gracefully
	assert.NoError(t, WriteNoAutoRestartFile(""))
	assert.NoError(t, RemoveNoAutoRestartFile(""))
	assert.False(t, IsNoAutoRestartSet(""))

	tmpDir := t.TempDir()

	assert.False(t, IsNoAutoRestartSet(tmpDir))
	markerPath := NoAutoRestartFilePath(tmpDir)
	assert.Contains(t, markerPath, paths.SchedulerNoAutoRestartFile)

	require.NoError(t, WriteNoAutoRestartFile(tmpDir))
	assert.True(t, IsNoAutoRestartSet(tmpDir))

	require.NoError(t, RemoveNoAutoRestartFile(tmpDir))
	assert.False(t, IsNoAutoRestartSet(tmpDir))
}

func TestHumanInTheLoomInterrupt_EdgeCases(t *testing.T) {
	t.Parallel()

	// Empty root returns error
	err := EmitHumanInTheLoomInterrupt(context.Background(), "", "CVS-1", "reason", "POL-1", 0.95, false, "main")
	assert.Error(t, err)

	// Valid project root appends interrupt record
	tmpDir := t.TempDir()
	err = EmitHumanInTheLoomInterrupt(context.Background(), tmpDir, "CVS-TEST-1", "Needs human signoff", "POL-SAFETY-01", 0.90, false, "integration/cycle-43")
	assert.NoError(t, err)
}

func TestSchedulerIssues_CoreCoverage(t *testing.T) {
	t.Parallel()

	// Empty project root or jobID returns safely
	ReportIssue("", "JOB-1", "type", "err")
	ReportIssue("/tmp/foo", "", "type", "err")
	ClearIssuesIfOk("")

	_, err := ReadIssues("")
	assert.NoError(t, err)

	_, err = ClearIssues("")
	assert.Error(t, err)

	tmpDir := t.TempDir()

	// Read when missing returns status ok payload
	payload, err := ReadIssues(tmpDir)
	assert.NoError(t, err)
	require.NotNil(t, payload)
	assert.Equal(t, issuesStatusOK, payload.Status)

	// Report an issue
	ReportIssue(tmpDir, "SCH-TEST-ERR", "validation", "simulated job failure")

	payload, err = ReadIssues(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, payload)
	assert.Equal(t, issuesStatusIssues, payload.Status)
	require.Len(t, payload.Issues, 1)
	assert.Equal(t, "SCH-TEST-ERR", payload.Issues[0].JobID)
	assert.Equal(t, "simulated job failure", payload.Issues[0].Error)

	// Clear issues if ok doesn't clear fresh issues
	ClearIssuesIfOk(tmpDir)
	payload, err = ReadIssues(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, issuesStatusIssues, payload.Status)

	// Explicit ClearIssues clears and sets status ok
	writtenPath, err := ClearIssues(tmpDir)
	require.NoError(t, err)
	assert.NotEmpty(t, writtenPath)

	payload, err = ReadIssues(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, payload)
	assert.Equal(t, issuesStatusOK, payload.Status)
	assert.Empty(t, payload.Issues)
}

func TestCategoryLogs_NormalizationAndAppends(t *testing.T) {
	t.Parallel()

	// 1. Normalization
	assert.Equal(t, "uncategorized", normalizeCategoryName(""))
	assert.Equal(t, "uncategorized", normalizeCategoryName("   "))
	assert.Equal(t, "maintenance", normalizeCategoryName("Maintenance"))
	assert.Equal(t, "code-review", normalizeCategoryName("Code  Review"))
	assert.Equal(t, "test-suite", normalizeCategoryName("test_suite"))

	// 2. Safe with nil or empty
	appendCategoryLogEntry("", nil, "start", 0, nil)
	appendCategoryLogEntry("/tmp", nil, "start", 0, nil)
	appendCategoryLogEntry("", &ScheduledJob{ID: "SCH-1"}, "start", 0, nil)

	// 3. Valid append
	tmpDir := t.TempDir()
	job := &ScheduledJob{
		ID:          "SCH-CAT-1",
		Title:       "Test Category Job",
		Category:    "maintenance",
		JobType:     "audit",
		TriggerType: TriggerTypeImmediate,
	}

	appendCategoryLogEntry(tmpDir, job, "complete", 1500*time.Millisecond, nil)

	catDir := jobCategoryLogsDir(tmpDir)
	assert.True(t, fileutil.Exists(catDir))
}

func TestJobLogPaths_StemsAndPrefixes(t *testing.T) {
	t.Parallel()

	// 1. IsTestBundleJob
	assert.True(t, IsTestBundleJob("SCH-run-test-bundle-123"))
	assert.False(t, IsTestBundleJob("SCH-12345"))
	assert.False(t, IsTestBundleJob(""))

	// 2. IsChurnStyleSchedulerJobID
	assert.False(t, IsChurnStyleSchedulerJobID(""))
	assert.False(t, IsChurnStyleSchedulerJobID("SCH-run-test"))
	assert.False(t, IsChurnStyleSchedulerJobID("NON-SCH-123-abc"))
	assert.False(t, IsChurnStyleSchedulerJobID("SCH-notanumber-abc"))
	assert.False(t, IsChurnStyleSchedulerJobID("SCH-1771968798")) // No dash after ts
	assert.True(t, IsChurnStyleSchedulerJobID("SCH-1771968798-zqk-session-ZQK-028"))

	// 3. JobLogFileStem
	assert.Equal(t, "unknown", JobLogFileStem(""))
	assert.Equal(t, "SCH-SHORT", JobLogFileStem("SCH-SHORT"))

	churnStem := JobLogFileStem("SCH-1771968798-zqk-session-ZQK-028")
	assert.True(t, strings.HasPrefix(churnStem, "SCH-1771968798-"))

	// Very long job ID (> 200 bytes) gets shortened to SCH-LONG-<hash>
	longID := "SCH-" + strings.Repeat("a", 250)
	longStem := JobLogFileStem(longID)
	assert.True(t, strings.HasPrefix(longStem, "SCH-LONG-"))
	assert.LessOrEqual(t, len(longStem), 50)

	// Fallback stem
	fb := churnFallbackLogStem("INVALID")
	assert.True(t, strings.HasPrefix(fb, "SCH-CHURN-"))
}

func TestActivityCache_Operations(t *testing.T) {
	t.Parallel()

	// 1. activityCacheableJobID
	assert.True(t, activityCacheableJobID("SCH-NORMAL-ID"))
	veryLongID := strings.Repeat("x", 250)
	assert.False(t, activityCacheableJobID(veryLongID))

	// 2. ActivityCache lifecycle
	cache := NewActivityCache()
	require.NotNil(t, cache)

	cache.Stop() // Idempotent stop
	cache.Stop()
}

func TestHandlerFactory_AllRegisteredJobTypes_AndTestHooks(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	t.Cleanup(func() {
		if err := fileutil.RemoveAll(tmpDir); err != nil {
			t.Logf("cleanup: %v", err)
		}
	})
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tmpDir, sp)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	sched := &Scheduler{projectRoot: tmpDir, storage: sp, logger: logger}
	factory := NewHandlerFactory(sp, nil, nil, tmpDir, logger, nil, nil, nil, nil).(*HandlerFactory)
	factory.scheduler = sched

	// 1. Cover every registered job_type builder
	keys := jobTypeHandlerRegistryKeys()
	require.NotEmpty(t, keys)
	for _, k := range keys {
		job := &ScheduledJob{
			ID:          "SCH-TEST-" + k,
			JobType:     k,
			Title:       "Test " + k,
			TriggerType: TriggerTypeImmediate,
			Category:    CategoryTesting,
		}
		h := factory.CreateHandler(job)
		assert.NotNil(t, h, "handler should not be nil for job_type: %s", k)
	}

	// 2. Unregistered job type falls back to NoOp
	unregJob := &ScheduledJob{
		ID:      "SCH-TEST-UNREG",
		JobType: "non_existent_special_job_type_xyz",
	}
	hNoop := factory.CreateHandler(unregJob)
	assert.NotNil(t, hNoop)

	// 3. createMetricsCollectionHandler title variations
	hMetric1 := factory.createMetricsCollectionHandler(&ScheduledJob{Title: "Check File Lock Contention"})
	assert.NotNil(t, hMetric1)
	hMetric2 := factory.createMetricsCollectionHandler(&ScheduledJob{Title: "General Metric"})
	assert.NotNil(t, hMetric2)
	hMetric3 := factory.createMetricsCollectionHandler(&ScheduledJob{Title: ""})
	assert.NotNil(t, hMetric3)

	// 4. TestCommandDetector caching
	det1 := factory.getCachedTestCommandDetector()
	det2 := factory.getCachedTestCommandDetector()
	assert.Equal(t, det1, det2)

	// 5. Whitebox accessors in handler_factory_fortest.go
	assert.Equal(t, sp, factory.StorageForTest())
	assert.Nil(t, factory.SpecLoaderForTest())
	assert.Nil(t, factory.LifecycleLoaderForTest())
	assert.Equal(t, tmpDir, factory.ProjectRootForTest())
	assert.Nil(t, factory.ObjectIDCacheBuilderForTest())
	assert.Nil(t, factory.ValidationScannerForTest())
	assert.Equal(t, logger, factory.LoggerForTest())
	assert.Nil(t, factory.NotificationContextForTest())
	assert.Nil(t, factory.AsyncRouterForTest())
	assert.Equal(t, sched, factory.SchedulerForTest())
	assert.NotNil(t, factory.CreateMetricsCollectionHandlerForTest(&ScheduledJob{Title: "lock"}))
	assert.NotNil(t, factory.GetCachedTestCommandDetectorForTest())

	// Nil factory safety
	var nilFactory *HandlerFactory
	assert.Nil(t, nilFactory.StorageForTest())
	assert.Nil(t, nilFactory.SpecLoaderForTest())
	assert.Nil(t, nilFactory.LifecycleLoaderForTest())
	assert.Empty(t, nilFactory.ProjectRootForTest())
	assert.Nil(t, nilFactory.ObjectIDCacheBuilderForTest())
	assert.Nil(t, nilFactory.ValidationScannerForTest())
	assert.Nil(t, nilFactory.LoggerForTest())
	assert.Nil(t, nilFactory.NotificationContextForTest())
	assert.Nil(t, nilFactory.AsyncRouterForTest())
	assert.Nil(t, nilFactory.SchedulerForTest())
}

func TestScheduler_ForTestAccessors(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	t.Cleanup(func() {
		if err := fileutil.RemoveAll(tmpDir); err != nil {
			t.Logf("cleanup: %v", err)
		}
	})
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tmpDir, sp)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	s := &Scheduler{
		projectRoot: tmpDir,
		storage:     sp,
		logger:      logger,
	}

	assert.Equal(t, sp, s.StorageForTest())
	assert.Equal(t, logger, s.LoggerForTest())
	assert.Equal(t, tmpDir, s.ProjectRootForTest())
	assert.Equal(t, tmpDir, s.GetProjectRootForTest())
	assert.Nil(t, s.SpecLoaderForTest())
	assert.Nil(t, s.ExecutorForTest())
	assert.Nil(t, s.StateRegistryForTest())
	assert.Nil(t, s.CoordinationChannelForTest())
	assert.Nil(t, s.ProcessGroupManagerForTest())
	assert.Nil(t, s.AsyncRouterForTest())

	s.SetStorageForTest(sp)
	s.SetLoggerForTest(logger)
	s.SetExecutorForTest(&NativeExecutor{})
	assert.NotNil(t, s.ExecutorForTest())

	// CreateJobAuditEventForTest
	ctx := context.Background()
	_ = s.CreateJobAuditEventForTest(ctx, "job_completed", "SCH-1", "test_runner", "testing", true, time.Second, nil)

	// DisableJobInStorageForTest
	job := &ScheduledJob{ID: "SCH-1", Enabled: true}
	s.DisableJobInStorageForTest(ctx, job)
}

func TestCapOrchestrator_ForTestAccessors(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	t.Cleanup(func() {
		if err := fileutil.RemoveAll(tmpDir); err != nil {
			t.Logf("cleanup: %v", err)
		}
	})
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tmpDir, sp)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	h := NewCapOrchestratorHandler(sp, tmpDir, logger).(*CapOrchestratorHandler)
	require.NotNil(t, h)

	h.SetStorageForTest(sp)
	p := h.CapStatePathForTest("test-state")
	assert.Contains(t, p, "test-state")

	h.WriteStateFileForTest("test-state", map[string]string{"key": "value"})
	val, err := h.ReadStateFileForTest("test-state")
	assert.NoError(t, err)
	assert.NotNil(t, val)

	_, _ = h.ReadPendingStageForTest()
	h.MarkStageEnteredForTest("grooming", "PRI-1")
	h.ClearCAPStageFailureAttemptsForTest("grooming")

	pending := CapStagePending{Stage: "grooming", FailureAttempts: 5}
	_ = h.CapStageQuarantineActiveForTest(pending)

	ctx := context.Background()
	_ = h.MaybeAdvanceCAPStageForTest(ctx, "grooming")
	_, _ = h.DispatchDeliveryCompleteForTest(ctx, "dispatch")
	_, _ = h.ReviewDeliveryCompleteForTest()
	_, _ = h.ResolveBoundCVSForTest(ctx, "PRI-1")
	_ = h.RefreshPendingBoundCVSForTest(ctx, &pending)

	secCtx := pkgctx.NewSystemSecurityContext()
	_, _, _ = h.VerifyCriticalPackagesHealthForTest(ctx, secCtx)
	_ = h.HasOpenAgentInstructionForTest(ctx, "PRI-1", "architect", "refactor")
	_ = h.HasOpenTPMGroomingInstructionForTest(ctx, "PRI-1")
	_ = h.HasOpenTaskForPlanForTest(ctx, "PRI-1", "ATK-1")
	_ = h.ExecuteGroomingStageForTest(ctx, "/bin/echo", "PRI-1", 1, "Plan 1", "active", map[string]int{"ready": 1})
	_ = h.ExecuteDispatchStageForTest(ctx, "/bin/echo", "PRI-1", "inst", nil, true)
	_ = h.ResumePendingVerificationTasksForTest(ctx, "/bin/echo", "PRI-1")
	h.WakeAgentAndScheduleHourglassForTest("ATK-1", "architect")
	_ = h.AutoRecoverPlanTasksForTest(ctx, "PRI-1")
	_ = h.ExecuteMetricsStageForTest(ctx, "/bin/echo")
	_ = h.NestPlanWorkstreamsForTest(ctx, "PRI-1", map[string]any{"id": "PRI-1"})
	_ = h.ExecuteSentinelStageForTest(ctx, "/bin/echo")
}

func TestPackageConcurrencySync_EdgeCases(t *testing.T) {
	t.Parallel()

	// 1. intFromMetadata
	n, ok := intFromMetadata(10)
	assert.True(t, ok)
	assert.Equal(t, 10, n)
	n, ok = intFromMetadata(int32(12))
	assert.True(t, ok)
	assert.Equal(t, 12, n)
	n, ok = intFromMetadata(int64(14))
	assert.True(t, ok)
	assert.Equal(t, 14, n)
	n, ok = intFromMetadata(float64(16))
	assert.True(t, ok)
	assert.Equal(t, 16, n)
	n, ok = intFromMetadata(json.Number("18"))
	assert.True(t, ok)
	assert.Equal(t, 18, n)
	_, ok = intFromMetadata(json.Number("not-a-number"))
	assert.False(t, ok)
	_, ok = intFromMetadata("string-value")
	assert.False(t, ok)
	_, ok = intFromMetadata(nil)
	assert.False(t, ok)

	// 2. commandArgsToStrings
	assert.Nil(t, commandArgsToStrings(nil))
	assert.Equal(t, []string{"a", "b"}, commandArgsToStrings([]string{"a", "b"}))
	assert.Equal(t, []string{"x", "y"}, commandArgsToStrings([]any{"x", 123, "y"}))

	// 3. packageConcurrencyLimitsFromSchedulerJobRaw
	rawJobs := []map[string]any{
		{
			objects.FieldKeyID:       "SCH-run-test-1",
			objects.FieldKeyCategory: CategoryTesting,
			objects.FieldKeyJobType:  JobTypeRunWrapper,
			objects.FieldKeyMetadata: map[string]any{
				KeyTestBundleMetaPackagePath:              "./pkg/scheduler",
				KeyTestBundleMetaMaxConcurrentSamePackage: 4,
			},
		},
		{
			// Cap at 32
			objects.FieldKeyID:       "SCH-run-test-2",
			objects.FieldKeyCategory: CategoryTesting,
			objects.FieldKeyJobType:  JobTypeRunWrapper,
			objects.FieldKeyMetadata: map[string]any{
				KeyTestBundleMetaPackagePath:              "pkg/storage",
				KeyTestBundleMetaMaxConcurrentSamePackage: 100,
			},
		},
		{
			// Skip non-SCH-run
			objects.FieldKeyID:       "SCH-other-1",
			objects.FieldKeyCategory: CategoryTesting,
			objects.FieldKeyJobType:  JobTypeRunWrapper,
		},
	}
	limits := packageConcurrencyLimitsFromSchedulerJobRaw(rawJobs)
	assert.Equal(t, 4, limits["pkg/scheduler"])
	assert.Equal(t, 32, limits["pkg/storage"])

	// 4. syncPackageConcurrencyLimits
	var nilSched *Scheduler
	nilSched.syncPackageConcurrencyLimits(rawJobs)

	tmpDir := t.TempDir()
	sched := &Scheduler{
		projectRoot: tmpDir,
	}
	sched.syncPackageConcurrencyLimits(rawJobs)
}

func TestScheduledJobGuard_Combinations(t *testing.T) {
	t.Parallel()

	// RequirementNone
	block, warn := EvaluateSchedulerGuard(RequirementNone, false, false)
	assert.False(t, block)
	assert.False(t, warn)

	// Running true
	block, warn = EvaluateSchedulerGuard(RequirementRequired, true, false)
	assert.False(t, block)
	assert.False(t, warn)

	// RequirementOptional, running false
	block, warn = EvaluateSchedulerGuard(RequirementOptional, false, false)
	assert.False(t, block)
	assert.True(t, warn)

	// RequirementRequired, running false, allowDegraded false
	block, warn = EvaluateSchedulerGuard(RequirementRequired, false, false)
	assert.True(t, block)
	assert.False(t, warn)

	// RequirementRequired, running false, allowDegraded true
	block, warn = EvaluateSchedulerGuard(RequirementRequired, false, true)
	assert.False(t, block)
	assert.True(t, warn)

	// IsSchedulerRunningForRoot
	assert.True(t, IsSchedulerRunningForRoot(""))
	assert.True(t, IsSchedulerRunningForRoot("   "))

	tmpDir := t.TempDir()
	assert.False(t, IsSchedulerRunningForRoot(tmpDir))

	// scheduledJobEnvMissing
	assert.True(t, scheduledJobEnvMissing(nil))
	assert.True(t, scheduledJobEnvMissing(&ScheduledJob{}))
	assert.False(t, scheduledJobEnvMissing(&ScheduledJob{EnvironmentVariables: map[string]string{}}))
}

func TestStaleLockTTL_Comprehensive(t *testing.T) {
	t.Parallel()

	// 1. IsProcessAlive
	assert.False(t, IsProcessAlive(0))
	assert.False(t, IsProcessAlive(-1))
	assert.True(t, IsProcessAlive(os.Getpid()))
	assert.False(t, IsProcessAlive(99999999))

	// 2. ReadLockMetadata and WriteLockMetadata
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	// Empty path
	_, err := ReadLockMetadata(lockPath)
	assert.Error(t, err)

	// Write nil metadata
	assert.NoError(t, WriteLockMetadata(lockPath, nil))

	// Write valid JSON metadata
	meta := &LockMetadata{
		PID:       os.Getpid(),
		CreatedAt: time.Now().Add(-1 * time.Hour),
		JobID:     "SCH-TEST-LOCK",
	}
	err = WriteLockMetadata(lockPath, meta)
	require.NoError(t, err)

	readMeta, err := ReadLockMetadata(lockPath)
	require.NoError(t, err)
	assert.Equal(t, os.Getpid(), readMeta.PID)
	assert.Equal(t, "SCH-TEST-LOCK", readMeta.JobID)

	// Plain integer fallback
	intLockPath := filepath.Join(tmpDir, "int.lock")
	require.NoError(t, fileutil.WriteFile(intLockPath, []byte("12345\n"), 0644))
	intMeta, err := ReadLockMetadata(intLockPath)
	require.NoError(t, err)
	assert.Equal(t, 12345, intMeta.PID)

	// Empty file
	emptyLockPath := filepath.Join(tmpDir, "empty.lock")
	require.NoError(t, fileutil.WriteFile(emptyLockPath, []byte("   \n"), 0644))
	emptyMeta, err := ReadLockMetadata(emptyLockPath)
	require.NoError(t, err)
	assert.Equal(t, 0, emptyMeta.PID)

	// 3. ReconcileStaleLock
	_, err = ReconcileStaleLock("", time.Minute)
	assert.Error(t, err)

	// Non-existent lock file
	reconciled, err := ReconcileStaleLock(filepath.Join(tmpDir, "non_existent.lock"), time.Minute)
	assert.NoError(t, err)
	assert.False(t, reconciled)

	// Stale lock with dead PID
	staleDeadLock := filepath.Join(tmpDir, "stale_dead.lock")
	deadMeta := &LockMetadata{
		PID:       99999999,
		CreatedAt: time.Now().Add(-10 * time.Minute),
	}
	require.NoError(t, WriteLockMetadata(staleDeadLock, deadMeta))
	reconciled, err = ReconcileStaleLock(staleDeadLock, time.Hour)
	assert.NoError(t, err)
	assert.True(t, reconciled)
	assert.False(t, fileutil.Exists(staleDeadLock))

	// 4. CleanStaleLocksWithPID
	_, err = CleanStaleLocksWithPID("", time.Minute)
	assert.Error(t, err)

	count, err := CleanStaleLocksWithPID(filepath.Join(tmpDir, "non_existent_dir"), time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, 0, count)

	lockDir := filepath.Join(tmpDir, "locks")
	require.NoError(t, fileutil.MkdirAll(lockDir, 0755))

	// Put one stale dead lock, and one non-lock file
	staleInDir := filepath.Join(lockDir, "stale.lock")
	require.NoError(t, WriteLockMetadata(staleInDir, deadMeta))

	nonLock := filepath.Join(lockDir, "some.txt")
	require.NoError(t, fileutil.WriteFile(nonLock, []byte("hello"), 0644))

	count, err = CleanStaleLocksWithPID(lockDir, 5*time.Second)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestCriteriaAutoValidate_Branches(t *testing.T) {
	t.Parallel()

	// 1. criteriaAutoValidateDisabled
	_ = criteriaAutoValidateDisabled()

	// 2. applyCriterionValidatedFromTestBundle edge cases
	ctx := context.Background()
	did, err := applyCriterionValidatedFromTestBundle(ctx, nil, "CRIT-1")
	assert.False(t, did)
	assert.NoError(t, err)

	tmpDir := t.TempDir()
	t.Cleanup(func() {
		if err := fileutil.RemoveAll(tmpDir); err != nil {
			t.Logf("cleanup: %v", err)
		}
	})
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tmpDir, sp)

	did, err = applyCriterionValidatedFromTestBundle(ctx, sp, "")
	assert.False(t, did)
	assert.NoError(t, err)

	// Non-existent criterion
	did, err = applyCriterionValidatedFromTestBundle(ctx, sp, "CRIT-NONEXISTENT")
	assert.False(t, did)
	assert.Error(t, err)

	// Criterion with kind not matching criteria
	secCtx := pkgctx.NewSystemSecurityContext()
	taskObj := map[string]any{
		objects.FieldKeyID:     "ATK-WRONG-KIND",
		objects.FieldKeyKind:   objects.KindAgentTask,
		objects.FieldKeyStatus: "in_progress",
	}
	require.NoError(t, sp.Create(ctx, secCtx, taskObj))
	did, err = applyCriterionValidatedFromTestBundle(ctx, sp, "ATK-WRONG-KIND")
	assert.False(t, did)
	assert.Error(t, err)

	// Criterion already validated/completed
	validCrit := map[string]any{
		objects.FieldKeyID:          "CRIT-ALREADY-VALID",
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyStatus:      objects.ObjectStatusValidated,
		objects.FieldKeyTitle:       "Test Criteria Already Valid",
		objects.FieldKeyCategory:    "functional",
		objects.FieldKeyDescription: "Valid criteria description minimum 10 chars",
	}
	require.NoError(t, sp.Create(ctx, secCtx, validCrit))
	did, err = applyCriterionValidatedFromTestBundle(ctx, sp, "CRIT-ALREADY-VALID")
	assert.False(t, did)
	assert.NoError(t, err)

	// Valid criterion updated
	inProgCrit := map[string]any{
		objects.FieldKeyID:          "CRIT-IN-PROG",
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyStatus:      "in_progress",
		objects.FieldKeyTitle:       "Test Criteria In Progress",
		objects.FieldKeyCategory:    "functional",
		objects.FieldKeyDescription: "In progress criteria description minimum 10 chars",
	}
	require.NoError(t, sp.Create(ctx, secCtx, inProgCrit))
	did, err = applyCriterionValidatedFromTestBundle(ctx, sp, "CRIT-IN-PROG")
	assert.True(t, did)
	assert.NoError(t, err)

	// Verify status updated to validated
	updated, err := sp.Read(ctx, secCtx, "CRIT-IN-PROG")
	require.NoError(t, err)
	assert.Equal(t, objects.ObjectStatusValidated, updated[objects.FieldKeyStatus])

	// 3. maybeAutoValidateCriteriaFromSatisfiedTestBundle
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	runHandler := &RunWrapperHandler{
		storage:     sp,
		logger:      logger,
		projectRoot: tmpDir,
	}

	// Early returns: nil receiver, nil storage, nil job, empty project root
	var nilHandler *RunWrapperHandler
	nilHandler.maybeAutoValidateCriteriaFromSatisfiedTestBundle(ctx, &ScheduledJob{}, "green", 0)

	// Unsatisfied: tests failed > 0 or TF outcome
	job := &ScheduledJob{
		ID: "SCH-run-test",
		Metadata: map[string]any{
			KeyTestBundleMetaCriteriaRefs: []any{"CRIT-IN-PROG"},
		},
	}
	runHandler.maybeAutoValidateCriteriaFromSatisfiedTestBundle(ctx, job, "green", 1)
	runHandler.maybeAutoValidateCriteriaFromSatisfiedTestBundle(ctx, job, runWrapperTestOutcomeTF, 0)

	// Satisfied run
	runHandler.maybeAutoValidateCriteriaFromSatisfiedTestBundle(ctx, job, "green", 0)
}

func TestJobTypeViewCache_PureAndAtomic(t *testing.T) {
	t.Parallel()

	// 1. jobTypeToStaticGroup all branches
	assert.Equal(t, "convergence", jobTypeToStaticGroup("convergence_session"))
	assert.Equal(t, "cache", jobTypeToStaticGroup("cache_prewarm"))
	assert.Equal(t, "retention", jobTypeToStaticGroup("retention_tolerance"))
	assert.Equal(t, "aggregation", jobTypeToStaticGroup("audit_aggregation"))
	assert.Equal(t, "metrics", jobTypeToStaticGroup("metrics_collection"))
	assert.Equal(t, objects.KindLifecycle, jobTypeToStaticGroup("lifecycle_check"))
	assert.Equal(t, "validation", jobTypeToStaticGroup("spec_validation"))
	assert.Equal(t, "operations", jobTypeToStaticGroup("operation_cascade"))
	assert.Equal(t, "maintenance", jobTypeToStaticGroup("maintenance_task"))
	assert.Equal(t, "callbacks", jobTypeToStaticGroup("callback_listener"))
	assert.Equal(t, "other", jobTypeToStaticGroup("custom_job_type"))

	// 2. findUnhandledJobTypes
	missing := findUnhandledJobTypes([]string{"agent_sync", "completely_unknown_job_type_xyz", "manifest_snapshot"})
	assert.Contains(t, missing, "completely_unknown_job_type_xyz")
	assert.NotContains(t, missing, "agent_sync")
	assert.NotContains(t, missing, "manifest_snapshot") // In intentional no-handler allowlist

	// 3. MarshalForDebug
	var nilCache *JobTypeViewCache
	assert.Empty(t, nilCache.MarshalForDebug())
	cache := &JobTypeViewCache{
		SchemaVersion: 1,
		BuiltAtUTC:    "2026-11-05T00:00:00Z",
		EnumValues:    []string{"agent_sync"},
	}
	debugStr := cache.MarshalForDebug()
	assert.Contains(t, debugStr, "agent_sync")

	// 4. Save and load atomic
	tmpDir := t.TempDir()
	assert.NoError(t, SaveJobTypeViewCacheAtomic("", cache))
	assert.NoError(t, SaveJobTypeViewCacheAtomic(tmpDir, nil))

	err := SaveJobTypeViewCacheAtomic(tmpDir, cache)
	require.NoError(t, err)

	loaded, err := LoadJobTypeViewCache(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, 1, loaded.SchemaVersion)
	assert.Equal(t, []string{"agent_sync"}, loaded.EnumValues)

	// Load from non-existent root
	loadedEmpty, err := LoadJobTypeViewCache(filepath.Join(tmpDir, "missing"))
	assert.NoError(t, err)
	assert.Nil(t, loadedEmpty)

	// Load from empty root
	loadedBlank, err := LoadJobTypeViewCache("")
	assert.NoError(t, err)
	assert.Nil(t, loadedBlank)

	// EnsureJobTypeViewCacheReady with empty root or nil specLoader
	assert.NoError(t, EnsureJobTypeViewCacheReady(context.Background(), "", nil))
	assert.NoError(t, EnsureJobTypeViewCacheReady(context.Background(), tmpDir, nil))

	// 5. extractJobTypeEnumValuesFromSchedulerJobSpec negative validation
	_, err = extractJobTypeEnumValuesFromSchedulerJobSpec(nil)
	assert.Error(t, err)

	specNoField := &objects.Spec{ResolvedFields: map[string]any{}}
	_, err = extractJobTypeEnumValuesFromSchedulerJobSpec(specNoField)
	assert.Error(t, err)

	specNoValidation := &objects.Spec{
		ResolvedFields: map[string]any{
			objects.FieldKeyJobType: map[string]any{},
		},
	}
	_, err = extractJobTypeEnumValuesFromSchedulerJobSpec(specNoValidation)
	assert.Error(t, err)

	specNoEnum := &objects.Spec{
		ResolvedFields: map[string]any{
			objects.FieldKeyJobType: map[string]any{
				"validation": map[string]any{},
			},
		},
	}
	_, err = extractJobTypeEnumValuesFromSchedulerJobSpec(specNoEnum)
	assert.Error(t, err)

	specBadEnum := &objects.Spec{
		ResolvedFields: map[string]any{
			objects.FieldKeyJobType: map[string]any{
				"validation": map[string]any{
					"enum": []any{123},
				},
			},
		},
	}
	_, err = extractJobTypeEnumValuesFromSchedulerJobSpec(specBadEnum)
	assert.Error(t, err)

	specValid := &objects.Spec{
		ResolvedFields: map[string]any{
			objects.FieldKeyJobType: map[string]any{
				"validation": map[string]any{
					"enum": []any{"agent_sync", "watchdog_evaluation"},
				},
			},
		},
	}
	enums, err := extractJobTypeEnumValuesFromSchedulerJobSpec(specValid)
	assert.NoError(t, err)
	assert.Equal(t, []string{"agent_sync", "watchdog_evaluation"}, enums)

	// 6. logJobTypeViewParity
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	logJobTypeViewParity(nil, enums, map[string]bool{})
	logJobTypeViewParity(logger, nil, map[string]bool{})
	logJobTypeViewParity(logger, enums, map[string]bool{"agent_sync": true, "watchdog_evaluation": true})
	logJobTypeViewParity(logger, enums, map[string]bool{"agent_sync": true}) // Missing one
}

func TestCoordinationChannel_PipelineAndLifecycle(t *testing.T) {
	t.Parallel()

	// 1. migrateLegacyEventLog
	assert.NoError(t, migrateLegacyEventLog("", ""))
	assert.NoError(t, migrateLegacyEventLog("same", "same"))

	tmpDir := t.TempDir()
	legacyFile := filepath.Join(tmpDir, "legacy.log")
	newFile := filepath.Join(tmpDir, "new.jsonl")

	// Neither file exists
	assert.NoError(t, migrateLegacyEventLog(legacyFile, newFile))

	// Legacy file exists, new file doesn't
	require.NoError(t, fileutil.WriteFile(legacyFile, []byte("data\n"), 0644))
	assert.NoError(t, migrateLegacyEventLog(legacyFile, newFile))
	assert.True(t, fileutil.Exists(newFile))

	// Both files exist (no-op)
	require.NoError(t, fileutil.WriteFile(legacyFile, []byte("data2\n"), 0644))
	assert.NoError(t, migrateLegacyEventLog(legacyFile, newFile))

	// 2. Subscribe and Publish
	var nilCC *CoordinationChannel
	ch := nilCC.Subscribe()
	_, open := <-ch
	assert.False(t, open)

	assert.Error(t, nilCC.PublishEvent(Event{}))
	assert.Error(t, nilCC.WatchEvents(context.Background()))

	ccInterface := NewCoordinationChannel(tmpDir)
	require.NotNil(t, ccInterface)
	cc, ok := ccInterface.(*CoordinationChannel)
	require.True(t, ok)

	subCh := cc.Subscribe()
	require.NotNil(t, subCh)

	ev := Event{
		Type:      "test_event",
		JobID:     "SCH-COORD-1",
		Timestamp: time.Now().UTC(),
	}
	err := cc.PublishEvent(ev)
	require.NoError(t, err)

	// Event with zero timestamp (auto fills UTC)
	evZero := Event{
		Type:  "test_event_2",
		JobID: "SCH-COORD-2",
	}
	err = cc.PublishEvent(evZero)
	require.NoError(t, err)

	// 3. buildWatchEventsPipeline
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	pipe := cc.buildWatchEventsPipeline(logger)
	require.NotNil(t, pipe)

	rawEv, err := json.Marshal(ev)
	require.NoError(t, err)

	res, err := pipe.Run(&pipeline.Context{Ctx: context.Background()}, rawEv)
	require.NoError(t, err)
	assert.NotNil(t, res)

	// Check subscriber received event
	select {
	case received := <-subCh:
		assert.Equal(t, "test_event", received.Type)
		assert.Equal(t, "SCH-COORD-1", received.JobID)
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for event on subscription channel")
	}

	// 4. WatchEvents with cancelled context
	ctxCancel, cancel := context.WithCancel(context.Background())
	cancel() // Pre-cancelled
	assert.NoError(t, cc.WatchEvents(ctxCancel))
}

func TestJobTypeRegistry_RegistrationAndQueries(t *testing.T) {
	t.Parallel()

	customJobType := "test_custom_job_type_dyn"
	assert.False(t, HasJobTypeHandler(customJobType))

	RegisterJobTypeHandler(customJobType, "custom_key", func(f *HandlerFactory, job *ScheduledJob) JobHandler {
		return NewNoOpHandler()
	})
	assert.True(t, HasJobTypeHandler(customJobType))

	handlerMap := handlerKeyRegistry()
	assert.NotNil(t, handlerMap["custom_key"])

	UnregisterJobTypeHandler(customJobType)
	assert.False(t, HasJobTypeHandler(customJobType))

	assert.True(t, isJobTypeIntentionalNoHandler("manifest_snapshot"))
	assert.False(t, isJobTypeIntentionalNoHandler("unknown_xyz"))
}

func TestRuntimeThreadCount_Direct(t *testing.T) {
	t.Parallel()
	count := GetRuntimeThreadCount()
	assert.GreaterOrEqual(t, count, 0)
}

func TestCleanupConfigHandler_StepsAndParsers(t *testing.T) {
	t.Parallel()

	// 1. parseDuration
	_, err := parseDuration("")
	assert.Error(t, err)
	d, err := parseDuration("7d")
	assert.NoError(t, err)
	assert.Equal(t, 7*24*time.Hour, d)
	d, err = parseDuration("2h")
	assert.NoError(t, err)
	assert.Equal(t, 2*time.Hour, d)

	// 2. intParam, strParam, strSliceParam
	m := map[string]any{
		"int_val":   42,
		"float_val": float64(43),
		"str_val":   "hello",
		"slice_val": []any{"a", 1, "b"},
		"str_slice": []string{"c", "d"},
	}
	assert.Equal(t, 42, intParam(m, "int_val"))
	assert.Equal(t, 43, intParam(m, "float_val"))
	assert.Equal(t, 0, intParam(m, "missing"))
	assert.Equal(t, "hello", strParam(m, "str_val"))
	assert.Equal(t, "", strParam(m, "missing"))
	assert.Equal(t, []string{"a", "b"}, strSliceParam(m, "slice_val"))
	assert.Nil(t, strSliceParam(m, "missing"))

	// 3. runStep with various step types
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	h := NewCleanupConfigHandler(tmpDir, logger).(*CleanupConfigHandler)
	ctx := context.Background()
	job := &ScheduledJob{ID: "SCH-CLEANUP"}

	// unknown step
	err = h.runStep(ctx, job, tmpDir, cleanup.Step{Type: "unknown_future_step"}, 0)
	assert.NoError(t, err)

	// cli step missing command
	err = h.runStep(ctx, job, tmpDir, cleanup.Step{Type: "cli", Params: map[string]any{}}, 0)
	assert.Error(t, err)

	// delete_files step
	testFile := filepath.Join(tmpDir, "to_delete.txt")
	require.NoError(t, fileutil.WriteFile(testFile, []byte("delete me"), 0644))
	err = h.runStep(ctx, job, tmpDir, cleanup.Step{
		Type: "delete_files",
		Params: map[string]any{
			"path": "to_delete.txt",
		},
	}, 1)
	assert.NoError(t, err)
	assert.False(t, fileutil.Exists(testFile))

	// truncate_files step
	truncFile := filepath.Join(tmpDir, "to_trunc.txt")
	require.NoError(t, fileutil.WriteFile(truncFile, []byte("line1\nline2\nline3\n"), 0644))
	err = h.runStep(ctx, job, tmpDir, cleanup.Step{
		Type: "truncate_files",
		Params: map[string]any{
			"path":            "to_trunc.txt",
			"keep_last_lines": 1,
		},
	}, 2)
	assert.NoError(t, err)

	// reap_stale_locks step
	err = h.runStep(ctx, job, tmpDir, cleanup.Step{
		Type: "reap_stale_locks",
		Params: map[string]any{
			"older_than": "1h",
		},
	}, 3)
	assert.NoError(t, err)

	// reap_temp_files step
	err = h.runStep(ctx, job, tmpDir, cleanup.Step{
		Type: "reap_temp_files",
		Params: map[string]any{
			"older_than": "1h",
		},
	}, 4)
	assert.NoError(t, err)

	// enforce_log_retention step
	err = h.runStep(ctx, job, tmpDir, cleanup.Step{
		Type: "enforce_log_retention",
		Params: map[string]any{
			"older_than": "1h",
		},
	}, 5)
	assert.NoError(t, err)
}

func TestRetentionTolerance_HelpersAndStrategies(t *testing.T) {
	t.Parallel()

	// 1. isCatalogKind
	assert.True(t, isCatalogKind("goal"))
	assert.True(t, isCatalogKind("requirement"))
	assert.True(t, isCatalogKind("roadmap"))
	assert.True(t, isCatalogKind("policy"))
	assert.False(t, isCatalogKind(objects.KindAuditEvent))
	assert.False(t, isCatalogKind(objects.KindChangeJournalEntry))
	assert.False(t, isCatalogKind("unknown_custom_kind"))

	// 2. retentionKindPriority
	pAudit := retentionKindPriority(objects.KindAuditEvent)
	pJournal := retentionKindPriority(objects.KindChangeJournalEntry)
	pDefault := retentionKindPriority("unknown_custom_kind")
	assert.Greater(t, pDefault, pAudit)
	assert.Equal(t, 3, pJournal)

	// 3. getBulkDeleteWorkers
	workers := getBulkDeleteWorkers(nil)
	assert.GreaterOrEqual(t, workers, 1)
	jobWithWorkers := &ScheduledJob{
		EnvironmentVariables: map[string]string{
			EnvKeyBulkDeleteWorkers: "4",
		},
	}
	assert.Equal(t, 4, getBulkDeleteWorkers(jobWithWorkers))

	// 4. normalizeRetentionBatchLimits
	batchSize, maxBatches := normalizeRetentionBatchLimits(0, 0)
	assert.Greater(t, batchSize, 0)
	assert.Greater(t, maxBatches, 0)

	batchSize, maxBatches = normalizeRetentionBatchLimits(50, 10)
	assert.Equal(t, 50, batchSize)
	assert.Equal(t, 10, maxBatches)

	// 5. buildStatusExclusionFilter
	filter := buildStatusExclusionFilter([]string{"validated", "in_progress"})
	assert.NotNil(t, filter)

	// 6. getKindFilter
	jobWithKinds := &ScheduledJob{
		EnvironmentVariables: map[string]string{
			EnvKeyKinds: "audit_event,change_journal_entry",
		},
	}
	kindFilter := getKindFilter(jobWithKinds)
	assert.True(t, kindFilter[objects.KindAuditEvent])
	assert.True(t, kindFilter[objects.KindChangeJournalEntry])
	assert.False(t, kindFilter["other"])
}

func TestAggregationMetricsCleanup_EdgeCases(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	t.Cleanup(func() {
		if err := fileutil.RemoveAll(tmpDir); err != nil {
			t.Logf("cleanup: %v", err)
		}
	})
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tmpDir, sp)

	handler := NewAggregationMetricsCleanupHandler(sp).(*AggregationMetricsCleanupHandler)
	require.NotNil(t, handler)

	ctx := context.Background()

	// 1. retentionDays <= 0 skips cleanup
	jobSkip := &ScheduledJob{
		ID: "SCH-METRICS-CLEANUP-SKIP",
		EnvironmentVariables: map[string]string{
			EnvKeyRetentionDays: "0",
		},
	}
	err = handler.executeAggregationMetricsCleanupCore(ctx, jobSkip)
	assert.NoError(t, err)

	// 2. No old metrics found
	jobRun := &ScheduledJob{
		ID: "SCH-METRICS-CLEANUP-RUN",
		EnvironmentVariables: map[string]string{
			EnvKeyRetentionDays: "30",
		},
	}
	err = handler.executeAggregationMetricsCleanupCore(ctx, jobRun)
	assert.NoError(t, err)
}

func TestCapOrchestrator_FailureTrackingAndEscalation(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	t.Cleanup(func() {
		if err := fileutil.RemoveAll(tmpDir); err != nil {
			t.Logf("cleanup: %v", err)
		}
	})
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tmpDir, sp)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	chain := defaultEscalationChain(tmpDir)
	h := NewCapOrchestratorHandlerWithEscalation(sp, tmpDir, logger, chain).(*CapOrchestratorHandler)
	require.NotNil(t, h)

	// Record failures to hit both warning and critical human escalation tiers
	h.recordFailure("grooming", assert.AnError)
	tracker := h.readFailureTracker()
	assert.Equal(t, 1, tracker.ConsecutiveFailures)

	// Simulate reaching human escalation threshold
	tracker.ConsecutiveFailures = capHumanEscalationThreshold
	h.writeStateFile(capFailureTrackerFile, tracker)
	h.recordFailure("dispatch", assert.AnError)

	// clearFailures resets tracker
	h.clearFailures()
	cleared := h.readFailureTracker()
	assert.Equal(t, 0, cleared.ConsecutiveFailures)
}

func TestPIDFile_KeepAliveAndProcessChecks(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// 1. KeepAlive file operations
	assert.NoError(t, writeKeepAlive(tmpDir))
	ts, err := readKeepAlive(tmpDir)
	assert.NoError(t, err)
	assert.False(t, ts.IsZero())

	alive, _, err := IsSchedulerAlive(tmpDir)
	assert.NoError(t, err)
	assert.False(t, alive) // scheduler not running, so cleans up keepalive

	assert.NoError(t, RemoveKeepAlive(tmpDir))
	// Non-existent keep-alive read returns error
	_, err = readKeepAlive(tmpDir)
	assert.Error(t, err)

	// 2. Process check functions
	assert.True(t, IsProcessRunning(os.Getpid()))
	assert.False(t, IsProcessRunning(-1))
	assert.False(t, IsProcessRunning(99999999))
	assert.False(t, isZombieProcess(os.Getpid()))
	assert.False(t, isZombieProcess(0))

	// 3. Stop / Signal against non-running scheduler
	_, err = SignalSchedulerByPID(tmpDir, syscall.SIGTERM)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrSchedulerDown)

	err = ForceKillSchedulerByPID(tmpDir)
	assert.Error(t, err)

	err = StopSchedulerByPIDWithWait(tmpDir, 50*time.Millisecond)
	assert.Error(t, err)
}

func TestScheduler_CheckAndRecoverMissedJobs(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	t.Cleanup(func() {
		if err := fileutil.RemoveAll(tmpDir); err != nil {
			t.Logf("cleanup: %v", err)
		}
	})
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tmpDir, sp)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	s := &Scheduler{
		projectRoot: tmpDir,
		storage:     sp,
		logger:      logger,
	}

	// 1. Not running, no timer jobs -> 0, 0
	ctx := context.Background()
	checked, rec := s.checkAndRecoverMissedJobs(ctx)
	assert.Equal(t, 0, checked)
	assert.Equal(t, 0, rec)

	// 2. onlyManualJobs -> returns 0, 0
	s.running = true
	s.onlyManualJobs = true
	checked, rec = s.checkAndRecoverMissedJobs(ctx)
	assert.Equal(t, 0, checked)
	assert.Equal(t, 0, rec)

	// 3. Running with empty timer jobs
	s.onlyManualJobs = false
	checked, rec = s.checkAndRecoverMissedJobs(ctx)
	assert.Equal(t, 0, checked)
	assert.Equal(t, 0, rec)
}

func TestRetentionTolerance_MergeStrategyTolerance(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	t.Cleanup(func() {
		if err := fileutil.RemoveAll(tmpDir); err != nil {
			t.Logf("cleanup: %v", err)
		}
	})
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tmpDir, sp)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	h := NewRetentionToleranceHandler(sp, tmpDir).(*RetentionToleranceHandler)
	h.logger = logger

	ctx := context.Background()
	cfg := &config.RetentionToleranceConfig{}
	effective := h.mergeStrategyTolerance(ctx, cfg, "SCH-MERGE-TOL")
	assert.NotNil(t, effective)
}

func TestJobStateRegistry_AccessorsAndMigrations(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	t.Cleanup(func() {
		if err := fileutil.RemoveAll(tmpDir); err != nil {
			t.Logf("cleanup: %v", err)
		}
	})
	reg := NewJobStateRegistry(tmpDir).(*JobStateRegistry)
	require.NotNil(t, reg)

	// 1. Accessors
	assert.NotEmpty(t, reg.StateDirForTest())
	assert.NotEmpty(t, reg.LocksDirForTest())
	p := reg.StateFilePathForJobExecutionForTest("SCH-1", "EXE-1")
	assert.Contains(t, p, "EXE-1.yaml")

	var nilReg *JobStateRegistry
	assert.Empty(t, nilReg.StateDirForTest())
	assert.Empty(t, nilReg.LocksDirForTest())
	nilReg.WaitForRetentionCleanup()

	// 2. Retention cleanup hooks
	reg.ScheduleFullRetentionCleanupForTest()
	reg.WaitForRetentionCleanup()

	// 3. MigrateLegacyFlatStateFilesBestEffort
	stateDir := reg.StateDirForTest()
	require.NoError(t, fileutil.MkdirAll(stateDir, 0755))
	flatYAML := filepath.Join(stateDir, "legacy_exec.yaml")
	flatContent := `job_id: SCH-LEGACY
execution_id: EXE-LEGACY
status: in_progress
started_at: "2026-11-05T00:00:00Z"
`
	require.NoError(t, fileutil.WriteFile(flatYAML, []byte(flatContent), 0644))
	moved, err := reg.MigrateLegacyFlatStateFilesBestEffort()
	assert.NoError(t, err)
	assert.Equal(t, 1, moved)
	assert.False(t, fileutil.Exists(flatYAML))

	// 4. MigrateUnbucketedJobStateDirsBestEffort
	unbucketedDir := filepath.Join(stateDir, "SCH-UNBUCKETED")
	require.NoError(t, fileutil.MkdirAll(unbucketedDir, 0755))
	execInDir := filepath.Join(unbucketedDir, "EXE-SUB.yaml")
	execContent := `job_id: SCH-UNBUCKETED
execution_id: EXE-SUB
status: in_progress
`
	require.NoError(t, fileutil.WriteFile(execInDir, []byte(execContent), 0644))
	movedDirs, err := reg.MigrateUnbucketedJobStateDirsBestEffort()
	assert.NoError(t, err)
	assert.Equal(t, 1, movedDirs)
}

func TestHourglass_HelpersAndMissedCheckin(t *testing.T) {
	t.Parallel()

	// 1. actionForExpiredTimer
	assert.Equal(t, actionWakeOrchestrator, actionForExpiredTimer(agentclaim.TimerTypeCheckin))
	assert.Equal(t, actionWakeOrchestrator, actionForExpiredTimer(agentclaim.TimerTypeSeatHeartbeat))
	assert.Equal(t, actionEscalateDeadline, actionForExpiredTimer(timerTypeDeadline))
	assert.Equal(t, actionKillStuckProcess, actionForExpiredTimer("unknown_timer_type"))

	// 2. refsContainID
	assert.True(t, refsContainID([]string{"A", "B"}, "B"))
	assert.False(t, refsContainID([]string{"A", "B"}, "C"))
	assert.True(t, refsContainID([]any{"A", "B"}, "A"))
	assert.False(t, refsContainID([]any{"A", 123}, "B"))
	assert.False(t, refsContainID(nil, "A"))

	// 3. newMissedDeadlineRiskBlockerID
	id := newMissedDeadlineRiskBlockerID()
	assert.True(t, strings.HasPrefix(id, "RIS-"))

	// 4. hourglassSourcePresent
	assert.False(t, hourglassSourcePresent(context.Background(), nil, nil, ""))
	assert.False(t, hourglassSourcePresent(context.Background(), nil, nil, "ATK-1"))

	// 5. hasOpenMissedDeadlineEscalation
	assert.False(t, hasOpenMissedDeadlineEscalation(context.Background(), nil, nil, ""))

	// 6. handleMissedCheckin
	tmpDir := t.TempDir()
	t.Cleanup(func() {
		if err := fileutil.RemoveAll(tmpDir); err != nil {
			t.Logf("cleanup: %v", err)
		}
	})
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tmpDir, sp)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	sched := &Scheduler{projectRoot: tmpDir, storage: sp, logger: logger}
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Unreadable file removed
	badTimerFile := filepath.Join(tmpDir, "bad_timer.json")
	require.NoError(t, fileutil.WriteFile(badTimerFile, []byte("invalid json"), 0644))
	sched.handleMissedCheckin(ctx, secCtx, badTimerFile)
	assert.False(t, fileutil.Exists(badTimerFile))

	// Missing task removed
	validTimerFile := filepath.Join(tmpDir, "missing_task_timer.json")
	timerData := `{"task_id":"ATK-MISSING","claimed_by":"architect","armed_at":"2026-11-05T00:00:00Z"}`
	require.NoError(t, fileutil.WriteFile(validTimerFile, []byte(timerData), 0644))
	sched.handleMissedCheckin(ctx, secCtx, validTimerFile)
	assert.False(t, fileutil.Exists(validTimerFile))
}
