package scheduler

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// setupCycle126IsolatedEnv provisions hermetic storage and temp directories with zero swallowed errors.
func setupCycle126IsolatedEnv(t *testing.T) (string, storagepkg.ObjectStorageProvider) {
	tempDir := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Name(), tempDir)
	t.Setenv(zqkenv.TestRoot().Name(), tempDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		shutdownErr := sp.Shutdown(context.Background())
		require.NoError(t, shutdownErr)
	})
	return tempDir, sp
}

func TestCycle126_HandlersCapDispatch_Helpers(t *testing.T) {
	tempDir, sp := setupCycle126IsolatedEnv(t)
	ctx := context.Background()

	// 1. decodeCAPPlanOutput
	var parsed map[string]any
	validJSON := []byte(`{"stage":"planning","plan_id":"PRI-100"}`)
	err := decodeCAPPlanOutput(validJSON, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "planning", parsed["stage"])

	invalidJSON := []byte(`{invalid`)
	err = decodeCAPPlanOutput(invalidJSON, &parsed)
	assert.Error(t, err)

	// 2. mintAgentTaskID
	id1 := mintAgentTaskID()
	id2 := mintAgentTaskID()
	assert.True(t, strings.HasPrefix(id1, "ATK-"))
	assert.True(t, strings.HasPrefix(id2, "ATK-"))
	assert.NotEqual(t, id1, id2)

	// 3. capDispatchPlanIDs
	plans := []whatsnext.WhatsNextPriorityPlan{
		{ID: "PRI-200"},
		{ID: "PRI-100"},
		{ID: "PRI-300"},
	}
	ids := capDispatchPlanIDs("PRI-100", plans)
	assert.Equal(t, []string{"PRI-100", "PRI-200", "PRI-300"}, ids)
	emptyIDs := capDispatchPlanIDs("", nil)
	assert.Empty(t, emptyIDs)

	// 4. nestPlanWorkstreams
	h := &CapOrchestratorHandler{
		storage:     sp,
		projectRoot: tempDir,
	}
	planObj := make(map[string]any)
	bliMap := h.nestPlanWorkstreams(ctx, "PRI-100", planObj)
	assert.NotNil(t, bliMap)
}

func TestCycle126_HandlersCapStageGates_GroomingAndQuarantine(t *testing.T) {
	tempDir, sp := setupCycle126IsolatedEnv(t)
	h := &CapOrchestratorHandler{
		storage:     sp,
		projectRoot: tempDir,
	}

	// 1. groomingPlannedZeroExhausted
	h.markStageEntered("cap_stage_grooming", "PRI-201")
	pending, exhausted := h.groomingPlannedZeroExhausted(5)
	assert.False(t, exhausted)
	assert.Empty(t, pending.Stage)

	pending2, exhausted2 := h.groomingPlannedZeroExhausted(0)
	assert.False(t, exhausted2)
	assert.Equal(t, "cap_stage_grooming", pending2.Stage)

	// 2. capStageAttemptExhausted
	pUnexhausted := CapStagePending{FailureAttempts: 1}
	assert.False(t, capStageAttemptExhausted(pUnexhausted))
	pExhausted := CapStagePending{FailureAttempts: capStageMaxAttempts + 1}
	assert.True(t, capStageAttemptExhausted(pExhausted))

	// 3. quarantine and retry ready
	pQ := CapStagePending{
		Stage:           "cap_stage_planning",
		PlanID:          "PRI-201",
		CvsID:           "CVS-201",
		Attempts:        1,
		FailureAttempts: capStageMaxAttempts + 1,
	}
	h.quarantineCAPStage(pQ)
	assert.True(t, h.capStageQuarantineActive(pQ))

	pDiff := pQ
	pDiff.PlanID = "PRI-other"
	assert.False(t, h.capStageQuarantineActive(pDiff))
	assert.True(t, h.capStageQuarantineRetryReady(pDiff, time.Now()))

	assert.False(t, h.capStageQuarantineRetryReady(pQ, time.Now()))
	assert.True(t, h.capStageQuarantineRetryReady(pQ, time.Now().Add(capStageQuarantineRetryAfter+time.Minute)))

	h.clearCAPStageQuarantine()
	assert.False(t, h.capStageQuarantineActive(pQ))
}

