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
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtended_AuditAggregationSession_Cleanups_Wave45(t *testing.T) {
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

	// 1. Lookback helpers
	_ = catchUpAgeLookback(1*time.Hour, 10*time.Minute)
	_ = catchUpAgeLookback(10*time.Minute, 1*time.Hour)
	_ = metricLimitAuditEventLookback(1*time.Hour, 2*time.Hour, 10*time.Minute)
	_ = metricLimitAuditEventLookback(1*time.Hour, 0, 10*time.Minute)

	handler := NewAuditAggregationHandler(sp).(*AuditAggregationHandler)

	job := &ScheduledJob{
		ID:       DefaultAuditEventAggregationSchedulerJobID,
		JobType:  JobTypeAuditEventAggregation,
		Category: CategoryMaintenance,
		EnvironmentVariables: map[string]string{
			"PRE_AGGREGATION_CLEANUP": "true",
			"ARCHIVE_ENABLED":         "true",
			"DELETE_ENABLED":          "false",
			"BATCH_SIZE":              "10",
			"MAX_BATCHES":             "2",
			"MAX_RUNTIME_SECONDS":     "600",
		},
		MaxRuntimeSeconds: 600,
	}

	ctxDeadline, cancel := context.WithDeadline(ctx, time.Now().Add(5*time.Minute))
	defer cancel()

	sess := &auditAggregationSession{
		handler: handler,
		ctx:     ctxDeadline,
		job:     job,
	}
	_ = sess.loadConfiguration()
	sess.setupPhaseContexts()

	// 2. runPreChecks & determineEffectiveRetention
	_, _ = sess.runPreChecks()
	sess.determineEffectiveRetention()

	// 3. runRetentionFirstPass & runCatchUp & runAggressiveCleanup & runProactiveCleanup
	sess.runRetentionFirstPass()
	sess.runCatchUp()
	sess.runAggressiveCleanup()
	sess.runProactiveCleanup()
	sess.runRetentionSecondPass()

	// 4. Cancelled context branches
	cancelCtx, doCancel := context.WithCancel(ctx)
	doCancel()
	cancelledSess := &auditAggregationSession{
		handler:   handler,
		ctx:       cancelCtx,
		preAggCtx: cancelCtx,
		job:       job,
	}
	cancelledSess.runCatchUp()
	cancelledSess.runAggressiveCleanup()
	cancelledSess.runProactiveCleanup()
}

func TestExtended_RunWrapper_ExecutionAndRetries_Wave45(t *testing.T) {
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

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	h := NewRunWrapperHandlerWithProjectRoot(sp, logger, nil, nil, tmpDir).(*RunWrapperHandler)

	job := &ScheduledJob{
		ID:          "SCH-rw-wave45",
		JobType:     JobTypeRunWrapper,
		Category:    CategoryMaintenance,
		Command:     "go test -v ./...",
		CommandArgs: []string{"-v", "./..."},
		EnvironmentVariables: map[string]string{
			"COMMAND":                  "go test -v ./...",
			"WRITE_JOB_LOG_FILES":      "true",
			"WRITE_SEPARATE_JOB_LOGS":  "true",
			"STREAM_LOG_DIR":           filepath.Join(tmpDir, "logs"),
			"TEST_BUNDLE_METADATA_FINGERPRINT": "expected_fp",
		},
		Metadata: map[string]any{
			"command": "go test -v ./...",
		},
	}

	// 1. prepareRunWrapperExecution
	prep := h.prepareRunWrapperExecution(job)
	if prep.Command != "go test -v ./..." {
		t.Errorf("unexpected prep command: %s", prep.Command)
	}

	// 2. warnIfTestBundleMetadataFingerprintMismatch & logRunWrapperExecutionStart
	h.warnIfTestBundleMetadataFingerprintMismatch(job, "go test -run TestX")
	h.logRunWrapperExecutionStart(job, prep)

	// 3. runWrapperHandlePanic
	err = h.runWrapperHandlePanic(job, "test panic value", []byte("stack trace bytes"))
	if err == nil {
		t.Errorf("expected non-nil error from runWrapperHandlePanic")
	}

	// 4. File and stream helpers
	copyStreamFilesToRedirectTarget(tmpDir, filepath.Join(tmpDir, "logs"), "SCH-rw-wave45", filepath.Join(tmpDir, "target.log"))
	writeSeparateJobLogsIfConfigured(tmpDir, "SCH-rw-wave45", "test stdout", "test stderr")

	// 5. emitBundleProgress
	h.emitBundleProgress(ctx, job, "phase_start")
}

