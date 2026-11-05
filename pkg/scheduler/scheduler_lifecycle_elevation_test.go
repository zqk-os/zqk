package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestSchedulerLifecycle_TriggerParsingAndValidation tests trigger type parsing and requirement validation.
func TestSchedulerLifecycle_TriggerParsingAndValidation(t *testing.T) {
	t.Parallel()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// 1. ParseTriggerType
	tests := []struct {
		input string
		want  string
	}{
		{TriggerTypeTimer, TriggerTypeTimer},
		{TriggerTypeManual, TriggerTypeManual},
		{TriggerTypeImmediate, TriggerTypeImmediate},
		{TriggerTypeWorkflow, TriggerTypeWorkflow},
		{TriggerTypeEvent, TriggerTypeEvent},
		{TriggerTypeLifecycle, TriggerTypeLifecycle},
		{"", "timer"},
		{"unrecognized_trigger", "timer"},
	}

	for _, tt := range tests {
		got := ParseTriggerType(tt.input, logger, "SCH-TEST-PARSE")
		assert.Equal(t, tt.want, got, "input=%q", tt.input)
	}

	// 2. ValidateTriggerRequirements table
	valTests := []struct {
		name        string
		trigger     string
		sched       string
		workflow    string
		event       string
		lifecycle   string
		expectValid bool
		reasonSub   string
	}{
		{
			name:        "timer valid",
			trigger:     "timer",
			sched:       "0 * * * *",
			expectValid: true,
		},
		{
			name:        "timer missing expr",
			trigger:     "timer",
			sched:       "",
			expectValid: false,
			reasonSub:   "schedule_expression",
		},
		{
			name:        "workflow valid",
			trigger:     TriggerTypeWorkflow,
			workflow:    "WFL-001",
			expectValid: true,
		},
		{
			name:        "workflow missing ref",
			trigger:     TriggerTypeWorkflow,
			workflow:    "",
			expectValid: false,
			reasonSub:   "workflow_ref",
		},
		{
			name:        "event valid",
			trigger:     "event",
			event:       "object.created",
			expectValid: true,
		},
		{
			name:        "event missing filter",
			trigger:     "event",
			event:       "",
			expectValid: false,
			reasonSub:   "event_filter",
		},
		{
			name:        "lifecycle valid",
			trigger:     TriggerTypeLifecycle,
			lifecycle:   "status == completed",
			expectValid: true,
		},
		{
			name:        "lifecycle missing filter",
			trigger:     TriggerTypeLifecycle,
			lifecycle:   "",
			expectValid: false,
			reasonSub:   "lifecycle_filter",
		},
		{
			name:        "manual trigger",
			trigger:     "manual",
			expectValid: true,
		},
		{
			name:        "immediate trigger",
			trigger:     "immediate",
			expectValid: true,
		},
		{
			name:        "unknown trigger",
			trigger:     "bogus_trigger",
			expectValid: false,
			reasonSub:   "unknown trigger type",
		},
	}

	for _, vt := range valTests {
		res := ValidateTriggerRequirements(
			vt.trigger,
			vt.sched,
			vt.workflow,
			vt.event,
			vt.lifecycle,
			logger,
			"SCH-REQ-TEST",
			nil,
		)
		assert.Equal(t, vt.expectValid, res.Valid, "case %s valid match", vt.name)
		if !vt.expectValid {
			assert.Contains(t, res.Reason, vt.reasonSub, "case %s reason match", vt.name)
		}
	}
}