func TestCycle126_HandlersCapStageGates_TimeAndStatus(t *testing.T) {
	// 1. parseFlexibleTime
	t1, err := parseFlexibleTime("2026-10-23T01:02:03Z")
	require.NoError(t, err)
	assert.Equal(t, 2026, t1.Year())

	t2, err := parseFlexibleTime("2026-10-23T01:02:03.123456789Z")
	require.NoError(t, err)
	assert.Equal(t, 123456789, t2.Nanosecond())

	_, err = parseFlexibleTime("invalid-time-format")
	assert.Error(t, err)

	// 2. objectTouchedSince
	now := time.Now()
	objNil := map[string]any{}
	assert.False(t, objectTouchedSince(objNil, now))

	objRecent := map[string]any{
		objects.FieldKeyUpdatedAt: now.Add(time.Minute).Format(time.RFC3339),
	}
	assert.True(t, objectTouchedSince(objRecent, now))

	objOld := map[string]any{
		objects.FieldKeyCreatedAt: now.Add(-10 * time.Minute).Format(time.RFC3339),
	}
	assert.False(t, objectTouchedSince(objOld, now))

	// 3. reviewResultStaleReason
	assert.NotEmpty(t, reviewResultStaleReason(nil, now, time.Hour))
	staleMap := map[string]any{
		"timestamp": now.Add(-2 * time.Hour).Format(time.RFC3339),
	}
	assert.NotEmpty(t, reviewResultStaleReason(staleMap, now, time.Hour))
	freshMap := map[string]any{
		"timestamp": now.Add(-10 * time.Minute).Format(time.RFC3339),
	}
	assert.Empty(t, reviewResultStaleReason(freshMap, now, time.Hour))

	// 4. agentTaskDeliveryTerminal and cvsStatusEligibleForCAP
	assert.True(t, agentTaskDeliveryTerminal(objects.ObjectStatusCompleted))
	assert.True(t, agentTaskDeliveryTerminal(objects.ObjectStatusImplemented))
	assert.True(t, agentTaskDeliveryTerminal("verified"))
	assert.False(t, agentTaskDeliveryTerminal("failed"))
	assert.False(t, agentTaskDeliveryTerminal("in_progress"))
	assert.False(t, agentTaskDeliveryTerminal("pending"))

	assert.True(t, cvsStatusEligibleForCAP("active"))
	assert.True(t, cvsStatusEligibleForCAP("paused"))
	assert.True(t, cvsStatusEligibleForCAP("escalated"))
	assert.False(t, cvsStatusEligibleForCAP("completed"))
	assert.False(t, cvsStatusEligibleForCAP("abandoned"))
	assert.False(t, cvsStatusEligibleForCAP("draft"))

	// 5. relatedStringRefs
	assert.Nil(t, relatedStringRefs(nil))
	assert.Equal(t, []string{"A", "B"}, relatedStringRefs([]string{"A", "B"}))
	assert.Equal(t, []string{"X", "Y"}, relatedStringRefs([]any{"X", "Y", ""}))
	assert.Nil(t, relatedStringRefs(12345))
}

