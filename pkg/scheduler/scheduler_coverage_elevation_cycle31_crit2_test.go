package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/circuitbreaker"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// setupCycle31OperationalEnv initializes an isolated project root and storage provider.
func setupCycle31OperationalEnv(t *testing.T) (string, storagepkg.ObjectStorageProvider) {
	t.Helper()
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

// TestSchedulerCycle31_Operational_WorkerPool_ConcurrentDispatchAndBudget tests concurrent pool execution.
func TestSchedulerCycle31_Operational_WorkerPool_ConcurrentDispatchAndBudget(t *testing.T) {
	budget := goroutinelabels.NewBudget(goroutinelabels.BudgetConfig{MaxTotal: 20})
	pool := budget.NewPool("test_op_pool", "concurrent dispatch test", 4, 30)
	require.NotNil(t, pool)
	require.False(t, pool.IsFallback())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx)

	const taskCount = 16
	var completedCount int64
	var wg sync.WaitGroup

	for i := 0; i < taskCount; i++ {
		wg.Add(1)
		err := pool.Submit(ctx, func(runCtx context.Context) error {
			defer wg.Done()
			atomic.AddInt64(&completedCount, 1)
			return nil
		})
		require.NoError(t, err)
	}

	wg.Wait()
	assert.Equal(t, int64(taskCount), atomic.LoadInt64(&completedCount))

	workers, queued, queueCap := pool.QueuePressureSnapshot()
	assert.Equal(t, 4, workers)
	assert.Equal(t, 0, queued)
	assert.Equal(t, 30, queueCap)
	pool.Stop()
}

// TestSchedulerCycle31_Operational_WorkerPool_NonBlockingOverflowAndBackpressure tests pool backpressure.
func TestSchedulerCycle31_Operational_WorkerPool_NonBlockingOverflowAndBackpressure(t *testing.T) {
	budget := goroutinelabels.NewBudget(goroutinelabels.BudgetConfig{MaxTotal: 10})
	pool := budget.NewPool("test_overflow_pool", "overflow test", 1, 1)
	require.NotNil(t, pool)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx)

	gate := make(chan struct{})
	workerStarted := make(chan struct{})

	// Task 1 occupies the single worker
	err := pool.Submit(ctx, func(runCtx context.Context) error {
		close(workerStarted)
		<-gate
		return nil
	})
	require.NoError(t, err)
	<-workerStarted

	// Task 2 fills the single queue slot
	err = pool.SubmitNonBlocking(ctx, func(runCtx context.Context) error {
		return nil
	})
	require.NoError(t, err)

	// Task 3 must be rejected with ErrPoolFull
	err = pool.SubmitNonBlocking(ctx, func(runCtx context.Context) error {
		return nil
	})
	assert.ErrorIs(t, err, goroutinelabels.ErrPoolFull)

	close(gate)
	pool.Stop()
}

// TestSchedulerCycle31_Operational_WorkerPool_LifecycleStatesAndDrainOnShutdown tests pool lifecycle & stop.
func TestSchedulerCycle31_Operational_WorkerPool_LifecycleStatesAndDrainOnShutdown(t *testing.T) {
	budget := goroutinelabels.NewBudget(goroutinelabels.BudgetConfig{MaxTotal: 10})
	pool := budget.NewPool("test_drain_pool", "drain lifecycle test", 2, 10)
	require.NotNil(t, pool)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx)

	var executed int64
	err := pool.Submit(ctx, func(runCtx context.Context) error {
		atomic.AddInt64(&executed, 1)
		return nil
	})
	require.NoError(t, err)

	// Idempotent Stop
	pool.Stop()
	pool.Stop()

	// Submit after Stop must return an error or context canceled
	errPostStop := pool.Submit(context.Background(), func(runCtx context.Context) error {
		return nil
	})
	assert.Error(t, errPostStop)
}