func TestExtended_PIDFile_Comprehensive_Wave45(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	// 1. readFileWithTimeout
	testFilePath := filepath.Join(tmpDir, "test_read.txt")
	_ = fileutil.WriteFile(testFilePath, []byte("hello pid"), 0600)
	content, err := readFileWithTimeout(testFilePath, 500*time.Millisecond)
	if err != nil || string(content) != "hello pid" {
		t.Errorf("readFileWithTimeout failed: %v, got %s", err, string(content))
	}
	_, _ = readFileWithTimeout(filepath.Join(tmpDir, "non_existent.txt"), 100*time.Millisecond)

	// 2. ResolveProjectRootFromCWD
	cwdRoot := ResolveProjectRootFromCWD()
	t.Logf("ResolveProjectRootFromCWD: %s", cwdRoot)

	// 3. Keepalive helpers
	_ = writeKeepAlive(tmpDir)
	kaTime, kaErr := readKeepAlive(tmpDir)
	if kaErr != nil || kaTime.IsZero() {
		t.Errorf("readKeepAlive failed: %v", kaErr)
	}
	_ = RemoveKeepAlive(tmpDir)
	_ = removeKeepAlive(tmpDir)

	// 4. PID file functions
	_ = WritePIDFile(tmpDir)
	_ = writeKeepAlive(tmpDir)
	alive, _, _ := IsSchedulerAlive(tmpDir)
	t.Logf("IsSchedulerAlive with current PID: %v", alive)

	_ = isSchedulerDaemonProcess(os.Getpid())
	_ = processBelongsToProjectRoot(os.Getpid(), tmpDir)

	_ = StopSchedulerByPIDWithWait(tmpDir, 100*time.Millisecond)
	_ = RemovePIDFile(tmpDir)
}

func TestExtended_EnvelopeTickDispatch_Wave45(t *testing.T) {
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

	sched := NewScheduler(sp, objects.GetGlobalSpecLoader(), objects.GetGlobalLifecycleLoader()).(*Scheduler)

	// 1. Static dispatch helpers
	_ = envelopeTickDispatchJobTypeAllowed(JobTypeCachePrewarm, false, nil)
	_ = envelopeTickDispatchJobTypeAllowed(JobTypeCleanup, true, map[string]struct{}{JobTypeCleanup: {}})
	_ = envelopeTickDispatchCommaSeparatedSet("a, b, c")
	_ = envelopeTickDispatchAllowlistExtraFromEnv(map[string]string{EnvKeyEnvelopeTickDispatchAllowlistExtra: "job1,job2"})
	_ = envelopeTickDispatchDenyExtraFromEnv(map[string]string{EnvKeyEnvelopeTickDispatchDenyExtra: "job3"})
	_, _ = envelopeTickDispatchMaxTriggersFromEnv(map[string]string{EnvKeyEnvelopeTickDispatchMaxTriggers: "5"})
	_, _ = envelopeTickDispatchMaxTriggersFromEnv(map[string]string{EnvKeyEnvelopeTickDispatchMaxTriggers: "invalid"})
	_ = envelopeTickDispatchHardDeny(JobTypeDataCellEnvelopeTick)
	_ = envelopeTickDispatchHardDeny(JobTypeCachePrewarm)
	_ = envelopeTickDispatchModeFromJob(map[string]string{EnvKeyEnvelopeTickDispatchMode: "explicit"})
	_ = envelopeTickDispatchParseTruthy("true")
	_ = envelopeTickDispatchParseTruthy("1")
	_ = envelopeTickDispatchParseTruthy("false")
	_ = envelopeTickDispatchEnvBool(map[string]string{"TEST_BOOL": "true"}, "TEST_BOOL", false)
	_ = envelopeTickDispatchExpandFromJob(map[string]string{EnvKeyEnvelopeTickDispatchExpand: "true"})

	deduped, dropped := dedupeEnvelopeTickResolvedJobTypes([]string{"typeA", "typeB", "typeA"})
	if len(deduped) != 2 || dropped != 1 {
		t.Errorf("unexpected dedupe result: %v, %d", deduped, dropped)
	}

	// 2. DispatchEnvelopeTickResolvedJobs
	envJob := &ScheduledJob{
		ID:       "SCH-env-tick",
		JobType:  JobTypeDataCellEnvelopeTick,
		Category: CategoryMaintenance,
		EnvironmentVariables: map[string]string{
			EnvKeyEnvelopeTickDispatchMode: "explicit",
		},
	}
	outcome := sched.DispatchEnvelopeTickResolvedJobs(ctx, envJob, []string{JobTypeCachePrewarm})
	t.Logf("DispatchEnvelopeTickResolvedJobs outcome: %+v", outcome)

	// 3. firstEnabledTriggerableJobIDForJobType
	_ = sched.firstEnabledTriggerableJobIDForJobType(JobTypeCachePrewarm)
}