func TestCycle126_ConvergenceSessionTick_Helpers(t *testing.T) {
	// 1. envLookup
	assert.Empty(t, envLookup(nil, "KEY"))
	jobWithoutEnv := &ScheduledJob{ID: "J1"}
	assert.Empty(t, envLookup(jobWithoutEnv, "KEY"))
	jobWithEnv := &ScheduledJob{
		ID: "J2",
		EnvironmentVariables: map[string]string{
			"TEST_KEY": "val1",
		},
	}
	assert.Equal(t, "val1", envLookup(jobWithEnv, "TEST_KEY"))
	assert.Empty(t, envLookup(jobWithEnv, "MISSING_KEY"))

	// 2. mergeStringAnyMaps
	assert.Empty(t, mergeStringAnyMaps(nil, nil))
	m1 := map[string]any{"a": 1, "b": 2}
	m2 := map[string]any{"b": 20, "c": 30}
	merged := mergeStringAnyMaps(m1, m2)
	assert.Equal(t, 1, merged["a"])
	assert.Equal(t, 20, merged["b"])
	assert.Equal(t, 30, merged["c"])

	// 3. truncateOrchestrateOutputPreview
	assert.Equal(t, "", truncateOrchestrateOutputPreview("", 10))
	assert.Equal(t, "short", truncateOrchestrateOutputPreview("short", 10))
	longStr := "1234567890abcdefghij"
	trunc := truncateOrchestrateOutputPreview(longStr, 10)
	assert.Equal(t, "1234567890…", trunc)

	// 4. thresholdMaxTicksPerHour
	assert.Equal(t, 4, thresholdMaxTicksPerHour(nil))
	assert.Equal(t, 42, thresholdMaxTicksPerHour(map[string]any{"max_ticks_per_hour": 42}))
	assert.Equal(t, 4, thresholdMaxTicksPerHour(map[string]any{"max_ticks_per_hour": "bad"}))

	// 5. countMeasureTicksInLastHour
	now := time.Now()
	assert.Equal(t, 0, countMeasureTicksInLastHour(nil, now))
	activity := []any{
		map[string]any{
			"action":    "measure_test_bundle_health",
			"timestamp": now.Add(-10 * time.Minute).Format(time.RFC3339),
		},
		map[string]any{
			"action":    "measure_test_bundle_health",
			"timestamp": now.Add(-2 * time.Hour).Format(time.RFC3339),
		},
		map[string]any{
			"action":    "other_action",
			"timestamp": now.Add(-5 * time.Minute).Format(time.RFC3339),
		},
		map[string]any{
			"action":    "measure_test_bundle_health",
			"timestamp": now.Add(-5 * time.Minute).Format(time.RFC3339),
		},
	}
	assert.Equal(t, 2, countMeasureTicksInLastHour(activity, now))
}

func TestCycle126_JobStateRegistry_HelpersAndLeases(t *testing.T) {
	tempDir, _ := setupCycle126IsolatedEnv(t)
	now := time.Now()

	// 1. JobExecutionLease.IsValid
	var nilLease *JobExecutionLease
	assert.False(t, nilLease.IsValid(now))

	zeroLease := &JobExecutionLease{}
	assert.True(t, zeroLease.IsValid(now))

	validLease := &JobExecutionLease{ExpiresAt: now.Add(10 * time.Minute)}
	assert.True(t, validLease.IsValid(now))

	expiredLease := &JobExecutionLease{ExpiresAt: now.Add(-10 * time.Minute)}
	assert.False(t, expiredLease.IsValid(now))

	// 2. sanitizeJobIDForPathSegment
	assert.Equal(t, "job:test_1", sanitizeJobIDForPathSegment("job:test/1"))
	assert.Equal(t, "normal-job", sanitizeJobIDForPathSegment("normal-job"))

	// 3. isReservedStateEntry
	assert.True(t, isReservedStateEntry(".job_state.index"))
	assert.True(t, isReservedStateEntry("_README.yaml"))
	assert.True(t, isReservedStateEntry(".hidden"))
	assert.False(t, isReservedStateEntry("active.lock"))
	assert.False(t, isReservedStateEntry("regular_job.yaml"))

	// 4. cloneJobExecutionState
	assert.Nil(t, cloneJobExecutionState(nil))
	completedAt := now
	origState := &JobExecutionState{
		JobID:       "JOB-1",
		ExecutionID: "EXEC-1",
		State:       "completed",
		StartedAt:   now.Add(-time.Hour),
		CompletedAt: &completedAt,
	}
	cloned := cloneJobExecutionState(origState)
	assert.Equal(t, origState.JobID, cloned.JobID)
	assert.Equal(t, origState.ExecutionID, cloned.ExecutionID)
	assert.Equal(t, *origState.CompletedAt, *cloned.CompletedAt)

	// 5. JobStateRegistry lease registration and clean
	reg := NewJobStateRegistry(tempDir).(*JobStateRegistry)
	lease, err := reg.RegisterExecutionWithLease("JOB-lease-1", "EXEC-1", os.Getpid(), 60)
	require.NoError(t, err)
	assert.NotNil(t, lease)
	assert.True(t, lease.IsValid(time.Now()))

	activeLease, err := reg.GetActiveLease("JOB-lease-1")
	require.NoError(t, err)
	assert.NotNil(t, activeLease)
	assert.Equal(t, "EXEC-1", activeLease.ExecutionID)

	cleaned, err := reg.CleanStaleLocks(0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, cleaned, 0)
}