// TestSchedulerCycle31_Operational_WorkerPool_PriorityDispatchClassification tests priority job categorization.
func TestSchedulerCycle31_Operational_WorkerPool_PriorityDispatchClassification(t *testing.T) {
	assert.False(t, priorityDispatchJob(nil))

	critTypeJob := &ScheduledJob{JobType: JobTypeAuditEventAggregation}
	assert.True(t, priorityDispatchJob(critTypeJob))

	highPrioJob := &ScheduledJob{Priority: JobPriorityHigh}
	assert.True(t, priorityDispatchJob(highPrioJob))

	criticalPrioJob := &ScheduledJob{Priority: JobPriorityCritical}
	assert.True(t, priorityDispatchJob(criticalPrioJob))

	manualImmediate := &ScheduledJob{
		TriggerType: TriggerTypeImmediate,
		Category:    CategoryManual,
	}
	assert.True(t, priorityDispatchJob(manualImmediate))

	userImmediate := &ScheduledJob{
		TriggerType: TriggerTypeImmediate,
		Category:    CategoryUser,
	}
	assert.True(t, priorityDispatchJob(userImmediate))

	callbackImmediate := &ScheduledJob{
		TriggerType:     TriggerTypeImmediate,
		Category:        CategorySystem,
		CallbackOnError: "https://callback.internal/err",
	}
	assert.True(t, priorityDispatchJob(callbackImmediate))

	timerMaintJob := &ScheduledJob{
		TriggerType: "timer",
		Category:    CategoryMaintenance,
	}
	assert.True(t, priorityDispatchJob(timerMaintJob))

	regularJob := &ScheduledJob{
		TriggerType: "timer",
		Category:    CategorySystem,
		Priority:    JobPriorityNormal,
	}
	assert.False(t, priorityDispatchJob(regularJob))
}

// TestSchedulerCycle31_Operational_WorkerPool_GlobalTestJobLimiter tests limiter acquire and release.
func TestSchedulerCycle31_Operational_WorkerPool_GlobalTestJobLimiter(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	limiter := circuitbreaker.NewConcurrencyLimiter(logger, 2*time.Second)

	s := &Scheduler{
		logger:                       logger,
		globalTestConcurrencyLimiter: limiter,
	}

	ctx := context.Background()
	err := s.acquireGlobalTestJobSlot(ctx)
	require.NoError(t, err)

	s.releaseGlobalTestJobSlot()

	// Verify nil scheduler / nil limiter safety
	var nilSched *Scheduler
	assert.NoError(t, nilSched.acquireGlobalTestJobSlot(ctx))
	nilSched.releaseGlobalTestJobSlot()

	emptySched := &Scheduler{}
	assert.NoError(t, emptySched.acquireGlobalTestJobSlot(ctx))
	emptySched.releaseGlobalTestJobSlot()
}

// TestSchedulerCycle31_Operational_TaskDispatcher_FIFOOrdering tests trigger queue FIFO dispatch.
func TestSchedulerCycle31_Operational_TaskDispatcher_FIFOOrdering(t *testing.T) {
	tempDir := t.TempDir()
	queue := NewJobTriggerQueue(tempDir)
	require.NotNil(t, queue)

	jobIDs := []string{"SCH-FIFO-001", "SCH-FIFO-002", "SCH-FIFO-003", "SCH-FIFO-004"}
	for _, id := range jobIDs {
		err := queue.EnqueueTriggerRequest(id)
		require.NoError(t, err)
	}

	// Dequeue batch 1: limit 2
	batch1, err := queue.DequeueTriggerRequests(2)
	require.NoError(t, err)
	require.Len(t, batch1, 2)
	assert.Equal(t, "SCH-FIFO-001", batch1[0].JobID)
	assert.Equal(t, "SCH-FIFO-002", batch1[1].JobID)

	// Dequeue batch 2: limit 2
	batch2, err := queue.DequeueTriggerRequests(2)
	require.NoError(t, err)
	require.Len(t, batch2, 2)
	assert.Equal(t, "SCH-FIFO-003", batch2[0].JobID)
	assert.Equal(t, "SCH-FIFO-004", batch2[1].JobID)

	// Dequeue batch 3: should be empty
	batch3, err := queue.DequeueTriggerRequests(2)
	require.NoError(t, err)
	assert.Empty(t, batch3)
}