// TestSchedulerLifecycle_TriggerQueueHelpers tests queue filtering and classification heuristics.
func TestSchedulerLifecycle_TriggerQueueHelpers(t *testing.T) {
	t.Parallel()

	// 1. extractJobIDs
	reqs := []JobTriggerRequest{
		{JobID: "SCH-1"},
		{JobID: "SCH-2"},
		{JobID: "SCH-3"},
	}
	ids := extractJobIDs(reqs)
	assert.Equal(t, []string{"SCH-1", "SCH-2", "SCH-3"}, ids)

	// 2. batchContainsTestBundleJobID
	assert.True(t, batchContainsTestBundleJobID([]JobTriggerRequest{{JobID: TestBundleJobIDPrefix + "sample"}}))
	assert.False(t, batchContainsTestBundleJobID([]JobTriggerRequest{{JobID: "regular_job"}}))

	// 3. jobIDLooksLikeCrossProcessTimestampTestRunner
	assert.True(t, jobIDLooksLikeCrossProcessTimestampTestRunner("SCH-1234567890"))
	assert.False(t, jobIDLooksLikeCrossProcessTimestampTestRunner("SCH-short"))

	// 4. jobIDLooksLikeCLISubmitNanos
	assert.True(t, jobIDLooksLikeCLISubmitNanos("SCH-1234567890123456"))
	assert.False(t, jobIDLooksLikeCLISubmitNanos("other-job"))

	// 5. jobIDAllDigitSuffixLen
	assert.True(t, jobIDAllDigitSuffixLen("SCH-12345", 3, 6))
	assert.False(t, jobIDAllDigitSuffixLen("SCH-123a", 3, 6))
	assert.False(t, jobIDAllDigitSuffixLen("SCH-12", 3, 6))

	// 6. triggerWarrantsCacheMissRetry
	assert.True(t, triggerWarrantsCacheMissRetry(JobTriggerRequest{JobID: TestBundleJobIDPrefix + "testbundle"}))
	assert.True(t, triggerWarrantsCacheMissRetry(JobTriggerRequest{TriggerOrigin: TriggerOriginCLISubmit}))
	assert.False(t, triggerWarrantsCacheMissRetry(JobTriggerRequest{JobID: "standard_background_timer"}))

	// 7. triggerIsCLIOneShot
	assert.True(t, triggerIsCLIOneShot(JobTriggerRequest{TriggerOrigin: TriggerOriginCLISubmit}))
	assert.False(t, triggerIsCLIOneShot(JobTriggerRequest{TriggerOrigin: "cron"}))

	// 8. jobIDLooksLikeCASInstanceSchedulerJobID
	assert.True(t, jobIDLooksLikeCASInstanceSchedulerJobID("SCH-1775021610902509000-965bf042"))
	assert.False(t, jobIDLooksLikeCASInstanceSchedulerJobID("short"))

	// 9. prioritizeCachePrewarmTriggers
	mixed := []JobTriggerRequest{
		{JobID: "regular-1"},
		{JobID: DefaultCachePrewarmJobID},
		{JobID: "regular-2"},
	}
	prioritized := prioritizeCachePrewarmTriggers(mixed)
	require.Len(t, prioritized, 3)
	assert.Equal(t, DefaultCachePrewarmJobID, prioritized[0].JobID)

	// 10. batchNeedsCASReconcileBeforeTriggerReload
	assert.True(t, batchNeedsCASReconcileBeforeTriggerReload([]JobTriggerRequest{
		{JobID: "SCH-1775021610902509000-965bf042"},
	}))
	assert.False(t, batchNeedsCASReconcileBeforeTriggerReload([]JobTriggerRequest{
		{JobID: "plain-job"},
	}))
}

// TestSchedulerLifecycle_TriggerQueueDiskPersistence tests enqueue, peek, dequeue, and lifecycle triggers.
func TestSchedulerLifecycle_TriggerQueueDiskPersistence(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	qInterface := NewJobTriggerQueue(tmpDir)
	q, ok := qInterface.(*JobTriggerQueue)
	require.True(t, ok)

	// Check CAS debounce logic
	q.SetCASDebounceInterval(20 * time.Millisecond)
	assert.True(t, q.shouldReconcileCASForBatch(true, true))
	assert.False(t, q.shouldReconcileCASForBatch(false, false))
	q.markCASReconciled()
	// Immediately after markCASReconciled, without missing job or needsCAS it remains false
	assert.False(t, q.shouldReconcileCASForBatch(false, true))

	// 1. EnqueueTriggerRequest
	err := q.EnqueueTriggerRequest("SCH-ENQUEUE-1")
	require.NoError(t, err)

	// 2. EnqueueTriggerRequestWithOrigin
	err = q.EnqueueTriggerRequestWithOrigin("SCH-ENQUEUE-2", "pre_commit")
	require.NoError(t, err)

	// 3. EnqueueTriggerRequests (batch)
	err = q.EnqueueTriggerRequests([]string{"SCH-ENQUEUE-3", "SCH-ENQUEUE-4"}, "integration_suite")
	require.NoError(t, err)

	// 4. EnqueueLifecycleTrigger
	err = q.EnqueueLifecycleTrigger("backlog_item", "draft", "in_progress", map[string]any{"id": "BLI-1"})
	require.NoError(t, err)

	// 5. PeekTriggerRequests
	peeked, err := q.PeekTriggerRequests()
	require.NoError(t, err)
	assert.Len(t, peeked, 5)

	// 6. HasPendingTriggerWithOrigin
	hasPending, err := q.HasPendingTriggerWithOrigin("SCH-ENQUEUE-2", "pre_commit")
	require.NoError(t, err)
	assert.True(t, hasPending)

	hasPendingWrong, err := q.HasPendingTriggerWithOrigin("SCH-ENQUEUE-2", "different_origin")
	require.NoError(t, err)
	assert.False(t, hasPendingWrong)

	// 7. DequeueTriggerRequests limit
	dequeuedPart, err := q.DequeueTriggerRequests(2)
	require.NoError(t, err)
	assert.Len(t, dequeuedPart, 2)
	assert.Equal(t, "SCH-ENQUEUE-1", dequeuedPart[0].JobID)
	assert.Equal(t, "SCH-ENQUEUE-2", dequeuedPart[1].JobID)

	// Dequeue remainder
	dequeuedRest, err := q.DequeueTriggerRequests(10)
	require.NoError(t, err)
	assert.Len(t, dequeuedRest, 3)

	// Empty peek
	peekEmpty, err := q.PeekTriggerRequests()
	require.NoError(t, err)
	assert.Empty(t, peekEmpty)
}