func TestCycle126_Watchdog_HeartbeatAndRecovery(t *testing.T) {
	tempDir, _ := setupCycle126IsolatedEnv(t)
	now := time.Now()

	// 1. HeartbeatTracker
	tracker := NewHeartbeatTracker()
	meta := WorkerMeta{
		Kind: "worker_test",
	}
	err := tracker.Register("W1", meta)
	require.NoError(t, err)

	w, found := tracker.GetWorker("W1")
	assert.True(t, found)
	assert.Equal(t, "W1", w.ID)

	err = tracker.Ping("W1")
	require.NoError(t, err)

	err = tracker.Ping("W_MISSING")
	assert.Error(t, err)

	tracker.MarkStalled("W1")
	wStalled, _ := tracker.GetWorker("W1")
	assert.Equal(t, WorkerStateStalled, wStalled.State)

	staleWorkers := tracker.GetStaleWorkers(now.Add(10 * time.Minute))
	assert.Len(t, staleWorkers, 1)

	workers := tracker.ListWorkers()
	assert.Len(t, workers, 1)

	tracker.Unregister("W1")
	_, foundAfterUnreg := tracker.GetWorker("W1")
	assert.False(t, foundAfterUnreg)

	tracker.Reset()
	assert.Empty(t, tracker.ListWorkers())

	// 2. WatchdogInterceptor
	interceptor := NewWatchdogInterceptor(tracker, tempDir, time.Second)
	stalledNotified := false
	interceptor.OnStalledWorker = func(status WorkerStatus) {
		stalledNotified = true
	}

	err = tracker.Register("W2", WorkerMeta{Kind: "stalled_test"})
	require.NoError(t, err)

	interceptor.SweepAndRecover(context.Background())
	assert.False(t, stalledNotified)
}

func TestCycle126_Retention_ToleranceHelpers(t *testing.T) {
	// 1. getBatchConfig
	bSize, maxB := getBatchConfig(nil)
	assert.Equal(t, defaultRetentionToleranceBatchSize, bSize)
	assert.Equal(t, defaultRetentionToleranceMaxBatches, maxB)

	customJob := &ScheduledJob{
		EnvironmentVariables: map[string]string{
			EnvKeyBatchSize:  "25",
			EnvKeyMaxBatches: "4",
		},
	}
	bSize2, maxB2 := getBatchConfig(customJob)
	assert.Equal(t, 25, bSize2)
	assert.Equal(t, 4, maxB2)

	// 2. getBulkDeleteWorkers
	assert.Equal(t, defaultBulkDeleteWorkers, getBulkDeleteWorkers(nil))
	workersJob := &ScheduledJob{
		EnvironmentVariables: map[string]string{
			EnvKeyBulkDeleteWorkers: "8",
		},
	}
	assert.Equal(t, 8, getBulkDeleteWorkers(workersJob))

	// 3. retentionKindPriority
	assert.Equal(t, 0, retentionKindPriority("audit_event"))
	assert.Equal(t, 1, retentionKindPriority("mcp_session"))
	assert.Equal(t, 3, retentionKindPriority("unknown_kind"))

	// 4. getKindFilter
	assert.Nil(t, getKindFilter(nil))
	filterJob := &ScheduledJob{
		EnvironmentVariables: map[string]string{
			EnvKeyKinds: "audit_event, log_event",
		},
	}
	kinds := getKindFilter(filterJob)
	assert.True(t, kinds["audit_event"])
	assert.True(t, kinds["log_event"])

	// 5. normalizeRetentionBatchLimits
	nSize, nMax := normalizeRetentionBatchLimits(0, -1)
	assert.Equal(t, defaultRetentionToleranceBatchSize, nSize)
	assert.Equal(t, retentionToleranceUnlimitedMaxBatches, nMax)

	// 6. buildStatusExclusionFilter
	filter := buildStatusExclusionFilter([]string{"active", "running"})
	assert.NotNil(t, filter)

	// 7. isCatalogKind, isHighVolumeKind, skipArchiveForOldestIDsPath
	assert.True(t, isCatalogKind(objects.KindGlossaryTerm))
	assert.False(t, isCatalogKind(objects.KindAuditEvent))

	assert.True(t, isHighVolumeKind(objects.KindAuditEvent))
	assert.False(t, isHighVolumeKind(objects.KindGoal))

	assert.True(t, skipArchiveForOldestIDsPath(objects.KindAuditEvent, nil))
	assert.False(t, skipArchiveForOldestIDsPath(objects.KindAuditEvent, []string{"protected"}))
}