// TestSchedulerCycle31_Operational_TaskDispatcher_PriorityQueueCachePrewarm tests cache prewarm prioritizing.
func TestSchedulerCycle31_Operational_TaskDispatcher_PriorityQueueCachePrewarm(t *testing.T) {
	// Empty and single-element edge cases
	assert.Empty(t, prioritizeCachePrewarmTriggers(nil))
	single := []JobTriggerRequest{{JobID: "SCH-normal"}}
	assert.Equal(t, single, prioritizeCachePrewarmTriggers(single))

	// Prewarm in the middle
	input := []JobTriggerRequest{
		{JobID: "SCH-alpha"},
		{JobID: DefaultCachePrewarmJobID},
		{JobID: "SCH-beta"},
	}
	output := prioritizeCachePrewarmTriggers(input)
	require.Len(t, output, 3)
	assert.Equal(t, DefaultCachePrewarmJobID, output[0].JobID)
	assert.Equal(t, "SCH-alpha", output[1].JobID)
	assert.Equal(t, "SCH-beta", output[2].JobID)

	// Prewarm absent
	noPrewarm := []JobTriggerRequest{
		{JobID: "SCH-one"},
		{JobID: "SCH-two"},
	}
	assert.Equal(t, noPrewarm, prioritizeCachePrewarmTriggers(noPrewarm))
}

// TestSchedulerCycle31_Operational_TaskDispatcher_OriginFiltering tests origin-targeted trigger queries.
func TestSchedulerCycle31_Operational_TaskDispatcher_OriginFiltering(t *testing.T) {
	tempDir := t.TempDir()
	queue := NewJobTriggerQueue(tempDir)
	require.NotNil(t, queue)

	targetJob := "SCH-ORIGIN-TARGET"
	origin := TriggerOriginPreCommit

	has, err := queue.HasPendingTriggerWithOrigin(targetJob, origin)
	require.NoError(t, err)
	assert.False(t, has)

	err = queue.EnqueueTriggerRequestWithOrigin(targetJob, origin)
	require.NoError(t, err)

	has, err = queue.HasPendingTriggerWithOrigin(targetJob, origin)
	require.NoError(t, err)
	assert.True(t, has)

	// Different origin must return false
	hasOtherOrigin, err := queue.HasPendingTriggerWithOrigin(targetJob, TriggerOriginCLISubmit)
	require.NoError(t, err)
	assert.False(t, hasOtherOrigin)

	// Different job must return false
	hasOtherJob, err := queue.HasPendingTriggerWithOrigin("SCH-OTHER", origin)
	require.NoError(t, err)
	assert.False(t, hasOtherJob)
}

// TestSchedulerCycle31_Operational_TaskDispatcher_CacheMissAndCASPredicates tests trigger classification helpers.
func TestSchedulerCycle31_Operational_TaskDispatcher_CacheMissAndCASPredicates(t *testing.T) {
	reqCLI := JobTriggerRequest{JobID: "SCH-any", TriggerOrigin: TriggerOriginCLISubmit}
	assert.True(t, triggerWarrantsCacheMissRetry(reqCLI))

	reqTimer := JobTriggerRequest{JobID: "SCH-any", TriggerOrigin: "timer"}
	assert.False(t, triggerWarrantsCacheMissRetry(reqTimer))

	cliReq := JobTriggerRequest{TriggerOrigin: TriggerOriginCLISubmit}
	assert.True(t, triggerIsCLIOneShot(cliReq))

	schedReq := JobTriggerRequest{TriggerOrigin: "timer"}
	assert.False(t, triggerIsCLIOneShot(schedReq))

	casBatch := []JobTriggerRequest{{JobID: "SCH-1775021610902509000-965bf042"}}
	assert.True(t, batchNeedsCASReconcileBeforeTriggerReload(casBatch))

	normalBatch := []JobTriggerRequest{{JobID: "SCH-001"}}
	assert.False(t, batchNeedsCASReconcileBeforeTriggerReload(normalBatch))

	// isSilentStaleDrop ensures DefaultCachePrewarmJobID is never silently dropped
	assert.False(t, isSilentStaleTriggerDrop(DefaultCachePrewarmJobID))
}