// TestSchedulerLifecycle_CancellationTokens verifies context cancellation binding and propagation.
func TestSchedulerLifecycle_CancellationTokens(t *testing.T) {
	t.Parallel()

	job := &ScheduledJob{
		ID: "SCH-CANC-001",
	}

	assert.Nil(t, job.ExecutionCancel())

	ctx, cancel := context.WithCancel(context.Background())
	job.BindExecutionContext(ctx, cancel)

	require.NotNil(t, job.ExecutionCancel())

	// Trigger cancellation via job handle
	job.ExecutionCancel()()

	assert.ErrorIs(t, ctx.Err(), context.Canceled)

	// Clearing execution context resets cancel func
	job.ClearExecutionContext()
	assert.Nil(t, job.ExecutionCancel())
}

// TestSchedulerLifecycle_WatchdogRecoveryStalledCancellation tests worker stalling recovery.
func TestSchedulerLifecycle_WatchdogRecoveryStalledCancellation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tracker := NewHeartbeatTracker()

	ctx, cancel := context.WithCancel(context.Background())
	workerID := "worker-stalled-1"
	err := tracker.Register(workerID, WorkerMeta{
		Kind:           "agent-worker",
		Interval:       10 * time.Millisecond,
		StaleThreshold: 20 * time.Millisecond,
		CancelFunc:     cancel,
	})
	require.NoError(t, err)

	// Artificially age the worker
	tracker.mu.Lock()
	if ws, exists := tracker.workers[workerID]; exists {
		ws.LastPingAt = time.Now().Add(-1 * time.Minute)
	}
	tracker.mu.Unlock()

	stalledReceived := false
	watchdog := NewWatchdogInterceptor(tracker, tmpDir, 50*time.Millisecond)
	watchdog.OnStalledWorker = func(w WorkerStatus) {
		if w.ID == workerID {
			stalledReceived = true
		}
	}

	watchdog.SweepAndRecover(context.Background())

	assert.True(t, stalledReceived, "stalled callback must be called")
	assert.ErrorIs(t, ctx.Err(), context.Canceled, "worker context must be cancelled by watchdog")

	// Worker should now be marked stalled
	ws, found := tracker.GetWorker(workerID)
	assert.True(t, found)
	assert.Equal(t, WorkerStateStalled, ws.State)

	// Ping restores healthy
	err = tracker.Ping(workerID)
	require.NoError(t, err)
	wsHealthy, _ := tracker.GetWorker(workerID)
	assert.Equal(t, WorkerStateHealthy, wsHealthy.State)

	// Unregister removes worker
	tracker.Unregister(workerID)
	_, foundAfter := tracker.GetWorker(workerID)
	assert.False(t, foundAfter)
}