func TestCycle126_ActivityCache_Deep(t *testing.T) {
	tempDir, _ := setupCycle126IsolatedEnv(t)

	// 1. activityCacheableJobID
	assert.True(t, activityCacheableJobID("job_normal"))
	longJobID := strings.Repeat("x", maxSchedulerJobIDLenForCache+1)
	assert.False(t, activityCacheableJobID(longJobID))

	// 2. ActivityCache operations
	cache := NewActivityCache().(*ActivityCache)
	defer cache.Stop()

	cache.UpdateEvent("JOB-A", "scheduler_job_started", 0, nil)
	cache.UpdateEvent("JOB-A", "scheduler_job_completed", 100*time.Millisecond, nil)
	cache.UpdateEvent("JOB-A", "scheduler_job_failed", 200*time.Millisecond, fmt.Errorf("sample error"))
	cache.UpdateEvent("JOB-B", "scheduler_job_started", 0, nil)

	entryA, foundA := cache.GetEntry("JOB-A")
	assert.True(t, foundA)
	assert.Equal(t, "JOB-A", entryA.JobID)
	assert.Equal(t, 1, entryA.TotalStarted)
	assert.Equal(t, 1, entryA.TotalCompleted)
	assert.Equal(t, 1, entryA.TotalFailed)
	assert.Equal(t, "sample error", entryA.LastError)

	allEntries := cache.GetAllEntries()
	assert.Len(t, allEntries, 2)

	// 3. doSaveCache and LoadCache
	err := cache.doSaveCache(tempDir)
	require.NoError(t, err)

	newCache := NewActivityCache().(*ActivityCache)
	defer newCache.Stop()
	err = newCache.LoadCache(tempDir)
	require.NoError(t, err)

	loadedA, foundLoadedA := newCache.GetEntry("JOB-A")
	assert.True(t, foundLoadedA)
	assert.Equal(t, entryA.TotalCompleted, loadedA.TotalCompleted)
}

func TestCycle126_HandlersCapDispatch_ResumePendingVerificationTasks(t *testing.T) {
	tempDir, sp := setupCycle126IsolatedEnv(t)
	ctx := pkgctx.WithPromoteOnCreate(context.Background())

	h := &CapOrchestratorHandler{
		storage:     sp,
		projectRoot: tempDir,
		logger:      logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}

	// 1. With no pending verification tasks
	err := h.resumePendingVerificationTasks(ctx, "echo", "PRI-100")
	require.NoError(t, err)

	// 2. With a pending verification task but no commit evidence
	secCtx := pkgctx.NewSystemSecurityContext()
	taskObj := map[string]any{
		objects.FieldKeyID:              "ATK-test-1",
		objects.FieldKeyKind:            objects.KindAgentTask,
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:           "Test Task",
		objects.FieldKeyDescription:     "Test task description for verification",
		objects.FieldKeyStatus:          objects.ObjectStatusPendingVerification,
		objects.FieldKeyPriorityPlanRef: "PRI-100",
	}
	err = sp.Create(ctx, secCtx, taskObj)
	require.NoError(t, err)

	err = h.resumePendingVerificationTasks(ctx, "echo", "PRI-100")
	assert.Error(t, err)
	if err != nil {
		assert.Contains(t, err.Error(), "has no durable commit evidence")
	}
}