// TestSchedulerCycle31_Operational_TaskDispatcher_RecordDispatchAttemptDropped tests dispatch pressure logs.
func TestSchedulerCycle31_Operational_TaskDispatcher_RecordDispatchAttemptDropped(t *testing.T) {
	tempDir := t.TempDir()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	s := &Scheduler{
		projectRoot: tempDir,
		logger:      logger,
	}

	job := &ScheduledJob{
		ID:          "SCH-PRESSURE-TEST",
		JobType:     JobTypeAuditEventAggregation,
		Category:    CategoryMaintenance,
		TriggerType: "timer",
	}

	s.recordDispatchAttemptDropped("test_source", "test_reason_timeout", job)

	logPath := filepath.Join(tempDir, paths.ProjectDataDir, paths.SchedulerSubdir, dispatchPressureJSONLFile)
	data, err := fileutil.ReadFile(logPath)
	require.NoError(t, err)

	var record map[string]any
	err = json.Unmarshal(data, &record)
	require.NoError(t, err)

	assert.Equal(t, "dispatch_pressure", record[objects.FieldKeyEventType])
	assert.Equal(t, "attempt_dropped", record["dispatch_subtype"])
	assert.Equal(t, "SCH-PRESSURE-TEST", record["job_id"])
	assert.Equal(t, "test_source", record[objects.FieldKeySource])
	assert.Equal(t, "test_reason_timeout", record[objects.FieldKeyReason])
}

// TestSchedulerCycle31_Operational_HeartbeatTracker_RegisterPingAndUnregister tests worker tracker mechanics.
func TestSchedulerCycle31_Operational_HeartbeatTracker_RegisterPingAndUnregister(t *testing.T) {
	tracker := NewHeartbeatTracker()
	require.NotNil(t, tracker)

	meta := WorkerMeta{
		Kind:           "agent_worker",
		Interval:       2 * time.Second,
		StaleThreshold: 10 * time.Second,
	}

	// Empty ID must fail
	errEmpty := tracker.Register("", meta)
	assert.Error(t, errEmpty)

	// Valid registration
	err := tracker.Register("worker-alpha", meta)
	require.NoError(t, err)

	// Duplicate registration must fail
	errDup := tracker.Register("worker-alpha", meta)
	assert.Error(t, errDup)

	status, ok := tracker.GetWorker("worker-alpha")
	require.True(t, ok)
	assert.Equal(t, "worker-alpha", status.ID)
	assert.Equal(t, WorkerStateHealthy, status.State)

	// Ping updates timestamp
	initialPing := status.LastPingAt
	time.Sleep(10 * time.Millisecond)
	err = tracker.Ping("worker-alpha")
	require.NoError(t, err)

	updatedStatus, _ := tracker.GetWorker("worker-alpha")
	assert.True(t, updatedStatus.LastPingAt.After(initialPing))

	// Unregister
	tracker.Unregister("worker-alpha")
	_, okAfterUnreg := tracker.GetWorker("worker-alpha")
	assert.False(t, okAfterUnreg)
}

// TestSchedulerCycle31_Operational_HeartbeatTracker_MissedHeartbeatAndStaleDetection tests stale worker checks.
func TestSchedulerCycle31_Operational_HeartbeatTracker_MissedHeartbeatAndStaleDetection(t *testing.T) {
	tracker := NewHeartbeatTracker()
	require.NotNil(t, tracker)

	meta := WorkerMeta{
		Kind:           "background_daemon",
		Interval:       1 * time.Second,
		StaleThreshold: 50 * time.Millisecond,
	}

	err := tracker.Register("fresh-worker", meta)
	require.NoError(t, err)
	err = tracker.Register("stale-worker", meta)
	require.NoError(t, err)

	// Simulate stale worker by evaluating relative to future time
	futureTime := time.Now().Add(200 * time.Millisecond)
	staleList := tracker.GetStaleWorkers(futureTime)
	require.Len(t, staleList, 2)

	// Mark stalled updates state and increments MissedCount
	tracker.MarkStalled("stale-worker")
	stalledStatus, ok := tracker.GetWorker("stale-worker")
	require.True(t, ok)
	assert.Equal(t, WorkerStateStalled, stalledStatus.State)
	assert.Equal(t, 1, stalledStatus.MissedCount)

	// Ping clears stalled state back to healthy
	err = tracker.Ping("stale-worker")
	require.NoError(t, err)
	healthyStatus, _ := tracker.GetWorker("stale-worker")
	assert.Equal(t, WorkerStateHealthy, healthyStatus.State)
}