// TestSchedulerLifecycle_JobStateRegistryTransitions tests state machine transitions and leases.
func TestSchedulerLifecycle_JobStateRegistryTransitions(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	regInterface := NewJobStateRegistry(tmpDir)
	reg, ok := regInterface.(*JobStateRegistry)
	require.True(t, ok)

	jobID := "SCH-STATE-TRANS-01"
	execID := "EXEC-100"

	// 1. Initial lookup is nil
	initialState, err := reg.GetExecutionState(jobID)
	require.NoError(t, err)
	assert.Nil(t, initialState)

	// 2. RegisterExecution
	err = reg.RegisterExecution(jobID, execID, os.Getpid())
	require.NoError(t, err)

	inProgState, err := reg.GetExecutionState(jobID)
	require.NoError(t, err)
	require.NotNil(t, inProgState)
	assert.Equal(t, jobExecutionStateInProgress, inProgState.State)
	assert.Equal(t, execID, inProgState.ExecutionID)

	inProgressList, err := reg.ListInProgress()
	require.NoError(t, err)
	assert.NotEmpty(t, inProgressList)

	// 3. RegisterExecutionWithLease
	leaseJobID := "SCH-LEASED-JOB"
	leaseExecID := "EXEC-200"
	lease, err := reg.RegisterExecutionWithLease(leaseJobID, leaseExecID, os.Getpid(), 120)
	require.NoError(t, err)
	require.NotNil(t, lease)
	assert.True(t, lease.IsValid(time.Now()))
	assert.False(t, lease.IsValid(time.Now().Add(10*time.Hour)))

	activeLease, err := reg.GetActiveLease(leaseJobID)
	require.NoError(t, err)
	require.NotNil(t, activeLease)
	assert.Equal(t, leaseExecID, activeLease.ExecutionID)

	// 4. CompleteExecution
	err = reg.CompleteExecution(jobID, execID, "completed")
	require.NoError(t, err)

	completedState, err := reg.GetExecutionState(jobID)
	require.NoError(t, err)
	assert.Nil(t, completedState, "completed job has no active in_progress execution")

	// 5. DeferExecution
	deferJobID := "SCH-DEFERRED-JOB"
	err = reg.RegisterExecution(deferJobID, "EXEC-DEF-1", os.Getpid())
	require.NoError(t, err)

	deferUntil := time.Now().Add(30 * time.Minute)
	err = reg.DeferExecution(deferJobID, "backpressure capacity reached", &deferUntil)
	require.NoError(t, err)

	defState, err := reg.GetExecutionState(deferJobID)
	require.NoError(t, err)
	require.NotNil(t, defState)
	assert.Equal(t, jobExecutionStateDeferred, defState.State)
	assert.Equal(t, "backpressure capacity reached", defState.PolicyReason)

	// 6. Summarize
	summary, err := reg.Summarize(5 * time.Minute)
	require.NoError(t, err)
	require.NotNil(t, summary)
	assert.GreaterOrEqual(t, summary.TotalFiles, 2)

	// 7. Sanitization helpers
	assert.Equal(t, "job:test_123", sanitizeJobIDForPathSegment("job:test/123"))
	assert.True(t, isReservedStateEntry(".state_hints.yaml"))
	assert.False(t, isReservedStateEntry("regular_job"))

	// 8. Lookup dirs
	dirs := jobStateDirsForLookup("/tmp/state", "SCH-LOOKUP-01")
	assert.Len(t, dirs, 2)
}

type mockCoordinationChannel struct {
	ch chan Event
}

func (m *mockCoordinationChannel) PublishEvent(ev Event) error {
	m.ch <- ev
	return nil
}

func (m *mockCoordinationChannel) Subscribe() <-chan Event {
	return m.ch
}

func (m *mockCoordinationChannel) WatchEvents(ctx context.Context) error {
	return nil
}