func TestCycle126_HandlersCapStageGates_StateFiles(t *testing.T) {
	tempDir, sp := setupCycle126IsolatedEnv(t)
	h := &CapOrchestratorHandler{
		storage:     sp,
		projectRoot: tempDir,
	}

	assert.False(t, h.stateFileExists("custom_state.json"))
	assert.False(t, h.stateFileNonEmpty("custom_state.json"))
	assert.False(t, h.stateFileFresh("custom_state.json", "1h"))

	h.writeStateFile("custom_state.json", map[string]any{"key": "value"})
	assert.True(t, h.stateFileExists("custom_state.json"))
	assert.True(t, h.stateFileNonEmpty("custom_state.json"))
	assert.True(t, h.stateFileFresh("custom_state.json", "1h"))

	data, err := h.readStateFile("custom_state.json")
	require.NoError(t, err)
	m, ok := data.(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "value", m["key"])
}

func TestCycle126_HandlersCapStageGates_MaybeAdvance(t *testing.T) {
	tempDir, sp := setupCycle126IsolatedEnv(t)
	ctx := context.Background()
	h := &CapOrchestratorHandler{
		storage:     sp,
		projectRoot: tempDir,
		logger:      logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}

	// 1. Non-CAP stage returns nil
	err := h.maybeAdvanceCAPStage(ctx, "regular_stage")
	require.NoError(t, err)

	// 2. Unbound stage returns hold error
	err = h.maybeAdvanceCAPStage(ctx, "cap_stage_planning")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unbound parent convergence_session")
}

func TestCycle126_JobStateRegistry_LifecycleTransitions(t *testing.T) {
	tempDir, _ := setupCycle126IsolatedEnv(t)
	reg := NewJobStateRegistry(tempDir).(*JobStateRegistry)

	// 1. RegisterExecution
	err := reg.RegisterExecution("JOB-lifecycle-1", "EXEC-1", os.Getpid())
	require.NoError(t, err)

	// 2. GetExecutionState
	state, err := reg.GetExecutionState("JOB-lifecycle-1")
	require.NoError(t, err)
	assert.NotNil(t, state)
	assert.Equal(t, "in_progress", state.State)

	// 3. ListInProgress
	inProgress, err := reg.ListInProgress()
	require.NoError(t, err)
	assert.NotEmpty(t, inProgress)

	// 4. CompleteExecution
	err = reg.CompleteExecution("JOB-lifecycle-1", "EXEC-1", "completed")
	require.NoError(t, err)

	stateCompleted, err := reg.GetExecutionState("JOB-lifecycle-1")
	require.NoError(t, err)
	assert.Nil(t, stateCompleted) // Completed job is no longer active in progress/deferred

	// 5. DeferExecution
	err = reg.RegisterExecution("JOB-lifecycle-2", "EXEC-2", os.Getpid())
	require.NoError(t, err)

	deferTime := time.Now().Add(time.Hour)
	err = reg.DeferExecution("JOB-lifecycle-2", "maintenance", &deferTime)
	require.NoError(t, err)

	stateDeferred, err := reg.GetExecutionState("JOB-lifecycle-2")
	require.NoError(t, err)
	require.NotNil(t, stateDeferred)
	assert.Equal(t, "deferred", stateDeferred.State)

	// 6. Summarize
	summary, err := reg.Summarize(time.Hour)
	require.NoError(t, err)
	assert.NotNil(t, summary)
}