// TestSchedulerCycle31_Operational_WatchdogInterceptor_SweepAndEviction tests watchdog sweep & cancellation.
func TestSchedulerCycle31_Operational_WatchdogInterceptor_SweepAndEviction(t *testing.T) {
	tracker := NewHeartbeatTracker()
	tempDir := t.TempDir()

	interceptor := NewWatchdogInterceptor(tracker, tempDir, 50*time.Millisecond)
	require.NotNil(t, interceptor)

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	meta := WorkerMeta{
		Kind:           "task_executor",
		Interval:       10 * time.Millisecond,
		StaleThreshold: 20 * time.Millisecond,
		CancelFunc:     workerCancel,
	}
	err := tracker.Register("stalled-agent-worker", meta)
	require.NoError(t, err)

	var callbackInvoked bool
	var callbackWorkerID string
	interceptor.OnStalledWorker = func(status WorkerStatus) {
		callbackInvoked = true
		callbackWorkerID = status.ID
	}

	time.Sleep(50 * time.Millisecond)
	interceptor.SweepAndRecover(context.Background())

	assert.True(t, callbackInvoked)
	assert.Equal(t, "stalled-agent-worker", callbackWorkerID)
	assert.ErrorIs(t, workerCtx.Err(), context.Canceled)

	status, ok := tracker.GetWorker("stalled-agent-worker")
	require.True(t, ok)
	assert.Equal(t, WorkerStateStalled, status.State)
}

// TestSchedulerCycle31_Operational_SwarmWorker_ResolveClaimant tests claimant mapping and fallback.
func TestSchedulerCycle31_Operational_SwarmWorker_ResolveClaimant(t *testing.T) {
	tempDir := t.TempDir()

	seats := map[string]agentfeed.PeerSeatRecord{
		"seat-orchestrator-1": {
			Wake:       agentfeed.WakeMembraneAgentAPI,
			PersonaRef: objects.ConstPersonaOrchestratorAlpha,
		},
	}
	err := agentfeed.SavePeerSeats(tempDir, agentfeed.PeerSeatsFile{
		SchemaVersion: "1",
		Seats:         seats,
	})
	require.NoError(t, err)

	// Case 1: Unassigned persona falls back to swarmSchedulerClaimant
	unassignedTask := map[string]any{}
	claimant1 := resolveSwarmClaimant(tempDir, unassignedTask)
	assert.Equal(t, swarmSchedulerClaimant, claimant1)

	// Case 2: Matching persona maps to seated peer
	assignedTask := map[string]any{
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaOrchestratorAlpha,
	}
	claimant2 := resolveSwarmClaimant(tempDir, assignedTask)
	assert.Equal(t, "seat-orchestrator-1", claimant2)

	// Case 3: Persona without assigned seat skips lease (returns empty string)
	unseatedTask := map[string]any{
		objects.FieldKeyAssigneePersonaRef: "PER-UNKNOWN-RESEARCHER",
	}
	claimant3 := resolveSwarmClaimant(tempDir, unseatedTask)
	assert.Empty(t, claimant3)
}