// TestSchedulerLifecycle_DriftCoordinationAndEventHandling tests drift detection and cancellation.
func TestSchedulerLifecycle_DriftCoordinationAndEventHandling(t *testing.T) {
	t.Parallel()

	coord := &mockCoordinationChannel{
		ch: make(chan Event, 16),
	}

	s := &Scheduler{
		coordinationChannel: coord,
		jobs:                make(map[string]*ScheduledJob),
		logger:              logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}

	jobID := "SCH-DRIFT-TARGET"
	job := &ScheduledJob{
		ID: jobID,
	}
	jobCtx, jobCancel := context.WithCancel(context.Background())
	job.BindExecutionContext(jobCtx, jobCancel)
	s.jobs[jobID] = job

	monitorCtx, monitorCancel := context.WithCancel(context.Background())
	defer monitorCancel()

	go s.handleCoordinationEvents(monitorCtx)

	// Publish drift detected
	driftEvent := Event{
		Type:      "drift_detected",
		Timestamp: time.Now(),
		JobID:     jobID,
		ProcessID: os.Getpid(),
	}
	err := coord.PublishEvent(driftEvent)
	require.NoError(t, err)

	// Assert context gets canceled within timeout
	assert.Eventually(t, func() bool {
		return jobCtx.Err() == context.Canceled
	}, 1*time.Second, 10*time.Millisecond, "job context should be canceled upon drift detection")

	// Publish other events (process_started, process_completed, process_errored) to exercise lines safely
	events := []Event{
		{Type: "process_started", Timestamp: time.Now(), JobID: "SUB-1", Metadata: map[string]any{"cmd": "run"}},
		{Type: "process_completed", Timestamp: time.Now(), JobID: "SUB-1", Metadata: map[string]any{"exit": 0}},
		{Type: "process_errored", Timestamp: time.Now(), JobID: "SUB-1", Metadata: map[string]any{"error": "timed out"}},
	}
	for _, ev := range events {
		err = coord.PublishEvent(ev)
		require.NoError(t, err)
	}
}

// TestSchedulerLifecycle_CleanStaleLocksByAge verifies cleanup of expired file locks.
func TestSchedulerLifecycle_CleanStaleLocksByAge(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, paths.ProjectDataDir, "locks")
	err := fileutil.MkdirAll(locksDir, paths.DirPerm755)
	require.NoError(t, err)

	// Fresh lock
	freshLock := filepath.Join(locksDir, "fresh.lock")
	err = fileutil.WriteFile(freshLock, []byte("fresh"), paths.FilePerm644)
	require.NoError(t, err)

	// Stale lock (modified 2 hours ago)
	staleLock := filepath.Join(locksDir, "stale.lock")
	err = fileutil.WriteFile(staleLock, []byte("stale"), paths.FilePerm644)
	require.NoError(t, err)
	twoHoursAgo := time.Now().Add(-2 * time.Hour)
	err = fileutil.Chtimes(staleLock, twoHoursAgo, twoHoursAgo)
	require.NoError(t, err)

	removed, err := CleanStaleLocksByAge(locksDir, 30*time.Minute)
	require.NoError(t, err)
	assert.Equal(t, 1, removed)

	// Stale lock should no longer exist
	_, err = fileutil.Stat(staleLock)
	assert.True(t, fileutil.IsNotExist(err))

	// Fresh lock should still exist
	_, err = fileutil.Stat(freshLock)
	assert.NoError(t, err)
}

// TestSchedulerLifecycle_PermissionsAndStopSafeguards tests scheduler permissions and stopping idempotency.
func TestSchedulerLifecycle_PermissionsAndStopSafeguards(t *testing.T) {
	t.Parallel()

	// 1. Permission checks
	s := &Scheduler{}
	assert.Error(t, s.checkPermission("manage:scheduler"), "nil secCtx must fail")

	s.secCtx = &pkgctx.SecurityContext{
		Roles: []string{"admin"},
	}
	assert.NoError(t, s.checkPermission("manage:scheduler"), "admin role grants access")

	s.secCtx = &pkgctx.SecurityContext{
		Permissions: []string{"*"},
	}
	assert.NoError(t, s.checkPermission("manage:scheduler"), "wildcard permission grants access")

	s.secCtx = &pkgctx.SecurityContext{
		Permissions: []string{"manage:*"},
	}
	assert.NoError(t, s.checkPermission("manage:scheduler"), "prefix wildcard grants access")

	s.secCtx = &pkgctx.SecurityContext{
		Permissions: []string{"read:scheduler_job"},
	}
	assert.Error(t, s.checkPermission("manage:scheduler"), "unauthorized permission fails")
	assert.NoError(t, s.CheckJobReadPermission())
	assert.Error(t, s.CheckJobWritePermission())

	// 2. IsRunning, Metrics snapshot, RunningJobIDs
	assert.False(t, s.IsRunning())
	snap := s.GetMetricsSnapshot()
	assert.Equal(t, int64(0), snap.Jobs.Executed)
	assert.Nil(t, s.GetRunningJobIDs())

	// 3. Stop on unstarted scheduler
	s.Stop()
	assert.False(t, s.IsRunning())
	// Calling Stop again should be safe and idempotent
	s.Stop()
	assert.False(t, s.IsRunning())
}