// TestSchedulerCycle31_Operational_SwarmWorker_OrphanTaskRecovery tests recovering in-progress tasks.
func TestSchedulerCycle31_Operational_SwarmWorker_OrphanTaskRecovery(t *testing.T) {
	// Status transition predicate checks
	nextStatus, canRecover := orphanRecoveryStatus(objects.ObjectStatusInProgress)
	assert.True(t, canRecover)
	assert.Equal(t, objects.ObjectStatusApproved, nextStatus)

	nextPending, canRecoverPending := orphanRecoveryStatus(objects.ObjectStatusPendingVerification)
	assert.False(t, canRecoverPending)
	assert.Empty(t, nextPending)

	mockStore := &mockSwarmStorage{
		task: map[string]any{
			objects.FieldKeyID:        "ATK-ORPHAN-001",
			objects.FieldKeyKind:      objects.KindAgentTask,
			objects.FieldKeyStatus:    objects.ObjectStatusInProgress,
			objects.FieldKeyClaimedBy: "crashed-worker-seat",
			objects.FieldKeyClaimedAt: time.Now().Format(time.RFC3339),
		},
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	secCtx := pkgctx.NewSystemSecurityContext()

	sched := &Scheduler{
		storage: mockStore,
		logger:  logger,
		secCtx:  secCtx,
	}

	sched.recoverOrphanedTasks(context.Background())

	recoveredObj, err := mockStore.Read(context.Background(), secCtx, "ATK-ORPHAN-001")
	require.NoError(t, err)
	assert.Equal(t, objects.ObjectStatusApproved, recoveredObj[objects.FieldKeyStatus])
	assert.Equal(t, storagepkg.FieldUnset, recoveredObj[objects.FieldKeyClaimedBy])
	assert.Equal(t, storagepkg.FieldUnset, recoveredObj[objects.FieldKeyClaimedAt])
}

// TestSchedulerCycle31_Operational_SwarmWorker_LLMCodeDraftSampling tests LLM config sampling.
func TestSchedulerCycle31_Operational_SwarmWorker_LLMCodeDraftSampling(t *testing.T) {
	// Nil safety
	applyCodeDraftSampling(nil)

	// Non-code draft model retains nil temperature
	chatCfg := &llm.Config{ChatModel: "qwen3.6:latest"}
	applyCodeDraftSampling(chatCfg)
	assert.Nil(t, chatCfg.Temperature)

	// Code draft model sets zero temperature when nil
	codeCfg := &llm.Config{ChatModel: "coder:7b"}
	applyCodeDraftSampling(codeCfg)
	require.NotNil(t, codeCfg.Temperature)
	assert.Equal(t, 0.0, *codeCfg.Temperature)

	// Code draft model preserves explicit temperature
	explicitTemp := 0.5
	explicitCfg := &llm.Config{ChatModel: "coder:7b", Temperature: &explicitTemp}
	applyCodeDraftSampling(explicitCfg)
	assert.Equal(t, 0.5, *explicitCfg.Temperature)

	t.Setenv(zqkenv.LLMChatModel().Name(), "custom-env-chat-model")
	assert.Equal(t, "custom-env-chat-model", fallbackChatModel())
}

// TestSchedulerCycle31_Operational_Heartbeat_ConcurrentPingAndSweep_RaceFree tests race-free concurrent heartbeats.
func TestSchedulerCycle31_Operational_Heartbeat_ConcurrentPingAndSweep_RaceFree(t *testing.T) {
	tracker := NewHeartbeatTracker()
	const numWorkers = 8
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		workerID := fmt.Sprintf("race-worker-%d", i)
		err := tracker.Register(workerID, WorkerMeta{
			Kind:           "concurrent_worker",
			Interval:       5 * time.Millisecond,
			StaleThreshold: 20 * time.Millisecond,
		})
		require.NoError(t, err)
	}

	stopCh := make(chan struct{})

	// Spawn ping workers
	for i := 0; i < numWorkers; i++ {
		workerID := fmt.Sprintf("race-worker-%d", i)
		wg.Add(1)
		goroutinelabels.NewGoroutine("c31_race", "concurrent pinger").StartSimple(func() {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
					_ = tracker.Ping(workerID)
					time.Sleep(time.Millisecond)
				}
			}
		})
	}

	// Spawn reader worker
	wg.Add(1)
	goroutinelabels.NewGoroutine("c31_race", "concurrent inspector").StartSimple(func() {
		defer wg.Done()
		for {
			select {
			case <-stopCh:
				return
			default:
				_ = tracker.GetStaleWorkers(time.Now())
				_ = tracker.ListWorkers()
				time.Sleep(time.Millisecond)
			}
		}
	})

	time.Sleep(50 * time.Millisecond)
	close(stopCh)
	wg.Wait()
}
